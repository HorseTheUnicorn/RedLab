GO ?= go
BINARY ?= redlab

.PHONY: build test vet race lint

build:
	$(GO) build -trimpath -o bin/$(BINARY) ./cmd/redlab

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

race:
	$(GO) test -race ./...

lint:
	$(GO) vet ./...
