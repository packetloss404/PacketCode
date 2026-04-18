.PHONY: build test lint run clean

VERSION ?= dev
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)

build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/packetcode ./cmd/packetcode

test:
	go test -race -count=1 ./...

lint:
	golangci-lint run ./...

run: build
	./bin/packetcode

clean:
	rm -rf bin/ dist/
