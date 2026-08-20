package huffman

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// Regression suite.
//
// The point of this file is to lock down the *observable behaviour* of the
// package byte-for-byte so that performance work cannot silently change the
// wire format, the dictionary construction, or the error semantics.
//
// Golden vectors live in testdata/golden.txt and are regenerated with:
//
//	go test -run TestGolden -update
//
// Regenerating is only ever correct when a format change is intended.

var updateGolden = flag.Bool("update", false, "regenerate testdata golden files")

const goldenPath = "testdata/golden.txt"

func hashOf(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// goldenLines builds the full golden record set from the current implementation.
func goldenLines(t *testing.T) []string {
	t.Helper()

	var lines []string
	rec := func(format string, args ...any) {
		lines = append(lines, fmt.Sprintf(format, args...))
	}

	// 1. dictionary structure. Any change to tree construction or the decode
	//    LUT changes the emitted codes, so pin it explicitly.
	d := NewDictionary()
	var dictBuf bytes.Buffer
	for i := 0; i < int(d.numNodes); i++ {
		n := &d.nodes[i]
		fmt.Fprintf(&dictBuf, "%d %d %d %d %d %d\n", i, n.Bits, n.NumBits, n.Leafs[0], n.Leafs[1], n.Symbol)
	}
	rec("dict/nodes\t%s", hashOf(dictBuf.Bytes()))

	// The symbol -> code mapping is the actual wire contract. The decode
	// lookup table is a pure accelerator derived from the tree, so its shape
	// (and lookupTableBits) is deliberately not pinned here; it is verified
	// against the tree by TestDecodeLUTMatchesTree instead.
	var codeBuf bytes.Buffer
	for i := 0; i <= EofSymbol; i++ {
		fmt.Fprintf(&codeBuf, "%d %d %d\n", i, d.nodes[i].Bits, d.nodes[i].NumBits)
	}
	rec("dict/codes\t%s", hashOf(codeBuf.Bytes()))

	// 2. compressed output for every corpus entry, bit-exact.
	huff := NewHuffman()
	for _, e := range regressionCorpus() {
		c, err := huff.Compress(e.data)
		if err != nil {
			t.Fatalf("compress %q: %v", e.name, err)
		}
		rec("compress/%s\tlen=%d\t%s", e.name, len(c), hashOf(c))
	}

	// Malformed-input behaviour is deliberately NOT pinned here: the
	// pre-optimization implementation does not terminate on inputs that lack
	// an EOF symbol, so there is no byte-exact behaviour to pin. It is
	// covered by TestMalformedTerminates instead.

	sort.Strings(lines)
	return lines
}

// malformedInputs are inputs that are not valid huffman streams produced by
// this package. They must never panic, hang, or read out of bounds.
func malformedInputs() []corpusEntry {
	out := []corpusEntry{
		{"empty", []byte{}},
		{"zero", []byte{0}},
		{"ff", []byte{0xff}},
		{"ffff", []byte{0xff, 0xff}},
		{"ff-x8", bytes.Repeat([]byte{0xff}, 8)},
		{"ff-x64", bytes.Repeat([]byte{0xff}, 64)},
		{"00-x64", make([]byte, 64)},
		{"aa-x64", bytes.Repeat([]byte{0xaa}, 64)},
		{"55-x64", bytes.Repeat([]byte{0x55}, 64)},
		{"random-16", randomBytes(101, 16)},
		{"random-64", randomBytes(102, 64)},
		{"random-1400", randomBytes(103, 1400)},
	}
	// truncations of a valid stream
	huff := NewHuffman()
	valid, err := huff.Compress(snapshotLike(104, 512))
	if err == nil {
		for _, n := range []int{1, 2, 3, 7, 8, 15, 16, len(valid) / 2, len(valid) - 1} {
			if n <= 0 || n >= len(valid) {
				continue
			}
			out = append(out, corpusEntry{"truncated/" + itoa(n), append([]byte(nil), valid[:n]...)})
		}
		// bit-flipped stream
		for _, pos := range []int{0, 3, len(valid) / 2, len(valid) - 1} {
			cp := append([]byte(nil), valid...)
			cp[pos] ^= 0x40
			out = append(out, corpusEntry{"flipped/" + itoa(pos), cp})
		}
	}
	return out
}

func TestGolden(t *testing.T) {
	got := goldenLines(t)

	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatal(err)
		}
		content := strings.Join(got, "\n") + "\n"
		if err := os.WriteFile(goldenPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %d golden records to %s", len(got), goldenPath)
		return
	}

	raw, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden (run: go test -run TestGolden -update): %v", err)
	}
	want := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")

	wantSet := map[string]string{}
	for _, l := range want {
		k, v, _ := strings.Cut(l, "\t")
		wantSet[k] = v
	}
	gotSet := map[string]string{}
	for _, l := range got {
		k, v, _ := strings.Cut(l, "\t")
		gotSet[k] = v
	}

	for k, wv := range wantSet {
		gv, ok := gotSet[k]
		if !ok {
			t.Errorf("golden record %q disappeared", k)
			continue
		}
		if gv != wv {
			t.Errorf("golden mismatch for %q:\n  want %s\n  got  %s", k, wv, gv)
		}
	}
	for k := range gotSet {
		if _, ok := wantSet[k]; !ok {
			t.Errorf("new golden record %q not in golden file (run -update if intended)", k)
		}
	}
}

// TestRoundtripCorpus verifies compress->decompress identity over the whole
// corpus, for several dictionaries.
func TestRoundtripCorpus(t *testing.T) {
	for _, dc := range testDictionaries() {
		huff := NewHuffmanDict(dc.dict)
		for _, e := range regressionCorpus() {
			c, err := huff.Compress(e.data)
			if err != nil {
				t.Fatalf("%s/%s: compress: %v", dc.name, e.name, err)
			}
			got, err := huff.Decompress(c)
			if err != nil {
				t.Fatalf("%s/%s: decompress: %v", dc.name, e.name, err)
			}
			if !bytes.Equal(got, e.data) {
				t.Fatalf("%s/%s: roundtrip mismatch: got %d bytes, want %d bytes", dc.name, e.name, len(got), len(e.data))
			}
		}
	}
}

type namedDict struct {
	name string
	dict *Dictionary
}

func testDictionaries() []namedDict {
	// flat: every symbol equally likely -> short, uniform codes
	var flat [MaxSymbols]uint32
	for i := range flat {
		flat[i] = 1
	}
	// skewed: one dominant symbol -> one very short code, many long ones
	var skewed [MaxSymbols]uint32
	for i := range skewed {
		skewed[i] = 1
	}
	skewed[0] = 1 << 30
	// fibonacci: deliberately produces long but still uint32-representable
	// codes. This is what overflowed the old 32 bit accumulator when combined
	// with leftover bits.
	var fib [MaxSymbols]uint32
	a, b := uint32(1), uint32(1)
	for i := range fib {
		fib[i] = a
		a, b = b, a+b
		if b > 1<<28 {
			a, b = 1, 1
		}
	}
	return []namedDict{
		{"default", DefaultDictionary},
		{"flat", NewDictionaryWithFrequencies(flat)},
		{"skewed", NewDictionaryWithFrequencies(skewed)},
		{"fibonacci", NewDictionaryWithFrequencies(fib)},
	}
}

// TestLongCodesRoundtrip pins the deep-tree case explicitly: with a fibonacci
// frequency table code lengths exceed 24 bits, which a 32 bit bit accumulator
// cannot hold together with leftover bits.
func TestLongCodesRoundtrip(t *testing.T) {
	var fib [MaxSymbols]uint32
	a, b := uint32(1), uint32(1)
	for i := range fib {
		fib[i] = a
		a, b = b, a+b
		if b > 1<<28 {
			a, b = 1, 1
		}
	}
	d := NewDictionaryWithFrequencies(fib)

	maxBits := uint8(0)
	for i := 0; i <= EofSymbol; i++ {
		if n := d.nodes[i].NumBits; n != 0xff && n > maxBits {
			maxBits = n
		}
	}
	if maxBits <= 24 {
		t.Fatalf("expected the fibonacci dictionary to produce codes longer than 24 bits, got max %d", maxBits)
	}
	t.Logf("fibonacci dictionary max code length: %d bits", maxBits)

	huff := NewHuffmanDict(d)

	// hammer the symbols that own the longest codes
	var longSymbols []byte
	for i := 0; i < MaxSymbols; i++ {
		if d.nodes[i].NumBits >= maxBits-2 && d.nodes[i].NumBits != 0xff {
			longSymbols = append(longSymbols, byte(i))
		}
	}
	if len(longSymbols) == 0 {
		t.Fatal("no long symbols found")
	}

	for n := 1; n <= 512; n++ {
		data := repeatTo(longSymbols, n)
		c, err := huff.Compress(data)
		if err != nil {
			t.Fatalf("n=%d: compress: %v", n, err)
		}
		got, err := huff.Decompress(c)
		if err != nil {
			t.Fatalf("n=%d: decompress: %v", n, err)
		}
		if !bytes.Equal(got, data) {
			t.Fatalf("n=%d: long-code roundtrip mismatch\n got %x\nwant %x", n, got, data)
		}
	}
}

// TestUnsupportedDeepDictionaryIsRejected covers frequency tables whose
// generated codes cannot be represented by the dictionary's uint32 code
// storage. Zero frequencies deliberately produce a maximally skewed tree.
// Such a dictionary must fail explicitly instead of silently emitting a
// corrupt stream from the nominal "deep" encoder path.
func TestUnsupportedDeepDictionaryIsRejected(t *testing.T) {
	var zero [MaxSymbols]uint32
	d := NewDictionaryWithFrequencies(zero)
	if d.maxCodeLen <= maxStoredCodeBits {
		t.Fatalf("zero-frequency dictionary max code length = %d, want > %d", d.maxCodeLen, maxStoredCodeBits)
	}

	huff := NewHuffmanDict(d)
	if _, err := huff.Compress([]byte{0}); err == nil {
		t.Fatal("Compress accepted a dictionary with codes wider than uint32")
	}
	if _, err := huff.Decompress([]byte{0}); err == nil {
		t.Fatal("Decompress accepted a dictionary with codes wider than uint32")
	}

	var compressed bytes.Buffer
	w := NewWriterDict(d, &compressed)
	if _, err := w.Write([]byte{0}); err == nil {
		t.Fatal("Writer accepted a dictionary with codes wider than uint32")
	}
	r := NewReaderDict(d, bytes.NewReader([]byte{0}))
	if _, err := r.Read(make([]byte, 1)); err == nil {
		t.Fatal("Reader accepted a dictionary with codes wider than uint32")
	}
}

func TestNilAndZeroValueDictionariesAreRejected(t *testing.T) {
	for name, dict := range map[string]*Dictionary{
		"nil":  nil,
		"zero": new(Dictionary),
	} {
		t.Run(name, func(t *testing.T) {
			huff := NewHuffmanDict(dict)
			for _, data := range [][]byte{nil, []byte("payload")} {
				if _, err := huff.Compress(data); !errors.Is(err, ErrHuffmanCompress) {
					t.Errorf("Compress(%d bytes) error = %v, want ErrHuffmanCompress", len(data), err)
				}
				if _, err := huff.Decompress(data); !errors.Is(err, ErrHuffmanDecompress) {
					t.Errorf("Decompress(%d bytes) error = %v, want ErrHuffmanDecompress", len(data), err)
				}
				if _, err := huff.DecompressTo(nil, data); !errors.Is(err, ErrHuffmanDecompress) {
					t.Errorf("DecompressTo(%d bytes) error = %v, want ErrHuffmanDecompress", len(data), err)
				}
			}

			var compressed bytes.Buffer
			if n, err := NewWriterDict(dict, &compressed).Write([]byte("payload")); n != 0 || !errors.Is(err, ErrHuffmanCompress) {
				t.Errorf("Writer.Write = (%d, %v), want (0, ErrHuffmanCompress)", n, err)
			}
			if n, err := NewReaderDict(dict, bytes.NewReader([]byte{0})).Read(make([]byte, 1)); n != 0 || !errors.Is(err, ErrHuffmanDecompress) {
				t.Errorf("Reader.Read = (%d, %v), want (0, ErrHuffmanDecompress)", n, err)
			}
		})
	}

	var nilHuffman *Huffman
	if _, err := nilHuffman.Compress(nil); !errors.Is(err, ErrHuffmanCompress) {
		t.Errorf("nil Huffman.Compress error = %v, want ErrHuffmanCompress", err)
	}
	if _, err := nilHuffman.Decompress(nil); !errors.Is(err, ErrHuffmanDecompress) {
		t.Errorf("nil Huffman.Decompress error = %v, want ErrHuffmanDecompress", err)
	}
	var nilWriter *Writer
	if _, err := nilWriter.Write(nil); !errors.Is(err, ErrHuffmanCompress) {
		t.Errorf("nil Writer.Write error = %v, want ErrHuffmanCompress", err)
	}
	var nilReader *Reader
	if _, err := nilReader.Read(make([]byte, 1)); !errors.Is(err, ErrHuffmanDecompress) {
		t.Errorf("nil Reader.Read error = %v, want ErrHuffmanDecompress", err)
	}
	if _, err := CompressDict(nil, nil); !errors.Is(err, ErrHuffmanCompress) {
		t.Errorf("CompressDict(nil) error = %v, want ErrHuffmanCompress", err)
	}
	if _, err := DecompressDict(nil, nil); !errors.Is(err, ErrHuffmanDecompress) {
		t.Errorf("DecompressDict(nil) error = %v, want ErrHuffmanDecompress", err)
	}
}

func TestCopiedDictionaryRemainsUsable(t *testing.T) {
	dict := *NewDictionary()
	huff := NewHuffmanDict(&dict)
	payload := snapshotLike(61, 1400)
	compressed, err := huff.Compress(payload)
	if err != nil {
		t.Fatal(err)
	}
	decompressed, err := huff.Decompress(compressed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decompressed, payload) {
		t.Fatal("copied dictionary roundtrip mismatch")
	}
}

// TestWriterMatchesCompress: the streaming Writer must emit exactly what the
// one-shot Compress emits.
func TestWriterMatchesCompress(t *testing.T) {
	for _, dc := range testDictionaries() {
		huff := NewHuffmanDict(dc.dict)
		for _, e := range regressionCorpus() {
			want, err := huff.Compress(e.data)
			if err != nil {
				t.Fatalf("%s/%s: compress: %v", dc.name, e.name, err)
			}
			var buf bytes.Buffer
			w := NewWriterDict(dc.dict, &buf)
			n, err := w.Write(e.data)
			if err != nil {
				t.Fatalf("%s/%s: write: %v", dc.name, e.name, err)
			}
			if n != len(e.data) {
				t.Fatalf("%s/%s: Write returned %d, want %d", dc.name, e.name, n, len(e.data))
			}
			if !bytes.Equal(buf.Bytes(), want) {
				t.Fatalf("%s/%s: Writer output differs from Compress\n got %x\nwant %x", dc.name, e.name, buf.Bytes(), want)
			}
		}
	}
}

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) {
	return len(p) / 2, nil
}

func TestWriterReportsShortWrite(t *testing.T) {
	for _, payload := range [][]byte{
		[]byte("must not be silently truncated"),
		bytes.Repeat([]byte{0xff}, 4096),
	} {
		w := NewWriter(shortWriter{})
		if n, err := w.Write(payload); n != 0 || !errors.Is(err, io.ErrShortWrite) {
			t.Fatalf("Writer.Write(%d bytes) = (%d, %v), want (0, io.ErrShortWrite)", len(payload), n, err)
		}
	}
}

// TestReaderMatchesDecompress: the streaming Reader must decode exactly what
// the one-shot Decompress decodes.
func TestReaderMatchesDecompress(t *testing.T) {
	for _, dc := range testDictionaries() {
		huff := NewHuffmanDict(dc.dict)
		for _, e := range regressionCorpus() {
			compressed, err := huff.Compress(e.data)
			if err != nil {
				t.Fatalf("%s/%s: compress: %v", dc.name, e.name, err)
			}
			r := NewReaderDict(dc.dict, bytes.NewReader(compressed))
			dst := make([]byte, len(e.data)+16)
			n, err := r.Read(dst)
			if err != nil && err != io.EOF {
				t.Fatalf("%s/%s: read: %v", dc.name, e.name, err)
			}
			if !bytes.Equal(dst[:n], e.data) {
				t.Fatalf("%s/%s: Reader output differs (got %d bytes, want %d)", dc.name, e.name, n, len(e.data))
			}
		}
	}
}

// TestReaderShortBuffer pins the behaviour when the destination buffer is
// smaller than the decompressed payload.
func TestReaderShortBuffer(t *testing.T) {
	data := snapshotLike(21, 1000)
	compressed, err := Compress(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []int{1, 2, 7, 63, 64, 999} {
		r := NewReader(bytes.NewReader(compressed))
		dst := make([]byte, size)
		n, err := r.Read(dst)
		if err != nil && err != io.EOF {
			t.Fatalf("size=%d: %v", size, err)
		}
		if n != size {
			t.Fatalf("size=%d: got n=%d, want %d", size, n, size)
		}
		if !bytes.Equal(dst[:n], data[:n]) {
			t.Fatalf("size=%d: prefix mismatch", size)
		}
	}
}

// TestReaderContinuesAfterShortBuffer exercises Reader as an actual
// io.Reader: filling p before reaching the Huffman EOF symbol must return a
// non-terminal read and preserve the bit accumulator for the next call.
func TestReaderContinuesAfterShortBuffer(t *testing.T) {
	data := snapshotLike(22, 1000)
	compressed, err := Compress(data)
	if err != nil {
		t.Fatal(err)
	}

	r := NewReader(bytes.NewReader(compressed))
	var got []byte
	buf := make([]byte, 7)
	for {
		n, err := r.Read(buf)
		got = append(got, buf[:n]...)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Read after %d output bytes: %v", len(got), err)
		}
		if n == 0 {
			t.Fatal("Reader made no progress before EOF")
		}
	}

	if !bytes.Equal(got, data) {
		t.Fatalf("Reader with short buffers returned %d bytes, want %d", len(got), len(data))
	}
}

// TestMalformedTerminates is the security-relevant regression test.
//
// A huffman stream that never contains the EOF symbol must not make the
// decoder spin forever, and must not make it allocate without bound. This
// library decodes untrusted network packets, so a non-terminating decoder is
// a remote memory-exhaustion DoS.
//
// Each case runs on its own goroutine with a deadline so that a regression
// reports a failure instead of hanging the whole test binary.
func TestMalformedTerminates(t *testing.T) {
	huff := NewHuffman()

	type result struct {
		out []byte
		err error
	}

	check := func(t *testing.T, name string, in []byte) {
		t.Helper()
		done := make(chan result, 1)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					done <- result{nil, fmt.Errorf("panic: %v", r)}
				}
			}()
			out, err := huff.Decompress(in)
			done <- result{out, err}
		}()

		select {
		case res := <-done:
			if strings.HasPrefix(fmt.Sprint(res.err), "panic:") {
				t.Errorf("%s: %v", name, res.err)
				return
			}
			// A malformed stream must not decode to an unbounded amount of
			// data. The tightest sound bound is one symbol per input bit.
			if max := 8*len(in) + 1; len(res.out) > max {
				t.Errorf("%s: produced %d bytes from %d input bytes (max %d): decoder is inventing data",
					name, len(res.out), len(in), max)
			}
		case <-time.After(5 * time.Second):
			t.Errorf("%s: Decompress did not terminate (infinite loop on malformed input)", name)
		}
	}

	for _, e := range malformedInputs() {
		check(t, e.name, e.data)
	}
	for seed := int64(0); seed < 200; seed++ {
		check(t, "random/"+itoa(int(seed)), randomBytes(seed, 1+int(seed)%97))
	}
}

// TestTruncatedStreamErrors: a valid stream cut short must report an error
// rather than silently returning a short read as if it were complete.
func TestTruncatedStreamErrors(t *testing.T) {
	huff := NewHuffman()
	payload := snapshotLike(51, 512)
	valid, err := huff.Compress(payload)
	if err != nil {
		t.Fatal(err)
	}

	for n := 1; n < len(valid); n += 7 {
		truncated := valid[:n]
		done := make(chan error, 1)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					done <- fmt.Errorf("panic: %v", r)
				}
			}()
			_, err := huff.Decompress(truncated)
			done <- err
		}()
		select {
		case err := <-done:
			if err == nil {
				t.Errorf("truncated to %d/%d bytes: expected an error, got nil", n, len(valid))
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("truncated to %d/%d bytes: Decompress did not terminate", n, len(valid))
		}
	}
}

// TestReaderMalformedNoPanic mirrors TestMalformedNoPanic for the Reader.
func TestReaderMalformedNoPanic(t *testing.T) {
	dst := make([]byte, 4096)
	for _, e := range malformedInputs() {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("panic on malformed input %q: %v", e.name, r)
				}
			}()
			r := NewReader(bytes.NewReader(e.data))
			_, _ = r.Read(dst)
		}()
	}
}

// FuzzMalformedDecodersBounded feeds arbitrary network-style bytes through
// both decoder APIs. Panics are reported by the fuzzing engine; the explicit
// bound also ensures malformed input cannot make either decoder invent more
// than one output symbol per supplied bit.
func FuzzMalformedDecodersBounded(f *testing.F) {
	for _, seed := range [][]byte{
		nil,
		{0x00},
		{0xff},
		{0x1a},
		{0x62, 0x91, 0x62, 0xa9},
		{0x4c, 0x04, 0xfe, 0x00, 0x68},
		bytes.Repeat([]byte{0xff}, 64),
		bytes.Repeat([]byte{0x00}, 64),
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		bound := uint64(len(data))*8 + 1
		huff := NewHuffman()
		out, _ := huff.Decompress(data)
		if uint64(len(out)) > bound {
			t.Fatalf("Decompress produced %d bytes from %d input bytes (bound %d)", len(out), len(data), bound)
		}

		r := NewReader(bytes.NewReader(data))
		buf := make([]byte, 7)
		var total uint64
		for {
			n, err := r.Read(buf)
			total += uint64(n)
			if total > bound {
				t.Fatalf("Reader produced %d bytes from %d input bytes (bound %d)", total, len(data), bound)
			}
			if err != nil {
				break
			}
			if n == 0 {
				t.Fatal("Reader made no progress before a terminal result")
			}
		}
	})
}

// TestWriterReset / TestReaderReset pin reuse semantics.
func TestWriterReset(t *testing.T) {
	var buf1, buf2 bytes.Buffer
	w := NewWriter(&buf1)
	payload := snapshotLike(31, 300)
	if _, err := w.Write(payload); err != nil {
		t.Fatal(err)
	}
	w.Reset(&buf2)
	if _, err := w.Write(payload); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf1.Bytes(), buf2.Bytes()) {
		t.Fatal("output after Reset differs from output before Reset")
	}
}

func TestReaderReset(t *testing.T) {
	payload := snapshotLike(32, 300)
	compressed, err := Compress(payload)
	if err != nil {
		t.Fatal(err)
	}
	r := NewReader(bytes.NewReader(compressed))
	dst1 := make([]byte, len(payload))
	n1, err := r.Read(dst1)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
	r.Reset(bytes.NewReader(compressed))
	dst2 := make([]byte, len(payload))
	n2, err := r.Read(dst2)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
	if n1 != n2 || !bytes.Equal(dst1[:n1], dst2[:n2]) {
		t.Fatal("output after Reset differs from output before Reset")
	}
}

// TestCompressDoesNotAliasInput guards against optimizations that return a
// slice backed by the caller's buffer, or that mutate the input.
func TestCompressDoesNotAliasInput(t *testing.T) {
	huff := NewHuffman()
	for _, e := range regressionCorpus() {
		if len(e.data) == 0 {
			continue
		}
		in := append([]byte(nil), e.data...)
		c, err := huff.Compress(in)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(in, e.data) {
			t.Fatalf("%s: Compress mutated its input", e.name)
		}
		if len(c) > 0 && len(in) > 0 && &c[0] == &in[0] {
			t.Fatalf("%s: Compress aliased its input", e.name)
		}

		out, err := huff.Decompress(c)
		if err != nil {
			t.Fatal(err)
		}
		before := append([]byte(nil), c...)
		if !bytes.Equal(c, before) {
			t.Fatalf("%s: Decompress mutated its input", e.name)
		}
		if len(out) > 0 && &out[0] == &c[0] {
			t.Fatalf("%s: Decompress aliased its input", e.name)
		}
	}
}

// TestConcurrentUse pins that a *Huffman / *Dictionary is safe for concurrent
// readers, which it must remain: DefaultDictionary is a shared global.
func TestConcurrentUse(t *testing.T) {
	huff := NewHuffman()
	payload := snapshotLike(41, 4096)
	want, err := huff.Compress(payload)
	if err != nil {
		t.Fatal(err)
	}

	const goroutines = 8
	errCh := make(chan error, goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			for i := 0; i < 50; i++ {
				c, err := huff.Compress(payload)
				if err != nil {
					errCh <- err
					return
				}
				if !bytes.Equal(c, want) {
					errCh <- fmt.Errorf("concurrent compress mismatch")
					return
				}
				d, err := huff.Decompress(c)
				if err != nil {
					errCh <- err
					return
				}
				if !bytes.Equal(d, payload) {
					errCh <- fmt.Errorf("concurrent decompress mismatch")
					return
				}
			}
			errCh <- nil
		}()
	}
	for g := 0; g < goroutines; g++ {
		if err := <-errCh; err != nil {
			t.Fatal(err)
		}
	}
}

// TestDecodeLUTMatchesTree verifies the flat decode table against the tree it
// is derived from, for every possible lookupTableBits-wide bit pattern. This
// replaces pinning the table's bytes: it checks the property that matters
// (the accelerator agrees with the tree) and stays valid if lookupTableBits
// is retuned for performance.
func TestDecodeLUTMatchesTree(t *testing.T) {
	for _, dc := range testDictionaries() {
		d := dc.dict
		for i := 0; i < lookupTableSize; i++ {
			// walk the tree manually for this bit pattern
			bits := uint32(i)
			n := d.startNode
			depth := 0
			for ; depth < lookupTableBits; depth++ {
				n = &d.nodes[n.Leafs[bits&1]]
				bits >>= 1
				if n.NumBits > 0 {
					depth++
					break
				}
			}

			entry := d.decLut[i]
			codeLen := int(entry & lutLenMask)

			if n.NumBits > 0 {
				// resolvable: table must report the same symbol and length
				if codeLen != int(n.NumBits) {
					t.Fatalf("%s: lut[%d] length %d, tree says %d", dc.name, i, codeLen, n.NumBits)
				}
				if codeLen != depth {
					t.Fatalf("%s: lut[%d] length %d, walked %d bits", dc.name, i, codeLen, depth)
				}
				isEOF := entry&lutEOFBit != 0
				wantEOF := n == &d.nodes[EofSymbol]
				if isEOF != wantEOF {
					t.Fatalf("%s: lut[%d] EOF flag %v, want %v", dc.name, i, isEOF, wantEOF)
				}
				if !wantEOF && byte(entry>>lutSymShift) != n.Symbol {
					t.Fatalf("%s: lut[%d] symbol %d, tree says %d", dc.name, i, byte(entry>>lutSymShift), n.Symbol)
				}
			} else {
				// not resolvable: table must point at the internal node the
				// walk ended on
				if codeLen != 0 {
					t.Fatalf("%s: lut[%d] claims length %d but the tree needs a deeper walk", dc.name, i, codeLen)
				}
				idx := entry >> lutNodeShift
				if int(idx) >= len(d.nodes) || &d.nodes[idx] != n {
					t.Fatalf("%s: lut[%d] node index %d does not match the tree walk", dc.name, i, idx)
				}
			}
		}
	}
}

// TestDecompressToSemantics covers the append contract of DecompressTo across
// the whole corpus: it must append rather than overwrite, must agree with
// Decompress byte for byte, and must not reallocate a buffer that is already
// large enough.
func TestDecompressToSemantics(t *testing.T) {
	huff := NewHuffman()
	prefix := []byte("KEEP-ME")

	for _, e := range regressionCorpus() {
		compressed, err := huff.Compress(e.data)
		if err != nil {
			t.Fatalf("%s: compress: %v", e.name, err)
		}
		want, err := huff.Decompress(compressed)
		if err != nil {
			t.Fatalf("%s: decompress: %v", e.name, err)
		}

		// appends to existing content, leaving it intact
		dst := append([]byte(nil), prefix...)
		got, err := huff.DecompressTo(dst, compressed)
		if err != nil {
			t.Fatalf("%s: DecompressTo: %v", e.name, err)
		}
		if !bytes.HasPrefix(got, prefix) {
			t.Fatalf("%s: DecompressTo overwrote the caller's existing bytes", e.name)
		}
		if !bytes.Equal(got[len(prefix):], want) {
			t.Fatalf("%s: DecompressTo payload differs from Decompress", e.name)
		}

		// a buffer with ample spare capacity must be reused, not replaced
		big := make([]byte, 0, len(e.data)+len(compressed)+8192)
		before := &big[:1][0]
		out, err := huff.DecompressTo(big[:0], compressed)
		if err != nil {
			t.Fatalf("%s: DecompressTo(reused): %v", e.name, err)
		}
		if !bytes.Equal(out, want) {
			t.Fatalf("%s: reused-buffer output differs", e.name)
		}
		if len(out) > 0 && &out[0] != before {
			t.Fatalf("%s: DecompressTo reallocated a buffer that was already big enough", e.name)
		}
	}
}

// TestDecompressToZeroAlloc pins the property the API exists for.
func TestDecompressToZeroAlloc(t *testing.T) {
	huff := NewHuffman()
	payload := snapshotLike(61, 1400)
	compressed, err := huff.Compress(payload)
	if err != nil {
		t.Fatal(err)
	}

	buf := make([]byte, 0, 4096)
	buf, err = huff.DecompressTo(buf[:0], compressed) // warm up
	if err != nil {
		t.Fatal(err)
	}

	allocs := testing.AllocsPerRun(100, func() {
		buf, _ = huff.DecompressTo(buf[:0], compressed)
	})
	if allocs != 0 {
		t.Errorf("DecompressTo with a reused buffer: %.1f allocs/op, want 0", allocs)
	}
}
