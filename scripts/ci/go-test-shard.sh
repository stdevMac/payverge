#!/usr/bin/env bash
# Print the backend Go packages that belong to one CI test shard.
#
#   scripts/ci/go-test-shard.sh <shard> <total> [./pkg-pattern...]   # shard is 1-based
#
# Every package of `go list <patterns>` (run in backend/; default ./...) lands
# in exactly one shard. Patterns must be relative to backend/: "." or
# "./<path>". GO_TEST_SHARD_TAGS=<tags> lists packages with those build tags, so a
# tagged test run shards over the files that build under its tags.
#
# Packages are weighted by the bytes of their _test.go files, a cheap and
# deterministic proxy for test runtime, and assigned largest-first to the
# currently lightest shard (ties go to the lowest shard number). The same tree
# always yields the same split, so a failing shard reproduces locally:
#
#   cd backend && go test -short $(../scripts/ci/go-test-shard.sh 2 3)
#
# GO_TEST_SHARD_WEIGHTS=<file> replaces `go list` with "<weight> <package>"
# lines; scripts/ci/go-test-shard_test.sh uses it to test the assignment.
set -euo pipefail

usage() {
  echo "usage: $0 <shard> <total> [./pkg-pattern...]   (1 <= shard <= total)" >&2
  exit 2
}

[[ $# -ge 2 ]] || usage
shard="$1"
total="$2"
shift 2
[[ "$shard" =~ ^[1-9][0-9]*$ && "$total" =~ ^[1-9][0-9]*$ ]] || usage
((shard <= total)) || usage
patterns=("$@")
if [[ ${#patterns[@]} -eq 0 ]]; then
  patterns=(./...)
fi
for pattern in "${patterns[@]}"; do
  [[ "$pattern" == . || "$pattern" == ./* ]] || usage
done
tags=()
if [[ -n "${GO_TEST_SHARD_TAGS:-}" ]]; then
  [[ "$GO_TEST_SHARD_TAGS" =~ ^[A-Za-z0-9_,.]+$ ]] || usage
  tags=(-tags "$GO_TEST_SHARD_TAGS")
fi

weights() {
  if [[ -n "${GO_TEST_SHARD_WEIGHTS:-}" ]]; then
    cat "$GO_TEST_SHARD_WEIGHTS"
    return
  fi
  local root
  root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
  cd "$root/backend"
  local pkgs files
  pkgs="$(go list ${tags[@]+"${tags[@]}"} -f '{{.ImportPath}} {{.Dir}}' "${patterns[@]}")"
  # One line per test file; wc -c prints "<bytes> <path>" plus a total line.
  files="$(go list -f '{{range .TestGoFiles}}{{$.Dir}}/{{.}}{{"\n"}}{{end}}{{range .XTestGoFiles}}{{$.Dir}}/{{.}}{{"\n"}}{{end}}' ${tags[@]+"${tags[@]}"} "${patterns[@]}" | sed '/^$/d')"
  {
    printf '%s\n' "$pkgs" | sed 's/^/P /'
    if [[ -n "$files" ]]; then
      printf '%s\n' "$files" | tr '\n' '\0' | xargs -0 wc -c | sed '/ total$/d' | sed 's/^ *//; s/^/F /'
    fi
  } | awk '
    $1 == "P" { dir[$3] = $2; order[++n] = $3; w[$3] = 1; next }
    $1 == "F" {
      path = $3
      sub(/\/[^\/]*$/, "", path)
      if (path in dir) w[path] += $2
    }
    END { for (i = 1; i <= n; i++) print w[order[i]], dir[order[i]] }
  '
}

weights | LC_ALL=C sort -k1,1nr -k2,2 | awk -v shard="$shard" -v total="$total" '
  {
    best = 1
    for (s = 2; s <= total; s++) if (load[s] < load[best]) best = s
    load[best] += $1
    if (best == shard) print $2
  }
'
