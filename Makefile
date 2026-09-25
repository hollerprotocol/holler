VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//')
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64
LDFLAGS   := -s -w
PLUGIN    := plugin/holler

.PHONY: build test test-go test-python interop dist plugin plugin-local clean

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/holler ./cmd/holler

test: test-go test-python interop

test-go:
	go vet ./...
	go test -race ./...

test-python:
	cd python && python3 -m unittest test_holler_peer

interop: build
	cd python && HOLLER_BIN=$(CURDIR)/bin/holler python3 -m unittest -v interop_test

# Release artifacts in dist/: per-platform archives, the plugin bundle and
# SHA256SUMS. Tagging vX.Y.Z runs the same script in CI and publishes them.
dist:
	scripts/dist.sh $(VERSION)

# The agent plugin, with binaries for every supported platform.
plugin:
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; \
		echo "building $(PLUGIN)/libexec/holler-$$os-$$arch"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "$(LDFLAGS)" \
			-o $(PLUGIN)/libexec/holler-$$os-$$arch ./cmd/holler || exit 1; \
	done
	claude plugin validate $(PLUGIN) 2>/dev/null || true

# Just this machine's platform, for quick local testing.
plugin-local:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" \
		-o $(PLUGIN)/libexec/holler-$$(go env GOOS)-$$(go env GOARCH) ./cmd/holler

clean:
	rm -rf bin dist $(PLUGIN)/libexec/holler-*
