package huffman

import (
	"encoding/binary"
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
	d := w.d

	// Codes longer than 32 bits cannot share a 64 bit accumulator with up to
	// 31 leftover bits. Only pathological frequency tables get there.
	if d.maxCodeLen > 32 {
		return w.writeDeep(data)
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
				if _, err = w.w.Write(buf); err != nil {
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
			if _, err = w.w.Write(buf); err != nil {
				w.buf = buf[:0]
				return 0, err
			}
			buf = buf[:0]
		}
		buf = append(buf, byte(acc))
		acc >>= 8
		bitCount -= 8
	}
	if len(buf) == cap(buf) {
		if _, err = w.w.Write(buf); err != nil {
			w.buf = buf[:0]
			return 0, err
		}
		buf = buf[:0]
	}
	buf = append(buf, byte(acc))

	w.buf = buf
	if err = w.flush(); err != nil {
		return 0, err
	}

	return len(data), nil
}

// writeDeep is the fallback for dictionaries with codes longer than 32 bits.
func (w *Writer) writeDeep(data []byte) (written int, err error) {
	d := w.d

	var (
		bits     uint64
		bitCount uint
	)

	emit := func(symbol int) error {
		bits |= uint64(d.encBits[symbol]) << bitCount
		bitCount += uint(d.encLen[symbol])
		for bitCount >= 8 {
			if err := w.append(byte(bits)); err != nil {
				return err
			}
			bits >>= 8
			bitCount -= 8
		}
		return nil
	}

	for _, symbol := range data {
		if err = emit(int(symbol)); err != nil {
			return 0, err
		}
	}
	if err = emit(EofSymbol); err != nil {
		return 0, err
	}
	if err = w.append(byte(bits)); err != nil {
		return 0, err
	}
	if err = w.flush(); err != nil {
		return 0, err
	}
	return len(data), nil
}
