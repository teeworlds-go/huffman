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
		cursor   = 0
		dstEnd   = len(decompressed)
		bits     uint32
		bitCount uint8
		eof      *node = &r.d.nodes[EofSymbol]
		n        *node = nil
		b        byte
	)

	for cursor = 0; cursor < dstEnd; cursor++ {
		n = nil
		if bitCount >= lookupTableBits {
			n = r.d.decodeLut[bits&lookupTableMask]
		}

		for bitCount < 24 {
			b, err = r.br.ReadByte()
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				// unexpected error, abort
				return cursor, err
			}

			bits |= uint32(b) << bitCount
			bitCount += 8
		}

		if n == nil {
			n = r.d.decodeLut[bits&lookupTableMask]
		}

		if n == nil {
			return cursor, fmt.Errorf("%w: decoding error: symbol not found in lookup table: %x (masked: %x)", ErrHuffmanDecompress, bits, bits&lookupTableMask)
		}

		if n.NumBits > 0 {
			// leaf nodes
			bits >>= n.NumBits
			bitCount -= n.NumBits
		} else {
			bits >>= lookupTableBits
			bitCount -= lookupTableBits

			// walk the tree bit by bit
			for {
				// traverse tree
				n = &r.d.nodes[n.Leafs[bits&1]]

				// remove bit
				bitCount--
				bits >>= 1

				// check if we hit a symbol
				if n.NumBits > 0 {
					break
				}

				if bitCount == 0 {
					return cursor, fmt.Errorf("%w: decoding error: symbol not found in tree", ErrHuffmanDecompress)
				}
			}
		}

		if n == eof {
			break
		}

		decompressed[cursor] = n.Symbol
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
