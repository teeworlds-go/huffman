package huffman

import (
	"errors"
)

const (
	HuffmanEOFSymbol  = 256
	HuffmanMaxSymbols = HuffmanEOFSymbol
)

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

func (huff *Huffman) Decompress(data []byte) ([]byte, error) {

	dst := []byte{}
	srcIndex := 0
	size := len(data)
	bits := uint32(0)
	bitcount := uint8(0)
	eof := &huff.nodes[HuffmanEOFSymbol]
	var n *node

	for {
		n = nil
		if bitcount >= huffmanLookupTableBits {
			n = huff.decodeLut[bits&huffmanLookupTableMask]
		}

		for bitcount < 24 && srcIndex < size {
			bits |= uint32(data[srcIndex]) << bitcount
			srcIndex += 1
			bitcount += 8
		}

		if n == nil {
			n = huff.decodeLut[bits&huffmanLookupTableMask]
		}

		if n == nil {
			return nil, errors.New("Failed to decompress data (node is nil).")
		}

		if n.NumBits != 0 {
			bits >>= n.NumBits
			bitcount -= n.NumBits
		} else {
			bits >>= huffmanLookupTableBits
			bitcount -= huffmanLookupTableBits

			for {
				n = &huff.nodes[n.Leafs[bits&1]]

				bitcount--
				bits >>= 1

				if n.NumBits != 0 {
					break
				}

				if bitcount == 0 {
					return nil, errors.New("No more bits, decoding error")
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

func (huff *Huffman) Compress(data []byte) ([]byte, error) {

	srcIndex := 0
	end := len(data)
	bits := uint32(0)
	bitcount := uint8(0)
	dst := []byte{}

	if len(data) == 0 {
		return []byte{}, errors.New("Input empty")
	}

	Symbol := data[srcIndex]
	srcIndex++

	for srcIndex < end {
		bits |= huff.nodes[Symbol].Bits << bitcount
		bitcount += huff.nodes[Symbol].NumBits

		Symbol = data[srcIndex]
		srcIndex++

		for bitcount >= 8 {
			dst = append(dst, byte(bits&0xff))
			bits >>= 8
			bitcount -= 8
		}
	}

	bits |= huff.nodes[Symbol].Bits << bitcount
	bitcount += huff.nodes[Symbol].NumBits
	for bitcount >= 8 {
		dst = append(dst, byte(bits&0xff))
		bits >>= 8
		bitcount -= 8
	}

	bits |= huff.nodes[HuffmanEOFSymbol].Bits << bitcount
	bitcount += huff.nodes[HuffmanEOFSymbol].NumBits
	for bitcount >= 8 {
		dst = append(dst, byte(bits&0xff))
		bits >>= 8
		bitcount -= 8
	}
	dst = append(dst, byte(bits))
	return dst, nil
}
