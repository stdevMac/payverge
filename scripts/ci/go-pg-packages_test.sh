#!/usr/bin/env bash
# Contract test for scripts/ci/go-pg-packages.sh.
#   bash scripts/ci/go-pg-packages_test.sh
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="${ROOT}/scripts/ci/go-pg-packages.sh"
failures=0
pass() { echo "PASS: $1"; }
fail() {
  echo "FAIL: $1" >&2
  failures=$((failures + 1))
}

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

mk() {
  mkdir -p "${tmp}/mod/$(dirname "$1")"
  printf '%s\n' "$2" >"${tmp}/mod/$1"
}
mk internal/envurl/a_test.go 'package envurl // os.Getenv("TEST_DATABASE_URL")'
mk internal/perfurl/a_test.go 'package perfurl // TESTPERF_DATABASE_URL'
mk internal/testperf/a_test.go 'package x // testperf.StartPostgres(t)'
mk internal/isolated/a_test.go 'package x // testperf.StartIsolatedPostgres(t)'
mk internal/tc/a_test.go 'package x // tcpostgres.Run(ctx)'
mk internal/tagged/a_test.go $'//go:build integration_postgres\n\npackage tagged'
mk internal/tagged2/a_test.go $'//go:build linux && integration_postgres\n\npackage tagged2'
# The bare `integration` tag marks live payment-sandbox tests, not Postgres.
mk internal/sandbox/a_test.go $'//go:build integration\n\npackage sandbox // PAYPAL_SANDBOX_CLIENT_ID'
mk rootpkg_test.go 'package mod // TEST_DATABASE_URL'
mk internal/plain/a_test.go 'package plain // no database here'
mk internal/nontest/a.go 'package nontest // TEST_DATABASE_URL in a non-test file'
mk internal/commented/a_test.go $'package commented\n\n// see //go:build integration elsewhere'

want=".
./internal/envurl
./internal/isolated
./internal/perfurl
./internal/tagged
./internal/tagged2
./internal/tc
./internal/testperf"
got="$(GO_PG_PACKAGES_ROOT="${tmp}/mod" bash "$SCRIPT")"
if [[ "$got" == "$want" ]]; then
  pass "fixture detection selects exactly the Postgres packages"
else
  fail "fixture detection: got
${got}
want
${want}"
fi

empty="${tmp}/empty"
mkdir -p "$empty"
if got="$(GO_PG_PACKAGES_ROOT="$empty" bash "$SCRIPT")" && [[ -z "$got" ]]; then
  pass "a tree without Postgres tests yields an empty list and exit 0"
else
  fail "empty tree: exit non-zero or output '${got}'"
fi

real="$(bash "$SCRIPT")"
for pkg in ./internal/database ./internal/server ./internal/llm ./internal/testperf; do
  if grep -qx "$pkg" <<<"$real"; then
    pass "real tree lists ${pkg}"
  else
    fail "real tree is missing ${pkg}"
  fi
done
for pkg in ./internal/plugins/paypal ./internal/plugins/mercadopago; do
  if grep -qx "$pkg" <<<"$real"; then
    fail "real tree lists ${pkg}, whose integration tests are live payment sandboxes"
  else
    pass "real tree leaves out the ${pkg} sandbox tests"
  fi
done
while IFS= read -r pkg; do
  if [[ ! -d "${ROOT}/backend/${pkg#./}" ]]; then
    fail "listed package ${pkg} is not a backend directory"
  fi
done <<<"$real"

if [[ "$failures" -gt 0 ]]; then
  echo "go-pg-packages_test: ${failures} failure(s)" >&2
  exit 1
fi
echo "go-pg-packages_test: passed"
