.PHONY: fmt tidy build test lint check

# Stamp the binary with a real version for local builds; goreleaser sets the tag
# on release. Override with `make build VERSION=x.y.z`.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)

# Format all Go source (gofmt); golangci-lint's formatters also cover goimports.
fmt:
	gofmt -w .

tidy:
	go mod tidy

build:
	go build ./...
	go build -ldflags "$(LDFLAGS)" -o bin/ ./cmd/...

# All tests, no skips — including the consumer-position gate (e2e/, its
# own module under a path outside this namespace so internal/ imports
# cannot compile, booting a real soulnode realm at its published tag).
test:
	go test ./...
	cd e2e && go test ./...

lint:
	golangci-lint run

# The one gate to run before every commit: everything green.
check: fmt tidy build test lint
