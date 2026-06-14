package huffman

import (
	"fmt"
)

const (
	EofSymbol  = 256
	MaxSymbols = EofSymbol
)

// Compress compresses the given data using the default Teeworlds' dictionary.
func Compress(data []byte) ([]byte, error) {
	return NewHuffmanDict(DefaultDictionary).Compress(data)
}

// CompressDict compresses the given data using the given dictionary.
func CompressDict(dict *Dictionary, data []byte) ([]byte, error) {
	return NewHuffmanDict(dict).Compress(data)
}

// Decompress decompresses the given data using the default Teeworlds' dictionary.
func Decompress(data []byte) ([]byte, error) {
	return NewHuffmanDict(DefaultDictionary).Decompress(data)
}

// DecompressDict decompresses the given data using the given dictionary.
func DecompressDict(dict *Dictionary, data []byte) ([]byte, error) {
	return NewHuffmanDict(dict).Decompress(data)
}

type Huffman struct {
	*Dictionary
}

// NewHuffman creates a new Huffman instance with the default dictionary.
func NewHuffman() *Huffman {
	return NewHuffmanDict(DefaultDictionary)
}

// NewHuffmanDict creates a new Huffman instance with the given dictionary.
func NewHuffmanDict(d *Dictionary) *Huffman {
	return &Huffman{
		Dictionary: d,
	}
}

// Decompress decompresses the given data.
func (huff *Huffman) Decompress(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return []byte{}, nil
	}
	return huff.DecompressTo(nil, data)
}

// DecompressTo decompresses data, APPENDING the decompressed bytes to dst and
// returning the extended slice (like Go's append). Pass a reused buffer's
// dst[:0] to avoid allocating a fresh output slice on every call — useful on
// hot paths that decompress many packets (e.g. a 50Hz snapshot stream). dst may
// be nil. huff is not modified, so a single Huffman value is safe for
// concurrent DecompressTo calls with distinct dst buffers.
func (huff *Huffman) DecompressTo(dst, data []byte) ([]byte, error) {
	if len(data) == 0 {
		return dst, nil
	}

	srcIndex := 0
	size := len(data)
	bits := uint32(0)
	bitcount := uint8(0)
	eof := &huff.nodes[EofSymbol]
	var n *node

	for {
		n = nil
		if bitcount >= lookupTableBits {
			n = huff.decodeLut[bits&lookupTableMask]
		}

		for bitcount < 24 && srcIndex < size {
			bits |= uint32(data[srcIndex]) << bitcount
			srcIndex += 1
			bitcount += 8
		}

		if n == nil {
			n = huff.decodeLut[bits&lookupTableMask]
		}

		if n == nil {
			return nil, fmt.Errorf("%w: node is nil", ErrHuffmanDecompress)
		}

		if n.NumBits != 0 {
			bits >>= n.NumBits
			bitcount -= n.NumBits
		} else {
			bits >>= lookupTableBits
			bitcount -= lookupTableBits

			for {
				n = &huff.nodes[n.Leafs[bits&1]]

				bitcount--
				bits >>= 1

				if n.NumBits != 0 {
					break
				}

				if bitcount == 0 {
					return nil, fmt.Errorf("%w: no more bits", ErrHuffmanDecompress)
				}
			}
		}
		if n == eof {
			break
		}

		dst = append(dst, n.Symbol)
	}

	return dst, nil
}

// Compress compresses the given data.
func (huff *Huffman) Compress(data []byte) ([]byte, error) {

	srcIndex := 0
	end := len(data)
	bits := uint32(0)
	bitcount := uint8(0)
	dst := []byte{}

	if len(data) == 0 {
		return []byte{}, nil
	}

	symbol := data[srcIndex]
	srcIndex++

	for srcIndex < end {
		bits |= huff.nodes[symbol].Bits << bitcount
		bitcount += huff.nodes[symbol].NumBits

		symbol = data[srcIndex]
		srcIndex++

		for bitcount >= 8 {
			dst = append(dst, byte(bits&0xff))
			bits >>= 8
			bitcount -= 8
		}
	}

	bits |= huff.nodes[symbol].Bits << bitcount
	bitcount += huff.nodes[symbol].NumBits
	for bitcount >= 8 {
		dst = append(dst, byte(bits&0xff))
		bits >>= 8
		bitcount -= 8
	}

	bits |= huff.nodes[EofSymbol].Bits << bitcount
	bitcount += huff.nodes[EofSymbol].NumBits
	for bitcount >= 8 {
		dst = append(dst, byte(bits&0xff))
		bits >>= 8
		bitcount -= 8
	}
	dst = append(dst, byte(bits))
	return dst, nil
}
