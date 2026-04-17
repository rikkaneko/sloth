BINARY := sloth

.PHONY: build test lint tidy

build:
	go build -o bin/$(BINARY) ./cmd/sloth

test:
	go test ./...

lint:
	go test ./...

tidy:
	go mod tidy
