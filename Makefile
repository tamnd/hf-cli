# Build into bin/ (gitignored) so the binary never collides with the hf/
# source package at the repo root.
BINARY  := bin/hf
PKG     := ./cmd/hf
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
	-X github.com/tamnd/hf-cli/cli.Version=$(VERSION) \
	-X github.com/tamnd/hf-cli/cli.Commit=$(COMMIT) \
	-X github.com/tamnd/hf-cli/cli.Date=$(DATE)

.PHONY: build install test fixtures goldens vet fmt clean run

build:
	@mkdir -p $(dir $(BINARY))
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) $(PKG)

install:
	CGO_ENABLED=0 go install -trimpath -ldflags "$(LDFLAGS)" $(PKG)

test:
	go test ./...

# fixtures re-records every exchange the tests replay, against the live hub. It
# is deliberately not part of test: an ordinary run has to work offline and has
# to give the same answer today as it did last week. Run it when the hub changes
# something, then read the golden diff before committing it.
fixtures:
	HF_RECORD=1 go test ./hf -run TestRecord -v -timeout 900s
	$(MAKE) goldens

# goldens rewrites the recorded shapes from the fixtures on disk. It touches no
# network.
goldens:
	go test ./hf -run TestScenarios -update
	HF_UPDATE=1 go test ./cli

vet:
	go vet ./...

fmt:
	gofmt -w -s .

clean:
	rm -rf bin dist

run: build
	./$(BINARY) $(ARGS)
