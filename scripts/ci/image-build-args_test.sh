#!/usr/bin/env bash
# Contract tests for scripts/ci/image-build-args.sh, against a throwaway git
# repository.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
script="$here/image-build-args.sh"
tmp="$(mktemp -d "${TMPDIR:-/tmp}/image-build-args-test.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT

fail() {
  echo "image-build-args_test: FAIL: $*" >&2
  exit 1
}

repo="$tmp/repo"
mkdir -p "$repo"
git -C "$repo" init -q
printf 'fixture\n' >"$repo/README"
git -C "$repo" add README
GIT_COMMITTER_DATE="2026-01-02T03:04:05+00:00" \
  git -C "$repo" -c user.name=test -c user.email=test@example.invalid -c commit.gpgsign=false \
  commit -q -m fixture
sha="$(git -C "$repo" rev-parse HEAD)"

run() {
  (cd "$repo" && bash "$script" "$@")
}

# Frontend: exactly the four build-time-only keys, nothing deployment-specific.
out="$(run frontend 1.2.3)"
want="NEXT_PUBLIC_RELEASE_SHA=$sha
NEXT_PUBLIC_VERSION=1.2.3
NEXT_PUBLIC_BUILD_TIMESTAMP=2026-01-02T03:04:05+00:00
NEXT_PUBLIC_SENTRY_RELEASE=1.2.3"
[[ "$out" == "$want" ]] || fail "frontend args:
$out
want:
$want"

# Prereleases and the CI placeholder version are accepted verbatim.
run frontend 1.2.3-rc.1 | grep -qx 'NEXT_PUBLIC_VERSION=1.2.3-rc.1' || fail "prerelease version not passed through"
run frontend 0.0.0-ci | grep -qx 'NEXT_PUBLIC_VERSION=0.0.0-ci' || fail "CI version not passed through"

# No runtime-class setting may be baked in (URLs, network, support email, DSN).
if run frontend 1.2.3 | grep -Eq '://|_URL=|NETWORK=|SUPPORT_EMAIL=|SENTRY_DSN='; then
  fail "frontend args carry a runtime setting"
fi

# Backend: no build arguments at all.
out="$(run backend 1.2.3)"
[[ -z "$out" ]] || fail "backend args should be empty, got: $out"

# Usage errors exit 2 and print nothing on stdout.
for bad in "frontend v1.2.3" "frontend 1.2" "worker 1.2.3" "frontend" "frontend 1.2.3 extra"; do
  set +e
  # shellcheck disable=SC2086 # word-splitting the case into arguments is the point
  out="$(run $bad 2>/dev/null)"
  rc=$?
  set -e
  [[ $rc -eq 2 ]] || fail "'$bad' exited $rc, want 2"
  [[ -z "$out" ]] || fail "'$bad' printed build arguments: $out"
done

# A version with an embedded newline must not inject a second argument.
set +e
out="$(run frontend "$(printf '1.2.3\nNEXT_PUBLIC_API_URL=https://x.invalid')" 2>/dev/null)"
rc=$?
set -e
[[ $rc -eq 2 && -z "$out" ]] || fail "multi-line version was accepted (rc=$rc)"

echo "image-build-args_test: ok"
