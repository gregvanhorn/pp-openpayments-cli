.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/openpayments-pp-cli$(BIN_EXT) ./cmd/openpayments-pp-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/openpayments-pp-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/openpayments-pp-mcp$(BIN_EXT) ./cmd/openpayments-pp-mcp

install-mcp:
	go install ./cmd/openpayments-pp-mcp

build-all: build build-mcp
