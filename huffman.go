package huffman

import (
	"errors"
	"sort"
)

const (
	eofSymbol  = 256
	maxSymbols = eofSymbol + 1
	maxNodes   = maxSymbols*2 - 1
	lutbits    = 10
	lutsize    = (1 << lutbits)
	lutmask    = (lutsize - 1)
)

var frequencyTable = [257]int{1 << 30, 4545, 2657, 431, 1950, 919, 444, 482, 2244,
	617, 838, 542, 715, 1814, 304, 240, 754, 212, 647, 186,
	283, 131, 146, 166, 543, 164, 167, 136, 179, 859, 363, 113, 157, 154, 204, 108, 137, 180, 202, 176,
	872, 404, 168, 134, 151, 111, 113, 109, 120, 126, 129, 100, 41, 20, 16, 22, 18, 18, 17, 19,
	16, 37, 13, 21, 362, 166, 99, 78, 95, 88, 81, 70, 83, 284, 91, 187, 77, 68, 52, 68,
	59, 66, 61, 638, 71, 157, 50, 46, 69, 43, 11, 24, 13, 19, 10, 12, 12, 20, 14, 9,
	20, 20, 10, 10, 15, 15, 12, 12, 7, 19, 15, 14, 13, 18, 35, 19, 17, 14, 8, 5,
	15, 17, 9, 15, 14, 18, 8, 10, 2173, 134, 157, 68, 188, 60, 170, 60, 194, 62, 175, 71,
	148, 67, 167, 78, 211, 67, 156, 69, 1674, 90, 174, 53, 147, 89, 181, 51, 174, 63, 163, 80,
	167, 94, 128, 122, 223, 153, 218, 77, 200, 110, 190, 73, 174, 69, 145, 66, 277, 143, 141, 60,
	136, 53, 180, 57, 142, 57, 158, 61, 166, 112, 152, 92, 26, 22, 21, 28, 20, 26, 30, 21,
	32, 27, 20, 17, 23, 21, 30, 22, 22, 21, 27, 25, 17, 27, 23, 18, 39, 26, 15, 21,
	12, 18, 18, 27, 20, 18, 15, 19, 11, 17, 33, 12, 18, 15, 19, 18, 16, 26, 17, 18,
	9, 10, 25, 22, 22, 17, 20, 16, 6, 16, 15, 20, 14, 18, 24, 335, 1517}

type ConstructNode struct {
	nodeId    uint16
	frequency int
}

func compareNodesByFrequencyDesc(nodes []*ConstructNode) (func(int, int) bool) {
	return func(i, j int) bool {
		return nodes[i].frequency > nodes[j].frequency
	}
}

type Node struct {
	bits    uint32
	numBits uint32
	leafs   [2]uint16
	symbol  byte
}

type Huffman struct {
	nodes       [maxNodes]Node
	decodedLuts [lutsize]*Node
	startNode   *Node
	numNodes    int
}

func (huff *Huffman) setbitsR(node *Node, bits int, depth uint32) {
	if node.leafs[1] != 0xffff {
		huff.setbitsR(&huff.nodes[1], bits|(1<<depth), depth+1)
	}
	if node.leafs[0] != 0xffff {
		huff.setbitsR(&huff.nodes[0], bits, depth+1)
	}

	if node.numBits != 0 {
		node.bits = uint32(bits)
		node.numBits = depth
	}
}

func (huff *Huffman) constructTree(frequencies []int) {
	nodesLeftStorage := [maxSymbols]ConstructNode{}
	nodesLeft := [maxSymbols]*ConstructNode{}
	numNodesLeft := maxSymbols

	for i := 0; i < maxSymbols; i++ {
		huff.nodes[i].numBits = 0xFFFFFFFF
		huff.nodes[i].symbol = byte(i)
		huff.nodes[i].leafs[0] = 0xffff
		huff.nodes[i].leafs[1] = 0xffff

		if i == eofSymbol {
			nodesLeftStorage[i].frequency = 1
		} else {
			nodesLeftStorage[i].frequency = frequencies[i]
		}
		nodesLeftStorage[i].nodeId = uint16(i)
		nodesLeft[i] = &nodesLeftStorage[i]
	}

	huff.numNodes = maxSymbols


	for numNodesLeft > 1 {
		sort.Slice(nodesLeft[:], compareNodesByFrequencyDesc(nodesLeft[:]))

		huff.nodes[huff.numNodes].numBits = 0
		huff.nodes[huff.numNodes].leafs[0] = nodesLeft[numNodesLeft-1].nodeId
		huff.nodes[huff.numNodes].leafs[1] = nodesLeft[numNodesLeft-2].nodeId
		nodesLeft[numNodesLeft-2].nodeId = uint16(huff.numNodes)
		nodesLeft[numNodesLeft-2].frequency =
			nodesLeft[numNodesLeft-1].frequency +
				nodesLeft[numNodesLeft-2].frequency

		huff.numNodes++
		numNodesLeft--
	}

	huff.startNode = &huff.nodes[huff.numNodes-1]

	huff.setbitsR(huff.startNode, 0, 0)
}

func (huff *Huffman) init() {
	huff.nodes = [maxNodes]Node{}
	huff.decodedLuts = [lutsize]*Node{}
	huff.startNode = nil
	huff.numNodes = 0

	huff.constructTree(frequencyTable[:])

	for i := 0; i < lutsize; i++ {
		bits := i
		k := 0
		node := huff.startNode
		for k = 0; k < lutbits; k++ {
			node = &huff.nodes[node.leafs[bits&1]]
			bits >>= 1

			if node == nil {
				break
			}

			if node.numBits != 0 {
				huff.decodedLuts[i] = node
				break
			}
		}

		if k == lutbits {
			huff.decodedLuts[i] = node
		}
	}
}

func (huff *Huffman) compress(data []byte) ([]byte, error) {
	srcIndex := 0
	end := len(data)
	bits := uint32(0)
	bitcount := uint32(0)
	dst := []byte{}

	if len(data) == 0 {
		return []byte{}, errors.New("Input empty")
	}

	symbol := data[srcIndex]
	srcIndex++

	for srcIndex < end {
		bits |= huff.nodes[symbol].bits << bitcount
		bitcount += huff.nodes[symbol].numBits

		symbol = data[srcIndex]
		srcIndex++

		for bitcount >= 8 {
			dst = append(dst, byte(bits&0xff))
			bits >>= 8
			bitcount -= 8
		}
	}

	bits |= huff.nodes[symbol].bits << bitcount
	bitcount += huff.nodes[symbol].numBits
	for bitcount >= 8 {
		dst = append(dst, byte(bits&0xff))
		bits >>= 8
		bitcount -= 8
	}

	bits |= huff.nodes[eofSymbol].bits << bitcount
	bitcount += huff.nodes[eofSymbol].numBits
	for bitcount >= 8 {
		dst = append(dst, byte(bits&0xff))
		bits >>= 8
		bitcount -= 8
	}
	dst = append(dst, byte(bits))
	return dst, nil
}
