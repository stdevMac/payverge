#!/usr/bin/env bash
# Licence gate for what Payverge ships:
#   1. Go: go-licenses check --disallowed_types=forbidden,restricted,unknown
#      over the three binaries in the backend image (default build, no
#      whatsapp tag). Modules in go-overrides.tsv are ignored there and
#      checked by their guards. Licences in subdirectories of linked modules
#      must be named in NOTICE and reproduced in LICENSES/go-subpackages.txt.
#   2. Fonts: every tracked font directory carries its licence text.
#   3. Frontend: production npm dependencies via npm-licenses.mjs (fails on GPL,
#      AGPL, SSPL, BUSL, NC/ND, and on unreviewed weak copyleft or missing data).
#
# Usage: scripts/licenses/check.sh [--skip-go] [--skip-npm]
# Needs: go (module cache or network), node, and frontend/node_modules installed
# (npm ci) so packages the lockfile has no licence for can be read from disk.
set -euo pipefail

# shellcheck source=scripts/licenses/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

run_go=1
run_npm=1
for arg in "$@"; do
  case "$arg" in
    --skip-go) run_go=0 ;;
    --skip-npm) run_npm=0 ;;
    -h | --help)
      sed -n '2,14p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    *) lic_die "unknown argument $arg" ;;
  esac
done

check_go() {
  lic_setup_go
  local golic
  golic="$(lic_go_licenses_bin)"

  local graph_modules
  graph_modules="$(go list -deps -f '{{with .Module}}{{.Path}}{{end}}' "${LIC_GO_TARGETS[@]}" | sort -u)"

  # Read the rows before looping: a failure inside `< <(...)` would not stop
  # the script, and a half-read table would silently drop overrides.
  local overrides
  overrides="$(lic_go_overrides)" || lic_die "could not read $LIC_GO_OVERRIDES"

  local ignores=(--ignore "$LIC_GO_MODULE/")
  local module detected license
  while IFS=$'\t' read -r module detected license; do
    [[ -n "$module" ]] || continue
    grep -qxF "$module" <<<"$graph_modules" ||
      lic_die "override $module is no longer a dependency of ${LIC_GO_TARGETS[*]}; remove its row"
    lic_run_guard "$module"
    lic_log "override ok: $module ($detected -> $license)"
    ignores+=(--ignore "$module")
  done <<<"$overrides"

  lic_log "go-licenses check (${LIC_GO_TARGETS[*]}, GOOS=$GOOS, own module $LIC_GO_MODULE ignored)"
  # klog prints a warning per package with assembly files; keep real errors.
  local out status=0
  # `unknown` too: a licence go-licenses cannot classify fails instead of
  # passing unreviewed. go-overrides.tsv is the way to accept one.
  out="$("$golic" check --disallowed_types=forbidden,restricted,unknown "${ignores[@]}" "${LIC_GO_TARGETS[@]}" 2>&1)" || status=$?
  grep -vE "contains non-Go code that can't be inspected|^/.*\.(s|c|h|S)$" <<<"$out" >&2 || true
  [[ $status -eq 0 ]] || lic_die "go-licenses check failed (exit $status)"

  # Apache-2.0 4(d): a linked module's NOTICE must be reproduced in ours. The
  # module path must appear as a whole name (lic_notice_names), so a module is
  # not covered by a longer path that merely contains it.
  local deps dir notice_missing=0
  deps="$(go list -deps -f '{{with .Module}}{{.Path}}|{{.Dir}}{{end}}' "${LIC_GO_TARGETS[@]}" | sort -u)" ||
    lic_die "go list -deps failed"
  while IFS='|' read -r module dir; do
    # Our own module ships backend/NOTICE, which is this project's notice,
    # not a dependency's.
    [[ "$module" == "$LIC_GO_MODULE" ]] && continue
    [[ -n "$dir" ]] && compgen -G "$dir/NOTICE*" >/dev/null || continue
    if ! lic_notice_names "$LIC_ROOT/NOTICE" "$module"; then
      lic_log "$module ships a NOTICE file that the root NOTICE does not reproduce"
      notice_missing=1
    fi
  done <<<"$deps"
  [[ $notice_missing -eq 0 ]] || lic_die "root NOTICE is missing dependency notices"

  # Licences in subdirectories of a linked module (vendored or forked code under
  # its own BSD/MIT terms) bind the binary too, and go-licenses only reports the
  # module licence. Each one must be named in the root NOTICE as module/subdir
  # and its text reproduced verbatim in LICENSES/go-subpackages.txt.
  local subs sub file sub_missing=0
  subs="$(go list -deps -f '{{with .Module}}{{.Path}}|{{.Dir}}{{end}}|{{.Dir}}' "${LIC_GO_TARGETS[@]}" |
    lic_subpackage_licenses)" || lic_die "go list -deps failed"
  while IFS=$'\t' read -r sub file; do
    [[ -n "$sub" ]] || continue
    [[ "$sub" == "$LIC_GO_MODULE"/* ]] && continue
    if ! lic_notice_names "$LIC_ROOT/NOTICE" "$sub"; then
      lic_log "$sub carries its own licence ($file) that the root NOTICE does not name"
      sub_missing=1
    fi
    if ! lic_licence_reproduced "$file" "$LIC_GO_SUBPACKAGE_TEXTS"; then
      lic_log "$sub licence text is not reproduced in ${LIC_GO_SUBPACKAGE_TEXTS#"$LIC_ROOT"/}"
      sub_missing=1
    fi
  done <<<"$subs"
  [[ $sub_missing -eq 0 ]] || lic_die "subdirectory licences are not reproduced"
  lic_log "go: ok"
}

check_fonts() {
  local fonts dir missing=0
  fonts="$(git -C "$LIC_ROOT" ls-files -- '*.ttf' '*.otf' '*.woff' '*.woff2' '*.ttc')" ||
    lic_die "git ls-files failed"
  while IFS= read -r dir; do
    [[ -n "$dir" ]] || continue
    if ! compgen -G "$LIC_ROOT/$dir/OFL.txt" >/dev/null && ! compgen -G "$LIC_ROOT/$dir/LICENSE*" >/dev/null; then
      lic_log "font directory without OFL.txt/LICENSE: $dir"
      missing=1
    fi
  done <<<"$(sed -e '/^$/d' -e '/\//!s/.*/./' -e 's#/[^/]*$##' <<<"$fonts" | sort -u)"
  [[ $missing -eq 0 ]] || lic_die "fonts are missing their licence text"
  lic_log "fonts: ok"
}

check_npm() {
  command -v node >/dev/null 2>&1 || lic_die "node is not on PATH"
  node "$LIC_DIR/npm-licenses.mjs" check || lic_die "frontend production dependencies"
}

[[ $run_go -eq 1 ]] && (check_go)
check_fonts
[[ $run_npm -eq 1 ]] && check_npm
lic_log "all licence checks passed"
