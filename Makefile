.PHONY: fmt tidy build test lint check screens

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

# Stand the whole thing up and leave it running — for looking at rather than
# reading about. Same rig the gate uses: a founded realm, the shell, a
# conversation with more than one voice in it, and a signed-in session. It
# prints
#
#   SHELL_URL=http://127.0.0.1:<port>/?topic=<conversation>
#   SESSION_COOKIE=helm_session=<value>
#
# then blocks until ^C. Open that address in a browser with that cookie set
# for 127.0.0.1; everything lives in a temp dir that goes away on exit.
screens:
	cd e2e && go run ./cmd/screens
