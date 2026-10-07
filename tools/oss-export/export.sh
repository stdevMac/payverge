#!/usr/bin/env bash
# Build the public single-commit Payverge tree from a ref of the private repo.
#
#   tools/oss-export/export.sh [<ref>] <outdir> --author-email <email> [options]
#
# Steps: git archive <ref> -> drop -> scrub -> fatal gates -> git init + one
# commit. Nothing is pushed, ever; the push is a manual owner step (README.md).
# Exit codes: 0 exported, 1 a gate failed (nothing committed), 2 usage/config.
#
# Bash 3.2 compatible (macOS /bin/bash).

set -euo pipefail

# A caller running inside a git hook or worktree script may export these; they
# would redirect every git call below to the wrong repository or identity.
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY \
  GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_PREFIX GIT_AUTHOR_NAME GIT_AUTHOR_EMAIL \
  GIT_AUTHOR_DATE GIT_COMMITTER_NAME GIT_COMMITTER_EMAIL GIT_COMMITTER_DATE

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)
CLI="$SCRIPT_DIR/lib/cli.mjs"
CONFIG_SUBDIR="docs/superpowers/oss-export"

usage() {
  cat <<'EOF'
usage: tools/oss-export/export.sh [<ref>] <outdir> --author-email <email> [options]

  <ref>                    commit-ish to export (default: oss/main)
  <outdir>                 new or empty directory OUTSIDE the repository

required:
  --author-email EMAIL     author/committer email of the single public commit

options:
  --author-name NAME       author/committer name (default: Marcos Maceo)
  --copyright-holder NAME  the only holder LICENSE's appendix copyright line may
                           name, if it is filled in (default: the author name)
  --message MSG            commit message (default: "Initial public release")
  --repo DIR               private repository (default: current directory)
  --forbidden FILE         forbidden-strings list (default: see "config lookup")
  --drop FILE              extra drop list
  --scrub FILE             scrub rules
  --secrets-allow FILE     reviewed scanner fixtures, pinned by sha256
  --config-dir DIR         read all four config files from DIR only
  --report-dir DIR         where reports go (default: <outdir>.export-report)
  --max-file-mb N          size gate limit (default: 10)
  --size-allow GLOB        allow a large path (repeatable; default: Noto CJK fonts)
  --allow-env-file PATH    allow one tracked env file (repeatable; owner sign-off)
  --allow-opaque GLOB      let an archive gate (c) cannot read ship unscanned
                           (repeatable; owner sign-off)
  --allow-missing-scanner  downgrade a missing gitleaks/trufflehog to a warning
  --trufflehog-verify      let trufflehog verify candidates against live APIs
  --no-commit              run every step and gate, but do not create the repo
  -h, --help               show this help

config lookup (each of forbidden.txt, drop.txt, scrub.rules, secrets-allow.txt):
  1. the explicit --<name> flag
  2. --config-dir DIR/<name>
  3. <parent of the main checkout>/oss-private/<name>
  4. <ref>:docs/superpowers/oss-export/<name> (dropped from the export)
  forbidden.txt is mandatory; the others may be absent.

environment:
  GITLEAKS_BIN, TRUFFLEHOG_BIN   scanner binaries (default: found on PATH)
EOF
}

die() {
  local code=$1
  shift
  printf 'oss-export: %s\n' "$*" >&2
  exit "$code"
}

say() { printf '==> %s\n' "$*"; }

# ---------------------------------------------------------------------------
# Arguments

REF=""
OUTDIR=""
AUTHOR_NAME="Marcos Maceo"
AUTHOR_EMAIL=""
COPYRIGHT_HOLDER=""
MESSAGE="Initial public release"
REPO_DIR="$PWD"
FORBIDDEN=""
DROP=""
SCRUB=""
SECRETS_ALLOW=""
CONFIG_DIR=""
REPORT_DIR=""
MAX_FILE_MB=10
ALLOW_MISSING=0
TH_VERIFY=0
NO_COMMIT=0
EXTRA_GATE_ARGS=()
POSITIONALS=()

need_value() {
  [ $# -ge 2 ] && [ -n "$2" ] || die 2 "$1 needs a value"
}

while [ $# -gt 0 ]; do
  arg=$1
  case "$arg" in
    --*=*)
      flag=${arg%%=*}
      value=${arg#*=}
      shift
      set -- "$flag" "$value" "$@"
      continue
      ;;
  esac
  case "$arg" in
    -h | --help) usage; exit 0 ;;
    --author-email) need_value "$@"; AUTHOR_EMAIL=$2; shift 2 ;;
    --author-name) need_value "$@"; AUTHOR_NAME=$2; shift 2 ;;
    --copyright-holder) need_value "$@"; COPYRIGHT_HOLDER=$2; shift 2 ;;
    --message) need_value "$@"; MESSAGE=$2; shift 2 ;;
    --repo) need_value "$@"; REPO_DIR=$2; shift 2 ;;
    --forbidden) need_value "$@"; FORBIDDEN=$2; shift 2 ;;
    --drop) need_value "$@"; DROP=$2; shift 2 ;;
    --scrub) need_value "$@"; SCRUB=$2; shift 2 ;;
    --secrets-allow) need_value "$@"; SECRETS_ALLOW=$2; shift 2 ;;
    --config-dir) need_value "$@"; CONFIG_DIR=$2; shift 2 ;;
    --report-dir) need_value "$@"; REPORT_DIR=$2; shift 2 ;;
    --max-file-mb) need_value "$@"; MAX_FILE_MB=$2; shift 2 ;;
    --size-allow) need_value "$@"; EXTRA_GATE_ARGS+=(--size-allow "$2"); shift 2 ;;
    --allow-env-file) need_value "$@"; EXTRA_GATE_ARGS+=(--allow-env-file "$2"); shift 2 ;;
    --allow-opaque) need_value "$@"; EXTRA_GATE_ARGS+=(--allow-opaque "$2"); shift 2 ;;
    --allow-missing-scanner) ALLOW_MISSING=1; shift ;;
    --trufflehog-verify) TH_VERIFY=1; shift ;;
    --no-commit) NO_COMMIT=1; shift ;;
    --push | --remote) die 2 "$arg is not supported: export.sh never pushes (see README.md, owner-only step)" ;;
    -*) die 2 "unknown option: $arg (see --help)" ;;
    *) POSITIONALS+=("$arg"); shift ;;
  esac
done

case ${#POSITIONALS[@]} in
  1) REF="oss/main"; OUTDIR=${POSITIONALS[0]} ;;
  2) REF=${POSITIONALS[0]}; OUTDIR=${POSITIONALS[1]} ;;
  *) usage >&2; die 2 "expected [<ref>] <outdir>" ;;
esac

[ -n "$AUTHOR_EMAIL" ] || die 2 "--author-email is required (no default)"
printf '%s' "$AUTHOR_EMAIL" | grep -Eq '^[^[:space:]@<>]+@[^[:space:]@<>]+\.[^[:space:]@<>]+$' \
  || die 2 "--author-email does not look like an email address"
case "$AUTHOR_NAME" in *'<'* | *'>'* | '') die 2 "--author-name must be non-empty and must not contain < or >" ;; esac
[ -n "$COPYRIGHT_HOLDER" ] || COPYRIGHT_HOLDER=$AUTHOR_NAME
case "$COPYRIGHT_HOLDER" in *$'\n'* | *$'\r'*) die 2 "--copyright-holder must be one line" ;; esac
case "$MAX_FILE_MB" in '' | *[!0-9.]*) die 2 "--max-file-mb must be a number" ;; esac

command -v git >/dev/null 2>&1 || die 2 "git is required"
command -v node >/dev/null 2>&1 || die 2 "node is required"
command -v tar >/dev/null 2>&1 || die 2 "tar is required"

# ---------------------------------------------------------------------------
# Repository, ref, output directory

REPO=$(git -C "$REPO_DIR" rev-parse --show-toplevel 2>/dev/null) || die 2 "not a git repository: $REPO_DIR"
REPO=$(cd "$REPO" && pwd -P)
COMMON_DIR=$(cd "$REPO" && cd "$(git rev-parse --git-common-dir)" && pwd -P)
MAIN_ROOT=$(dirname "$COMMON_DIR")
PRIVATE_DIR="$(dirname "$MAIN_ROOT")/oss-private"

COMMIT=$(git -C "$REPO" rev-parse --verify --quiet "${REF}^{commit}") || die 2 "unknown ref: $REF"

if [ -e "$OUTDIR" ]; then
  [ -d "$OUTDIR" ] || die 2 "outdir exists and is not a directory: $OUTDIR"
  [ -z "$(ls -A "$OUTDIR")" ] || die 2 "outdir is not empty: $OUTDIR (use a fresh directory)"
  CREATED_OUT=0
else
  mkdir -p "$OUTDIR"
  CREATED_OUT=1
fi
OUT=$(cd "$OUTDIR" && pwd -P)

refuse_out() {
  [ "$CREATED_OUT" = 1 ] && rmdir "$OUT" 2>/dev/null || true
  die 2 "$1"
}
case "$OUT/" in
  "$REPO/"* | "$MAIN_ROOT/"*) refuse_out "outdir must be outside the repository ($OUT)" ;;
esac
case "$REPO/" in
  "$OUT/"*) refuse_out "outdir must not contain the repository ($OUT)" ;;
esac

[ -n "$REPORT_DIR" ] || REPORT_DIR="${OUT}.export-report"
mkdir -p "$REPORT_DIR"
REPORT_DIR=$(cd "$REPORT_DIR" && pwd -P)
case "$REPORT_DIR/" in
  "$OUT/"* | "$REPO/"* | "$MAIN_ROOT/"*) refuse_out "--report-dir must be outside the outdir and the repository ($REPORT_DIR)" ;;
esac
rm -f "$REPORT_DIR"/{drop,scrub,gates,stats}.json "$REPORT_DIR"/secrets-allow.candidates.txt "$REPORT_DIR"/summary.txt

TMP=$(mktemp -d "${TMPDIR:-/tmp}/oss-export.XXXXXX")
chmod 700 "$TMP"

# Until the drop and scrub steps have run, $OUT holds the raw archive of the
# private ref (docs/superpowers included). If anything fails in that window,
# wipe it so an unsanitized tree is never left looking like an export. $OUT
# was verified empty (or created) above, so only our own files are removed.
PHASE=setup
cleanup() {
  local rc=$?
  rm -rf "$TMP"
  if [ "$rc" -ne 0 ] && [ "$PHASE" = raw ]; then
    if [ "$CREATED_OUT" = 1 ]; then
      rm -rf "$OUT"
    else
      find "$OUT" -mindepth 1 -maxdepth 1 -exec rm -rf {} +
    fi
    printf 'oss-export: removed the unsanitized archive from %s\n' "$OUT" >&2
  fi
  return "$rc"
}
trap cleanup EXIT
trap 'exit 130' INT TERM HUP

say "repo    $REPO"
say "ref     $REF ($COMMIT)"
say "outdir  $OUT"
say "reports $REPORT_DIR"

# ---------------------------------------------------------------------------
# 1. git archive (tracked content of the ref only; never the working tree)

PHASE=raw
# The user's global and system git config and attributes are switched off
# here too: core.autocrlf, core.eol or a global attributes file (export-ignore,
# export-subst, eol, filter) would otherwise change which bytes are exported.
# Unset, core.attributesFile still defaults to ~/.config/git/attributes, so it
# is pointed at /dev/null explicitly. The private repository's own
# .gitattributes, .git/config and .git/info/attributes still apply.
GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1 GIT_ATTR_NOSYSTEM=1 \
  git -C "$REPO" -c core.attributesFile=/dev/null -c core.autocrlf=false -c core.eol=lf \
  archive --format=tar "$COMMIT" | tar -xf - -C "$OUT"
say "archived $(find "$OUT" ! -type d | wc -l | tr -d ' ') paths from $REF"

# ---------------------------------------------------------------------------
# 2. Config lookup

RESOLVED=""
RESOLVED_FROM=""
mkdir -p "$TMP/config"
resolve_config() {
  local name=$1 explicit=$2
  RESOLVED=""
  RESOLVED_FROM=""
  if [ -n "$explicit" ]; then
    [ -f "$explicit" ] || die 2 "config file not found: $explicit"
    RESOLVED=$(cd "$(dirname "$explicit")" && pwd -P)/$(basename "$explicit")
    RESOLVED_FROM="flag"
  elif [ -n "$CONFIG_DIR" ]; then
    if [ -f "$CONFIG_DIR/$name" ]; then
      RESOLVED=$(cd "$CONFIG_DIR" && pwd -P)/$name
      RESOLVED_FROM="--config-dir"
    fi
  elif [ -f "$PRIVATE_DIR/$name" ]; then
    RESOLVED="$PRIVATE_DIR/$name"
    RESOLVED_FROM="private dir"
  elif [ -f "$OUT/$CONFIG_SUBDIR/$name" ] && [ ! -L "$OUT/$CONFIG_SUBDIR/$name" ]; then
    cp "$OUT/$CONFIG_SUBDIR/$name" "$TMP/config/$name"
    RESOLVED="$TMP/config/$name"
    RESOLVED_FROM="$REF:$CONFIG_SUBDIR/$name"
  fi
  if [ -z "$RESOLVED" ]; then
    say "config  $name <- (none)"
    return 0
  fi
  case "$RESOLVED" in
    "$OUT/"*) die 2 "config $name must not live inside the export" ;;
  esac
  case "$RESOLVED_FROM" in
    "$REF:"*) say "config  $name <- $RESOLVED_FROM (copied out before the drop)" ;;
    *) say "config  $name <- $RESOLVED_FROM ($RESOLVED)" ;;
  esac
}

resolve_config forbidden.txt "$FORBIDDEN"
FORBIDDEN_FILE=$RESOLVED
[ -n "$FORBIDDEN_FILE" ] || die 2 "no forbidden.txt found (flag, --config-dir, $PRIVATE_DIR, or $REF:$CONFIG_SUBDIR); refusing to export without the forbidden-strings gate"
resolve_config drop.txt "$DROP"
DROP_FILE=$RESOLVED
resolve_config scrub.rules "$SCRUB"
SCRUB_FILE=$RESOLVED
resolve_config secrets-allow.txt "$SECRETS_ALLOW"
ALLOW_FILE=$RESOLVED

CONFIG_ARGS=(--forbidden "$FORBIDDEN_FILE")
[ -z "$DROP_FILE" ] || CONFIG_ARGS+=(--drop "$DROP_FILE")
[ -z "$SCRUB_FILE" ] || CONFIG_ARGS+=(--scrub "$SCRUB_FILE")
[ -z "$ALLOW_FILE" ] || CONFIG_ARGS+=(--secrets-allow "$ALLOW_FILE")

node "$CLI" validate "${CONFIG_ARGS[@]}" || die 2 "configuration is invalid"

# ---------------------------------------------------------------------------
# 3. Drop, 4. scrub

say "drop"
node "$CLI" drop --root "$OUT" "${CONFIG_ARGS[@]}" --report "$REPORT_DIR/drop.json"
say "scrub"
node "$CLI" scrub --root "$OUT" "${CONFIG_ARGS[@]}" --report "$REPORT_DIR/scrub.json"
PHASE=sanitized

# ---------------------------------------------------------------------------
# 5. Fatal gates

GATE_ARGS=(--root "$OUT" --tmp-dir "$TMP" --report "$REPORT_DIR/gates.json"
  --candidates "$REPORT_DIR/secrets-allow.candidates.txt"
  --max-file-mb "$MAX_FILE_MB" --author "$AUTHOR_NAME <$AUTHOR_EMAIL>"
  "--message=$MESSAGE" "--copyright-holder=$COPYRIGHT_HOLDER")
[ "$ALLOW_MISSING" = 0 ] || GATE_ARGS+=(--allow-missing-scanner)
[ "$TH_VERIFY" = 0 ] || GATE_ARGS+=(--trufflehog-verify)

say "gates"
set +e
node "$CLI" gates "${GATE_ARGS[@]}" "${CONFIG_ARGS[@]}" ${EXTRA_GATE_ARGS[@]+"${EXTRA_GATE_ARGS[@]}"}
GATE_RC=$?
set -e

say "stats"
node "$CLI" stats --root "$OUT" --report "$REPORT_DIR/stats.json"

if [ "$GATE_RC" -ne 0 ]; then
  {
    echo "result: FAILED (gate exit $GATE_RC); nothing committed"
    echo "ref: $REF $COMMIT"
    echo "tree: $OUT"
  } >"$REPORT_DIR/summary.txt"
  [ "$GATE_RC" -eq 1 ] || die "$GATE_RC" "gates could not run (exit $GATE_RC); nothing committed"
  die 1 "one or more fatal gates failed; nothing committed. Tree left for inspection at $OUT; reports in $REPORT_DIR"
fi

if [ "$NO_COMMIT" = 1 ]; then
  {
    echo "result: gates passed; --no-commit"
    echo "ref: $REF $COMMIT"
    echo "tree: $OUT"
  } >"$REPORT_DIR/summary.txt"
  say "all gates passed; --no-commit given, so no repository was created"
  exit 0
fi

# ---------------------------------------------------------------------------
# 6. Fresh repository with exactly one commit

# Isolated from the user's global/system config: no global excludes (which
# would silently skip files), no hooks, no signing, no URL rewrites.
gitx() {
  GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1 git --literal-pathspecs -C "$OUT" \
    -c core.excludesFile=/dev/null -c core.hooksPath=/dev/null -c commit.gpgSign=false \
    -c core.autocrlf=false -c core.safecrlf=false "$@"
}

FILE_COUNT=$(cd "$OUT" && find . ! -type d | wc -l | tr -d ' ')
GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1 git init -q --template= "$OUT"
gitx symbolic-ref HEAD refs/heads/main
(cd "$OUT" && find . -path ./.git -prune -o ! -type d -print0) |
  gitx add --pathspec-from-file=- --pathspec-file-nul
LEFTOVER=$(gitx status --porcelain=v1 --ignored --untracked-files=all | grep -E '^(\?\?|!!) ' || true)
[ -z "$LEFTOVER" ] || die 1 "export repository has untracked or ignored paths after add; refusing to commit"
STAGED=$(gitx ls-files | wc -l | tr -d ' ')
[ "$STAGED" = "$FILE_COUNT" ] || die 1 "staged $STAGED paths but the tree has $FILE_COUNT; refusing to commit"

GIT_AUTHOR_NAME="$AUTHOR_NAME" GIT_AUTHOR_EMAIL="$AUTHOR_EMAIL" \
  GIT_COMMITTER_NAME="$AUTHOR_NAME" GIT_COMMITTER_EMAIL="$AUTHOR_EMAIL" \
  gitx commit -q --no-verify --no-gpg-sign -m "$MESSAGE"

[ "$(gitx rev-list --count HEAD)" = 1 ] || die 1 "expected exactly one commit"
[ -z "$(gitx remote)" ] || die 1 "the export repository unexpectedly has a remote"
[ -z "$(gitx status --porcelain=v1 --ignored --untracked-files=all)" ] || die 1 "the export repository is not clean after the commit"

HEAD_SHA=$(gitx rev-parse HEAD)
{
  echo "result: exported"
  echo "ref: $REF $COMMIT"
  echo "tree: $OUT"
  echo "commit: $HEAD_SHA"
  echo "files: $STAGED"
} >"$REPORT_DIR/summary.txt"

say "committed $STAGED files"
gitx log -1 --format='    commit %H%n    tree   %T%n    author %an <%ae> %ad%n    committer %cn <%ce> %cd%n    %s'
say "branch main, no remote, nothing pushed. Owner-only push step: see tools/oss-export/README.md"
