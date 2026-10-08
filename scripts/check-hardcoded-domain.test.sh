#!/usr/bin/env bash
# Contract tests for scripts/check-hardcoded-domain.sh.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
GATE="$ROOT/scripts/check-hardcoded-domain.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

pass=0
fail=0

new_repo() {
  rm -rf "$TMP/repo"
  mkdir -p "$TMP/repo"
  git -C "$TMP/repo" init -q
}

put() { # put <path> <content>
  mkdir -p "$(dirname "$TMP/repo/$1")"
  printf '%s\n' "$2" >"$TMP/repo/$1"
}

expect() { # expect <name> <exit> <args...>
  local name="$1" want="$2"
  shift 2
  set +e
  CHECK_DOMAIN_ROOT="$TMP/repo" "${GATE_SHELL:-bash}" "$GATE" "$@" >"$TMP/out" 2>&1
  local rc=$?
  set -e
  if [ "$rc" -eq "$want" ]; then
    pass=$((pass + 1))
  else
    fail=$((fail + 1))
    echo "FAIL $name: expected exit $want got $rc"
    cat "$TMP/out"
  fi
}

# Clean tree passes.
new_repo
put backend/internal/x/x.go 'package x // uses config.PublicURL()'
put frontend/src/a.ts 'export const a = 1;'
expect "clean" 0
expect "clean-backend-only" 0 --backend-only

# Backend non-test Go literal fails (untracked files are scanned too).
put backend/internal/x/bad.go 'const u = "https://payverge.io/x"'
expect "backend-go" 1 --backend-only
grep -q 'backend/internal/x/bad.go:1:' "$TMP/out" || { fail=$((fail + 1)); echo "FAIL backend-go: hit not listed"; }
rm "$TMP/repo/backend/internal/x/bad.go"

# Case-insensitive.
put backend/cmd/app/bad.go 'const u = "PAYVERGE.IO"'
expect "case-insensitive" 1 --backend-only
rm "$TMP/repo/backend/cmd/app/bad.go"

# The legacy payverge.com apex is caught too.
put backend/internal/telegram/bad.go 'return "https://app.payverge.com"'
expect "legacy-com-apex" 1 --backend-only
grep -q 'backend/internal/telegram/bad.go:1:' "$TMP/out" || { fail=$((fail + 1)); echo "FAIL legacy-com-apex: hit not listed"; }
rm "$TMP/repo/backend/internal/telegram/bad.go"

# Email templates are scanned.
put backend/email/layout/base_eng.html '<a href="https://payverge.io">x</a>'
expect "email-template" 1 --backend-only
rm "$TMP/repo/backend/email/layout/base_eng.html"

# Tests, testdata, fixtures, and the backend brand-default module are allowed.
put backend/internal/x/x_test.go 'const u = "https://payverge.io"'
put backend/internal/llm/testdata/menu.json '{"u":"https://payverge.io"}'
put backend/internal/config/instance.go 'const UpstreamDomain = "payverge.io"'
expect "backend-allowlist" 0 --backend-only

# Out-of-scope backend paths (SQL demo scripts, perf) are not scanned.
put backend/scripts/demo.sql "-- https://payverge.io"
expect "backend-out-of-scope" 0 --backend-only

# Frontend hits fail the full run but not --backend-only.
put frontend/src/lib/site.ts 'export const SITE = "https://payverge.io";'
expect "frontend-full" 1
expect "frontend-backend-only" 0 --backend-only
expect "frontend-only" 1 --frontend-only
expect "frontend-count" 1 --count
grep -q 'frontend: 1 hard-coded' "$TMP/out" || { fail=$((fail + 1)); echo "FAIL frontend-count: summary missing"; }
grep -q 'site.ts:1:' "$TMP/out" && { fail=$((fail + 1)); echo "FAIL frontend-count: --count must not list hits"; }
rm "$TMP/repo/frontend/src/lib/site.ts"

# Frontend tests, mocks, and Markdown content are allowed.
put frontend/src/lib/site.test.ts 'it("x", () => "https://payverge.io")'
put frontend/src/lib/__mocks__/m.ts 'export default "https://payverge.io"'
put frontend/src/content/blog/post.md 'See https://payverge.io'
expect "frontend-allowlist" 0

# The frontend brand module is excluded. KNOWN_DEBT is empty, so the former
# debt files (legal/tools copy) are scanned like any other file.
put frontend/src/config/brand.ts '// never payverge.io'
expect "frontend-brand-allowed" 0 --frontend-only
grep -q 'known-debt' "$TMP/out" && { fail=$((fail + 1)); echo "FAIL frontend-debt: no debt is left, so no warning"; }
expect "release-brand-allowed" 0 --frontend-only --release
put frontend/src/i18n/messages/en/legal.json '{"m":"legal@payverge.io"}'
expect "frontend-former-debt-fails" 1 --frontend-only
expect "release-former-debt-fails" 1 --frontend-only --release
grep -q 'legal.json:1:' "$TMP/out" || { fail=$((fail + 1)); echo "FAIL release-debt: hit not listed"; }
rm -rf "$TMP/repo/frontend/src/i18n" "$TMP/repo/frontend/src/config"

# Gitignored files are skipped.
put .gitignore 'frontend/src/generated/'
put frontend/src/generated/out.ts 'export const u = "https://payverge.io";'
expect "gitignored" 0

# Usage errors.
expect "unknown-arg" 2 --nope
rm -rf "$TMP/notgit" && mkdir -p "$TMP/notgit"
set +e
CHECK_DOMAIN_ROOT="$TMP/notgit" "${GATE_SHELL:-bash}" "$GATE" >/dev/null 2>&1
rc=$?
set -e
if [ "$rc" -eq 2 ]; then pass=$((pass + 1)); else fail=$((fail + 1)); echo "FAIL not-git: expected exit 2 got $rc"; fi

echo "check-hardcoded-domain: $pass passed, $fail failed"
[ "$fail" -eq 0 ]
