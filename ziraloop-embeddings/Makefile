.PHONY: build build-linux test clean

VERSION ?= dev
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)"

build:
	CGO_ENABLED=1 go build $(LDFLAGS) -o bin/ziraloop-embeddings .

build-linux-amd64:
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 CC=x86_64-linux-musl-gcc go build $(LDFLAGS) -o bin/ziraloop-embeddings-linux-amd64 .

build-linux-arm64:
	CGO_ENABLED=1 GOOS=linux GOARCH=arm64 CC=aarch64-linux-musl-gcc go build $(LDFLAGS) -o bin/ziraloop-embeddings-linux-arm64 .

test:
	CGO_ENABLED=1 go test ./... -v -race -count=1

clean:
	rm -rf bin/
