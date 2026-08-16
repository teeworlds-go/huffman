package huffman

import (
	"bufio"
	"errors"
	"fmt"
	"io"
)

var (
	ErrHuffmanDecompress = errors.New("decompression error")
)

type Reader struct {
	d           *Dictionary
	br          io.ByteReader
	bufSize     int
	acc         uint64
	bitCount    uint
	srcDrained  bool
	terminalErr error
}

// New creates a new Reader with the default Teeworlds' dictionary.
func NewReader(r io.Reader) *Reader {
	// pass default global dictionary that is used in Teeworlds
	return NewReaderDict(DefaultDictionary, r)
}

// NewReaderDict expects a Dictionary (index -> symbol)
// You can use the default one if you just want to work with Teeworlds' default compression.
func NewReaderDict(d *Dictionary, r io.Reader) *Reader {
	var bufSize = 2048

	br, ok := r.(io.ByteReader)
	if !ok {
		br = bufio.NewReaderSize(r, bufSize)
	}

	h := Reader{
		d:       d,
		br:      br,
		bufSize: bufSize,
	}

	return &h
}

// Decompress decompresses 'data' and writes the result into 'decompressed'.
// The decompressed slice must be preallocated to fit the decompressed data.
// Read is the size that was decompressed and written into the 'decompressed' slice.
func (r *Reader) Read(decompressed []byte) (read int, err error) {
	if len(decompressed) == 0 {
		if r.terminalErr != nil {
			return 0, r.terminalErr
		}
		return 0, nil
	}
	if r.terminalErr != nil {
		return 0, r.terminalErr
	}
	if r.d.maxCodeLen > maxStoredCodeBits {
		err = fmt.Errorf("%w: dictionary contains %d-bit codes, maximum supported is %d", ErrHuffmanDecompress, r.d.maxCodeLen, maxStoredCodeBits)
		r.terminalErr = err
		return 0, err
	}

	var (
		cursor     int
		lut        = &r.d.decLut
		nodes      = &r.d.nodes
		acc        = r.acc
		bitCount   = r.bitCount
		srcDrained = r.srcDrained
		b          byte
	)

	for cursor < len(decompressed) {
		// Refill to at least 56 bits. Bytes are pulled through io.ByteReader
		// one at a time and every prefetched bit is retained in Reader state
		// when the caller's destination fills.
		for !srcDrained && bitCount <= 56 {
			b, err = r.br.ReadByte()
			if err != nil {
				if errors.Is(err, io.EOF) {
					srcDrained = true
					break
				}
				// unexpected error, abort
				r.terminalErr = err
				return cursor, err
			}

			acc |= uint64(b) << bitCount
			bitCount += 8
		}

		entry := lut[acc&lookupTableMask]
		codeLen := uint(entry & lutLenMask)

		if codeLen != 0 {
			// resolved straight out of the lookup table
			if codeLen > bitCount {
				err = fmt.Errorf("%w: truncated stream: need %d bits, have %d", ErrHuffmanDecompress, codeLen, bitCount)
				r.terminalErr = err
				return cursor, err
			}
			acc >>= codeLen
			bitCount -= codeLen

			if entry&lutEOFBit != 0 {
				r.terminalErr = io.EOF
				return cursor, io.EOF
			}
			decompressed[cursor] = byte(entry >> lutSymShift)
			cursor++
			continue
		}

		// walk the tree bit by bit from where the lookup table landed
		if bitCount < lookupTableBits {
			err = fmt.Errorf("%w: truncated stream: need %d bits, have %d", ErrHuffmanDecompress, lookupTableBits, bitCount)
			r.terminalErr = err
			return cursor, err
		}
		idx := entry >> lutNodeShift
		acc >>= lookupTableBits
		bitCount -= lookupTableBits

		for {
			if bitCount == 0 {
				err = fmt.Errorf("%w: decoding error: symbol not found in tree", ErrHuffmanDecompress)
				r.terminalErr = err
				return cursor, err
			}
			idx = uint32(nodes[idx].Leafs[acc&1])
			acc >>= 1
			bitCount--

			if idx >= uint32(len(nodes)) {
				err = fmt.Errorf("%w: invalid stream: walked off the tree", ErrHuffmanDecompress)
				r.terminalErr = err
				return cursor, err
			}
			if nodes[idx].NumBits > 0 {
				break
			}
		}

		if idx == EofSymbol {
			r.terminalErr = io.EOF
			return cursor, io.EOF
		}

		decompressed[cursor] = nodes[idx].Symbol
		cursor++
	}

	// The destination filled before the Huffman EOF symbol. Preserve every
	// prefetched bit and report a non-terminal read so the next call resumes
	// exactly where this one stopped.
	r.acc = acc
	r.bitCount = bitCount
	r.srcDrained = srcDrained
	return cursor, nil
}

func (r *Reader) Reset(rr io.Reader) {
	r.acc = 0
	r.bitCount = 0
	r.srcDrained = false
	r.terminalErr = nil

	// bufio.Reader implements this interface
	br, ok := rr.(io.ByteReader)
	if ok {
		r.br = br
		return
	}

	r.br = bufio.NewReaderSize(rr, r.bufSize)
}
