package huffman_test

import (
	"bytes"
	"io"
	"reflect"
	"testing"

	"github.com/teeworlds-go/huffman"
)

func TestReadRead(t *testing.T) {

	table := []struct {
		Name       string
		Compressed []byte
		Want       []byte
		Comment    string
	}{
		{
			Name:       "read foo",
			Compressed: []byte{0x74, 0xde, 0x16, 0xd9, 0x22, 0xc5, 0x0d},
			Want:       []byte("foo"),
		},
		{
			Name:       "match huffman py",
			Compressed: []byte{0x4a, 0x42, 0x88, 0x4a, 0x6e, 0x16, 0xba, 0x31, 0x46, 0xa2, 0x84, 0x9e, 0xbf, 0xe2, 0x06},
			Want:       []byte{0x40, 0x02, 0x02, 0x02, 0x00, 0x40, 0x07, 0x03, 0x22, 0x01, 0x00, 0x01, 0x00, 0x01, 0x08, 0x40, 0x01, 0x04, 0x0b},
			Comment:    "https://github.com/ChillerDragon/huffman-py/blob/d1f9e280fdf57b2f145fa896c095128ea752cab5/tests/basic_test.py#L5-L7",
		},
		{
			Name: "real snap single",
			Compressed: []byte{
				0x7d, 0x8d, 0x29, 0x15, 0xa8, 0x2b, 0xf4, 0xd9, 0xc7, 0x9e, 0xad, 0x2d, 0xda, 0x8c, 0xf5, 0x35,
				0x22, 0xac, 0xaf, 0xa3, 0x1f, 0xb4, 0x07, 0xe2, 0x4a, 0xc3, 0xfa, 0x3a, 0x9a, 0xd4, 0xbe, 0xbe,
				0x1e, 0xef, 0x9f, 0xac, 0xb8, 0x01,
			},
			Want: []byte{
				0x00, 0x36, 0x11, 0x9a, 0x01, 0x9b, 0x01, 0xa2, 0x9d, 0x04, 0x2d, 0x00, 0x03, 0x00, 0x06, 0x00,
				0x00, 0x01, 0x00, 0x0a, 0x00, 0x84, 0x01, 0xb0, 0xe6, 0x01, 0x91, 0x26, 0x00, 0x80, 0x02, 0x00,
				0x00, 0x00, 0x40, 0x00, 0x00, 0xb0, 0xe6, 0x01, 0x90, 0x26, 0x00, 0x00, 0x0a, 0x00, 0x0a, 0x01,
				0x00, 0x00, 0x00, 0x0b, 0x00, 0x08, 0x00, 0x00,
			},
			Comment: "https://github.com/ChillerDragon/huffman-tw/blob/46f419467bc7ea776074e2b3f1b332d89a9cdf9e/spec/03_real_traffic.rb#L30-L40",
		},
		{
			Name:       "huffman TW ABC",
			Compressed: []byte{188, 181, 98, 92, 113, 3},
			Want:       []byte("ABC"),
			Comment:    "https://github.com/ChillerDragon/huffman-tw/blob/2268be9e018758f44b42aaf90d232a2180a7cc0b/spec/02_basic_spec.rb#L28",
		},
		{},
	}

	for _, test := range table {
		t.Run(test.Name, func(t *testing.T) {
			readTest(t, test.Compressed, test.Want)
		})
	}
}

func readTest(t *testing.T, compressed, want []byte) {
	// must start with 1
	// increase size of buffer
	// we want to also have a buffer that is bigger than the compressed data
	// which is why we add +1 to the length of the compressed
	for i := 1; i < len(compressed)+1; i++ {
		var (
			r           = huffman.NewReader(bytes.NewReader(compressed))
			smallBuffer = make([]byte, i)
			output      = bytes.NewBuffer(make([]byte, 0, len(want)))
		)

		// we pass a small buffer to io.CopyBuffer to test if the reader is able to handle continuous streams
		// calling the Read method multiple times
		written, err := io.CopyBuffer(output, r, smallBuffer)
		if err != nil {
			t.Fatal(err)
		}

		got := output.Bytes()

		if written != int64(len(got)) {
			t.Fatalf("wanted written bytes %d, got %d", len(got), written)
		}

		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, wanted %v", got, want)
		}
	}
}
