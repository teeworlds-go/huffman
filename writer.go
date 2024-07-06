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

func (w *Writer) flush() error {
	// nothing to flush
	if len(w.buf) == 0 {
		return nil
	}
	_, err := w.w.Write(w.buf)
	w.buf = w.buf[:0]
	return err
}

func (w *Writer) flushIfFull() error {
	if len(w.buf) < cap(w.buf) {
		// not full yet
		return nil
	}
	return w.flush()
}

func (w *Writer) append(b byte) error {
	w.buf = append(w.buf, b)
	return w.flushIfFull()
}

func (w *Writer) Reset(rw io.Writer) {
	w.w = rw
	w.buf = w.buf[:0]
}

// Write compresses the pased data and writes it to the underlying writer.
// The returned returned value is the number of uncompressed bytes that were written.
func (w *Writer) Write(data []byte) (written int, err error) {

	var (
		bits     uint32
		bitCount uint8
		node     node
	)

	for _, symbol := range data {
		node = w.d.nodes[symbol]

		bits |= node.Bits << bitCount
		bitCount += node.NumBits

		for bitCount >= 8 {
			err = w.append(byte(bits))
			if err != nil {
				return
			}
			bits >>= 8
			bitCount -= 8
		}
	}

	nodeEOF := w.d.nodes[EofSymbol]
	bits |= nodeEOF.Bits << bitCount
	bitCount += nodeEOF.NumBits

	for bitCount >= 8 {
		err = w.append(byte(bits))
		if err != nil {
			return
		}
		bits >>= 8
		bitCount -= 8
	}

	// append EOF symbol
	err = w.append(byte(bits))
	if err != nil {
		return 0, err
	}
	err = w.flush()
	if err != nil {
		return 0, err
	}

	return len(data), nil
}
