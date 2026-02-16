# Build standalone binary (no Node.js required)
# Usage: make build   or   make build-all
# Produces gtc (and gtc-<platform> for build-all)
BINARY_NAME = gtc

build:
	go build -o $(BINARY_NAME) ./cmd/global-tunnel/

# Cross-compile for common platforms
build-all: build
	GOOS=linux GOARCH=amd64 go build -o $(BINARY_NAME)-linux-amd64 ./cmd/global-tunnel/
	GOOS=darwin GOARCH=amd64 go build -o $(BINARY_NAME)-darwin-amd64 ./cmd/global-tunnel/
	GOOS=darwin GOARCH=arm64 go build -o $(BINARY_NAME)-darwin-arm64 ./cmd/global-tunnel/
	GOOS=windows GOARCH=amd64 go build -o $(BINARY_NAME)-windows-amd64.exe ./cmd/global-tunnel/

.PHONY: build build-all
