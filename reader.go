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
	d       *Dictionary
	br      io.ByteReader
	bufSize int
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
	// read from underlying reader

	var (
		cursor     = 0
		dstEnd     = len(decompressed)
		lut        = &r.d.decLut
		nodes      = &r.d.nodes
		acc        uint64
		bitCount   uint
		srcDrained bool
		b          byte
	)

	for cursor = 0; cursor < dstEnd; cursor++ {
		// Refill to at least 56 bits. The underlying source is an
		// io.ByteReader, so we must not read further ahead than we consume;
		// bytes are pulled one at a time on purpose.
		for !srcDrained && bitCount <= 56 {
			b, err = r.br.ReadByte()
			if err != nil {
				if errors.Is(err, io.EOF) {
					srcDrained = true
					break
				}
				// unexpected error, abort
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
				return cursor, fmt.Errorf("%w: truncated stream: need %d bits, have %d", ErrHuffmanDecompress, codeLen, bitCount)
			}
			acc >>= codeLen
			bitCount -= codeLen

			if entry&lutEOFBit != 0 {
				break
			}
			decompressed[cursor] = byte(entry >> lutSymShift)
			continue
		}

		// walk the tree bit by bit from where the lookup table landed
		if bitCount < lookupTableBits {
			return cursor, fmt.Errorf("%w: truncated stream: need %d bits, have %d", ErrHuffmanDecompress, lookupTableBits, bitCount)
		}
		idx := entry >> lutNodeShift
		acc >>= lookupTableBits
		bitCount -= lookupTableBits

		for {
			if bitCount == 0 {
				return cursor, fmt.Errorf("%w: decoding error: symbol not found in tree", ErrHuffmanDecompress)
			}
			idx = uint32(nodes[idx].Leafs[acc&1])
			acc >>= 1
			bitCount--

			if idx >= uint32(len(nodes)) {
				return cursor, fmt.Errorf("%w: invalid stream: walked off the tree", ErrHuffmanDecompress)
			}
			if nodes[idx].NumBits > 0 {
				break
			}
		}

		if idx == EofSymbol {
			break
		}

		decompressed[cursor] = nodes[idx].Symbol
	}

	return cursor, io.EOF
}

func (r *Reader) Reset(rr io.Reader) {

	// bufio.Reader implements this interface
	br, ok := rr.(io.ByteReader)
	if ok {
		r.br = br
		return
	}

	r.br = bufio.NewReaderSize(rr, r.bufSize)
}
