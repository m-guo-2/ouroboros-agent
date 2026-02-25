.PHONY: all build run test clean

# Default target
all: build

# Run the agent locally (for development)
run:
	@echo "=> Starting agent-go..."
	go run ./cmd/agent/main.go

# Build the agent binary
build:
	@echo "=> Building agent-go..."
	go build -o bin/agent-server ./cmd/agent/...

# Run all tests
test:
	@echo "=> Running tests..."
	go test ./... -v

# Clean up build artifacts
clean:
	@echo "=> Cleaning up..."
	rm -rf bin/
