package huffman

import (
	"errors"
	"io"
)

var (
	ErrHuffmanCompress = errors.New("compression error")
)

type Writer struct {
	d   *Dictionary
	w   io.Writer
	buf []byte
}

// New creates a new Writer that uses the default Teeworlds dictionary in order to compress data.
func NewWriter(w io.Writer) *Writer {
	// pass default global dictionary that is used in Teeworlds
	return NewWriterDict(DefaultDictionary, w)
}

// NewWriterDict expects a Dictionary (index -> symbol)
// You can use the default one if you just want to work with Teeworlds' default compression.
func NewWriterDict(d *Dictionary, w io.Writer) *Writer {
	h := Writer{
		d:   d,
		w:   w,
		buf: make([]byte, 0, 2048),
	}
	return &h
}

func (h *Writer) flush() error {
	// nothing to flush
	if len(h.buf) == 0 {
		return nil
	}
	_, err := h.w.Write(h.buf)
	h.buf = h.buf[:0]
	return err
}

func (h *Writer) flushIfFull() error {
	if len(h.buf) < cap(h.buf) {
		// not full yet
		return nil
	}
	return h.flush()
}

func (h *Writer) append(b byte) error {
	h.buf = append(h.buf, b)
	return h.flushIfFull()
}

func (h *Writer) Reset(w io.Writer) {
	h.w = w
	h.buf = h.buf[:0]
}

// Write compresses the pased data and writes it to the underlying writer.
// The returned returned value is the number of uncompressed bytes that were written.
func (h *Writer) Write(data []byte) (written int, err error) {

	var (
		bits     uint32
		bitCount uint8
		node     node
	)

	for _, symbol := range data {
		node = h.d.nodes[symbol]

		bits |= node.Bits << bitCount
		bitCount += node.NumBits

		for bitCount >= 8 {
			err = h.append(byte(bits))
			if err != nil {
				return
			}
			bits >>= 8
			bitCount -= 8
		}
	}

	nodeEOF := h.d.nodes[HuffmanEOFSymbol]
	bits |= nodeEOF.Bits << bitCount
	bitCount += nodeEOF.NumBits

	for bitCount >= 8 {
		err = h.append(byte(bits))
		if err != nil {
			return
		}
		bits >>= 8
		bitCount -= 8
	}

	// append EOF symbol
	err = h.append(byte(bits))
	if err != nil {
		return 0, err
	}
	err = h.flush()
	if err != nil {
		return 0, err
	}

	return len(data), nil
}
