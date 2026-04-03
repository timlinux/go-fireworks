.PHONY: help build run clean dev install test fmt vet nix-build nix-run nix-dev

# Default target
help:
	@echo "Available targets:"
	@echo "  make build      - Build the binary"
	@echo "  make run        - Build and run the fireworks show"
	@echo "  make clean      - Remove built binaries and build cache"
	@echo "  make dev        - Enter nix development shell"
	@echo "  make install    - Install dependencies"
	@echo "  make test       - Run tests"
	@echo "  make fmt        - Format code"
	@echo "  make vet        - Run go vet"
	@echo "  make nix-build  - Build with Nix"
	@echo "  make nix-run    - Run with Nix"
	@echo "  make nix-dev    - Enter Nix dev shell"

# Build the binary
build:
	go build -o go-fireworks .

# Run the fireworks show
run: build
	./go-fireworks

# Clean build artifacts
clean:
	rm -f go-fireworks
	go clean
	rm -rf result result-*

# Enter nix development shell
dev:
	nix develop

# Install Go dependencies
install:
	go mod download
	go mod tidy

# Run tests
test:
	go test -v ./...

# Format code
fmt:
	go fmt ./...

# Run go vet
vet:
	go vet ./...

# Build with Nix
nix-build:
	nix build

# Run with Nix
nix-run:
	nix run

# Enter Nix development shell
nix-dev:
	nix develop
