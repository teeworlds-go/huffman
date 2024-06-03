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
