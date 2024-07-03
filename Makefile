
test:
	go test -v -race -count=1 ./...

fuzz_write:
	go test -v -race -count=1 -fuzz=FuzzWriterWrite -fuzztime 120s .


fuzz_compress_decompress:
	go test -fuzz=FuzzHuffmannCompressDecompress -fuzztime 120s .