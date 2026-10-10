#!/usr/bin/env bash
# Contract tests for scripts/ci/release-assets.sh, against a throwaway git
# repository with a fake deploy/ tree and a fake licence generator.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
script="$here/release-assets.sh"
tmp="$(mktemp -d "${TMPDIR:-/tmp}/release-assets-test.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT

fail() {
  echo "release-assets_test: FAIL: $*" >&2
  exit 1
}

make_repo() {
  local repo="$1"
  mkdir -p "$repo/deploy" "$repo/scripts/licenses" "$repo/docs/licensing"
  printf '#!/bin/sh\necho install\n' >"$repo/deploy/install.sh"
  printf 'services: {}\n' >"$repo/deploy/docker-compose.yml"
  printf 'import payverge.caddy\n' >"$repo/deploy/Caddyfile"
  printf ':80 {}\n' >"$repo/deploy/payverge.caddy"
  printf 'DOMAIN=localhost\n' >"$repo/deploy/.env.example"
  printf 'Apache License\n' >"$repo/LICENSE"
  printf 'Payverge NOTICE\n' >"$repo/NOTICE"
  cat >"$repo/scripts/licenses/generate-third-party.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '# Third-party licences\n\n| Package | License |\n' >docs/licensing/THIRD_PARTY_LICENSES.md
EOF
  git -C "$repo" init -q
  git -C "$repo" add .
  git -C "$repo" -c user.name=test -c user.email=test@example.invalid -c commit.gpgsign=false \
    commit -q -m fixture
}

run() {
  RELEASE_ASSETS_ROOT="$1" bash "$script" "${@:2}"
}

repo="$tmp/repo"
make_repo "$repo"

# Argument validation.
for bad in "" "v1.2.3" "1.2" "1.2.3;rm" "1.2.3 4"; do
  if run "$repo" "$bad" "$tmp/out-bad" >/dev/null 2>&1; then
    fail "accepted version '$bad'"
  fi
done
if run "$repo" 1.2.3 >/dev/null 2>&1; then
  fail "accepted a missing out-dir argument"
fi
mkdir "$tmp/exists"
if run "$repo" 1.2.3 "$tmp/exists" >/dev/null 2>&1; then
  fail "accepted an existing out-dir"
fi

# Happy path.
out="$tmp/out"
run "$repo" 1.2.3-rc.1 "$out" >/dev/null
for f in install.sh docker-compose.yml Caddyfile env.example THIRD_PARTY_LICENSES.md \
  payverge-deploy-1.2.3-rc.1.tar.gz SHA256SUMS; do
  [[ -s "$out/$f" ]] || fail "missing or empty asset $f"
done
[[ ! -e "$out/.env.example" ]] || fail "dot-file asset name must be dropped"
[[ -x "$out/install.sh" ]] || fail "install.sh must be executable"
grep -q 'DOMAIN=localhost' "$out/env.example" || fail "env.example content differs from deploy/.env.example"

# The tarball holds every tracked deploy/ file, plus LICENSE and NOTICE,
# under one prefix.
listing="$(tar -tzf "$out/payverge-deploy-1.2.3-rc.1.tar.gz" | LC_ALL=C sort)"
for entry in .env.example Caddyfile LICENSE NOTICE docker-compose.yml install.sh payverge.caddy; do
  grep -qx "payverge-deploy-1.2.3-rc.1/$entry" <<<"$listing" || fail "tarball is missing $entry"
done
[[ "$(tar -xzOf "$out/payverge-deploy-1.2.3-rc.1.tar.gz" payverge-deploy-1.2.3-rc.1/NOTICE)" == "Payverge NOTICE" ]] ||
  fail "tarball NOTICE differs from the repository's NOTICE"

# SHA256SUMS covers every other asset and verifies.
[[ "$(wc -l <"$out/SHA256SUMS" | tr -d ' ')" == 6 ]] || fail "SHA256SUMS must list 6 assets"
if command -v sha256sum >/dev/null 2>&1; then
  (cd "$out" && sha256sum -c --quiet SHA256SUMS) || fail "SHA256SUMS does not verify"
else
  (cd "$out" && shasum -a 256 -c --quiet SHA256SUMS) || fail "SHA256SUMS does not verify"
fi

# Assets come from HEAD: an uncommitted edit never ships.
printf 'TAMPERED=1\n' >>"$repo/deploy/.env.example"
printf 'TAMPERED=1\n' >>"$repo/LICENSE"
run "$repo" 1.2.3 "$tmp/out-head" >/dev/null
if grep -q TAMPERED "$tmp/out-head/env.example"; then
  fail "a working-tree edit leaked into the release assets"
fi
if tar -xzOf "$tmp/out-head/payverge-deploy-1.2.3.tar.gz" payverge-deploy-1.2.3/LICENSE | grep -q TAMPERED; then
  fail "a working-tree edit of LICENSE leaked into the deploy tarball"
fi
git -C "$repo" checkout -q -- deploy/.env.example LICENSE

# A missing deploy file fails closed.
git -C "$repo" rm -q deploy/Caddyfile
git -C "$repo" -c user.name=test -c user.email=test@example.invalid -c commit.gpgsign=false \
  commit -q -m "drop Caddyfile"
if run "$repo" 1.2.3 "$tmp/out-missing" >/dev/null 2>&1; then
  fail "staged assets without deploy/Caddyfile"
fi

# A missing NOTICE fails closed.
repo3="$tmp/repo3"
make_repo "$repo3"
git -C "$repo3" rm -q NOTICE
git -C "$repo3" -c user.name=test -c user.email=test@example.invalid -c commit.gpgsign=false \
  commit -q -m "drop NOTICE"
if run "$repo3" 1.2.3 "$tmp/out-nonotice" >/dev/null 2>&1; then
  fail "staged assets without NOTICE"
fi

# A generator that writes nothing fails closed.
repo2="$tmp/repo2"
make_repo "$repo2"
printf '#!/usr/bin/env bash\nexit 0\n' >"$repo2/scripts/licenses/generate-third-party.sh"
if run "$repo2" 1.2.3 "$tmp/out-nolic" >/dev/null 2>&1; then
  fail "staged assets without THIRD_PARTY_LICENSES.md"
fi

echo "release-assets_test: PASS"
