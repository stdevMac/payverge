#!/usr/bin/env bash
# Replace the committed CI-only secrets in a copied env file with fresh random
# values before a production-mode stack boots.
#
# .github/e2e/ci.env.fixture is public. Production preflight denylists every secret
# literal committed to this repository (backend/internal/config/
# secret_denylist.go), so a stack booted with ENV=production from the
# committed values would refuse to start. CI copies ci.env.fixture to .env and then
# runs this script, so each run gets its own JWT, plugin encryption key, and
# database password.
#
# Usage: scripts/ci/rotate-ci-secrets.sh [env-file]   (default: .env)
#
# Values are never printed. Under GitHub Actions each value is registered with
# ::add-mask:: so later log lines cannot echo it.
set -euo pipefail

ENV_FILE="${1:-.env}"
if [[ ! -f "$ENV_FILE" ]]; then
  echo "rotate-ci-secrets: ${ENV_FILE} not found" >&2
  exit 1
fi
command -v openssl >/dev/null 2>&1 || {
  echo "rotate-ci-secrets: openssl is required" >&2
  exit 1
}

# set_env_value KEY VALUE: replace the first KEY= line (dropping duplicates),
# or append KEY=VALUE when the key is absent. awk keeps it portable across
# GNU and BSD userlands, and the value travels through ENVIRON so no
# character in it needs escaping.
set_env_value() {
  local key="$1" value="$2" tmp
  tmp="$(mktemp "${ENV_FILE}.XXXXXX")"
  KEY="$key" VALUE="$value" awk '
    BEGIN { key = ENVIRON["KEY"]; value = ENVIRON["VALUE"]; done = 0 }
    index($0, key "=") == 1 {
      if (!done) { print key "=" value; done = 1 }
      next
    }
    { print }
    END { if (!done) print key "=" value }
  ' "$ENV_FILE" >"$tmp"
  cat "$tmp" >"$ENV_FILE"
  rm -f "$tmp"
}

mask() {
  if [[ "${GITHUB_ACTIONS:-}" == "true" ]]; then
    echo "::add-mask::$1"
  fi
}

jwt_secret="$(openssl rand -hex 32)"
plugin_key="$(openssl rand -base64 32 | tr -d '\n')"
# Hex only: the DB password is interpolated into DSNs and compose args.
db_password="$(openssl rand -hex 24)"

mask "$jwt_secret"
mask "$plugin_key"
mask "$db_password"

set_env_value JWT_SECRET_KEY "$jwt_secret"
set_env_value PLUGIN_SECRET_KEY "$plugin_key"
set_env_value DB_PASSWORD "$db_password"

echo "rotate-ci-secrets: rotated JWT_SECRET_KEY, PLUGIN_SECRET_KEY, DB_PASSWORD in ${ENV_FILE}"
