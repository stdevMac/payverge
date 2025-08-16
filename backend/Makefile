# Web3 Boilerplate Backend Makefile

.PHONY: test test-coverage test-unit test-integration clean build run help

# Default target
help:
	@echo "Available commands:"
	@echo "  test           - Run all tests"
	@echo "  test-coverage  - Run tests with coverage report"
	@echo "  test-unit      - Run unit tests only"
	@echo "  test-integration - Run integration tests only"
	@echo "  test-middleware - Run middleware tests only"
	@echo "  test-auth      - Run authentication tests only"
	@echo "  build          - Build the application"
	@echo "  run            - Run the application"
	@echo "  clean          - Clean build artifacts and test coverage"

# Test targets
test:
	@echo "🧪 Running all tests..."
	go test -v -race ./...

test-coverage:
	@echo "📊 Running tests with coverage..."
	./scripts/test-coverage.sh

test-unit:
	@echo "🔬 Running unit tests..."
	go test -v -race ./internal/middleware/... ./internal/server/... ./internal/mocks/...

test-integration:
	@echo "🔗 Running integration tests..."
	go test -v -race ./internal/tests/...

test-middleware:
	@echo "🛡️ Running middleware tests..."
	go test -v -race ./internal/middleware/...

test-auth:
	@echo "🔐 Running authentication tests..."
	go test -v -race ./internal/server/... -run ".*Auth.*"

# Build targets
build:
	@echo "🔨 Building application..."
	go build -o bin/app ./cmd/app

run:
	@echo "🚀 Running application..."
	go run ./cmd/app

# Utility targets
clean:
	@echo "🧹 Cleaning up..."
	rm -rf bin/
	rm -rf coverage/
	go clean -testcache

# Development helpers
fmt:
	@echo "📝 Formatting code..."
	go fmt ./...

lint:
	@echo "🔍 Running linter..."
	golangci-lint run

deps:
	@echo "📦 Installing dependencies..."
	go mod download
	go mod tidy

# Quick test for development
quick-test:
	@echo "⚡ Running quick tests (no race detection)..."
	go test ./...

# Benchmark tests
benchmark:
	@echo "🏃 Running benchmark tests..."
	go test -bench=. -benchmem ./...
