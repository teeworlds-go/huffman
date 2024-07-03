package huffman

import (
	"bytes"
	"io"
	"testing"
)

func FuzzWriteRead(f *testing.F) {
	f.Add([]byte("hello"))
	f.Add([]byte("1234567890"))
	f.Add([]byte("1234567890abcdef"))
	f.Add([]byte("1234567890abcdef1234567890abcdef"))
	f.Add([]byte("1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"))
	f.Add([]byte("cfgvlhböjnhavUBÖObv)öob"))
	f.Fuzz(writeReadTest)
}

func writeReadTest(t *testing.T, data []byte) {

	huff := NewHuffman()

	for i := 1; i < len(data)+1; i++ {
		var (
			smallBuffer = make([]byte, i)

			inputStream        = bytes.NewReader(data)
			compressedStream   = bytes.NewBuffer(make([]byte, 0, len(data)))
			w                  = NewWriter(compressedStream)
			decompressedStream = bytes.NewBuffer(make([]byte, 0, len(data)))
		)

		n, err := io.CopyBuffer(w, inputStream, smallBuffer)
		if err != nil {
			t.Fatalf("error writing: %v", err)
		}

		if n != int64(len(data)) {
			t.Fatalf("expected to write %d bytes, wrote %d bytes", len(data), n)
		}

		compressed, err := huff.Compress(data)
		if err != nil {
			t.Fatalf("error compressing: %v", err)
		}

		if !bytes.Equal(compressed, compressedStream.Bytes()) {
			t.Fatalf("expected %v(%s), got %v(%s)", compressed, string(compressed), compressedStream.Bytes(), compressedStream.String())
		}

		var (
			r = NewReader(compressedStream)
		)
		n, err = io.CopyBuffer(decompressedStream, r, smallBuffer)
		if err != nil {
			t.Fatalf("error reading: %v", err)
		}

		if n != int64(len(data)) {
			t.Fatalf("expected to read %d bytes, read %d (buffer size = %d)", len(data), n, i)
		}

		if !bytes.Equal(data, decompressedStream.Bytes()) {
			t.Fatalf("decompressed: expected %v, got %v", data, decompressedStream.Bytes())
		}
	}
}
