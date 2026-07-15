.PHONY: dev build check check-backend test test-backend fmt fmt-check web-install web-build web-check clean

dev: web-install web-build
	go run ./cmd/server

build: web-install web-build
	mkdir -p bin
	go build -o bin/traceframe ./cmd/server

check: check-backend web-check

check-backend: fmt-check
	go vet ./cmd/... ./internal/...
	go test ./cmd/... ./internal/...

test: test-backend

test-backend:
	go test ./cmd/... ./internal/...

fmt:
	gofmt -w cmd internal

fmt-check:
	@test -z "$$(gofmt -l cmd internal)" || (gofmt -l cmd internal && exit 1)

web-install:
	npm --prefix web ci

web-build:
	npm --prefix web run build

web-check:
	npm --prefix web ci
	npm --prefix web run check

clean:
	rm -rf bin web/dist coverage.out
