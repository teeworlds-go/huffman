package huffman_test

import (
	"fmt"

	"github.com/teeworlds-go/huffman/v2"
)

func ExampleCompress() {
	data, err := huffman.Compress([]byte("hello world"))
	if err != nil {
		panic(err)
	}
	fmt.Printf("data: %v\n", data)
	// Output:
	// data: [174 149 19 92 9 87 194 22 177 86 220 218 34 56 185 18 156 168 184 1]
}

func ExampleDecompress() {
	data, err := huffman.Decompress([]byte{174, 149, 19, 92, 9, 87, 194, 22, 177, 86, 220, 218, 34, 56, 185, 18, 156, 168, 184, 1})
	if err != nil {
		panic(err)
	}
	fmt.Printf("data: %v\n", string(data))
	// Output:
	// data: hello world
}

func ExampleHuffman_Compress() {
	huff := huffman.NewHuffman()

	data, err := huff.Compress([]byte("hello world"))
	if err != nil {
		panic(err)
	}
	fmt.Printf("data: %v\n", data)
	// Output:
	// data: [174 149 19 92 9 87 194 22 177 86 220 218 34 56 185 18 156 168 184 1]

}

func ExampleHuffman_Decompress() {
	huff := huffman.NewHuffman()

	data, err := huff.Decompress([]byte{174, 149, 19, 92, 9, 87, 194, 22, 177, 86, 220, 218, 34, 56, 185, 18, 156, 168, 184, 1})
	if err != nil {
		panic(err)
	}
	fmt.Printf("data: %v\n", string(data))
	// Output:
	// data: hello world
}
