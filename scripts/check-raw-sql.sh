#!/usr/bin/env bash
#
# Fails if any Go source file in backend/ contains raw SQL concatenation
# patterns. This is the defense-in-depth guard that replaced the URL-based
# SQL-injection pattern matching removed from internal/middleware/validation.go.
#
# Add allowed exceptions to the ALLOWLIST array below.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TARGET="${ROOT}/backend"

if [[ ! -d "${TARGET}" ]]; then
  echo "error: ${TARGET} does not exist" >&2
  exit 2
fi

# File globs that may legitimately use raw SQL (documented exceptions only).
# Each entry is a path suffix; partial matches allowed.
ALLOWLIST=(
  # Add here with a reason comment, e.g.:
  # "internal/foo/bar.go" # why the identifier cannot be a bound parameter
  "internal/database/production_startup_no_ddl_integration_test.go" # integration_postgres test; GRANTs privileges to a dynamically-named least-privilege test role — Postgres cannot bind a role identifier as a parameter.
)

# Patterns that indicate raw SQL concatenation.
# Ordered from highest-signal to lowest.
PATTERNS=(
  # db.Raw( with string concatenation
  'db\.Raw\([^)]*\+'
  # db.Exec( with string concatenation
  'db\.Exec\([^)]*\+'
  # fmt.Sprintf assembling SQL keywords
  'fmt\.Sprintf\(["`][^)]*(SELECT|INSERT|UPDATE|DELETE|DROP|ALTER|TRUNCATE)'
)

fail=0
for pattern in "${PATTERNS[@]}"; do
  # grep returns 1 when nothing matched; we want that to be the happy path.
  matches=$(grep -rEn --include='*.go' --exclude-dir=node_modules \
    "${pattern}" "${TARGET}" 2>/dev/null || true)

  if [[ -z "${matches}" ]]; then
    continue
  fi

  # Filter out allowlisted paths.
  filtered="${matches}"
  for allow in "${ALLOWLIST[@]:-}"; do
    [[ -z "${allow}" ]] && continue
    path_part="${allow%%:*}"
    filtered=$(echo "${filtered}" | grep -v "${path_part}" || true)
  done

  if [[ -n "${filtered}" ]]; then
    echo "error: raw SQL concatenation pattern detected (pattern: ${pattern})" >&2
    echo "${filtered}" >&2
    echo >&2
    fail=1
  fi
done

if [[ "${fail}" -eq 1 ]]; then
  echo "--------------------------------------------------------------" >&2
  echo "Raw SQL concatenation is forbidden. Use parameterized queries:" >&2
  echo '  BAD:  db.Raw("SELECT ... WHERE id = " + strconv.Itoa(id))'     >&2
  echo '  GOOD: db.Raw("SELECT ... WHERE id = ?", id)'                    >&2
  echo >&2
  echo "If the match is a false positive, add it to ALLOWLIST in"        >&2
  echo "${BASH_SOURCE[0]} with a one-line reason." >&2
  exit 1
fi

echo "check-raw-sql: no raw-SQL concatenation patterns found."
