package huffman

import (
	"bytes"
	"reflect"
	"testing"
)

// recompress

func TestRecompress(t *testing.T) {
	huff := NewHuffman()

	fakeData := []byte{1, 2, 3, 4, 0, 0, 0, 0, 0}

	for i := 0; i < 255; i++ {
		fakeData[6] = byte(i)

		compressed, err := huff.Compress(fakeData)
		if err != nil {
			t.Errorf("Unexpected compression error: %v", err)
		}
		decompressed, err := huff.Decompress(compressed)
		if err != nil {
			t.Errorf("Unexpected decompression error: %v", err)
		}

		if !reflect.DeepEqual(decompressed, fakeData) {
			t.Errorf("got %v, wanted %v", decompressed, fakeData)
		}
	}
}

// decompress

func TestDecompressFoo(t *testing.T) {
	huff := NewHuffman()

	// values tested against https://github.com/ChillerDragon/huffman-py
	got, err := huff.Decompress([]byte{0x74, 0xde, 0x16, 0xd9, 0x22, 0xc5, 0x0d})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	want := []byte("foo")

	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, wanted %v", got, want)
	}
}

func TestDecompressShouldMatchHuffmanPy(t *testing.T) {
	huff := NewHuffman()

	// values tested against huffman-py (python rewrite)
	// https://github.com/ChillerDragon/huffman-py/blob/d1f9e280fdf57b2f145fa896c095128ea752cab5/tests/basic_test.py#L5-L7
	compressed := []byte{0x4a, 0x42, 0x88, 0x4a, 0x6e, 0x16, 0xba, 0x31, 0x46, 0xa2, 0x84, 0x9e, 0xbf, 0xe2, 0x06}
	got, err := huff.Decompress(compressed)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	want := []byte{0x40, 0x02, 0x02, 0x02, 0x00, 0x40, 0x07, 0x03, 0x22, 0x01, 0x00, 0x01, 0x00, 0x01, 0x08, 0x40, 0x01, 0x04, 0x0b}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, wanted %v", got, want)
	}
}

func TestDecompressRealSnapSingle(t *testing.T) {
	huff := NewHuffman()

	// values tested against huffman-tw ruby wrapper around the reference C++ implementation
	// https://github.com/ChillerDragon/huffman-tw/blob/46f419467bc7ea776074e2b3f1b332d89a9cdf9e/spec/03_real_traffic.rb#L30-L40

	compressed := []byte{
		0x7d, 0x8d, 0x29, 0x15, 0xa8, 0x2b, 0xf4, 0xd9, 0xc7, 0x9e, 0xad, 0x2d, 0xda, 0x8c, 0xf5, 0x35,
		0x22, 0xac, 0xaf, 0xa3, 0x1f, 0xb4, 0x07, 0xe2, 0x4a, 0xc3, 0xfa, 0x3a, 0x9a, 0xd4, 0xbe, 0xbe,
		0x1e, 0xef, 0x9f, 0xac, 0xb8, 0x01,
	}
	got, err := huff.Decompress(compressed)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	want := []byte{
		0x00, 0x36, 0x11, 0x9a, 0x01, 0x9b, 0x01, 0xa2, 0x9d, 0x04, 0x2d, 0x00, 0x03, 0x00, 0x06, 0x00,
		0x00, 0x01, 0x00, 0x0a, 0x00, 0x84, 0x01, 0xb0, 0xe6, 0x01, 0x91, 0x26, 0x00, 0x80, 0x02, 0x00,
		0x00, 0x00, 0x40, 0x00, 0x00, 0xb0, 0xe6, 0x01, 0x90, 0x26, 0x00, 0x00, 0x0a, 0x00, 0x0a, 0x01,
		0x00, 0x00, 0x00, 0x0b, 0x00, 0x08, 0x00, 0x00,
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, wanted %v", got, want)
	}
}

func TestDecompressAbc(t *testing.T) {
	huff := NewHuffman()

	// values tested against huffman-tw
	// https://github.com/ChillerDragon/huffman-tw/blob/2268be9e018758f44b42aaf90d232a2180a7cc0b/spec/02_basic_spec.rb#L28
	compressed := []byte{188, 181, 98, 92, 113, 3}
	got, err := huff.Decompress(compressed)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	want := []byte("ABC")

	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, wanted %v", got, want)
	}
}

// compress

func TestCompressFoo(t *testing.T) {
	huff := NewHuffman()

	got, err := huff.Compress([]byte("foo"))
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	want := []byte{0x74, 0xde, 0x16, 0xd9, 0x22, 0xc5, 0x0d}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, wanted %v", got, want)
	}
}

func TestCompressHelloWorld(t *testing.T) {
	huff := NewHuffman()

	got, err := huff.Compress([]byte("hello world"))
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	want := []byte{174, 149, 19, 92, 9, 87, 194, 22, 177, 86, 220, 218, 34, 56, 185, 18, 156, 168, 184, 1}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, wanted %v", got, want)
	}
}

func TestCompressEmpty(t *testing.T) {
	huff := NewHuffman()

	_, err := huff.Compress([]byte{})
	if err != nil {
		t.Errorf("Expected error no error on empty input: %v", err)
	}
}

func TestDecompressEmpty(t *testing.T) {
	huff := NewHuffman()

	_, err := huff.Decompress([]byte{})
	if err != nil {
		t.Errorf("Expected error no error on empty input: %v", err)
	}
}

func FuzzHuffmannCompressDecompress(f *testing.F) {
	f.Add([]byte("hello"))
	f.Add([]byte("1234567890"))
	f.Add([]byte("1234567890abcdef"))
	f.Add([]byte("1234567890abcdef1234567890abcdef"))
	f.Add([]byte("1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"))
	f.Add([]byte("cfgvlhböjnhavUBÖObv)öob"))

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) == 0 {
			return
		}

		huff := NewHuffman()

		compressed, err := huff.Compress(data)
		if err != nil {
			t.Fatalf("Unexpected compression error: %v", err)
		}

		decompressed, err := huff.Decompress(compressed)
		if err != nil {
			t.Fatalf("Unexpected decompression error: %v", err)
		}

		if !bytes.Equal(decompressed, data) {
			t.Fatalf("wanted %v, got %v", data, decompressed)
		}
	})
}
