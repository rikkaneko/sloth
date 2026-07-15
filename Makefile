BINARY := sloth

.PHONY: build test lint tidy

build:
	go build -trimpath -ldflags "-s -w -X main.Version=$${VERSION:-$$(git describe --tags --always)}" -o $${OUT:-bin/$(BINARY)} ./cmd/sloth

build-dev:
	go build -trimpath -ldflags "-X main.Version=$${VERSION:-$$(git describe --tags --always)}" -o $${OUT:-bin/$(BINARY)} ./cmd/sloth

test:
	go test ./...

lint:
	go test ./...

tidy:
	go mod tidy
