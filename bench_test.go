package huffman

import (
	"bytes"
	"io"
	"testing"
)

// Benchmarks report ns/op and B/s relative to the *uncompressed* payload size,
// which is the number that matters for a network codec: how fast can we push
// application bytes through the codec.

func BenchmarkCompress(b *testing.B) {
	huff := NewHuffman()
	for _, e := range benchCorpus {
		e := e
		b.Run(e.name, func(b *testing.B) {
			b.SetBytes(int64(len(e.data)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				out, err := huff.Compress(e.data)
				if err != nil {
					b.Fatal(err)
				}
				sinkBytes = out
			}
		})
	}
}

func BenchmarkDecompress(b *testing.B) {
	huff := NewHuffman()
	for _, e := range benchCorpus {
		e := e
		compressed, err := huff.Compress(e.data)
		if err != nil {
			b.Fatal(err)
		}
		b.Run(e.name, func(b *testing.B) {
			b.SetBytes(int64(len(e.data)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				out, err := huff.Decompress(compressed)
				if err != nil {
					b.Fatal(err)
				}
				sinkBytes = out
			}
		})
	}
}

func BenchmarkWriter(b *testing.B) {
	for _, e := range benchCorpus {
		e := e
		b.Run(e.name, func(b *testing.B) {
			var buf bytes.Buffer
			buf.Grow(len(e.data) * 2)
			w := NewWriter(&buf)
			b.SetBytes(int64(len(e.data)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				buf.Reset()
				w.Reset(&buf)
				if _, err := w.Write(e.data); err != nil {
					b.Fatal(err)
				}
			}
			sinkInt = buf.Len()
		})
	}
}

func BenchmarkReader(b *testing.B) {
	huff := NewHuffman()
	for _, e := range benchCorpus {
		e := e
		compressed, err := huff.Compress(e.data)
		if err != nil {
			b.Fatal(err)
		}
		b.Run(e.name, func(b *testing.B) {
			dst := make([]byte, len(e.data)+16)
			src := bytes.NewReader(compressed)
			r := NewReader(src)
			b.SetBytes(int64(len(e.data)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				src.Reset(compressed)
				r.Reset(src)
				n, err := r.Read(dst)
				if err != nil && err != io.EOF {
					b.Fatal(err)
				}
				sinkInt = n
			}
		})
	}
}

// BenchmarkRoundtrip measures the full encode+decode path on a typical
// teeworlds-sized packet, which is the dominant real-world workload.
func BenchmarkRoundtrip(b *testing.B) {
	huff := NewHuffman()
	data := snapshotLike(2, 1400)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c, err := huff.Compress(data)
		if err != nil {
			b.Fatal(err)
		}
		d, err := huff.Decompress(c)
		if err != nil {
			b.Fatal(err)
		}
		sinkBytes = d
	}
}

// BenchmarkNewDictionary measures dictionary construction, which is the
// documented "expensive" one-time cost.
func BenchmarkNewDictionary(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sinkDict = NewDictionary()
	}
}

var (
	sinkBytes []byte
	sinkInt   int
	sinkDict  *Dictionary
)
