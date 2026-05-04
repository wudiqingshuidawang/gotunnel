# gotunnel/Makefile

.PHONY: build build-server build-client test clean

build: build-server build-client

build-server:
	go build -o bin/gotunnel-server ./cmd/server

build-client:
	go build -o bin/gotunnel-client ./cmd/client

test:
	go test ./... -v -race

clean:
	rm -rf bin/
