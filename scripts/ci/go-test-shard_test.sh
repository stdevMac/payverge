#!/usr/bin/env bash
# Contract test for scripts/ci/go-test-shard.sh.
#   bash scripts/ci/go-test-shard_test.sh
# The real-tree partition check needs Go; set GO_TEST_SHARD_SKIP_GO=1 to run
# only the fixture checks.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SHARD="${ROOT}/scripts/ci/go-test-shard.sh"
failures=0
pass() { echo "PASS: $1"; }
fail() {
  echo "FAIL: $1" >&2
  failures=$((failures + 1))
}

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# Largest-first onto the lightest shard, ties to the lowest shard number.
cat >"${tmp}/weights" <<'EOF'
10 p/a
70 p/big
30 p/c
30 p/b
5 p/e
1 p/z
EOF
expect() {
  local n="$1" want="$2" got
  got="$(GO_TEST_SHARD_WEIGHTS="${tmp}/weights" bash "$SHARD" "$n" 3 | tr '\n' ' ' | sed 's/ $//')"
  if [[ "$got" == "$want" ]]; then
    pass "fixture shard ${n}/3 = ${want}"
  else
    fail "fixture shard ${n}/3 = '${got}', want '${want}'"
  fi
}
# Sorted: big70, b30, c30, a10, e5, z1 -> s1 big; s2 b; s3 c; a->s2 (30<=30, lower
# index); e->s3 (30 < 40); z->s3 (35 < 40).
expect 1 "p/big"
expect 2 "p/b p/a"
expect 3 "p/c p/e p/z"

# Positional arguments after <total> are package patterns, which must be
# relative to backend/; GO_TEST_SHARD_TAGS must look like a tag list.
for args in "" "1" "0 3" "4 3" "x 3" "1 2 3" "1 3 internal/llm" "1 3 ../frontend" "1 3 -race"; do
  # shellcheck disable=SC2086
  if bash "$SHARD" $args >/dev/null 2>&1; then
    fail "go-test-shard.sh accepted invalid arguments '${args}'"
  else
    pass "go-test-shard.sh rejects '${args}'"
  fi
done

if GO_TEST_SHARD_TAGS='integration;rm' GO_TEST_SHARD_WEIGHTS="${tmp}/weights" bash "$SHARD" 1 3 >/dev/null 2>&1; then
  fail "go-test-shard.sh accepted GO_TEST_SHARD_TAGS='integration;rm'"
else
  pass "go-test-shard.sh rejects a malformed GO_TEST_SHARD_TAGS"
fi
if GO_TEST_SHARD_TAGS=integration,integration_postgres GO_TEST_SHARD_WEIGHTS="${tmp}/weights" \
  bash "$SHARD" 1 3 ./internal/... >/dev/null 2>&1; then
  pass "go-test-shard.sh accepts ./ patterns with a tag list"
else
  fail "go-test-shard.sh rejected ./internal/... with GO_TEST_SHARD_TAGS"
fi

if [[ "${GO_TEST_SHARD_SKIP_GO:-}" == "1" ]] || ! command -v go >/dev/null 2>&1; then
  echo "SKIP: real backend partition check (no Go toolchain or GO_TEST_SHARD_SKIP_GO=1)"
else
  (cd "${ROOT}/backend" && go list ./... | LC_ALL=C sort) >"${tmp}/all"
  : >"${tmp}/union"
  for n in 1 2 3; do
    bash "$SHARD" "$n" 3 >"${tmp}/shard${n}"
    if [[ ! -s "${tmp}/shard${n}" ]]; then
      fail "real shard ${n}/3 is empty"
    fi
    cat "${tmp}/shard${n}" >>"${tmp}/union"
  done
  if LC_ALL=C sort "${tmp}/union" | cmp -s - "${tmp}/all"; then
    pass "3 shards partition every backend package exactly once ($(wc -l <"${tmp}/all" | tr -d ' ') packages)"
  else
    fail "shards do not partition go list ./... (missing or duplicated packages)"
  fi
  bash "$SHARD" 2 3 >"${tmp}/again"
  if cmp -s "${tmp}/again" "${tmp}/shard2"; then
    pass "real shard assignment is deterministic"
  else
    fail "real shard assignment changed between two runs"
  fi

  # The Postgres job shards the tagged packages of go-pg-packages.sh.
  # shellcheck disable=SC2207
  pg=($(bash "${ROOT}/scripts/ci/go-pg-packages.sh"))
  tags=integration,integration_postgres
  (cd "${ROOT}/backend" && go list -tags "$tags" "${pg[@]}" | LC_ALL=C sort) >"${tmp}/pg-all"
  : >"${tmp}/pg-union"
  for n in 1 2; do
    GO_TEST_SHARD_TAGS="$tags" bash "$SHARD" "$n" 2 "${pg[@]}" >>"${tmp}/pg-union"
  done
  if [[ -s "${tmp}/pg-all" ]] && LC_ALL=C sort "${tmp}/pg-union" | cmp -s - "${tmp}/pg-all"; then
    pass "2 tagged shards partition the Postgres packages ($(wc -l <"${tmp}/pg-all" | tr -d ' ') packages)"
  else
    fail "tagged shards do not partition the go-pg-packages.sh list"
  fi
fi

if [[ "$failures" -gt 0 ]]; then
  echo "go-test-shard_test: ${failures} failure(s)" >&2
  exit 1
fi
echo "go-test-shard_test: passed"
