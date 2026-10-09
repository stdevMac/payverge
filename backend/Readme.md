# Payverge backend

This directory is the Go API of Payverge: Gin for HTTP, GORM on
PostgreSQL 18. The entry point is [`cmd/app/main.go`](cmd/app/main.go).
Handlers live in `internal/server/` and `internal/handlers/`, services in
`internal/services/`, and data access in `internal/database/`.

The product overview, install and full-stack development steps are in the
root [`../README.md`](../README.md). Configuration is documented in
[`../docs/self-hosting/configuration.md`](../docs/self-hosting/configuration.md),
and the schema and migration rules in
[`migrations/README.md`](migrations/README.md).

## Common commands

The Go version is in [`go.mod`](go.mod). From this directory:

```sh
make run          # dev server on :8080
make build        # binary to bin/app
make quick-test   # all tests, no race detector
make test         # all tests with -race
make lint         # golangci-lint
make fmt          # gofmt
```

Contribution rules, including the performance gate for hot paths, are in
[`../AGENTS.md`](../AGENTS.md).
