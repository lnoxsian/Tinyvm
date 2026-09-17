SHELL := /bin/bash
BINARY_NAME := tinyvm
VERSION ?= $(shell cat VERSION 2>/dev/null || echo 0.1.0-dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -trimpath -ldflags="-s -w \
	-X 'tinyvm/internal/version.Version=$(VERSION)' \
	-X 'tinyvm/internal/version.Commit=$(COMMIT)' \
	-X 'tinyvm/internal/version.BuildDate=$(BUILD_DATE)'"

.PHONY: all build test run clean docker fmt vet setup-env version version-patch version-minor version-major help

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

setup-env:
	@./scripts/dev/setup_env.sh

version:
	@./scripts/dev/update_version.sh

version-patch:
	@./scripts/dev/update_version.sh patch -y -b

version-minor:
	@./scripts/dev/update_version.sh minor -y -b

version-major:
	@./scripts/dev/update_version.sh major -y -b

help:
	@echo "TinyVM Build & Development Targets:"
	@echo "  build         - Compile the tinyvm binary"
	@echo "  test          - Run all Go unit and integration tests"
	@echo "  run           - Build and start tinyvm in server mode"
	@echo "  clean         - Remove compiled binary and temporary test data"
	@echo "  fmt           - Format all Go source files"
	@echo "  vet           - Run go vet static analysis"
	@echo "  docker        - Build Docker container image"
	@echo "  setup-env     - Install required QEMU/KVM virtualization binaries"
	@echo "  version       - Interactively update version, VERSION file, and rebuild"
	@echo "  version-patch - Bump patch version (e.g. 0.1.0 -> 0.1.1) and rebuild"
	@echo "  version-minor - Bump minor version (e.g. 0.1.0 -> 0.2.0) and rebuild"
	@echo "  version-major - Bump major version (e.g. 0.1.0 -> 1.0.0) and rebuild"
