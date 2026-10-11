# shellcheck shell=bash
# Shared helpers for check.sh and generate-third-party.sh. Source it; do not run it.

LIC_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
LIC_DIR="$LIC_ROOT/scripts/licenses"
LIC_BACKEND="$LIC_ROOT/backend"
# shellcheck disable=SC2034 # read by the scripts that source this file
LIC_FRONTEND="$LIC_ROOT/frontend"
LIC_GO_OVERRIDES="$LIC_DIR/go-overrides.tsv"

# Binaries the backend image ships (see backend/Dockerfile). Default build:
# the GPL-3.0 WhatsApp stack sits behind the opt-in `whatsapp` build tag and is
# deliberately outside this check.
LIC_GO_TARGETS=(./cmd/app ./cmd/email-smoke ./cmd/healthcheck)

# Pinned so reports are reproducible; override to try a newer release.
GO_LICENSES_VERSION="${GO_LICENSES_VERSION:-v2.0.1}"

lic_log() { printf 'licenses: %s\n' "$*" >&2; }
lic_die() { printf 'licenses: FAIL: %s\n' "$*" >&2; exit 1; }

# Use the toolchain go.mod asks for (go1.27.2 via GOTOOLCHAIN auto-switching),
# then pin it. go-licenses shells out to `go list` and type-checks against
# GOROOT; a mismatch fails with "compile: version X does not match go tool
# version Y".
lic_setup_go() {
  command -v go >/dev/null 2>&1 || lic_die "go is not on PATH"
  cd "$LIC_BACKEND" || lic_die "missing $LIC_BACKEND"
  local goroot
  goroot="$(go env GOROOT)" || lic_die "go env GOROOT failed"
  export GOROOT="$goroot"
  export PATH="$goroot/bin:$PATH"
  export GOTOOLCHAIN=local
  # Never inherit build tags (e.g. -tags=whatsapp) from the caller. Set
  # LICENSES_GO_TAGS to check a tagged build on purpose.
  if [[ -n "${GOFLAGS:-}" ]]; then
    lic_log "ignoring caller GOFLAGS=$GOFLAGS (set LICENSES_GO_TAGS for tagged builds)"
  fi
  unset GOFLAGS
  if [[ -n "${LICENSES_GO_TAGS:-}" ]]; then
    export GOFLAGS="-tags=$LICENSES_GO_TAGS"
    lic_log "checking tagged build: $GOFLAGS"
  fi
  # Mirror the image build (backend/Dockerfile: CGO_ENABLED=0 GOOS=linux).
  export GOOS="${LICENSES_GOOS:-linux}"
  export CGO_ENABLED=0
  # shellcheck disable=SC2034 # read by the scripts that source this file
  LIC_GO_MODULE="$(go list -m)" || lic_die "go list -m failed"
}

# Resolve go-licenses: $GO_LICENSES, then PATH, then a pinned `go install` into
# a per-version cache directory (needs network the first time only).
lic_go_licenses_bin() {
  if [[ -n "${GO_LICENSES:-}" ]]; then
    printf '%s\n' "$GO_LICENSES"
    return
  fi
  if command -v go-licenses >/dev/null 2>&1; then
    command -v go-licenses
    return
  fi
  local bindir="${XDG_CACHE_HOME:-$HOME/.cache}/payverge/go-licenses/$GO_LICENSES_VERSION"
  if [[ ! -x "$bindir/go-licenses" ]]; then
    lic_log "installing github.com/google/go-licenses/v2@$GO_LICENSES_VERSION into $bindir"
    mkdir -p "$bindir"
    # Build the tool for the host, whatever GOOS the check targets.
    env -u GOOS -u GOARCH -u GOFLAGS CGO_ENABLED=0 GOBIN="$bindir" \
      go install "github.com/google/go-licenses/v2@$GO_LICENSES_VERSION" >&2 ||
      lic_die "could not install go-licenses (offline? set GO_LICENSES=/path/to/go-licenses)"
  fi
  printf '%s\n' "$bindir/go-licenses"
}

# Prints the rows of go-overrides.tsv as "module<TAB>detected<TAB>license".
lic_go_overrides() {
  awk -F'\t' '!/^[[:space:]]*(#|$)/ { if (NF < 4) { exit 1 } print $1 "\t" $2 "\t" $3 }' "$LIC_GO_OVERRIDES" ||
    lic_die "malformed row in $LIC_GO_OVERRIDES (need module, detected, license, reason)"
}

# lic_notice_names FILE NAME: true when NAME (a Go module path or npm package
# name) appears in FILE as a whole name, not inside a longer one:
# gopkg.in/yaml does not match gopkg.in/yaml.v3, github.com/a/b does not match
# github.com/a/b-c or github.com/a/b/c. A URL (https://github.com/a/b) and a
# sentence-ending period still count. Same rule as mentionsName in
# npm-licenses.mjs.
lic_notice_names() {
  local file="$1" name="$2" esc chars='A-Za-z0-9._~@/-'
  # Escape ERE metacharacters so dots in module paths match only dots.
  esc="$(printf '%s' "$name" | sed 's/[][\.*^$()+?{}|]/\\&/g')"
  grep -qE "(^|[^$chars]|://)$esc(\\.?([^$chars]|\$))" "$file"
}

lic_module_dir() {
  go list -m -f '{{.Dir}}' "$1" 2>/dev/null
}

guard_go_ethereum() {
  local dir linked
  dir="$(lic_module_dir github.com/ethereum/go-ethereum)"
  [[ -n "$dir" && -d "$dir" ]] || lic_die "go-ethereum module directory not found (run go mod download)"
  grep -q 'GNU LESSER GENERAL PUBLIC LICENSE' "$dir/COPYING.LESSER" 2>/dev/null ||
    lic_die "go-ethereum: COPYING.LESSER missing or not LGPL in $dir"
  grep -q 'all code outside of the `cmd` directory) is licensed under the' "$dir/README.md" ||
    lic_die "go-ethereum: README no longer states the library/cmd licence split; re-review the LGPL override"
  linked="$(go list -deps "${LIC_GO_TARGETS[@]}" | grep -E '^github\.com/ethereum/go-ethereum/cmd(/|$)' || true)"
  [[ -z "$linked" ]] || lic_die "GPL-3.0 go-ethereum/cmd packages are linked into a shipped binary: $linked"
}

guard_freetype() {
  local dir
  dir="$(lic_module_dir github.com/golang/freetype)"
  [[ -n "$dir" && -d "$dir" ]] || lic_die "freetype module directory not found (run go mod download)"
  grep -q 'The FreeType License' "$dir/LICENSE" ||
    lic_die "freetype: LICENSE no longer offers the FreeType License; re-review the FTL override"
  [[ -f "$dir/licenses/ftl.txt" ]] || lic_die "freetype: licenses/ftl.txt missing in $dir"
}

lic_run_guard() {
  case "$1" in
    github.com/ethereum/go-ethereum) guard_go_ethereum ;;
    github.com/golang/freetype) guard_freetype ;;
    *) lic_die "override for $1 has no guard in lib.sh" ;;
  esac
}

# Subdirectory licences. A module may carry code under a licence of its own in
# a subdirectory (vendored or forked code, e.g. go-ethereum/crypto/keccak is
# BSD, The Go Authors). go-licenses reports only the module licence, so these
# are found here and must be reproduced too.
# shellcheck disable=SC2034 # read by check.sh
LIC_GO_SUBPACKAGE_TEXTS="$LIC_ROOT/LICENSES/go-subpackages.txt"

# lic_subpackage_licenses: reads "module|module_dir|package_dir" lines (go list
# -deps output) on stdin and prints "module/subdir<TAB>licence_file" for every
# LICENSE*/LICENCE*/COPYING* file between a package directory and its module
# root, root excluded, deduplicated.
lic_subpackage_licenses() {
  local module mdir pdir d f
  while IFS='|' read -r module mdir pdir; do
    [[ -n "$module" && -n "$mdir" && "$pdir" == "$mdir"/* ]] || continue
    d="$pdir"
    while [[ "$d" != "$mdir" && "$d" == "$mdir"/* ]]; do
      for f in "$d"/LICENSE* "$d"/LICENCE* "$d"/COPYING*; do
        [[ -f "$f" ]] && printf '%s/%s\t%s\n' "$module" "${d#"$mdir"/}" "$f"
      done
      d="$(dirname "$d")"
    done
  done | sort -u
}

# lic_licence_reproduced TEXT_FILE AGGREGATE: true when TEXT_FILE's text
# appears verbatim in AGGREGATE, ignoring whitespace layout only.
lic_licence_reproduced() {
  local text agg
  text="$(tr -s '[:space:]' ' ' <"$1" | sed 's/^ //; s/ $//')"
  agg="$(tr -s '[:space:]' ' ' <"$2")"
  [[ -n "$text" && "$agg" == *"$text"* ]]
}

# lic_mpl_modules MODULES_FILE: reads `go-licenses report` CSV
# (package,url,licence) on stdin and prints, deduplicated, the module (from
# MODULES_FILE, one path per line; longest prefix wins) of every package whose
# licence is MPL-*. MPL-2.0 3.2(a) requires telling recipients of the binary
# where the source of those files is, so check.sh requires each one in NOTICE.
lic_mpl_modules() {
  awk -F, -v mods="$1" '
    BEGIN { while ((getline m < mods) > 0) if (m != "") list[m] = 1 }
    $3 ~ /^MPL-/ {
      best = ""
      for (m in list)
        if (($1 == m || index($1, m "/") == 1) && length(m) > length(best)) best = m
      print (best != "" ? best : $1)
    }
  ' | sort -u
}
