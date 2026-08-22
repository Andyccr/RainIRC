GO ?= go

.PHONY: all build test vet fmt fmt-check ci

all: build

build:
	$(GO) build -o p2pirc ./cmd/p2pirc

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -w .

fmt-check:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "$$out"; exit 1; fi

ci: fmt-check vet test
