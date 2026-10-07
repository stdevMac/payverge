#!/usr/bin/env bash
# Print the backend test packages that need PostgreSQL, one "./internal/..."
# pattern per line, sorted.
#
#   scripts/ci/go-pg-packages.sh
#
# A package qualifies when one of its _test.go files
#   - reads TEST_DATABASE_URL or TESTPERF_DATABASE_URL,
#   - starts a database through internal/testperf or testcontainers' postgres
#     module, or
#   - carries an `integration_postgres` build constraint.
#
# The plain `integration` tag is not enough on its own: it is not specific to
# Postgres. (The live payment-provider sandbox tests under
# internal/plugins/{paypal,mercadopago} carry their own paypal_sandbox and
# mercadopago_sandbox tags and need provider credentials, not Postgres.)
# Tagged Postgres tests under `integration` (internal/server) are found
# through TEST_DATABASE_URL instead.
#
# Those tests skip under `go test -short`, so CI runs them in a separate job
# with a postgres:15 service and TEST_DATABASE_URL. Matching is deliberately
# broad: an extra package only costs a few seconds in that job, while a missed
# one would silently drop coverage. scripts/ci/go-pg-packages_test.sh checks
# the detection.
#
# GO_PG_PACKAGES_ROOT=<dir> scans <dir> instead of backend/ (tests only).
set -euo pipefail

root="${GO_PG_PACKAGES_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)/backend}"
cd "$root"

pattern='TEST_DATABASE_URL|TESTPERF_DATABASE_URL|testperf\.Start(Isolated)?Postgres|tcpostgres\.|^//go:build.*integration_postgres'

# grep exits 1 when nothing matches; an empty list is a valid answer.
# "./a/b/x_test.go" -> "./a/b"; a test file at the module root -> ".".
{ grep -rlE --include='*_test.go' "$pattern" . || true; } |
  awk '{ sub(/\/[^\/]*$/, ""); print }' |
  LC_ALL=C sort -u
