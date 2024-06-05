package huffman

import (
	"reflect"
	"testing"
)

func TestCompressFoo(t *testing.T) {
	huff := Huffman{}
	huff.init()

	got, err := huff.compress([]byte("foo"))
	if err != nil {
		t.Errorf("failed to compress foo %v", err)
	}

	want := []byte{0x74, 0xde, 0x16, 0xd9, 0x22, 0xc5, 0x0d}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, wanted %v", got, want)
	}
}

func TestCompressHelloWorld(t *testing.T) {
	huff := Huffman{}
	huff.init()

	got, err := huff.compress([]byte("hello world"))
	if err != nil {
		t.Errorf("failed to compress foo %v", err)
	}

	want := []byte{174, 149, 19, 92, 9, 87, 194, 22, 177, 86, 220, 218, 34, 56, 185, 18, 156, 168, 184, 1}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, wanted %v", got, want)
	}
}

func TestCompressEmpty(t *testing.T) {
	huff := Huffman{}
	huff.init()

	_, err := huff.compress([]byte{})
	if err == nil {
		t.Errorf("Expected error on empty input")
	}
}
