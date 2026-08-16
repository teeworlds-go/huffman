package huffman

import "sort"

const (
	maxNodes          = (MaxSymbols)*2 + 1 // +1 for additional EOF symbol
	maxStoredCodeBits = 32                 // node.Bits and encBits are uint32

	// lookupTableBits controls how many bits the decoder can resolve with a
	// single table load; anything longer falls back to a bit-by-bit tree
	// walk. It is a pure decode accelerator and does not affect the wire
	// format.
	//
	// 12 bits means a 16 KiB table. Measured against 10 bits (4 KiB) it is
	// worth -58% on uniformly random payloads and -35% on text, where codes
	// are long and tree walks dominate, and is neutral on teeworlds-sized
	// snapshot packets. 13 bits was faster still on random data but needs a
	// 32 KiB table, which would not co-exist with the working set in the
	// 32-48 KiB L1d of a typical x86-64 core.
	lookupTableBits = 12
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

// Decode LUT entry layout.
//
// The decoder's inner loop reads one uint32 per symbol instead of chasing a
// *node pointer into a 12 byte struct. The whole table is 16 KiB and stays
// resident in L1, which is the single biggest win in the decoder.
//
//	bits  0..5  code length in bits, 0 means "not resolvable within
//	            lookupTableBits, walk the tree from nodeIndex"
//	bits  8..15 decoded symbol
//	bit   16    set if this is the EOF symbol
//	bits 17..31 node index to start the tree walk from
const (
	// 0x3f, not 0xff: code lengths stored here never exceed lookupTableBits,
	// and masking to 6 bits lets the compiler prove the shift count is < 64
	// so it emits a bare shift instead of a guarded one. That guard sits
	// directly on the decoder's loop-carried dependency chain.
	lutLenMask   = 0x3f
	lutSymShift  = 8
	lutEOFBit    = 1 << 16
	lutNodeShift = 17
)

// Dictionary is a huffman lookup table/tree that is used to lookup symbols and their corresponding huffman codes.
type Dictionary struct {
	// hot tables first, they are what the encode/decode loops touch

	// decLut is the flattened decode lookup table, see the lut* constants.
	decLut [lookupTableSize]uint32

	// encBits/encLen are the flattened encode table. Two parallel arrays of
	// 257 entries (~1.3 KiB total) instead of indexing into nodes, whose 12
	// byte stride wastes two thirds of every cache line the encoder touches.
	encBits [MaxSymbols + 1]uint32
	encLen  [MaxSymbols + 1]uint8

	nodes     [maxNodes]node
	startNode *node
	numNodes  uint16

	// maxCodeLen is the longest code this dictionary can emit. The codec uses
	// it to size output buffers and reject trees wider than its uint32 code
	// storage can represent.
	maxCodeLen uint8
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

	d.buildFastTables()
	return &d
}

// buildFastTables derives the flat encode/decode tables from the tree. It is
// pure derivation: it adds no information and changes no codes, so the wire
// format is unaffected.
func (d *Dictionary) buildFastTables() {
	for i := 0; i <= EofSymbol; i++ {
		n := &d.nodes[i]
		d.encBits[i] = n.Bits
		d.encLen[i] = n.NumBits
		if n.NumBits != 0xff && n.NumBits > d.maxCodeLen {
			d.maxCodeLen = n.NumBits
		}
	}

	// index of the root, so the walk below can stay in index space and never
	// needs a pointer -> index reverse lookup
	rootIdx := uint32(d.numNodes - 1)

	for i := 0; i < lookupTableSize; i++ {
		bits := uint32(i)
		idx := rootIdx
		k := 0

		for ; k < lookupTableBits; k++ {
			idx = uint32(d.nodes[idx].Leafs[bits&1])
			bits >>= 1
			if idx >= uint32(len(d.nodes)) {
				break
			}
			if d.nodes[idx].NumBits > 0 {
				k++
				break
			}
		}

		if idx >= uint32(len(d.nodes)) {
			continue
		}

		n := &d.nodes[idx]
		var entry uint32
		if n.NumBits > 0 {
			// resolvable directly: code length + symbol
			entry = uint32(n.NumBits) | uint32(n.Symbol)<<lutSymShift
			if idx == EofSymbol {
				entry |= lutEOFBit
			}
		} else {
			// needs a tree walk after consuming lookupTableBits bits
			entry = idx << lutNodeShift
		}
		d.decLut[i] = entry
	}
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
		if i == EofSymbol {
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
