.PHONY: build test vet lint clean install coverage

BINARY := autobacklog
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X github.com/jvreagan/autobacklog/internal/cli.Version=$(VERSION)"

build:
	go build $(LDFLAGS) -o $(BINARY) ./cmd/autobacklog

install:
	go install $(LDFLAGS) ./cmd/autobacklog

test:
	go test ./...

vet:
	go vet ./...

lint: vet
	@echo "Lint passed"

clean:
	rm -f $(BINARY)

coverage:
	go test -race -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -func=coverage.out

all: clean build test vet
