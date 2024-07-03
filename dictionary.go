package huffman

import "sort"

const (
	maxNodes        = (MaxSymbols)*2 + 1 // +1 for additional EOF symbol
	lookupTableBits = 10
	lookupTableSize = (1 << lookupTableBits)
	lookupTableMask = (lookupTableSize - 1)
)

var (
	// DefaultDictionary is a huffman dictionary that is used to encode and decode data.
	// It is defined as a global variable in order to avoid re-creating it every time, as that is expensive.
	// This global value can be changed to a custom dictionary if needed which will then be reused globally.
	DefaultDictionary = NewDictionary()

	// TeeworldsFrequencyTable is the one used in Teeworlds by default.
	// The C++ implementation has an additional frequency on
	// the 256th index with the value 1517 which is overwritten
	// in the huffman constructor anyway, making it obsolete
	TeeworldsFrequencyTable = [MaxSymbols]uint32{
		1 << 30, 4545, 2657, 431, 1950, 919, 444, 482, 2244, 617, 838, 542, 715, 1814, 304, 240, 754, 212, 647, 186,
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
		9, 10, 25, 22, 22, 17, 20, 16, 6, 16, 15, 20, 14, 18, 24, 335,
	}
)

// Dictionary is a huffman lookup table/tree that is used to lookup symbols and their corresponding huffman codes.
type Dictionary struct {
	nodes     [maxNodes]node
	decodeLut [lookupTableSize]*node
	startNode *node
	numNodes  uint16
}

type node struct {
	// symbol
	Bits    uint32
	NumBits uint8

	// don't use pointers for this. shorts are smaller so we can fit more data into the cache
	Leafs [2]uint16

	// what the symbol represents
	Symbol byte
}

// NewDictionary returns a initialized lookup table that uses the Teeworlds' default frequency table,
// which can be found as TeeworldsFrequencyTable global variable.
func NewDictionary() *Dictionary {
	return NewDictionaryWithFrequencies(TeeworldsFrequencyTable)
}

func NewDictionaryWithFrequencies(frequencyTable [MaxSymbols]uint32) *Dictionary {

	d := Dictionary{}
	d.constructTree(frequencyTable)

	// build decode lookup table (LUT)
	for i := 0; i < lookupTableSize; i++ {
		var (
			bits uint32 = uint32(i)
			k    int
			n    = d.startNode
		)

		for k = 0; k < lookupTableBits; k++ {
			n = &d.nodes[n.Leafs[bits&1]]
			bits >>= 1

			if n.NumBits > 0 {
				d.decodeLut[i] = n
				break
			}
		}

		if k == lookupTableBits {
			d.decodeLut[i] = n
		}

	}
	return &d
}

func (d *Dictionary) setBitsR(n *node, bits uint32, depth uint8) {
	var (
		newBits uint32
		left    = n.Leafs[0]
		right   = n.Leafs[1]
	)

	if right < 0xffff {
		newBits = bits | (1 << depth)
		d.setBitsR(&d.nodes[right], newBits, depth+1)
	}
	if left < 0xffff {
		newBits = bits
		d.setBitsR(&d.nodes[left], newBits, depth+1)
	}

	if n.NumBits > 0 {
		n.Bits = bits
		n.NumBits = depth
	}
}

func (d *Dictionary) constructTree(frequencyTable [MaxSymbols]uint32) {

	var (
		// +1 for additional EOF symbol
		nodesLeftStorage [MaxSymbols + 1]constructNode
		nodesLeft        [MaxSymbols + 1]*constructNode
		numNodesLeft     = MaxSymbols + 1

		n  *node
		ns *constructNode
	)

	// +1 for EOF symbol
	for i := uint16(0); i < MaxSymbols+1; i++ {
		n = &d.nodes[i]
		n.NumBits = 0xff
		n.Symbol = byte(i)
		n.Leafs[0] = 0xffff
		n.Leafs[1] = 0xffff

		ns = &nodesLeftStorage[i]
		if i == EOFSymbol {
			ns.frequency = 1
		} else {
			ns.frequency = frequencyTable[i]
		}
		ns.nodeID = i
		nodesLeft[i] = ns
	}

	d.numNodes = MaxSymbols + 1 // +1 for EOF symbol
	for numNodesLeft > 1 {

		sort.Stable(byFrequencyDesc(nodesLeft[:numNodesLeft]))

		n = &d.nodes[d.numNodes]
		n1 := numNodesLeft - 1
		n2 := numNodesLeft - 2

		n.NumBits = 0
		n.Leafs[0] = nodesLeft[n1].nodeID
		n.Leafs[1] = nodesLeft[n2].nodeID

		freq1 := nodesLeft[n1].frequency
		freq2 := nodesLeft[n2].frequency

		nodesLeft[n2].nodeID = d.numNodes
		nodesLeft[n2].frequency = freq1 + freq2

		d.numNodes++
		numNodesLeft--
	}

	d.startNode = n
	d.setBitsR(n, 0, 0)
}

type constructNode struct {
	nodeID    uint16
	frequency uint32
}

type byFrequencyDesc []*constructNode

func (a byFrequencyDesc) Len() int           { return len(a) }
func (a byFrequencyDesc) Swap(i, j int)      { *a[i], *a[j] = *a[j], *a[i] }
func (a byFrequencyDesc) Less(i, j int) bool { return a[i].frequency > a[j].frequency }
