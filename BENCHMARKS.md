# Benchmarks

Before/after for the codec rewrite (commit `3a3eece` → `HEAD`).

## Method

Both revisions are benchmarked from the **same** working tree, so the benchmark
definitions and corpora are identical and only the codec differs. The two test
binaries are run **alternately**, six rounds each, so thermal drift and
background load hit both arms equally rather than biasing whichever ran first.
`benchstat` reports the delta with a Mann-Whitney p-value.

Reproduce with:

```shell
make benchcmp REV=3a3eece
```

Corpora: `snapshot` mimics teeworlds network traffic (runs of zeroes, small
integers, occasional ASCII), `skewed` is drawn from the default frequency
table, `random` is uniform white noise (the dictionary's worst case), `text`
and `zeroes` bracket the extremes. 1400 B is a typical teeworlds packet.

## Results, arm64 (Apple M2 Pro, go1.26.6, darwin)

Throughput over *uncompressed* bytes. Negative time = faster.

| Benchmark                     | before    | after     | delta      |
| ----------------------------- | --------- | --------- | ---------- |
| **Compress** snapshot/64B     | 103 ns    | 74 ns     | **-27.8%** |
| **Compress** snapshot/1400B   | 1.87 µs   | 1.82 µs   | ~          |
| **Compress** snapshot/64KB    | 199.5 µs  | 87.4 µs   | **-56.2%** |
| **Compress** skewed/64KB      | 73.5 µs   | 59.4 µs   | **-19.1%** |
| **Compress** random/1400B     | 2.75 µs   | 1.93 µs   | **-29.9%** |
| **Compress** random/64KB      | 272.3 µs  | 100.0 µs  | **-63.3%** |
| **Compress** text/64KB        | 133.3 µs  | 80.1 µs   | **-40.0%** |
| **Decompress** snapshot/64B   | 209 ns    | 167 ns    | **-20.1%** |
| **Decompress** snapshot/1400B | 4.62 µs   | 3.52 µs   | **-23.8%** |
| **Decompress** snapshot/64KB  | 295.6 µs  | 169.7 µs  | **-42.6%** |
| **Decompress** skewed/64KB    | 127.8 µs  | 120.2 µs  | ~          |
| **Decompress** random/64KB    | 647.6 µs  | 260.9 µs  | **-59.7%** |
| **Decompress** text/64KB      | 298.2 µs  | 156.0 µs  | **-47.7%** |
| **Writer** snapshot/1400B     | 1.96 µs   | 1.04 µs   | **-47.2%** |
| **Writer** snapshot/64KB      | 226.2 µs  | 77.3 µs   | **-65.8%** |
| **Writer** random/64KB        | 334.4 µs  | 107.9 µs  | **-67.7%** |
| **Writer** text/64KB          | 196.4 µs  | 78.4 µs   | **-60.1%** |
| **Reader** snapshot/1400B     | 5.37 µs   | 3.93 µs   | **-26.8%** |
| **Reader** skewed/64KB        | 138.1 µs  | 102.5 µs  | **-25.7%** |
| **Reader** random/64KB        | 843.3 µs  | 543.5 µs  | **-35.6%** |
| **Roundtrip** 1400B           | 6.86 µs   | 5.16 µs   | **-24.7%** |
| `NewDictionary`               | 196.8 µs  | 224.9 µs  | +14.3%     |
| **geomean (time)**            |           |           | **-37.4%** |
| **geomean (throughput)**      | 354 MiB/s | 554 MiB/s | **+56.4%** |

Peak throughput after: **~1.45 GiB/s** encode (`Writer` skewed/64KB),
**~715 MiB/s** one-shot `Compress`, **~625 MiB/s** decode.

`NewDictionary` is the one regression: the decode table is now 4x larger to
build. It is a one-time cost paid once per process for the package-level
`DefaultDictionary`, traded for the decode wins above.

### Allocations

The encoder now sizes its output buffer up front instead of growing it:

| Benchmark                 | before    | after    |
| ------------------------- | --------- | -------- |
| Compress snapshot/64KB    | 15 allocs | 2 allocs |
| Compress random/64KB      | 20 allocs | 1 alloc  |
| Compress snapshot/1400B   | 4 allocs  | 1 alloc  |
| Decompress snapshot/1400B | 6 allocs  | 1 alloc  |

A teeworlds-sized packet is one allocation in each direction.

## x86-64

The development machine is arm64, so **native x86-64 numbers were not
measured**. Running the amd64 build under Rosetta 2 gives the relative deltas
below. Emulated timings are *not* valid absolute x86-64 performance figures;
they are reported only as evidence that the wins are not arm64-specific.

| Benchmark                 | before   | after    | delta  |
| ------------------------- | -------- | -------- | ------ |
| Compress snapshot/64KB    | 255.2 µs | 128.2 µs | -49.8% |
| Compress random/64KB      | 361.1 µs | 145.2 µs | -59.8% |
| Decompress snapshot/1400B | 7.67 µs  | 4.49 µs  | -41.4% |
| Decompress snapshot/64KB  | 427.6 µs | 228.1 µs | -46.7% |
| Decompress random/64KB    | 750.5 µs | 335.7 µs | -55.3% |
| Roundtrip 1400B           | 10.17 µs | 6.44 µs  | -36.6% |

Nothing in the rewrite is architecture-specific: it is fewer loads, a smaller
and denser working set, 64-bit accumulators, and fewer allocations. The one
size that was tuned with a cache in mind is `lookupTableBits`, deliberately
capped at 12 (a 16 KiB table) rather than 13, so it still co-exists with the
working set in the 32-48 KiB L1d of a typical x86-64 core, where 32 KiB would
not. Confirming that on real x86-64 hardware is worth doing before relying on
these numbers there.

## What changed

- Decode table flattened from `[N]*node` (8 KiB of pointers into 12-byte
  structs) to `[N]uint32` packing symbol, code length, EOF flag and tree-walk
  node index; widened from 10 to 12 bits, which removes most bit-by-bit tree
  walks. Worth -58% alone on random payloads.
- Decoder refills 56 bits at a time, then decodes every symbol guaranteed to
  fit, instead of refilling per symbol.
- Encoder uses flat encode tables, flushes 4 bytes at a time, and sizes its
  output up front so the hot loop has no reallocation.
- `Writer`/`Reader` share the same tables and accumulator strategy.

## Cross-implementation: ddnet master (C++) vs this library (Go)

Against ddnet `master` HEAD, `src/engine/shared/huffman.cpp` compiled
**verbatim** (only `base/dbg.h` and `base/mem.h` stubbed), Apple clang 21,
`-O3 -DNDEBUG`, same machine, same corpora, binaries run alternately for six
rounds each.

Validity check first: **both implementations produce byte-identical compressed
output on all nine corpora**, and ddnet's bytes round-trip through our decoder.
So both decoders consume exactly the same input.

### Encode

The two APIs differ in shape: ddnet's `Compress` writes into a caller-supplied
buffer and never allocates, ours returns a fresh slice. Both are reported.

| Encoder | geomean | vs ddnet |
| ------------------------------------- | --------- | ---------- |
| ddnet `Compress` (caller buffer)       | 9.06 µs   | baseline   |
| ours `Compress` (allocates result)     | 10.24 µs  | +12.9%     |
| ours `Writer` (reuses buffer)          | 6.37 µs   | **-29.2%** |

Like for like — both writing into a reused buffer — the Go encoder is **28.8%
faster than the C++ one**. The gap in the middle row is the cost of allocating
a result slice per call, which is an API choice, not a codec difference.

Per case, `Writer` vs ddnet: snapshot/1400B -44.6%, skewed/64KB -47.8%,
zeroes/64KB -49.2%, text/64KB -16.6%, snapshot/64KB ~, random/64KB +32.0%.

### Decode

`DecompressTo(dst[:0], data)` reuses the caller's buffer and is the same API
shape as ddnet's `Decompress`, so this is the like-for-like row. The allocating
`Decompress` convenience wrapper is shown next to it for reference.

| | ddnet | ours (`DecompressTo`) | delta | ours (`Decompress`, allocates) |
| ----------------- | -------- | -------- | ---------- | -------- |
| snapshot/64B      | 162.4 ns | 121.2 ns | **-25.3%** | 171.0 ns |
| snapshot/1400B    | 3.94 µs  | 2.95 µs  | **-25.0%** | 3.54 µs  |
| snapshot/64KB     | 239.4 µs | 154.3 µs | **-35.6%** | 171.9 µs |
| random/1400B      | 3.82 µs  | 2.99 µs  | **-21.6%** | 3.96 µs  |
| random/64KB       | 545.0 µs | 234.1 µs | **-57.1%** | 259.3 µs |
| text/64KB         | 200.0 µs | 135.5 µs | **-32.2%** | 150.6 µs |
| zeroes/64KB       | 85.3 µs  | 79.9 µs  | **-6.3%**  | 101.0 µs |
| skewed/1400B      | 1.94 µs  | 2.69 µs  | +38.4%     | 2.94 µs  |
| skewed/64KB       | 85.1 µs  | 100.3 µs | +17.9%     | 119.2 µs |
| **geomean**       | 21.3 µs  | 16.9 µs  | **-20.6%** | 20.2 µs  |

The wins are where codes are long and ddnet falls back to bit-by-bit tree
walks; our 12-bit flat table resolves those in one load. `skewed` is the one
remaining loss: it is ~1 bit per symbol, so there is nothing to accelerate and
ddnet's tighter loop wins.

Correction to an earlier measurement: a probe suggested only ~5% of the
`zeroes` gap was allocation. That probe was faulty (it reported identical
B/op for both arms, i.e. it never took effect). Measured properly against
`DecompressTo`, `zeroes/64KB` flips from +15.5% to -6.3%, so allocation was
most of that gap, not a codec deficit.

### Summary

Comparing like for like — both sides writing into a reused caller buffer —
this Go implementation is **29.2% faster on encode** and **20.6% faster on
decode** than the C++ one, with the largest single win (-57%) on high-entropy
payloads. The convenience wrappers that allocate a result slice give up part
of that margin, which is an API cost rather than a codec one.
Note ddnet only optimized its encoder in #12519; its decoder is still the
classic 10-bit pointer-table version, which is where most of our decode
advantage comes from.
