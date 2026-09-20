GO ?= go

.PHONY: check fmt-check test vet build run

check: fmt-check vet test

fmt-check:
	@test -z "$$(gofmt -l app cmd config internal pkg test)" || { gofmt -l app cmd config internal pkg test; exit 1; }

vet:
	$(GO) vet ./...

test:
	$(GO) test -race -count=1 -timeout=180s ./...

build:
	$(GO) build -trimpath -o bin/zte-c300-monitoring ./cmd/api

run:
	$(GO) run ./cmd/api
