#!/usr/bin/env bash
# Poll the docker-compose E2E stack until Postgres, backend (live + ready),
# and frontend are up — or dump diagnostics and exit nonzero on timeout.
#
# Usage (from repo root, after `docker compose --env-file .env up -d --build`):
#   bash scripts/ci/wait-for-stack.sh
#
# Optional env overrides:
#   WAIT_POSTGRES_TIMEOUT   default 120
#   WAIT_BACKEND_LIVE_TIMEOUT default 180
#   WAIT_BACKEND_READY_TIMEOUT default 180
#   WAIT_FRONTEND_TIMEOUT   default 180
#   BACKEND_PORT            default 8080
#   FRONTEND_PORT           default 3000
#   COMPOSE_ENV_FILE        default .env
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

COMPOSE_ENV_FILE="${COMPOSE_ENV_FILE:-.env}"
BACKEND_PORT="${BACKEND_PORT:-8080}"
FRONTEND_PORT="${FRONTEND_PORT:-3000}"
WAIT_POSTGRES_TIMEOUT="${WAIT_POSTGRES_TIMEOUT:-120}"
WAIT_BACKEND_LIVE_TIMEOUT="${WAIT_BACKEND_LIVE_TIMEOUT:-180}"
WAIT_BACKEND_READY_TIMEOUT="${WAIT_BACKEND_READY_TIMEOUT:-180}"
WAIT_FRONTEND_TIMEOUT="${WAIT_FRONTEND_TIMEOUT:-180}"

BACKEND_LIVE_URL="http://localhost:${BACKEND_PORT}/api/v1/health/live"
BACKEND_READY_URL="http://localhost:${BACKEND_PORT}/api/v1/health/ready"
FRONTEND_URL="http://localhost:${FRONTEND_PORT}"

compose() {
  docker compose --env-file "$COMPOSE_ENV_FILE" "$@"
}

dump_diagnostics() {
  echo "" >&2
  echo "=== stack wait timed out — diagnostics ===" >&2
  # Use the literal `docker compose logs` form so operators can re-run the
  # same commands by hand; --env-file keeps CI env interpolation consistent.
  docker compose --env-file "$COMPOSE_ENV_FILE" ps >&2 || true
  echo "" >&2
  echo "--- backend logs (tail 200) ---" >&2
  docker compose --env-file "$COMPOSE_ENV_FILE" logs --tail=200 backend >&2 || true
  echo "" >&2
  echo "--- frontend logs (tail 200) ---" >&2
  docker compose --env-file "$COMPOSE_ENV_FILE" logs --tail=200 frontend >&2 || true
  echo "" >&2
  echo "--- postgres logs (tail 200) ---" >&2
  docker compose --env-file "$COMPOSE_ENV_FILE" logs --tail=200 postgres >&2 || true
  echo "=== end diagnostics ===" >&2
}

# Poll until COMMAND succeeds or TIMEOUT seconds elapse.
# $1 = label, $2 = timeout seconds, remaining args = command
wait_for() {
  local label="$1"
  local timeout="$2"
  shift 2
  local start now elapsed
  start="$(date +%s)"
  echo "Waiting for ${label} (timeout ${timeout}s)..."
  while true; do
    if "$@"; then
      echo "  ${label}: ok"
      return 0
    fi
    now="$(date +%s)"
    elapsed=$((now - start))
    if (( elapsed >= timeout )); then
      echo "ERROR: timed out waiting for ${label} after ${timeout}s" >&2
      dump_diagnostics
      return 1
    fi
    sleep 2
  done
}

postgres_ready() {
  compose exec -T postgres pg_isready -U payverge -d payverge >/dev/null 2>&1
}

backend_live() {
  curl -sf "$BACKEND_LIVE_URL" >/dev/null 2>&1
}

backend_ready() {
  curl -sf "$BACKEND_READY_URL" >/dev/null 2>&1
}

# Prefer a real 2xx/3xx from the frontend root (Next may redirect).
frontend_ready() {
  local code
  code="$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 "$FRONTEND_URL" 2>/dev/null || echo 000)"
  [[ "$code" =~ ^[23][0-9][0-9]$ ]]
}

wait_for "postgres (pg_isready)" "$WAIT_POSTGRES_TIMEOUT" postgres_ready
wait_for "backend health/live (${BACKEND_LIVE_URL})" "$WAIT_BACKEND_LIVE_TIMEOUT" backend_live
wait_for "backend health/ready (${BACKEND_READY_URL})" "$WAIT_BACKEND_READY_TIMEOUT" backend_ready
wait_for "frontend root (${FRONTEND_URL})" "$WAIT_FRONTEND_TIMEOUT" frontend_ready

echo "STACK READY: postgres + backend (live/ready) + frontend are up"
exit 0
