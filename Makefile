
test:
	go test -v -race -count=1 ./...

fuzz_write:
	go test -v -race -count=1 -fuzz=FuzzWrite -fuzztime 120s ./...