# huffman

[![Go Reference](https://pkg.go.dev/badge/github.com/teeworlds-go/huffman.svg)](https://pkg.go.dev/github.com/teeworlds-go/huffman) [![Go Report Card](https://goreportcard.com/badge/github.com/teeworlds-go/huffman)](https://goreportcard.com/report/github.com/teeworlds-go/huffman)

Teeworlds huffman compression library.

## Installation

```shell

// for latest tagged released
go get github.com/teeworlds-go/huffman@latest

// for bleeding edge master branch version
go get github.com/teeworlds-go/huffman@master
```

## Sample usage

```go
package main

import (
    "fmt"
    "github.com/teeworlds-go/huffman"
)

func main() {
    data, err := huffman.Compress([]byte("hello world"))
    if err != nil {
        panic(err)
    }
    // data: [174 149 19 92 9 87 194 22 177 86 220 218 34 56 185 18 156 168 184 1]
    fmt.Printf("data: %v\n", data)

    data, err = huffman.Decompress([]byte{174, 149, 19, 92, 9, 87, 194, 22, 177, 86, 220, 218, 34, 56, 185, 18, 156, 168, 184, 1})
    if err != nil {
        panic(err)
    }
    // data: hello world
    fmt.Printf("data: %v\n", string(data))
}
```
