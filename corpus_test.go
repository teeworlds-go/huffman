package huffman

import (
	"bytes"
	"math/rand"
)

// This file contains deterministic test/benchmark corpora shared by the
// regression tests and the benchmarks. Everything here is generated from a
// fixed seed so that golden vectors stay stable across runs and machines.

type corpusEntry struct {
	name string
	data []byte
}

// deterministicRand returns a *rand.Rand seeded with a fixed value so corpora
// are byte-for-byte reproducible.
func deterministicRand(seed int64) *rand.Rand {
	return rand.New(rand.NewSource(seed))
}

// repeatTo grows pattern until it is exactly n bytes long.
func repeatTo(pattern []byte, n int) []byte {
	if len(pattern) == 0 {
		return make([]byte, n)
	}
	out := make([]byte, 0, n)
	for len(out) < n {
		out = append(out, pattern...)
	}
	return out[:n]
}

// randomBytes returns n uniformly random bytes (worst case for huffman: the
// default dictionary is tuned for teeworlds snapshots, not white noise).
func randomBytes(seed int64, n int) []byte {
	r := deterministicRand(seed)
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(r.Intn(256))
	}
	return b
}

// skewedBytes returns n bytes drawn from a distribution close to the teeworlds
// frequency table, i.e. the case the default dictionary is actually tuned for.
func skewedBytes(seed int64, n int) []byte {
	r := deterministicRand(seed)

	// cumulative distribution over the default frequency table
	var total uint64
	cum := make([]uint64, MaxSymbols)
	for i, f := range TeeworldsFrequencyTable {
		total += uint64(f)
		cum[i] = total
	}

	b := make([]byte, n)
	for i := range b {
		pick := uint64(r.Int63n(int64(total)))
		lo, hi := 0, MaxSymbols-1
		for lo < hi {
			mid := (lo + hi) / 2
			if cum[mid] <= pick {
				lo = mid + 1
			} else {
				hi = mid
			}
		}
		b[i] = byte(lo)
	}
	return b
}

// snapshotLike returns n bytes that look like teeworlds network traffic:
// mostly zeroes and small integers with occasional runs of other values.
func snapshotLike(seed int64, n int) []byte {
	r := deterministicRand(seed)
	b := make([]byte, 0, n)
	for len(b) < n {
		switch r.Intn(10) {
		case 0, 1, 2, 3, 4:
			// run of zeroes (very common in delta-compressed snapshots)
			b = append(b, bytes.Repeat([]byte{0}, 1+r.Intn(8))...)
		case 5, 6, 7:
			// small varint-ish values
			b = append(b, byte(r.Intn(32)))
		case 8:
			// ascii-ish payload (chat, names)
			b = append(b, byte(0x20+r.Intn(0x5f)))
		default:
			b = append(b, byte(r.Intn(256)))
		}
	}
	return b[:n]
}

// allSymbols returns a slice containing every byte value exactly once, which
// exercises every leaf of the tree including the long codes.
func allSymbols() []byte {
	b := make([]byte, 256)
	for i := range b {
		b[i] = byte(i)
	}
	return b
}

// benchCorpus is the shared corpus for benchmarks: a spread of realistic
// payload shapes and sizes. Sizes are chosen around typical teeworlds packet
// sizes (up to ~1400 bytes) plus larger sizes to measure steady-state
// throughput without per-call overhead dominating.
var benchCorpus = []corpusEntry{
	{"snapshot/64B", snapshotLike(1, 64)},
	{"snapshot/1400B", snapshotLike(2, 1400)},
	{"snapshot/64KB", snapshotLike(3, 64<<10)},
	{"skewed/1400B", skewedBytes(4, 1400)},
	{"skewed/64KB", skewedBytes(5, 64<<10)},
	{"random/1400B", randomBytes(6, 1400)},
	{"random/64KB", randomBytes(7, 64<<10)},
	{"zeroes/64KB", make([]byte, 64<<10)},
	{"text/64KB", repeatTo([]byte("the quick brown fox jumps over the lazy dog 0123456789\n"), 64<<10)},
}

// regressionCorpus is the corpus that golden vectors are pinned against. It
// includes every benchmark input plus small edge cases.
func regressionCorpus() []corpusEntry {
	out := []corpusEntry{
		{"empty", []byte{}},
		{"single/0x00", []byte{0}},
		{"single/0xff", []byte{0xff}},
		{"foo", []byte("foo")},
		{"hello world", []byte("hello world")},
		{"all-symbols", allSymbols()},
		{"zeroes/1", make([]byte, 1)},
		{"zeroes/7", make([]byte, 7)},
		{"zeroes/8", make([]byte, 8)},
		{"zeroes/9", make([]byte, 9)},
		{"zeroes/255", make([]byte, 255)},
		{"zeroes/256", make([]byte, 256)},
		{"zeroes/257", make([]byte, 257)},
		{"0xff/64", bytes.Repeat([]byte{0xff}, 64)},
		{"snapshot/small", snapshotLike(11, 37)},
		{"snapshot/1400B", snapshotLike(2, 1400)},
		{"random/1400B", randomBytes(6, 1400)},
		{"skewed/1400B", skewedBytes(4, 1400)},
	}
	// every length from 0..64 of skewed data, to catch bit-accumulator and
	// flush-boundary regressions at every possible bit offset.
	base := skewedBytes(12, 64)
	for n := 0; n <= 64; n++ {
		out = append(out, corpusEntry{
			name: "lenscan/" + itoa(n),
			data: append([]byte(nil), base[:n]...),
		})
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [8]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
