package huffman

import (
	"encoding/binary"
	"errors"
	"fmt"
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
	err := writeBuffer(w.w, w.buf)
	w.buf = w.buf[:0]
	return err
}

func writeBuffer(w io.Writer, buf []byte) error {
	n, err := w.Write(buf)
	if n != len(buf) && err == nil {
		return io.ErrShortWrite
	}
	return err
}

func (w *Writer) Reset(rw io.Writer) {
	w.w = rw
	w.buf = w.buf[:0]
}

// Write compresses the passed data and writes it to the underlying writer.
// The returned value is the number of uncompressed bytes that were written.
func (w *Writer) Write(data []byte) (written int, err error) {
	if w == nil {
		return 0, fmt.Errorf("%w: writer is nil", ErrHuffmanCompress)
	}
	if !w.d.isInitialized() {
		return 0, fmt.Errorf("%w: dictionary is nil or uninitialized", ErrHuffmanCompress)
	}
	d := w.d

	// Dictionary codes are stored as uint32. Reject deeper custom trees rather
	// than silently truncating their codes and writing corrupt data.
	if d.maxCodeLen > maxStoredCodeBits {
		return 0, fmt.Errorf("%w: dictionary contains %d-bit codes, maximum supported is %d", ErrHuffmanCompress, d.maxCodeLen, maxStoredCodeBits)
	}

	var (
		encBits  = &d.encBits
		encLen   = &d.encLen
		acc      uint64
		bitCount uint
		buf      = w.buf[:0]
	)

	for _, symbol := range data {
		acc |= uint64(encBits[symbol]) << bitCount
		bitCount += uint(encLen[symbol])

		if bitCount >= 32 {
			if len(buf)+4 > cap(buf) {
				if err = writeBuffer(w.w, buf); err != nil {
					w.buf = buf[:0]
					return 0, err
				}
				buf = buf[:0]
			}
			buf = binary.LittleEndian.AppendUint32(buf, uint32(acc))
			acc >>= 32
			bitCount -= 32
		}
	}

	acc |= uint64(encBits[EofSymbol]) << bitCount
	bitCount += uint(encLen[EofSymbol])

	// drain whole bytes, then the trailing partial byte
	for bitCount >= 8 {
		if len(buf) == cap(buf) {
			if err = writeBuffer(w.w, buf); err != nil {
				w.buf = buf[:0]
				return 0, err
			}
			buf = buf[:0]
		}
		buf = append(buf, byte(acc))
		acc >>= 8
		bitCount -= 8
	}
	// trailing partial byte, only when bits actually remain (see Compress)
	if bitCount != 0 {
		if len(buf) == cap(buf) {
			if err = writeBuffer(w.w, buf); err != nil {
				w.buf = buf[:0]
				return 0, err
			}
			buf = buf[:0]
		}
		buf = append(buf, byte(acc))
	}

	w.buf = buf
	if err = w.flush(); err != nil {
		return 0, err
	}

	return len(data), nil
}
