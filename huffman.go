package huffman

import (
	"encoding/binary"
	"fmt"
	"unsafe"
)

const (
	EofSymbol  = 256
	MaxSymbols = EofSymbol

	// maxAlloc caps buffer sizing arithmetic so that it stays well inside
	// what an int can hold on both 32 and 64 bit platforms.
	maxAlloc = uint64(1)<<(31+(^uint(0)>>63)*32) - 1
)

// Compile-time assertions about the sizing arithmetic. These are checked by
// simply building for a target, so `GOARCH=386 go build` verifies the 32 bit
// case without needing 32 bit hardware to run on.
const (
	// maxAlloc must be representable as an int, otherwise every int(size)
	// conversion guarded by it could wrap. Underflows and fails to compile
	// if maxAlloc ever exceeds the platform's MaxInt.
	_ = uint64(^uint(0)>>1) - maxAlloc

	// and it must be exactly MaxInt, not merely below it, so the guards are
	// as permissive as the platform allows. Fails to compile if maxAlloc is
	// smaller than MaxInt.
	_ = maxAlloc - uint64(^uint(0)>>1)
)

// Compress compresses the given data using the default Teeworlds' dictionary.
func Compress(data []byte) ([]byte, error) {
	return NewHuffmanDict(DefaultDictionary).Compress(data)
}

// CompressDict compresses the given data using the given dictionary.
func CompressDict(dict *Dictionary, data []byte) ([]byte, error) {
	return NewHuffmanDict(dict).Compress(data)
}

// Decompress decompresses the given data using the default Teeworlds' dictionary.
func Decompress(data []byte) ([]byte, error) {
	return NewHuffmanDict(DefaultDictionary).Decompress(data)
}

// DecompressDict decompresses the given data using the given dictionary.
func DecompressDict(dict *Dictionary, data []byte) ([]byte, error) {
	return NewHuffmanDict(dict).Decompress(data)
}

type Huffman struct {
	*Dictionary
}

// NewHuffman creates a new Huffman instance with the default dictionary.
func NewHuffman() *Huffman {
	return NewHuffmanDict(DefaultDictionary)
}

// NewHuffmanDict creates a new Huffman instance with the given dictionary.
func NewHuffmanDict(d *Dictionary) *Huffman {
	return &Huffman{
		Dictionary: d,
	}
}

// Decompress decompresses the given data.
//
// Malformed input is always rejected in bounded time: the decoder never
// consumes more bits than the input actually contains, so a stream that does
// not carry an EOF symbol returns an error instead of looping forever.
func (huff *Huffman) Decompress(data []byte) ([]byte, error) {
	if huff == nil || !huff.Dictionary.isInitialized() {
		return nil, fmt.Errorf("%w: dictionary is nil or uninitialized", ErrHuffmanDecompress)
	}
	if len(data) == 0 {
		return []byte{}, nil
	}
	return huff.DecompressTo(nil, data)
}

// DecompressTo decompresses data, APPENDING the decompressed bytes to dst and
// returning the extended slice (like Go's append). Pass a reused buffer's
// dst[:0] to avoid allocating a fresh output slice on every call — useful on
// hot paths that decompress many packets (e.g. a 50Hz snapshot stream). dst may
// be nil and may overlap data; overlapping source bytes are preserved before
// output is appended. huff is not modified, so a single Huffman value is safe
// for concurrent DecompressTo calls with distinct dst buffers.
func (huff *Huffman) DecompressTo(dst, data []byte) ([]byte, error) {
	if huff == nil || !huff.Dictionary.isInitialized() {
		return nil, fmt.Errorf("%w: dictionary is nil or uninitialized", ErrHuffmanDecompress)
	}
	if len(data) == 0 {
		return dst, nil
	}

	d := huff.Dictionary
	if d.maxCodeLen > maxStoredCodeBits {
		return nil, fmt.Errorf("%w: dictionary contains %d-bit codes, maximum supported is %d", ErrHuffmanDecompress, d.maxCodeLen, maxStoredCodeBits)
	}
	lut := &d.decLut
	nodes := &d.nodes

	// Decompression can expand one input byte into several output bytes. If
	// dst's writable capacity overlaps data, appending output could therefore
	// overwrite compressed bytes before the decoder consumes them. Preserve
	// append-style in-place use by copying only in this exceptional case.
	if byteSlicesOverlap(dst[len(dst):cap(dst)], data) {
		data = append([]byte(nil), data...)
	}

	// Output sizing. Two competing costs: guessing low means realloc+copy,
	// guessing high wastes memory and page faults.
	//
	// The hard upper bound is one symbol per input bit (a one bit code), so
	// small inputs get that bound outright: at or below 1 KiB of input the
	// worst case still fits in 8 KiB, which covers every teeworlds packet
	// with exactly one allocation and no regrowth. Larger inputs fall back to
	// a relative estimate so a 64 KiB payload does not reserve half a MiB.
	// uint64 arithmetic throughout: on a 32 bit platform len(data)*8 would
	// wrap for inputs above 256 MiB and hand make() a negative capacity.
	initCap := decompressInitCap(len(data), maxAlloc)

	// Append semantics. The estimate above is deliberately generous, so it
	// must not be used as the bar a caller's buffer has to clear: demanding
	// it would reallocate a perfectly usable reused buffer on every call and
	// defeat the point of DecompressTo. Only pre-grow when the spare capacity
	// is below the compressed size, which is the point at which regrowth
	// becomes near certain; otherwise trust the caller and let append handle
	// the rare overflow. A fresh Decompress (dst == nil) always takes this
	// branch and so keeps its single-allocation behaviour.
	if uint64(cap(dst)-len(dst)) < uint64(len(data)) {
		// Clamp the total, not just initCap: on a 32 bit platform len(dst)
		// can already be close to the int limit, and make() panics rather
		// than failing gracefully if the capacity is not representable.
		// Clamping only costs a later append growth in a case that is about
		// to run out of address space anyway.
		grown := make([]byte, len(dst), int(growCap(len(dst), initCap, maxAlloc)))
		copy(grown, dst)
		dst = grown
	}

	var (
		acc      uint64 // bit accumulator, LSB first
		bitCount uint   // number of valid bits in acc
		srcIndex int
	)

	// While the accumulator holds at least maxLen bits, any single symbol is
	// guaranteed to be decodable without a further refill and without any
	// per-symbol bounds check. A 56 bit refill therefore feeds 3 symbols of
	// the default dictionary, and dozens for short-code data.
	maxLen := uint(d.maxCodeLen)
	if maxLen < lookupTableBits {
		maxLen = lookupTableBits
	}

bulk:
	// A refill guarantees 56 bits, so the unchecked bulk loop is only usable
	// when a single code can never exceed that.
	for maxLen <= 56 {
		// Refill to at least 56 valid bits. The fast path loads 8 bytes at
		// once and only claims the whole bytes it consumed; the leftover
		// partial byte is simply re-read on the next refill.
		if len(data)-srcIndex >= 8 {
			acc |= binary.LittleEndian.Uint64(data[srcIndex:]) << bitCount
			srcIndex += int(63-bitCount) >> 3
			bitCount |= 56
		} else {
			for bitCount < 56 && srcIndex < len(data) {
				acc |= uint64(data[srcIndex]) << bitCount
				srcIndex++
				bitCount += 8
			}
			if bitCount < maxLen {
				// input exhausted, finish under full bit-availability checks
				break bulk
			}
		}

		for bitCount >= maxLen {
			entry := lut[acc&lookupTableMask]
			codeLen := uint(entry & lutLenMask)

			if codeLen != 0 {
				acc >>= codeLen
				bitCount -= codeLen
				if entry&lutEOFBit != 0 {
					return dst, nil
				}
				dst = append(dst, byte(entry>>lutSymShift))
				continue
			}

			// Not resolvable within lookupTableBits: consume those bits and
			// walk the tree from the node the table landed on. The walk is
			// bounded by maxLen total bits, which we know we have.
			idx := entry >> lutNodeShift
			acc >>= lookupTableBits
			bitCount -= lookupTableBits

			for {
				idx = uint32(nodes[idx].Leafs[acc&1])
				acc >>= 1
				bitCount--

				if idx >= uint32(len(nodes)) {
					return nil, fmt.Errorf("%w: invalid stream: walked off the tree", ErrHuffmanDecompress)
				}
				if nodes[idx].NumBits != 0 {
					break
				}
			}

			if idx == EofSymbol {
				return dst, nil
			}
			dst = append(dst, nodes[idx].Symbol)
		}
	}

	// Tail: fewer than maxLen bits remain, so every consumption has to be
	// checked against what the input actually provided. This is what makes a
	// stream without an EOF symbol terminate with an error instead of
	// spinning forever on zero padding.
	for {
		for bitCount < 56 && srcIndex < len(data) {
			acc |= uint64(data[srcIndex]) << bitCount
			srcIndex++
			bitCount += 8
		}

		entry := lut[acc&lookupTableMask]
		codeLen := uint(entry & lutLenMask)

		if codeLen != 0 {
			if codeLen > bitCount {
				return nil, fmt.Errorf("%w: truncated stream: need %d bits, have %d", ErrHuffmanDecompress, codeLen, bitCount)
			}
			acc >>= codeLen
			bitCount -= codeLen
			if entry&lutEOFBit != 0 {
				return dst, nil
			}
			dst = append(dst, byte(entry>>lutSymShift))
			continue
		}

		if bitCount < lookupTableBits {
			return nil, fmt.Errorf("%w: truncated stream: need %d bits, have %d", ErrHuffmanDecompress, lookupTableBits, bitCount)
		}
		idx := entry >> lutNodeShift
		acc >>= lookupTableBits
		bitCount -= lookupTableBits

		for {
			if bitCount == 0 {
				return nil, fmt.Errorf("%w: truncated stream: symbol not terminated", ErrHuffmanDecompress)
			}
			idx = uint32(nodes[idx].Leafs[acc&1])
			acc >>= 1
			bitCount--

			if idx >= uint32(len(nodes)) {
				return nil, fmt.Errorf("%w: invalid stream: walked off the tree", ErrHuffmanDecompress)
			}
			if nodes[idx].NumBits != 0 {
				break
			}
		}

		if idx == EofSymbol {
			return dst, nil
		}
		dst = append(dst, nodes[idx].Symbol)
	}
}

// Compress compresses the given data.
//
// Empty input is not a special case: it compresses to the EOF symbol alone,
// which is what teeworlds 0.7 and ddnet both emit and what their decoders
// require. Returning an empty slice here would produce a stream neither of
// them can decode.
func (huff *Huffman) Compress(data []byte) ([]byte, error) {
	if huff == nil || !huff.Dictionary.isInitialized() {
		return nil, fmt.Errorf("%w: dictionary is nil or uninitialized", ErrHuffmanCompress)
	}
	d := huff.Dictionary

	// Codes are stored as uint32 in Dictionary. Reject deeper custom trees
	// explicitly instead of silently truncating their codes and emitting a
	// corrupt stream. The default Teeworlds dictionary tops out at 15 bits.
	if d.maxCodeLen > maxStoredCodeBits {
		return nil, fmt.Errorf("%w: dictionary contains %d-bit codes, maximum supported is %d", ErrHuffmanCompress, d.maxCodeLen, maxStoredCodeBits)
	}

	encBits := &d.encBits
	encLen := &d.encLen

	// Exact worst case: every symbol at the longest code, plus the EOF code
	// and the final partial byte. Sizing up front removes every bounds check
	// and every realloc from the hot loop.
	size, ok := compressBufSize(len(data), d.maxCodeLen, maxAlloc)
	if !ok {
		return nil, fmt.Errorf("%w: input of %d bytes needs more than %d bytes of output buffer", ErrHuffmanCompress, len(data), uint64(maxAlloc))
	}
	dst := make([]byte, int(size))

	var (
		acc      uint64
		bitCount uint
		pos      int
	)

	for _, symbol := range data {
		acc |= uint64(encBits[symbol]) << bitCount
		bitCount += uint(encLen[symbol])

		if bitCount >= 32 {
			binary.LittleEndian.PutUint32(dst[pos:], uint32(acc))
			pos += 4
			acc >>= 32
			bitCount -= 32
		}
	}

	acc |= uint64(encBits[EofSymbol]) << bitCount
	bitCount += uint(encLen[EofSymbol])

	for bitCount >= 8 {
		dst[pos] = byte(acc)
		pos++
		acc >>= 8
		bitCount -= 8
	}
	// Trailing partial byte, only when bits actually remain. Teeworlds 0.7
	// and older ddnet always wrote this byte even when empty; ddnet dropped
	// the redundant zero byte in 4354f8c6. It sits after the EOF symbol, so
	// every decoder ignores it either way.
	if bitCount != 0 {
		dst[pos] = byte(acc)
		pos++
	}

	// The worst-case buffer is ~1.9x the real output for the default
	// dictionary. Hand back a right-sized slice when we overshot badly,
	// rather than pinning the oversized array in the caller's heap -- but
	// only when the waste justifies a second allocation plus a copy. Packet
	// sized payloads stay at exactly one allocation.
	if len(dst)-pos > 8192 && uint64(pos)*2 < uint64(len(dst)) {
		out := make([]byte, pos)
		copy(out, dst[:pos])
		return out, nil
	}
	return dst[:pos], nil
}

// byteSlicesOverlap reports whether the two slice ranges share any byte. It
// compares addresses only; it never converts uintptr values back to pointers.
// Using subtraction instead of computing end addresses also avoids uintptr
// overflow on 32 bit platforms.
func byteSlicesOverlap(a, b []byte) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	aStart := uintptr(unsafe.Pointer(unsafe.SliceData(a)))
	bStart := uintptr(unsafe.Pointer(unsafe.SliceData(b)))
	if aStart <= bStart {
		return bStart-aStart < uintptr(len(a))
	}
	return aStart-bStart < uintptr(len(b))
}

// Buffer sizing arithmetic, kept in one place and parameterised by limit (the
// platform's maxAlloc) so the 32 bit behaviour is unit-testable on any host.
// All of it runs in uint64: on a 32 bit platform len(data)*8 would wrap and
// hand make() a nonsensical size.

// decompressInitCap estimates the output capacity for a compressed payload of
// inputLen bytes. Guessing low costs realloc+copy, guessing high wastes
// memory, so small inputs get the hard upper bound of one symbol per input bit
// (which still fits in 8 KiB at or below 1 KiB of input, covering every
// teeworlds packet in a single allocation) and larger ones a relative estimate.
func decompressInitCap(inputLen int, limit uint64) uint64 {
	n := uint64(inputLen)
	maxUint64 := ^uint64(0)
	initCap := maxUint64
	if n <= (maxUint64-64)/2 {
		initCap = n*2 + 64
	}
	if initCap < 8192 {
		initCap = 8192
	}
	bound := maxUint64
	if n <= (maxUint64-8)/8 {
		bound = n*8 + 8
	}
	if initCap > bound {
		initCap = bound
	}
	if initCap > limit {
		initCap = limit
	}
	return initCap
}

// growCap is the capacity for a buffer that already holds dstLen bytes and
// wants initCap more. The total is clamped, not just initCap: on a 32 bit
// platform dstLen alone can be close to the int limit and make() panics rather
// than failing gracefully on a capacity it cannot represent.
func growCap(dstLen int, initCap, limit uint64) uint64 {
	base := uint64(dstLen)
	if base >= limit || initCap >= limit-base {
		return limit
	}
	return base + initCap
}

// compressBufSize is the exact worst case output size for inputLen bytes:
// every symbol at the longest code, plus the EOF code and a final partial
// byte. Reports false when that cannot be represented on this platform.
func compressBufSize(inputLen int, maxCodeLen uint8, limit uint64) (uint64, bool) {
	symbols := uint64(inputLen) + 1
	codeLen := uint64(maxCodeLen)
	if codeLen != 0 && symbols > (^uint64(0)-8)/codeLen {
		return 0, false
	}
	maxBits := symbols*codeLen + 8
	size := maxBits/8 + 8
	if size > limit {
		return 0, false
	}
	return size, true
}
