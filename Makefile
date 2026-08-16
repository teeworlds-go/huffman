
test:
	go test -v -race -count=1 ./...

# Regenerate the golden wire-format vectors. Only ever correct when a change
# to the compressed output is intended.
golden:
	go test -run TestGolden -update .

bench:
	go test -run '^$$' -bench . -benchmem -count=8 .

# Compare against another revision, e.g. `make benchcmp REV=master`.
# Both revisions are benchmarked from the same working tree so the benchmark
# definitions match, and benchstat reports the delta.
REV ?= master
benchcmp:
	@command -v benchstat >/dev/null || go install golang.org/x/perf/cmd/benchstat@latest
	@tmp=$$(mktemp -d) && \
	git worktree add -q $$tmp/base $(REV) && \
	cp *_test.go testdata/golden.txt $$tmp/base/ 2>/dev/null || true; \
	(cd $$tmp/base && go test -run '^$$' -bench . -benchmem -count=8 . > $$tmp/old.txt 2>&1) ; \
	go test -run '^$$' -bench . -benchmem -count=8 . > $$tmp/new.txt 2>&1 ; \
	benchstat $$tmp/old.txt $$tmp/new.txt ; \
	git worktree remove --force $$tmp/base

fuzz_write:
	go test -v -race -count=1 -fuzz=FuzzWriterWrite -fuzztime 120s .

fuzz_compress_decompress:
	go test -fuzz=FuzzHuffmannCompressDecompress -fuzztime 120s .

fuzz_write_read:
	go test -fuzz=FuzzWriteRead -fuzztime 120s .