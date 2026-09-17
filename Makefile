SHELL := /bin/bash
BINARY_NAME := tinyvm
VERSION ?= $(shell cat VERSION 2>/dev/null || echo 0.1.0-dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -trimpath -ldflags="-s -w \
	-X 'tinyvm/internal/version.Version=$(VERSION)' \
	-X 'tinyvm/internal/version.Commit=$(COMMIT)' \
	-X 'tinyvm/internal/version.BuildDate=$(BUILD_DATE)'"

.PHONY: all build test run clean docker fmt vet

all: build

build:
	go build $(LDFLAGS) -o $(BINARY_NAME) ./cmd/tinyvm

test:
	go test -v ./...

run: build
	./$(BINARY_NAME) serve

clean:
	rm -f $(BINARY_NAME)
	rm -rf data test-data

docker:
	docker build -t $(BINARY_NAME):latest .

fmt:
	go fmt ./...

vet:
	go vet ./...
