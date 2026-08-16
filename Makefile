
test:
	go test -v -race -count=1 ./...

# Regenerate the golden wire-format vectors. Only ever correct when a change
# to the compressed output is intended.
golden:
	go test -run TestGolden -update .

bench:
	go test -run '^$$' -bench . -benchmem -count=8 .

# Compare against another revision, e.g. `make benchcmp REV=master`.
# Only the package sources differ: both binaries use the current benchmark and
# corpus definitions, and are run in alternating order to avoid thermal drift.
REV ?= master
BENCH_COUNT ?= 6
BENCH_TIME ?= 250ms
BENCH_RE ?= ^Benchmark(Compress|Decompress|Writer|Reader|DecompressToReuse|Roundtrip|NewDictionary)$$
benchcmp:
	@command -v benchstat >/dev/null || go install golang.org/x/perf/cmd/benchstat@latest
	@set -eu; \
	tmp=$$(mktemp -d); \
	trap 'rm -rf "$$tmp"' EXIT HUP INT TERM; \
	export GOCACHE="$$tmp/gocache"; \
	mkdir "$$tmp/base" "$$tmp/head"; \
	files=$$(go list -f '{{join .GoFiles " "}}' .); \
	for file in $$files; do \
		git show "$(REV):$$file" > "$$tmp/base/$$file"; \
		cp "$$file" "$$tmp/head/$$file"; \
	done; \
	git show "$(REV):go.mod" > "$$tmp/base/go.mod"; \
	cp go.mod "$$tmp/head/go.mod"; \
	cp bench_test.go corpus_test.go "$$tmp/base/"; \
	cp bench_test.go corpus_test.go "$$tmp/head/"; \
	(cd "$$tmp/base" && go test -c -o "$$tmp/base.test" .); \
	(cd "$$tmp/head" && go test -c -o "$$tmp/head.test" .); \
	: > "$$tmp/old.txt"; : > "$$tmp/new.txt"; \
	i=1; while [ $$i -le $(BENCH_COUNT) ]; do \
		if [ $$((i % 2)) -eq 1 ]; then \
			"$$tmp/base.test" -test.run '^$$' -test.bench '$(BENCH_RE)' -test.benchmem -test.benchtime=$(BENCH_TIME) >> "$$tmp/old.txt"; \
			"$$tmp/head.test" -test.run '^$$' -test.bench '$(BENCH_RE)' -test.benchmem -test.benchtime=$(BENCH_TIME) >> "$$tmp/new.txt"; \
		else \
			"$$tmp/head.test" -test.run '^$$' -test.bench '$(BENCH_RE)' -test.benchmem -test.benchtime=$(BENCH_TIME) >> "$$tmp/new.txt"; \
			"$$tmp/base.test" -test.run '^$$' -test.bench '$(BENCH_RE)' -test.benchmem -test.benchtime=$(BENCH_TIME) >> "$$tmp/old.txt"; \
		fi; \
		i=$$((i + 1)); \
	done; \
	benchstat "$$tmp/old.txt" "$$tmp/new.txt"

fuzz_write:
	go test -v -race -count=1 -fuzz=FuzzWriterWrite -fuzztime 120s .

fuzz_compress_decompress:
	go test -fuzz=FuzzHuffmannCompressDecompress -fuzztime 120s .

fuzz_write_read:
	go test -fuzz=FuzzWriteRead -fuzztime 120s .
