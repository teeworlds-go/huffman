package huffman

import (
	"bytes"
	"testing"
)

// Interoperability with the reference C++ implementations.
//
// Vectors are taken verbatim from ddnet/ddnet src/test/huffman_test.cpp at
// master (2026-08). The symbol table itself is shared with teeworlds 0.7:
// frequencies for symbols 0..255 are identical and the EOF symbol has an
// effective frequency of 1 in both, so all three implementations emit the
// same codes.
//
// Where teeworlds 0.7 and ddnet disagree it is noted per case: ddnet stopped
// writing the redundant trailing zero byte in 4354f8c6 (PR #12195), we follow
// ddnet there because the byte sits after the EOF symbol and is ignored by
// every decoder anyway.
func TestDDNetCompat(t *testing.T) {
	h := NewHuffman()

	t.Run("empty", func(t *testing.T) {
		// ddnet TEST(Huffman, CompressionInputSizeZero)
		// teeworlds 0.7 emits the same two bytes.
		want := []byte{0x8A, 0x1B}
		got, err := h.Compress(nil)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("Compress(empty) = %x, want %x", got, want)
		}
		back, err := h.Decompress(want)
		if err != nil {
			t.Fatalf("Decompress(%x): %v", want, err)
		}
		if len(back) != 0 {
			t.Errorf("Decompress(%x) = %x, want empty", want, back)
		}
	})

	t.Run("compatible", func(t *testing.T) {
		// ddnet TEST(Huffman, CompressionCompatible), explicitly labelled
		// there as the check against older/other implementations.
		in := make([]byte, 64)
		for i := range 8 {
			in[i] = byte(i)
		}
		want := []byte{
			0x51, 0x58, 0x78, 0x76, 0x1B, 0xB7, 0xFF, 0xFF,
			0xFF, 0xFF, 0xFF, 0xFF, 0x7F, 0xC5, 0x0D,
		}
		got, err := h.Compress(in)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("Compress = %x, want %x", got, want)
		}
	})

	t.Run("no trailing null", func(t *testing.T) {
		// ddnet TEST(Huffman, CompressionNoTrailingNull). teeworlds 0.7 and
		// ddnet before 4354f8c6 append an extra 0x00 here.
		in := make([]byte, 64)
		in[0] = 0x15
		want := []byte{0xBE, 0xFD, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x15, 0x37}
		got, err := h.Compress(in)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("Compress = %x, want %x", got, want)
		}
		// the older, one byte longer form must still decode
		legacy := append(append([]byte(nil), want...), 0x00)
		back, err := h.Decompress(legacy)
		if err != nil {
			t.Fatalf("Decompress(legacy trailing-null form): %v", err)
		}
		if !bytes.Equal(back, in) {
			t.Error("legacy trailing-null form did not round-trip")
		}
	})

	t.Run("rejects fuzz vectors", func(t *testing.T) {
		// ddnet TEST(Huffman, DecompressionTableLookupIntegerOverflow):
		// fuzz-found inputs that underflowed the bit counter. All three must
		// be rejected rather than decoded or looped on.
		for _, in := range [][]byte{
			{0x1A},
			{0x62, 0x91, 0x62, 0xA9},
			{0x4C, 0x04, 0xFE, 0x00, 0x68},
		} {
			if out, err := h.Decompress(in); err == nil {
				t.Errorf("Decompress(%x) = %x, want error", in, out)
			}
		}
	})
}
