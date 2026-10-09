#!/usr/bin/env bash
set -euo pipefail
umask 077

# Restore a verified gzip dump into an explicitly named, loopback-only scratch
# database. This helper intentionally cannot target production. Production
# recovery uses the reviewed break-glass procedure after an isolated drill.

DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-5432}"
DB_USER="${DB_USER:-postgres}"
DB_PASSWORD="${DB_PASSWORD:-}"
DB_NAME="${DB_NAME:-payverge}"
BACKUP_DIR="${BACKUP_DIR:-./backups}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESTORE_SCOPE="${RESTORE_SCOPE:-}"
RESTORE_CONFIRM="${RESTORE_CONFIRM:-}"
MIN_TABLES="${MIN_TABLES:-1}"

export PGPASSWORD="${DB_PASSWORD}"

usage() {
  cat >&2 <<EOF
Usage: $0 <backup_file.sql.gz>

This destructive helper is restricted to an isolated loopback target.
Required environment:
  RESTORE_SCOPE=isolated
  DB_HOST=127.0.0.1|localhost|::1
  DB_NAME=payverge_restore_<name>
  RESTORE_CONFIRM="DROP <host>:<port>/<database>"

The gzip stream is validated before any database command runs.
EOF
  exit 64
}

die() {
  echo "ERROR: $*" >&2
  exit 1
}

[ "$#" -eq 1 ] || usage
BACKUP_FILE="$1"

if [ ! -f "${BACKUP_FILE}" ] && [ -f "${BACKUP_DIR}/${BACKUP_FILE}" ]; then
  BACKUP_FILE="${BACKUP_DIR}/${BACKUP_FILE}"
fi
[ -f "${BACKUP_FILE}" ] || die "backup file not found: ${BACKUP_FILE}"

# Refuse ambiguous, remote, production-like, or injection-shaped targets before
# invoking psql. There is deliberately no override in this generic helper.
[ "${RESTORE_SCOPE}" = "isolated" ] || die "refusing restore: RESTORE_SCOPE must be isolated"
case "${DB_HOST}" in
  localhost|127.0.0.1|::1) ;;
  *) die "refusing restore: isolated targets must use a loopback DB_HOST" ;;
esac
[[ "${DB_PORT}" =~ ^[0-9]+$ ]] || die "refusing restore: DB_PORT must be numeric"
[[ "${DB_NAME}" =~ ^payverge_restore_[A-Za-z0-9_]+$ ]] || \
  die "refusing restore: DB_NAME must begin payverge_restore_ and contain only letters, digits, underscores"

EXPECTED_CONFIRM="DROP ${DB_HOST}:${DB_PORT}/${DB_NAME}"
[ "${RESTORE_CONFIRM}" = "${EXPECTED_CONFIRM}" ] || \
  die "refusing restore: set RESTORE_CONFIRM exactly to '${EXPECTED_CONFIRM}'"

if ! gzip -t "${BACKUP_FILE}"; then
  die "gzip integrity check failed (corrupt/truncated backup): ${BACKUP_FILE}"
fi

echo "=== Payverge Isolated Database Restore ==="
echo "Target:       ${DB_HOST}:${DB_PORT}/${DB_NAME}"
echo "Restore from: ${BACKUP_FILE}"

echo "[$(date '+%H:%M:%S')] Dropping confirmed scratch database..."
psql --host="${DB_HOST}" --port="${DB_PORT}" --username="${DB_USER}" --dbname=postgres \
  --set ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS \"${DB_NAME}\";"

echo "[$(date '+%H:%M:%S')] Creating scratch database..."
psql --host="${DB_HOST}" --port="${DB_PORT}" --username="${DB_USER}" --dbname=postgres \
  --set ON_ERROR_STOP=1 -c "CREATE DATABASE \"${DB_NAME}\";"

echo "[$(date '+%H:%M:%S')] Restoring verified backup..."
gunzip -c "${BACKUP_FILE}" | psql \
  --host="${DB_HOST}" --port="${DB_PORT}" --username="${DB_USER}" --dbname="${DB_NAME}" \
  --set ON_ERROR_STOP=1

# This helper is drill-only, so restored production identities/secrets must not
# remain in the scratch database even briefly beyond the restore operation.
SANITIZE_SQL="${SCRIPT_DIR}/sanitize-clone.sql"
[ -f "${SANITIZE_SQL}" ] || die "sanitize script missing: ${SANITIZE_SQL}"
echo "[$(date '+%H:%M:%S')] Sanitizing scratch clone..."
psql --host="${DB_HOST}" --port="${DB_PORT}" --username="${DB_USER}" \
  --dbname="${DB_NAME}" --set ON_ERROR_STOP=1 -f "${SANITIZE_SQL}"

TABLE_COUNT="$(psql --host="${DB_HOST}" --port="${DB_PORT}" --username="${DB_USER}" \
  --dbname="${DB_NAME}" --set ON_ERROR_STOP=1 -t -A \
  -c "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public';")"
TABLE_COUNT="$(printf '%s' "${TABLE_COUNT}" | tr -d '[:space:]')"
if ! [[ "${TABLE_COUNT}" =~ ^[0-9]+$ ]] || [ "${TABLE_COUNT}" -lt "${MIN_TABLES}" ]; then
  die "restore invariant failed: public table count '${TABLE_COUNT:-empty}' < MIN_TABLES=${MIN_TABLES}"
fi

echo "[$(date '+%H:%M:%S')] Verified: ${TABLE_COUNT} public tables restored."
echo "=== Isolated Restore Complete ==="
