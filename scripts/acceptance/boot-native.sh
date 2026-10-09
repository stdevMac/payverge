#!/usr/bin/env bash
# Run a natively built backend in production mode with zero third-party
# configuration: only generated secrets, PUBLIC_URL, the bootstrap admin and a
# PostgreSQL connection. Used by the CI "boot-zero-accounts" job and handy
# locally against any empty PostgreSQL 18 database.
#
#   scripts/acceptance/boot-native.sh start|stop|restart
#
# Environment:
#   ACCEPTANCE_STATE  directory for secrets, pid and log (required)
#   PAYVERGE_BIN      backend binary (default: backend/bin/app)
#   PORT              listen port (default 8080)
#   DB_HOST DB_PORT DB_USER DB_PASSWORD DB_NAME
#                     PostgreSQL (defaults 127.0.0.1 5432 payverge - payverge;
#                     DB_PASSWORD is required: preflight rejects known defaults)
#
# The first start writes $ACCEPTANCE_STATE/boot.env (mode 600) with
# JWT_SECRET_KEY, PLUGIN_SECRET_KEY, ADMIN_EMAIL and ADMIN_PASSWORD; later
# starts reuse it, so a restart keeps sessions and encrypted data readable.
# The backend log is $ACCEPTANCE_STATE/backend.log.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
STATE="${ACCEPTANCE_STATE:?set ACCEPTANCE_STATE to a writable directory}"
BIN="${PAYVERGE_BIN:-$ROOT/backend/bin/app}"
PORT="${PORT:-8080}"
DB_HOST="${DB_HOST:-127.0.0.1}"
DB_PORT="${DB_PORT:-5432}"
DB_USER="${DB_USER:-payverge}"
DB_PASSWORD="${DB_PASSWORD:?set DB_PASSWORD (production preflight rejects development defaults)}"
DB_NAME="${DB_NAME:-payverge}"
PIDFILE="$STATE/backend.pid"

mkdir -p "$STATE/storage"

secrets() {
  [[ -f "$STATE/boot.env" ]] && return
  (
    umask 077
    {
      echo "JWT_SECRET_KEY=$(openssl rand -base64 48 | tr -d '\n')"
      echo "PLUGIN_SECRET_KEY=$(openssl rand -hex 32)"
      echo "ADMIN_EMAIL=admin@acceptance.test"
      echo "ADMIN_PASSWORD=$(openssl rand -hex 16)"
    } >"$STATE/boot.env"
  )
}

start() {
  [[ -x "$BIN" ]] || { echo "backend binary not found: $BIN (go build -o $BIN ./cmd/app)" >&2; exit 1; }
  secrets
  # shellcheck disable=SC1091
  source "$STATE/boot.env"
  # env -i: nothing from the calling shell (provider keys, URLs, modes)
  # reaches the backend. The working directory is backend/ because the
  # email templates are read relative to it, as in the image.
  (
    cd "$ROOT/backend"
    exec env -i PATH="$PATH" HOME="${HOME:-/tmp}" \
      APP_ENV=production PORT="$PORT" PUBLIC_URL=https://localhost \
      DB_HOST="$DB_HOST" DB_PORT="$DB_PORT" DB_USER="$DB_USER" DB_PASSWORD="$DB_PASSWORD" \
      DB_NAME="$DB_NAME" DB_SSLMODE=disable \
      JWT_SECRET_KEY="$JWT_SECRET_KEY" PLUGIN_SECRET_KEY="$PLUGIN_SECRET_KEY" \
      ADMIN_EMAIL="$ADMIN_EMAIL" ADMIN_PASSWORD="$ADMIN_PASSWORD" \
      STORAGE_DRIVER=local STORAGE_DIR="$STATE/storage" \
      "$BIN" --production --db-host "$DB_HOST" --db-port "$DB_PORT" --db-user "$DB_USER" \
      --db-name "$DB_NAME" --db-sslmode disable
  ) >>"$STATE/backend.log" 2>&1 </dev/null &
  echo $! >"$PIDFILE"
}

stop() {
  [[ -f "$PIDFILE" ]] || return 0
  local pid
  pid="$(cat "$PIDFILE")"
  kill "$pid" 2>/dev/null || true
  for _ in $(seq 1 30); do
    kill -0 "$pid" 2>/dev/null || break
    sleep 1
  done
  kill -9 "$pid" 2>/dev/null || true
  rm -f "$PIDFILE"
}

case "${1:-}" in
  start) start ;;
  stop) stop ;;
  restart) stop; start ;;
  *) echo "usage: $0 start|stop|restart" >&2; exit 2 ;;
esac
