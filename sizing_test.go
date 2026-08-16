package huffman

import (
	"math"
	"testing"
)

// The buffer sizing arithmetic is the only part of the codec whose correctness
// depends on the platform's int width. These tests drive it with the 32 bit
// limit explicitly, so the 32 bit behaviour is verified on any host, including
// the 64 bit ones CI usually runs on.
const limit32 = uint64(math.MaxInt32)

func TestSizingNeverExceedsLimit32(t *testing.T) {
	lens := []int{
		0, 1, 7, 8, 1400, 65536,
		1 << 20, 1 << 24,
		// the sizes where the pre-uint64 arithmetic used to wrap
		math.MaxInt32 / 8, math.MaxInt32/8 + 1,
		math.MaxInt32 / 15, math.MaxInt32 / 2,
		math.MaxInt32 - 1, math.MaxInt32,
	}

	for _, n := range lens {
		got := decompressInitCap(n, limit32)
		if got > limit32 {
			t.Errorf("decompressInitCap(%d) = %d, exceeds the 32 bit limit %d", n, got, limit32)
		}
		if got == 0 {
			t.Errorf("decompressInitCap(%d) = 0, would force immediate regrowth", n)
		}
		// must never be below the floor unless the hard bound caps it lower
		if bound := uint64(n)*8 + 8; got < 8192 && got != bound {
			t.Errorf("decompressInitCap(%d) = %d, below the 8 KiB floor without the bound explaining it", n, got)
		}

		total := growCap(n, got, limit32)
		if total > limit32 {
			t.Errorf("growCap(%d, %d) = %d, exceeds the 32 bit limit; make() would panic", n, got, total)
		}
		if total < uint64(n) && uint64(n) <= limit32 {
			t.Errorf("growCap(%d, %d) = %d, smaller than the existing length", n, got, total)
		}

		for _, codeLen := range []uint8{1, 15, 32, 56} {
			size, ok := compressBufSize(n, codeLen, limit32)
			if !ok {
				continue // correctly refused
			}
			if size > limit32 {
				t.Errorf("compressBufSize(%d, %d) = %d, exceeds the 32 bit limit", n, codeLen, size)
			}
			// must actually be big enough for the worst case
			need := (uint64(n)+1)*uint64(codeLen) + 8
			if size*8 < need {
				t.Errorf("compressBufSize(%d, %d) = %d bytes, too small for %d bits", n, codeLen, size, need)
			}
		}
	}
}

// TestCompressRefusesOversizedOn32Bit: on a 32 bit platform a large input
// needs an output buffer that cannot be represented. That has to be a reported
// error, not a wrapped size that later panics or silently truncates.
func TestCompressRefusesOversizedOn32Bit(t *testing.T) {
	// 300 MiB of input at 15 bits per symbol needs ~562 MiB, fine on 32 bit
	if _, ok := compressBufSize(300<<20, 15, limit32); !ok {
		t.Error("compressBufSize refused an input that fits comfortably on 32 bit")
	}
	// 1.5 GiB at 15 bits per symbol needs ~2.8 GiB, which does not fit
	if _, ok := compressBufSize(1536<<20, 15, limit32); ok {
		t.Error("compressBufSize accepted an input whose output cannot be represented on 32 bit")
	}
	// and the same input is fine on 64 bit
	if _, ok := compressBufSize(1536<<20, 15, maxAlloc64); !ok {
		t.Error("compressBufSize refused a 1.5 GiB input on a 64 bit limit")
	}
}

// The same helpers also run on 64 bit hosts. Extremely large slices are not
// practically allocatable, but the arithmetic must still reject an
// unrepresentable result instead of wrapping to a small, apparently safe
// allocation size.
func TestSizingDoesNotWrapOn64Bit(t *testing.T) {
	if ^uint(0)>>63 == 0 {
		t.Skip("requires a 64 bit int")
	}

	maxInt := int(^uint(0) >> 1)
	if got := decompressInitCap(maxInt, maxAlloc64); got != maxAlloc64 {
		t.Errorf("decompressInitCap(MaxInt) = %d, want saturated %d", got, maxAlloc64)
	}
	if size, ok := compressBufSize(maxInt, 255, maxAlloc64); ok {
		t.Errorf("compressBufSize(MaxInt, 255) = %d, want overflow rejection", size)
	}
	if got := growCap(maxInt, maxAlloc64, maxAlloc64); got != maxAlloc64 {
		t.Errorf("growCap(MaxInt, MaxInt) = %d, want saturated %d", got, maxAlloc64)
	}
}

const maxAlloc64 = uint64(math.MaxInt64)

// TestMaxAllocMatchesPlatform documents what maxAlloc resolves to here. The
// compile-time assertions in huffman.go pin it to MaxInt on every target; this
// just makes the value visible when the test runs.
func TestMaxAllocMatchesPlatform(t *testing.T) {
	if maxAlloc != uint64(^uint(0)>>1) {
		t.Fatalf("maxAlloc = %d, want MaxInt = %d", maxAlloc, uint64(^uint(0)>>1))
	}
	t.Logf("maxAlloc on this platform = %d (int is %d bits)", maxAlloc, (^uint(0)>>63)*32+32)
}
