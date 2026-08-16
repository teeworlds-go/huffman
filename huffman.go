package huffman

import (
	"encoding/binary"
	"fmt"
)

const (
	EofSymbol  = 256
	MaxSymbols = EofSymbol
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
	if len(data) == 0 {
		return []byte{}, nil
	}

	d := huff.Dictionary
	lut := &d.decLut
	nodes := &d.nodes

	// Output sizing. Two competing costs: guessing low means realloc+copy,
	// guessing high wastes memory and page faults.
	//
	// The hard upper bound is one symbol per input bit (a one bit code), so
	// small inputs get that bound outright: at or below 1 KiB of input the
	// worst case still fits in 8 KiB, which covers every teeworlds packet
	// with exactly one allocation and no regrowth. Larger inputs fall back to
	// a relative estimate so a 64 KiB payload does not reserve half a MiB.
	initCap := len(data)*2 + 64
	if initCap < 8192 {
		initCap = 8192
	}
	if bound := len(data)*8 + 8; initCap > bound {
		initCap = bound
	}
	dst := make([]byte, 0, initCap)

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
		if srcIndex+8 <= len(data) {
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
func (huff *Huffman) Compress(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return []byte{}, nil
	}

	d := huff.Dictionary

	// A code longer than 32 bits cannot be accumulated together with up to 31
	// leftover bits in a 64 bit register. Only absurdly skewed frequency
	// tables reach that depth; the default dictionary tops out at 15 bits.
	if d.maxCodeLen > 32 {
		return huff.compressDeep(data)
	}

	encBits := &d.encBits
	encLen := &d.encLen

	// Exact worst case: every symbol at the longest code, plus the EOF code
	// and the final partial byte. Sizing up front removes every bounds check
	// and every realloc from the hot loop.
	maxBits := (len(data)+1)*int(d.maxCodeLen) + 8
	dst := make([]byte, maxBits/8+8)

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
	// trailing partial byte, always emitted to match the reference encoder
	dst[pos] = byte(acc)
	pos++

	// The worst-case buffer is ~1.9x the real output for the default
	// dictionary. Hand back a right-sized slice when we overshot badly,
	// rather than pinning the oversized array in the caller's heap -- but
	// only when the waste justifies a second allocation plus a copy. Packet
	// sized payloads stay at exactly one allocation.
	if len(dst)-pos > 8192 && pos*2 < len(dst) {
		out := make([]byte, pos)
		copy(out, dst[:pos])
		return out, nil
	}
	return dst[:pos], nil
}

// compressDeep is the fallback for dictionaries whose codes exceed 32 bits.
// It drains the accumulator down to a single partial byte before adding the
// next code, so it is correct for code lengths up to 56 bits at the cost of
// an extra inner loop.
func (huff *Huffman) compressDeep(data []byte) ([]byte, error) {
	d := huff.Dictionary
	encBits := &d.encBits
	encLen := &d.encLen

	maxBits := (len(data)+1)*int(d.maxCodeLen) + 8
	dst := make([]byte, maxBits/8+8)

	var (
		acc      uint64
		bitCount uint
		pos      int
	)

	emit := func(symbol int) {
		acc |= uint64(encBits[symbol]) << bitCount
		bitCount += uint(encLen[symbol])
		for bitCount >= 8 {
			dst[pos] = byte(acc)
			pos++
			acc >>= 8
			bitCount -= 8
		}
	}

	for _, symbol := range data {
		emit(int(symbol))
	}
	emit(EofSymbol)

	dst[pos] = byte(acc)
	pos++

	if len(dst)-pos > 8192 && pos*2 < len(dst) {
		out := make([]byte, pos)
		copy(out, dst[:pos])
		return out, nil
	}
	return dst[:pos], nil
}
