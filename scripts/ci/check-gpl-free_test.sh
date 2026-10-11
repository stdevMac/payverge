#!/usr/bin/env bash
# Contract test for scripts/ci/check-gpl-free.sh. Runs the gate against a fake
# `go` binary (GO=...) so the pass/fail logic is exercised without compiling
# the backend. The real gate is `make check-gpl-free`.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
GATE="${ROOT}/scripts/ci/check-gpl-free.sh"
failures=0

fail() {
  echo "FAIL: $*" >&2
  failures=$((failures + 1))
}

pass() {
  echo "OK: $*"
}

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# make_fake_go <name> <default-deps> <tagged-deps> <edges> <build-exit>
# writes an executable that answers the three go invocations the gate makes:
#   go list -tags "" -deps [-f fmt] <pkg>  -> default-deps (or edges with -f)
#   go list -tags whatsapp -deps <pkg>     -> tagged-deps
#   go build -tags whatsapp ./...          -> exit <build-exit>
make_fake_go() {
  local dir="$TMP/$1"
  mkdir -p "$dir"
  printf '%s\n' "$2" >"$dir/default.txt"
  printf '%s\n' "$3" >"$dir/tagged.txt"
  printf '%s\n' "$4" >"$dir/edges.txt"
  cat >"$dir/go" <<EOF
#!/usr/bin/env bash
case "\$1" in
  build) exit $5 ;;
  list)
    if [[ "\$*" == *"-f "* ]]; then cat "$dir/edges.txt"; exit 0; fi
    if [[ "\$3" == "whatsapp" ]]; then cat "$dir/tagged.txt"; else cat "$dir/default.txt"; fi
    ;;
  *) echo "unexpected go \$*" >&2; exit 2 ;;
esac
EOF
  chmod +x "$dir/go"
  echo "$dir/go"
}

CLEAN_DEPS=$'fmt\ngithub.com/stdevmac/payverge/backend/internal/services\ngithub.com/ethereum/go-ethereum/common\ngithub.com/golang/freetype/truetype'
TAGGED_DEPS=$'fmt\ngithub.com/stdevmac/payverge/backend/internal/services\ngo.mau.fi/whatsmeow\ngo.mau.fi/libsignal/ecc'

# --- unit: denylist matching ---
# shellcheck source=/dev/null
source "$GATE"
got="$(printf '%s\n' \
  go.mau.fi/libsignal/ecc \
  go.mau.fi \
  go.mau.fi.example.com/pkg \
  github.com/ethereum/go-ethereum/cmd/geth \
  github.com/ethereum/go-ethereum/common \
  github.com/ethereum/go-ethereum/cmdline \
  github.com/golang/freetype/truetype | gpl_offenders)"
want=$'go.mau.fi/libsignal/ecc\ngo.mau.fi\ngithub.com/ethereum/go-ethereum/cmd/geth'
if [[ "$got" == "$want" ]]; then
  pass "gpl_offenders flags go.mau.fi and geth cmd/, allows LGPL geth libraries and FTL freetype"
else
  fail "gpl_offenders output mismatch; got:"$'\n'"$got"
fi

# --- e2e: clean default graph passes ---
fake="$(make_fake_go clean "$CLEAN_DEPS" "$TAGGED_DEPS" "" 0)"
if out="$(GO="$fake" bash "$GATE" 2>&1)"; then
  pass "clean default graph passes"
else
  fail "clean default graph should pass; output:"$'\n'"$out"
fi

# --- e2e: GPL in the default graph fails and names the importer ---
fake="$(make_fake_go leak "$TAGGED_DEPS" "$TAGGED_DEPS" \
  $'github.com/stdevmac/payverge/backend/internal/services fmt go.mau.fi/whatsmeow\ngo.mau.fi/whatsmeow go.mau.fi/libsignal/ecc' 0)"
if out="$(GO="$fake" bash "$GATE" 2>&1)"; then
  fail "GPL package in the default graph should fail"
elif [[ "$out" == *"github.com/stdevmac/payverge/backend/internal/services -> go.mau.fi/whatsmeow"* && "$out" != *"go.mau.fi/whatsmeow -> go.mau.fi/libsignal"* ]]; then
  pass "GPL leak fails and names only the non-GPL entry point"
else
  fail "GPL leak output should name github.com/stdevmac/payverge/backend/internal/services -> go.mau.fi/whatsmeow; got:"$'\n'"$out"
fi

# --- e2e: tag that no longer wires whatsmeow fails (gate is not vacuous) ---
fake="$(make_fake_go vacuous "$CLEAN_DEPS" "$CLEAN_DEPS" "" 0)"
if out="$(GO="$fake" bash "$GATE" 2>&1)"; then
  fail "a whatsapp tag that links no go.mau.fi should fail"
elif [[ "$out" == *"tag split is broken"* ]]; then
  pass "vacuous whatsapp tag fails"
else
  fail "vacuous tag output unexpected:"$'\n'"$out"
fi

# --- e2e: tagged build that does not compile fails, unless skipped ---
fake="$(make_fake_go nobuild "$CLEAN_DEPS" "$TAGGED_DEPS" "" 1)"
if out="$(GO="$fake" bash "$GATE" 2>&1)"; then
  fail "a broken -tags whatsapp build should fail"
elif [[ "$out" == *"go build -tags whatsapp ./... does not compile"* ]]; then
  pass "broken tagged build fails"
else
  fail "broken tagged build output unexpected:"$'\n'"$out"
fi
if out="$(GO="$fake" CHECK_GPL_FREE_SKIP_TAGGED_BUILD=1 bash "$GATE" 2>&1)"; then
  pass "CHECK_GPL_FREE_SKIP_TAGGED_BUILD=1 skips the tagged compile"
else
  fail "skip flag should bypass the tagged compile; output:"$'\n'"$out"
fi

echo ""
if [[ "$failures" -gt 0 ]]; then
  echo "check-gpl-free_test: ${failures} failure(s)"
  exit 1
fi
echo "check-gpl-free_test: all passed"
