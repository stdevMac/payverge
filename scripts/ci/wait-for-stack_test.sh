#!/usr/bin/env bash
# Contract test for the nightly E2E CI env + stack wait helper.
# Asserts .github/e2e/ci.env.fixture satisfies production preflight with CI-only values
# and that scripts/ci/wait-for-stack.sh exists and is a polling wait script.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
ENV_CI="${ROOT}/.github/e2e/ci.env.fixture"
WAIT_SH="${ROOT}/scripts/ci/wait-for-stack.sh"
failures=0

fail() {
  echo "FAIL: $*" >&2
  failures=$((failures + 1))
}

pass() {
  echo "OK: $*"
}

# --- presence ---
if [[ ! -f "$ENV_CI" ]]; then
  fail ".github/e2e/ci.env.fixture is missing"
else
  pass ".github/e2e/ci.env.fixture exists"
fi

if [[ ! -f "$WAIT_SH" ]]; then
  fail "scripts/ci/wait-for-stack.sh is missing"
else
  pass "scripts/ci/wait-for-stack.sh exists"
fi

# Stop early if env file is missing — remaining assertions need it.
if [[ ! -f "$ENV_CI" ]]; then
  echo ""
  echo "wait-for-stack_test: ${failures} failure(s)"
  exit 1
fi

# Helper: require KEY=non-empty (ignores comments; allows quoted values)
require_nonempty() {
  local key="$1"
  local line
  line="$(grep -E "^${key}=" "$ENV_CI" | head -n1 || true)"
  if [[ -z "$line" ]]; then
    fail "${key} is missing"
    return
  fi
  local val="${line#*=}"
  # strip optional surrounding quotes
  val="${val%\"}"
  val="${val#\"}"
  val="${val%\'}"
  val="${val#\'}"
  if [[ -z "$val" ]]; then
    fail "${key} is empty"
    return
  fi
  pass "${key} is set"
}

require_exact() {
  local key="$1"
  local expected="$2"
  local line
  line="$(grep -E "^${key}=" "$ENV_CI" | head -n1 || true)"
  if [[ -z "$line" ]]; then
    fail "${key} is missing (expected ${key}=${expected})"
    return
  fi
  local val="${line#*=}"
  val="${val%\"}"
  val="${val#\"}"
  if [[ "$val" != "$expected" ]]; then
    fail "${key}=${val} (expected ${expected})"
    return
  fi
  pass "${key}=${expected}"
}

require_empty() {
  local key="$1"
  local line
  line="$(grep -E "^${key}=" "$ENV_CI" | head -n1 || true)"
  if [[ -z "$line" ]]; then
    fail "${key} is missing (expected an explicit empty value)"
    return
  fi
  local val="${line#*=}"
  val="${val%\"}"
  val="${val#\"}"
  if [[ -n "$val" ]]; then
    fail "${key} must be empty (got ${val})"
    return
  fi
  pass "${key} is explicitly empty"
}

# Production mode
require_exact "ENV" "production"

# Fixed CI contract values from the task
require_exact "NEXT_PUBLIC_API_URL" "http://localhost:8080/api/v1"
require_exact "FRONTEND_PORT" "3000"
require_exact "BACKEND_PORT" "8080"
require_exact "AI_DAILY_BUDGET_USD" "5"

# Production preflight-required vars (must be non-empty)
# See backend/internal/config/production_preflight.go
require_nonempty "JWT_SECRET_KEY"
require_nonempty "PLUGIN_SECRET_KEY"
require_nonempty "RPC_URL"
require_nonempty "EMAIL_PROVIDER"
require_nonempty "EMAIL_API_KEY"
require_nonempty "FROM_EMAIL"
require_nonempty "FROM_EMAIL_UPDATES"
require_nonempty "S3_BUCKET"
require_nonempty "AWS_ACCESS_KEY"
require_nonempty "AWS_SECRET_KEY"
require_nonempty "S3_PROTECTED_BUCKET"
require_nonempty "AWS_PROTECTED_ACCESS_KEY"
require_nonempty "AWS_PROTECTED_SECRET_KEY"
require_nonempty "ALLOWED_ORIGINS"
require_nonempty "COOKIE_DOMAIN"
require_nonempty "TRUSTED_PROXIES"

# Compose-interpolated DB / ports (docker-compose.yml)
require_nonempty "DB_HOST"
require_nonempty "DB_PORT"
require_nonempty "DB_USER"
require_nonempty "DB_PASSWORD"
require_nonempty "DB_NAME"

# JWT length + not a known placeholder prefix
jwt_line="$(grep -E '^JWT_SECRET_KEY=' "$ENV_CI" | head -n1 || true)"
jwt_val="${jwt_line#JWT_SECRET_KEY=}"
if [[ ${#jwt_val} -lt 32 ]]; then
  fail "JWT_SECRET_KEY must be >=32 characters (got ${#jwt_val})"
else
  pass "JWT_SECRET_KEY length >=32 (${#jwt_val})"
fi
jwt_lower="$(printf '%s' "$jwt_val" | tr '[:upper:]' '[:lower:]')"
case "$jwt_lower" in
  replace_with*|changeme*)
    fail "JWT_SECRET_KEY looks like a known-unsafe placeholder"
    ;;
  *)
    pass "JWT_SECRET_KEY is not a known-unsafe placeholder"
    ;;
esac

# PLUGIN_SECRET_KEY should be base64 of 32 bytes (or raw 32 bytes)
plugin_line="$(grep -E '^PLUGIN_SECRET_KEY=' "$ENV_CI" | head -n1 || true)"
plugin_val="${plugin_line#PLUGIN_SECRET_KEY=}"
if command -v openssl >/dev/null 2>&1; then
  decoded_len="$(printf '%s' "$plugin_val" | openssl base64 -d -A 2>/dev/null | wc -c | tr -d ' ')"
  if [[ "$decoded_len" == "32" ]] || [[ ${#plugin_val} -eq 32 ]]; then
    pass "PLUGIN_SECRET_KEY is 32 bytes (base64 or raw)"
  else
    fail "PLUGIN_SECRET_KEY must be base64 of 32 bytes or raw 32 bytes (decoded_len=${decoded_len}, raw_len=${#plugin_val})"
  fi
else
  if [[ ${#plugin_val} -ge 40 ]]; then
    pass "PLUGIN_SECRET_KEY looks base64-sized (${#plugin_val} chars; openssl unavailable to verify)"
  else
    fail "PLUGIN_SECRET_KEY too short to be base64 of 32 bytes"
  fi
fi

# Caddy is the only Cloudflare-aware hop. Gin must trust only the private
# compose/Caddy network and must never parse Cloudflare headers itself.
require_empty "TRUSTED_PLATFORM"
require_exact "TRUSTED_PROXIES" "172.16.0.0/12"

# No Telegram credentials in CI: enablement is credential-derived, so an
# empty token keeps the worker and webhook dark without any flag.
require_empty "TELEGRAM_TOKEN"

# Reject real production endpoints / credentials. Only assignments are
# checked: a comment that names the parent domain to explain why it fails
# preflight configures nothing.
if grep -Ev '^[[:space:]]*#' "$ENV_CI" | grep -Eiq 'payverge\.io|api\.payverge\.io|https://payverge'; then
  fail "ci.env.fixture must not contain payverge.io / api.payverge.io / https://payverge endpoints"
else
  pass "no payverge.io production endpoints in ci.env.fixture"
fi

# All http(s) URLs and origins should be localhost / 127.0.0.1 / docker service names
while IFS= read -r url; do
  host_part="${url#*://}"
  host_part="${host_part%%/*}"
  host_part="${host_part%%:*}"
  case "$host_part" in
    localhost|127.0.0.1|minio|postgres|backend|frontend)
      ;;
    *)
      fail "non-local URL host in ci.env.fixture: ${url} (host=${host_part})"
      ;;
  esac
done < <(grep -Eo 'https?://[^[:space:]#,;]+' "$ENV_CI" || true)

# ALLOWED_ORIGINS must be localhost-only
origins_line="$(grep -E '^ALLOWED_ORIGINS=' "$ENV_CI" | head -n1 || true)"
origins_val="${origins_line#ALLOWED_ORIGINS=}"
if [[ "$origins_val" == *payverge* ]]; then
  fail "ALLOWED_ORIGINS must not include payverge domains"
elif [[ "$origins_val" != *localhost* && "$origins_val" != *127.0.0.1* ]]; then
  fail "ALLOWED_ORIGINS must be localhost/127.0.0.1 only (got ${origins_val})"
else
  pass "ALLOWED_ORIGINS is localhost-only"
fi

# Committed secrets are public and denylisted by production preflight, so
# every workflow that boots ci.env.fixture in production mode must rotate them first.
ROTATE_SH="${ROOT}/scripts/ci/rotate-ci-secrets.sh"
if [[ ! -x "$ROTATE_SH" ]]; then
  fail "scripts/ci/rotate-ci-secrets.sh is missing or not executable"
else
  rot_dir="$(mktemp -d)"
  cp "$ENV_CI" "${rot_dir}/.env"
  if bash "$ROTATE_SH" "${rot_dir}/.env" >/dev/null; then
    for key in JWT_SECRET_KEY PLUGIN_SECRET_KEY DB_PASSWORD; do
      before="$(grep -E "^${key}=" "$ENV_CI" | head -n1)"
      after="$(grep -E "^${key}=" "${rot_dir}/.env" | head -n1)"
      count="$(grep -cE "^${key}=" "${rot_dir}/.env" || true)"
      if [[ -z "${after#*=}" || "$before" == "$after" || "$count" != "1" ]]; then
        fail "rotate-ci-secrets.sh did not replace ${key} exactly once"
      else
        pass "rotate-ci-secrets.sh rotates ${key}"
      fi
    done
    rotated_plugin="$(grep -E '^PLUGIN_SECRET_KEY=' "${rot_dir}/.env" | head -n1)"
    rotated_plugin="${rotated_plugin#PLUGIN_SECRET_KEY=}"
    rotated_len="$(printf '%s' "$rotated_plugin" | openssl base64 -d -A 2>/dev/null | wc -c | tr -d ' ')"
    if [[ "$rotated_len" == "32" ]]; then
      pass "rotated PLUGIN_SECRET_KEY is base64 of 32 bytes"
    else
      fail "rotated PLUGIN_SECRET_KEY must decode to 32 bytes (got ${rotated_len})"
    fi
    if [[ "$(grep -cv -E '^(JWT_SECRET_KEY|PLUGIN_SECRET_KEY|DB_PASSWORD)=' "$ENV_CI")" == \
      "$(grep -cv -E '^(JWT_SECRET_KEY|PLUGIN_SECRET_KEY|DB_PASSWORD)=' "${rot_dir}/.env")" ]]; then
      pass "rotate-ci-secrets.sh leaves every other line intact"
    else
      fail "rotate-ci-secrets.sh changed lines other than the rotated secrets"
    fi
  else
    fail "rotate-ci-secrets.sh exited non-zero"
  fi
  rm -rf "$rot_dir"
fi
for workflow in "${ROOT}/.github/workflows/"*.yml; do
  if grep -q 'cp .github/e2e/ci.env.fixture .env' "$workflow"; then
    if grep -q 'scripts/ci/rotate-ci-secrets.sh' "$workflow"; then
      pass "$(basename "$workflow") rotates committed CI secrets"
    else
      fail "$(basename "$workflow") boots ci.env.fixture without scripts/ci/rotate-ci-secrets.sh"
    fi
  fi
done

# wait-for-stack.sh content contract
if [[ -f "$WAIT_SH" ]]; then
  if ! grep -q 'health/live' "$WAIT_SH"; then
    fail "wait-for-stack.sh must poll /api/v1/health/live"
  else
    pass "wait-for-stack.sh polls health/live"
  fi
  if ! grep -q 'health/ready' "$WAIT_SH"; then
    fail "wait-for-stack.sh must poll /api/v1/health/ready"
  else
    pass "wait-for-stack.sh polls health/ready"
  fi
  if ! grep -qE 'localhost:3000|FRONTEND|frontend' "$WAIT_SH"; then
    fail "wait-for-stack.sh must poll the frontend root"
  else
    pass "wait-for-stack.sh polls frontend"
  fi
  if ! grep -qE 'pg_isready|psql' "$WAIT_SH"; then
    fail "wait-for-stack.sh must poll postgres (pg_isready or psql)"
  else
    pass "wait-for-stack.sh polls postgres"
  fi
  if ! grep -q 'docker compose logs' "$WAIT_SH" && ! grep -q 'docker-compose logs' "$WAIT_SH"; then
    fail "wait-for-stack.sh must dump compose logs on timeout"
  else
    pass "wait-for-stack.sh dumps logs on timeout"
  fi
fi

echo ""
if [[ "$failures" -gt 0 ]]; then
  echo "wait-for-stack_test: ${failures} failure(s)"
  exit 1
fi
echo "wait-for-stack_test: all assertions passed"
exit 0
