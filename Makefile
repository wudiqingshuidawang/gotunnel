# gotunnel/Makefile

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: build build-server build-client test clean install

build: build-server build-client

build-server:
	go build $(LDFLAGS) -o bin/gotunnel-server ./cmd/server

build-client:
	go build $(LDFLAGS) -o bin/gotunnel-client ./cmd/client

test:
	go test ./... -v -race

clean:
	rm -rf bin/

install:
	go install $(LDFLAGS) ./cmd/server
	go install $(LDFLAGS) ./cmd/client
