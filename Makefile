GO ?= go
VERSION ?= 0.5.1
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)
LDFLAGS ?= -s -w -X github.com/Andyccr/RainIRC/internal/version.Version=$(VERSION) -X github.com/Andyccr/RainIRC/internal/version.Commit=$(COMMIT)
PREFIX ?= $(HOME)/.local
BINDIR ?= $(PREFIX)/bin

.PHONY: all build install uninstall dist test vet fmt fmt-check ci

all: build

build:
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o p2pirc ./cmd/p2pirc

install: build
	mkdir -p "$(BINDIR)"
	install -m 755 p2pirc "$(BINDIR)/p2pirc"

uninstall:
	rm -f "$(BINDIR)/p2pirc"

dist:
	rm -rf dist
	mkdir -p dist
	set -e; \
	for spec in linux-amd64 linux-arm64 darwin-amd64 darwin-arm64 windows-amd64 windows-arm64; do \
		os=$${spec%-*}; arch=$${spec#*-}; ext=; \
		if [ "$$os" = windows ]; then ext=.exe; fi; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o dist/p2pirc-$$os-$$arch$$ext ./cmd/p2pirc; \
	done
	(cd dist && sha256sum p2pirc-* > SHA256SUMS)

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -w .

fmt-check:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "$$out"; exit 1; fi

ci: fmt-check vet test
	sh -n scripts/install.sh
