# Web3 Boilerplate Backend Makefile

# Lint baseline. Set LINT_BASE= for the full report.
LINT_BASE ?= origin/main

.PHONY: test test-docker test-coverage test-unit test-integration clean build run help quick-test

# Default target
help:
	@echo "Available commands:"
	@echo "  test           - Run the CI suite (-short, race; no Docker needed)"
	@echo "  test-docker    - Run every test, including Testcontainers suites (needs Docker)"
	@echo "  quick-test     - Run the CI suite without race detection"
	@echo "  test-coverage  - Run tests with coverage report"
	@echo "  test-unit      - -short suite, no Docker"
	@echo "  test-integration - Postgres-backed suites from scripts/ci/go-pg-packages.sh (needs Docker or TEST_DATABASE_URL)"
	@echo "  lint           - golangci-lint, new issues since $(LINT_BASE) (LINT_BASE= for the full report)"
	@echo "  test-middleware - Run middleware tests only"
	@echo "  test-auth      - Run authentication tests only"
	@echo "  build          - Build the application"
	@echo "  run            - Run the application"
	@echo "  clean          - Clean build artifacts and test coverage"
	@echo "  eval-offline   - Run hermetic LLM eval suites (fixture replay)"

# Test targets
# test matches the CI backend lane: -short skips the Testcontainers/Postgres
# suites. test-docker runs everything and needs a healthy Docker daemon.
test:
	@echo "🧪 Running the CI test suite (-short)..."
	go test -short -race ./...

test-docker:
	@echo "🐳 Running every test, including Docker-backed suites..."
	go test -race ./...

test-coverage:
	@echo "📊 Running tests with coverage..."
	./scripts/test-coverage.sh

test-unit:
	@echo "Running unit tests (-short, no Docker)..."
	go test -short -race ./...

# Needs Docker (Testcontainers) or TEST_DATABASE_URL pointing at a Postgres
# role allowed to CREATE DATABASE.
test-integration:
	@echo "Running Postgres-backed integration tests..."
	go test -race -count=1 -p 1 -tags integration,integration_postgres $$(bash ../scripts/ci/go-pg-packages.sh)

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
	golangci-lint run --new-from-rev=$(LINT_BASE) ./...

deps:
	@echo "📦 Installing dependencies..."
	go mod download
	go mod tidy

# Quick test for development
quick-test:
	@echo "⚡ Running quick tests (-short, no race detection)..."
	go test -short ./...

# Hermetic LLM eval suites (fixture replay, zero network). Safe in CI.
.PHONY: eval-offline
eval-offline:
	@echo "🧪 Running offline LLM eval suites (fixture replay)..."
	go test ./internal/llmeval/...

# Benchmark tests
benchmark:
	@echo "🏃 Running benchmark tests..."
	go test -bench=. -benchmem ./...

# Perf-fixture seeding. Idempotent. DATABASE_URL must be set (or pass --dsn).
# See perf/seed/README.md for usage.
.PHONY: perf-seed
perf-seed:
	@echo "🌱 Seeding perf fixtures..."
	go run ./perf/seed

# Perf benches. Local-run version; uses count=6 for stable stats via benchstat.
# Requires Docker for the testcontainers-backed benches.
# See perf/bench/README.md for details.
.PHONY: bench bench-ci
bench:
	@echo "🏃 Running perf benches (count=6)..."
	go test -run=^$$ -bench=. -benchmem -count=6 -timeout 30m \
		./internal/server/... \
		./internal/handlers/... \
		./internal/database/... \
		./internal/services/... \
		./internal/crm/... \
		./internal/splitting/... \
		./perf/bench/...

# CI variant: count=3 with a short benchtime for bounded runs. Outputs bench.txt
# for benchstat consumption.
# RBAC bootstrap and other log.Println calls land on stdout via the testing
# package's log capture, which means lines like
#   BenchmarkAuthMiddleware-8 \t 2026/05/23 13:37:21 RBAC data initialization...
# show up in raw output, and benchstat silently drops any bench whose name
# shares a line with a log timestamp. perf/bench/scripts/strip-log-noise.awk
# rebuilds clean bench output by stripping log lines and gluing each bench
# name to the next result line. The raw output is still preserved in
# bench.raw.txt for debugging.
bench-ci:
	@echo "🏃 Running perf benches for CI (count=3)..."
	@bash -o pipefail -c 'go test -run=^$$ -bench=. -benchmem -benchtime=100ms -count=3 -timeout 30m \
		./internal/server/... \
		./internal/handlers/... \
		./internal/database/... \
		./internal/services/... \
		./internal/crm/... \
		./internal/splitting/... \
		./perf/bench/... 2>&1 \
		| tee bench.raw.txt \
		| awk -f perf/bench/scripts/strip-log-noise.awk \
		> bench.txt'

# D5 capacity passthroughs. The perf Makefile builds a pinned k6+xk6-sse binary.
.PHONY: k6-build k6-smoke k6-burst k6-soak-1h k6-candidate-5x
k6-build:
	$(MAKE) -C perf/k6 build-k6

k6-smoke: ## Run k6 smoke profile (set BASE_URL=... to target staging)
	$(MAKE) -C perf/k6 smoke BASE_URL=$(if $(BASE_URL),$(BASE_URL),http://localhost:8080)

k6-burst: ## Run isolated two-minute burst (requires PERF_EXTERNAL_CONFIRM)
	$(MAKE) -C perf/k6 burst BASE_URL=$(BASE_URL) PERF_EXTERNAL_CONFIRM=$(PERF_EXTERNAL_CONFIRM)

k6-soak-1h: ## Run isolated one-hour projected-peak soak
	$(MAKE) -C perf/k6 soak_1h BASE_URL=$(BASE_URL) PERF_EXTERNAL_CONFIRM=$(PERF_EXTERNAL_CONFIRM)

k6-candidate-5x: ## Run exact-candidate one-hour five-times proof
	$(MAKE) -C perf/k6 candidate_5x BASE_URL=$(BASE_URL) \
	  PERF_EXTERNAL_CONFIRM=$(PERF_EXTERNAL_CONFIRM) CANDIDATE_SHA=$(CANDIDATE_SHA)

# staging-perf bring-up targets. PERF_HOST=user@host must be set.
# See perf/staging/README.md for the full bring-up flow.
PERF_HOST ?=

.PHONY: perf-staging-up perf-staging-down perf-staging-seed perf-staging-logs
perf-staging-up: ## Bring up staging-perf stack on $(PERF_HOST)
	@test -n "$(PERF_HOST)" || (echo "PERF_HOST not set — export PERF_HOST=user@host"; exit 1)
	@test -f perf/staging/.env || (echo "perf/staging/.env missing — copy from .env.example and fill in secrets"; exit 1)
	ssh $(PERF_HOST) "mkdir -p ~/payverge-perf"
	scp perf/staging/docker-compose.staging-perf.yml $(PERF_HOST):~/payverge-perf/docker-compose.yml
	scp perf/staging/.env                            $(PERF_HOST):~/payverge-perf/.env
	ssh $(PERF_HOST) "cd ~/payverge-perf && docker compose --env-file .env pull && docker compose --env-file .env up -d"

perf-staging-down: ## Tear down staging-perf stack (preserves volumes; add -v manually to wipe)
	@test -n "$(PERF_HOST)" || (echo "PERF_HOST not set"; exit 1)
	ssh $(PERF_HOST) "cd ~/payverge-perf && docker compose down"

perf-staging-seed: ## Seed perf fixtures (requires DATABASE_URL pointed at staging-perf)
	@test -n "$(DATABASE_URL)" || (echo "DATABASE_URL required — point at staging-perf Postgres (likely via ssh -L tunnel)"; exit 1)
	go run ./perf/seed --businesses=20 --products-per-business=50 \
	  --staff-per-business=5 --orders-history=1000

perf-staging-logs: ## Tail staging-perf logs
	@test -n "$(PERF_HOST)" || (echo "PERF_HOST not set"; exit 1)
	ssh $(PERF_HOST) "cd ~/payverge-perf && docker compose logs -f"
