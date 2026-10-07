#!/usr/bin/env bash
# Release gate: the default (untagged) backend build must link no GPL code.
#
# Payverge is Apache-2.0. The only GPL code reachable from the backend is the
# WhatsApp integration (go.mau.fi/whatsmeow -> go.mau.fi/libsignal, GPL-3.0),
# which compiles in only with `-tags whatsapp`. This script proves that:
#
#   1. `go list -deps` of every shipped binary (and of ./..., everything a
#      source user can build) contains no package from a denylisted module, in
#      the same GOOS/CGO configuration the Docker image builds with;
#   2. the `whatsapp` tag still pulls go.mau.fi in (the gate above is not
#      vacuous because the tag silently stopped wiring whatsmeow);
#   3. `go build -tags whatsapp ./...` still compiles, so the opt-in build does
#      not rot while the default build stays clean.
#
# Usage (from anywhere):
#   bash scripts/ci/check-gpl-free.sh        # or: make check-gpl-free
#
# Optional env overrides:
#   GO                         go binary (default: go)
#   CHECK_GPL_FREE_SKIP_TAGGED_BUILD=1  skip step 3 (quick local re-runs)
#
# Licence evidence: go-licenses report ./cmd/app (default build). Modules that
# go-licenses flags but that are deliberately NOT denylisted:
#   - github.com/ethereum/go-ethereum: reported as GPL-3.0 because the tool
#     reads the repo-root COPYING; every package outside cmd/ is LGPL-3.0
#     (COPYING.LESSER). We link library packages only, and the cmd/ tree is
#     denylisted below so a geth binary package can never sneak in.
#   - github.com/golang/freetype: dual FreeType License / GPL-2.0+; Payverge
#     elects the FTL (attribution in NOTICE).
set -euo pipefail

# Module or package path prefixes whose code is GPL. A dependency graph entry
# matches when it equals the prefix or starts with "<prefix>/".
GPL_DENYLIST=(
  # whatsmeow (MPL-2.0) is only usable together with libsignal (GPL-3.0); the
  # whole go.mau.fi namespace belongs behind the whatsapp tag.
  "go.mau.fi"
  # geth's binaries are GPL-3.0 (the libraries are LGPL-3.0, see header).
  "github.com/ethereum/go-ethereum/cmd"
)

GO="${GO:-go}"

# Binaries the backend Docker image ships (backend/Dockerfile).
SHIPPED_PACKAGES=(./cmd/app ./cmd/email-smoke ./cmd/healthcheck)

# gpl_offenders reads import paths on stdin and prints every one that falls
# under a denylisted prefix.
gpl_offenders() {
  local pkg prefix
  while IFS= read -r pkg; do
    [[ -z "$pkg" ]] && continue
    for prefix in "${GPL_DENYLIST[@]}"; do
      if [[ "$pkg" == "$prefix" || "$pkg" == "$prefix"/* ]]; then
        echo "$pkg"
        break
      fi
    done
  done
}

# gpl_entry_points <package> prints "importer -> import" for every edge where a
# non-GPL package of the default graph imports a denylisted one: the files that
# need a build tag.
gpl_entry_points() {
  local pkg imp imports
  list_deps "" -f '{{.ImportPath}}{{range .Imports}} {{.}}{{end}}' "$1" |
    while read -r pkg imports; do
      [[ -n "$(gpl_offenders <<<"$pkg")" ]] && continue
      for imp in $imports; do
        if [[ -n "$(gpl_offenders <<<"$imp")" ]]; then
          echo "$pkg -> $imp"
        fi
      done
    done
}

# list_deps <tags> <go list args...> lists the non-test dependency graph in the
# configuration backend/Dockerfile builds with (linux, CGO_ENABLED=0).
list_deps() {
  local tags="$1"
  shift
  CGO_ENABLED=0 GOOS=linux "$GO" list -tags "$tags" -deps "$@"
}

main() {
  local root failures=0
  root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
  cd "$root/backend"

  local target deps offenders
  for target in "${SHIPPED_PACKAGES[@]}" ./...; do
    if ! deps="$(list_deps "" "$target")"; then
      echo "FAIL: go list -deps $target failed" >&2
      failures=$((failures + 1))
      continue
    fi
    offenders="$(gpl_offenders <<<"$deps")"
    if [[ -n "$offenders" ]]; then
      echo "FAIL: default build of $target links $(wc -l <<<"$offenders" | tr -d ' ') GPL package(s), e.g.:" >&2
      head -n 10 <<<"$offenders" | sed 's/^/  /' >&2
      echo "  entry points (non-GPL package -> GPL import):" >&2
      gpl_entry_points "$target" | sed 's/^/    /' >&2
      echo "  Move the importer behind //go:build whatsapp (see internal/services/whatsapp_stub.go)." >&2
      failures=$((failures + 1))
    else
      echo "OK: default build of $target is GPL-free"
    fi
  done

  local tagged_count
  tagged_count="$(list_deps whatsapp ./cmd/app | gpl_offenders | grep -c '^go\.mau\.fi' || true)"
  if [[ "$tagged_count" -eq 0 ]]; then
    echo "FAIL: -tags whatsapp no longer links go.mau.fi into ./cmd/app; the tag split is broken" >&2
    failures=$((failures + 1))
  else
    echo "OK: -tags whatsapp links go.mau.fi into ./cmd/app (${tagged_count} packages, GPL-3.0 build)"
  fi

  if [[ "${CHECK_GPL_FREE_SKIP_TAGGED_BUILD:-}" == "1" ]]; then
    echo "SKIP: go build -tags whatsapp ./... (CHECK_GPL_FREE_SKIP_TAGGED_BUILD=1)"
  elif "$GO" build -tags whatsapp ./...; then
    echo "OK: go build -tags whatsapp ./... compiles"
  else
    echo "FAIL: go build -tags whatsapp ./... does not compile" >&2
    failures=$((failures + 1))
  fi

  if [[ "$failures" -gt 0 ]]; then
    echo "check-gpl-free: ${failures} failure(s)" >&2
    return 1
  fi
  echo "check-gpl-free: passed"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
