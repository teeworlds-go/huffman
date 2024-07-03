package huffman_test

import (
	"bytes"
	"fmt"
	"io"

	"github.com/teeworlds-go/huffman"
)

func ExampleReader_Read() {
	data, err := huffman.Compress([]byte("hello world"))
	if err != nil {
		panic(err)
	}
	r := huffman.NewReader(bytes.NewReader(data))
	out := bytes.NewBuffer(nil)

	_, err = io.Copy(out, r)
	if err != nil {
		panic(err)
	}

	fmt.Println(out.String())
	// Output:
	// hello world

}
