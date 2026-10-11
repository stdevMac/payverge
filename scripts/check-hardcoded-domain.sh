#!/usr/bin/env bash
#
# Fails if an upstream brand domain (payverge.io, payverge.com) is hard-coded
# in shipped runtime code. A self-hosted instance must never link, mail, redirect, or
# trust the upstream deployment: every public URL, contact address, and
# product name comes from the instance env (PUBLIC_URL, PRODUCT_NAME,
# SUPPORT_EMAIL, ...) through one brand-default module per tier.
#
# Scanned (tracked + untracked, .gitignore respected):
#   backend   backend/cmd, backend/internal, backend/email
#             (non-test Go, embedded prompts/text, email layouts + templates)
#   frontend  frontend/src
#
# Always allowed (never scanned):
#   tests      *_test.go, *.test.*, *.spec.*, __tests__/, __mocks__/
#   fixtures   testdata/, fixtures/ (demo + recorded fixtures)
#   docs       *.md / *.mdx under frontend/src (blog/legal copy is content)
#   brand      the brand-default modules in ALLOWLIST below
#   debt       KNOWN_DEBT files (temporary, owned elsewhere, warned on;
#              scanned and fatal under --release so no release ships them)
#
# Usage:
#   scripts/check-hardcoded-domain.sh                 # backend + frontend
#   scripts/check-hardcoded-domain.sh --backend-only  # backend gate (CI)
#   scripts/check-hardcoded-domain.sh --frontend-only
#   scripts/check-hardcoded-domain.sh --count         # per-scope totals only
#   scripts/check-hardcoded-domain.sh --release       # KNOWN_DEBT counts too
#                                                     # (release workflow gate)
#
# Exit codes: 0 clean, 1 hard-coded domain found, 2 usage/environment error.
# CHECK_DOMAIN_ROOT overrides the repository root (used by the contract test).

set -euo pipefail

ROOT="${CHECK_DOMAIN_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
# Both upstream apexes: payverge.io (product) and payverge.com (legacy app
# links, e.g. the old Telegram dashboard button default).
DOMAIN_RE='payverge\.(io|com)'
DOMAIN_LABEL='payverge.io/payverge.com'

# Brand-default modules: the ONLY runtime files allowed to name the upstream
# domain. Each entry is an exact repo-relative path with a reason.
ALLOWLIST=(
  # Backend brand-default module: names the upstream domain as a constant for
  # the upstream demo photography host; never a runtime link default.
  "backend/internal/config/instance.go"
  # Frontend brand-default module: the only hit is a doc comment stating
  # that no default ever points at the upstream payverge.io accounts.
  "frontend/src/config/brand.ts"
)

# Known debt: shipped frontend files that still name the upstream domain and
# are owned by the frontend workstream. Excluded so the gate blocks NEW hits
# while the fix lands; delete each entry when its file is cleaned. Every entry
# needs a reason and is reported as a warning on every run.
KNOWN_DEBT=(
  # Empty: the frontend debt (docs alias, legal and tools copy) is cleaned.
)

TEST_AND_FIXTURE_EXCLUDES=(
  ':(exclude,glob)**/*_test.go'
  ':(exclude,glob)**/*.test.*'
  ':(exclude,glob)**/*.spec.*'
  ':(exclude,glob)**/__tests__/**'
  ':(exclude,glob)**/__mocks__/**'
  ':(exclude,glob)**/testdata/**'
  ':(exclude,glob)**/fixtures/**'
)

BACKEND_PATHS=(backend/cmd backend/internal backend/email)
FRONTEND_PATHS=(frontend/src)
FRONTEND_DOC_EXCLUDES=(
  ':(exclude,glob)**/*.md'
  ':(exclude,glob)**/*.mdx'
)

mode="all"
count_only=0
release=0
for arg in "$@"; do
  case "${arg}" in
    --backend-only) mode="backend" ;;
    --frontend-only) mode="frontend" ;;
    --count) count_only=1 ;;
    --release) release=1 ;;
    -h | --help)
      sed -n '2,31p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    *)
      echo "error: unknown argument: ${arg}" >&2
      exit 2
      ;;
  esac
done

if ! git -C "${ROOT}" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  echo "error: ${ROOT} is not a git work tree" >&2
  exit 2
fi

# A release must not ship known debt: --release scans those files too.
excluded=("${ALLOWLIST[@]}")
[[ "${release}" -eq 1 ]] || excluded+=(${KNOWN_DEBT[@]+"${KNOWN_DEBT[@]}"})
allow_excludes=()
for path in "${excluded[@]}"; do
  allow_excludes+=(":(exclude)${path}")
done

# scan <label> <pathspec...>: prints matching lines; returns 0 always.
scan() {
  local existing=() spec
  for spec in "$@"; do
    case "${spec}" in
      :*) existing+=("${spec}") ;;
      *) [[ -e "${ROOT}/${spec}" ]] && existing+=("${spec}") ;;
    esac
  done
  # Only exclusion specs left means nothing to scan.
  local has_include=0
  for spec in "${existing[@]}"; do
    [[ "${spec}" != :* ]] && has_include=1
  done
  [[ "${has_include}" -eq 1 ]] || return 0
  git -C "${ROOT}" grep --untracked -n -I -i -E -e "${DOMAIN_RE}" -- "${existing[@]}" || true
}

backend_hits=""
frontend_hits=""
if [[ "${mode}" != "frontend" ]]; then
  backend_hits="$(scan "${BACKEND_PATHS[@]}" "${TEST_AND_FIXTURE_EXCLUDES[@]}" "${allow_excludes[@]}")"
fi
if [[ "${mode}" != "backend" ]]; then
  frontend_hits="$(scan "${FRONTEND_PATHS[@]}" "${TEST_AND_FIXTURE_EXCLUDES[@]}" "${FRONTEND_DOC_EXCLUDES[@]}" "${allow_excludes[@]}")"
fi

count_lines() {
  if [[ -z "$1" ]]; then echo 0; else printf '%s\n' "$1" | wc -l | tr -d ' '; fi
}
count_files() {
  if [[ -z "$1" ]]; then echo 0; else printf '%s\n' "$1" | cut -d: -f1 | sort -u | wc -l | tr -d ' '; fi
}

status=0
report() {
  local label="$1" hits="$2"
  local n files
  n="$(count_lines "${hits}")"
  files="$(count_files "${hits}")"
  if [[ "${n}" -eq 0 ]]; then
    echo "ok: ${label}: no hard-coded ${DOMAIN_LABEL} literals"
    return
  fi
  status=1
  echo "error: ${label}: ${n} hard-coded ${DOMAIN_LABEL} literal(s) in ${files} file(s)" >&2
  if [[ "${count_only}" -eq 0 ]]; then
    printf '%s\n' "${hits}" >&2
  fi
}

if [[ "${mode}" != "frontend" ]]; then
  report "backend" "${backend_hits}"
fi
if [[ "${mode}" != "backend" ]]; then
  report "frontend" "${frontend_hits}"
fi
if [[ "${mode}" != "backend" && "${release}" -eq 0 ]]; then
  debt=0
  for path in ${KNOWN_DEBT[@]+"${KNOWN_DEBT[@]}"}; do
    [[ -e "${ROOT}/${path}" ]] && debt=$((debt + 1))
  done
  if [[ "${debt}" -gt 0 ]]; then
    echo "warning: frontend: ${debt} known-debt file(s) still excluded (see KNOWN_DEBT)" >&2
  fi
fi

if [[ "${status}" -ne 0 ]]; then
  cat >&2 <<'EOF'
hint: read the value from the instance config instead
  backend:  config.PublicURL(), config.PublicHost(), config.SupportEmail(),
            config.ProductName() (backend/internal/config/instance.go)
  tests, fixtures, and docs are allowed; brand-default modules go in ALLOWLIST.
EOF
fi
exit "${status}"
