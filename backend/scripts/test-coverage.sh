#!/bin/bash

# Test Coverage Script for Web3 Boilerplate Backend
# This script runs tests with coverage reporting and generates HTML reports

set -e

echo "🧪 Running Go tests with coverage..."

# Create coverage directory if it doesn't exist
mkdir -p coverage

# Run tests with coverage for all packages
echo "📊 Generating coverage profile..."
go test -v -race -coverprofile=coverage/coverage.out ./...

# Generate HTML coverage report
echo "📄 Generating HTML coverage report..."
go tool cover -html=coverage/coverage.out -o coverage/coverage.html

# Generate text coverage summary
echo "📋 Generating coverage summary..."
go tool cover -func=coverage/coverage.out > coverage/coverage.txt

# Display coverage summary
echo ""
echo "📈 Coverage Summary:"
echo "==================="
go tool cover -func=coverage/coverage.out | tail -1

# Check if coverage meets minimum threshold (80%)
COVERAGE=$(go tool cover -func=coverage/coverage.out | tail -1 | awk '{print $3}' | sed 's/%//')
THRESHOLD=80

echo ""
if (( $(echo "$COVERAGE >= $THRESHOLD" | bc -l) )); then
    echo "✅ Coverage ($COVERAGE%) meets minimum threshold ($THRESHOLD%)"
    exit 0
else
    echo "❌ Coverage ($COVERAGE%) below minimum threshold ($THRESHOLD%)"
    echo "📁 View detailed report: coverage/coverage.html"
    exit 1
fi
