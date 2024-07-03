package huffman_test

import (
	"bytes"
	"fmt"
	"io"

	"github.com/teeworlds-go/huffman/v2"
)

func ExampleWriter_Write() {
	// buf can be anything you can write to, e.g. a network connection.
	buf := bytes.NewBuffer(nil)
	w := huffman.NewWriter(buf)

	_, err := io.WriteString(w, "hello world")
	if err != nil {
		panic(err)
	}

	data, err := huffman.Decompress(buf.Bytes())
	if err != nil {
		panic(err)
	}
	fmt.Println(string(data))
	// Output:
	// hello world
}
