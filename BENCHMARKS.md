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

| Benchmark | before | after | delta |
|---|---|---|---|
| **Compress** snapshot/64B    | 103 ns    | 74 ns     | **-27.8%** |
| **Compress** snapshot/1400B  | 1.87 µs   | 1.82 µs   | ~ |
| **Compress** snapshot/64KB   | 199.5 µs  | 87.4 µs   | **-56.2%** |
| **Compress** skewed/64KB     | 73.5 µs   | 59.4 µs   | **-19.1%** |
| **Compress** random/1400B    | 2.75 µs   | 1.93 µs   | **-29.9%** |
| **Compress** random/64KB     | 272.3 µs  | 100.0 µs  | **-63.3%** |
| **Compress** text/64KB       | 133.3 µs  | 80.1 µs   | **-40.0%** |
| **Decompress** snapshot/64B  | 209 ns    | 167 ns    | **-20.1%** |
| **Decompress** snapshot/1400B| 4.62 µs   | 3.52 µs   | **-23.8%** |
| **Decompress** snapshot/64KB | 295.6 µs  | 169.7 µs  | **-42.6%** |
| **Decompress** skewed/64KB   | 127.8 µs  | 120.2 µs  | ~ |
| **Decompress** random/64KB   | 647.6 µs  | 260.9 µs  | **-59.7%** |
| **Decompress** text/64KB     | 298.2 µs  | 156.0 µs  | **-47.7%** |
| **Writer** snapshot/1400B    | 1.96 µs   | 1.04 µs   | **-47.2%** |
| **Writer** snapshot/64KB     | 226.2 µs  | 77.3 µs   | **-65.8%** |
| **Writer** random/64KB       | 334.4 µs  | 107.9 µs  | **-67.7%** |
| **Writer** text/64KB         | 196.4 µs  | 78.4 µs   | **-60.1%** |
| **Reader** snapshot/1400B    | 5.37 µs   | 3.93 µs   | **-26.8%** |
| **Reader** skewed/64KB       | 138.1 µs  | 102.5 µs  | **-25.7%** |
| **Reader** random/64KB       | 843.3 µs  | 543.5 µs  | **-35.6%** |
| **Roundtrip** 1400B          | 6.86 µs   | 5.16 µs   | **-24.7%** |
| `NewDictionary`              | 196.8 µs  | 224.9 µs  | +14.3% |
| **geomean (time)**           |           |           | **-37.4%** |
| **geomean (throughput)**     | 354 MiB/s | 554 MiB/s | **+56.4%** |

Peak throughput after: **~1.45 GiB/s** encode (`Writer` skewed/64KB),
**~715 MiB/s** one-shot `Compress`, **~625 MiB/s** decode.

`NewDictionary` is the one regression: the decode table is now 4x larger to
build. It is a one-time cost paid once per process for the package-level
`DefaultDictionary`, traded for the decode wins above.

### Allocations

The encoder now sizes its output buffer up front instead of growing it:

| Benchmark | before | after |
|---|---|---|
| Compress snapshot/64KB | 15 allocs | 2 allocs |
| Compress random/64KB   | 20 allocs | 1 alloc  |
| Compress snapshot/1400B|  4 allocs | 1 alloc  |
| Decompress snapshot/1400B | 6 allocs | 1 alloc |

A teeworlds-sized packet is one allocation in each direction.

## x86-64

The development machine is arm64, so **native x86-64 numbers were not
measured**. Running the amd64 build under Rosetta 2 gives the relative deltas
below. Emulated timings are *not* valid absolute x86-64 performance figures;
they are reported only as evidence that the wins are not arm64-specific.

| Benchmark | before | after | delta |
|---|---|---|---|
| Compress snapshot/64KB   | 255.2 µs | 128.2 µs | -49.8% |
| Compress random/64KB     | 361.1 µs | 145.2 µs | -59.8% |
| Decompress snapshot/1400B| 7.67 µs  | 4.49 µs  | -41.4% |
| Decompress snapshot/64KB | 427.6 µs | 228.1 µs | -46.7% |
| Decompress random/64KB   | 750.5 µs | 335.7 µs | -55.3% |
| Roundtrip 1400B          | 10.17 µs | 6.44 µs  | -36.6% |

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
