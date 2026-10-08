# 0001. One Go backend, structured as a modular monolith

- Status: Accepted
- Date: 2026-10-03 (records a decision already in effect)

## Context

Payverge handles money, permissions and fiscal records for restaurants. The
same rules must hold for a guest paying from a phone, a waiter on a tablet,
a payment webhook and a background job. The team is small, and the project
must also be easy for one person to self-host.

## Decision

All server-side logic lives in **one Go binary** that uses
[Gin](https://github.com/gin-gonic/gin) for HTTP and
[GORM](https://gorm.io) over Postgres for data access. Inside it, code is
split into domain packages under `backend/internal/` (money, splitting,
fiscal, events, plugins, llm, agents, and others). There are no internal
network calls between them. Background jobs run as schedulers inside the same
process.

The Next.js frontend holds no business rules. It renders pages and calls the
API.

## Consequences

- **Easier:** one deployable, one database, one place to enforce a rule. A
  settlement and its fiscal job commit in the same transaction. Self-hosting
  is two containers and Postgres.
- **Easier:** Go builds a single static binary with fast tests and a strong
  standard library for HTTP, crypto and concurrency.
- **Harder:** `cmd/app/main.go` is the composition root for every service,
  route and scheduler, and it is several thousand lines long.
- **Harder:** scaling is mainly vertical. Realtime hubs, rate limiters and
  some quotas are in memory, and schedulers have no leader election, so a
  single backend replica is the tested setup.
- **Accepted debt:** handlers are split between `internal/server` and
  `internal/handlers` for historical reasons, not by layer.

See [architecture/overview.md](../architecture/overview.md).
