#!/bin/bash
set -euo pipefail
umask 077

# Database backup script for Payverge
# Creates timestamped gzipped pg_dump backups with:
#   - atomic tmp-then-rename writes (a truncated dump can never be mistaken
#     for a complete one)
#   - gzip integrity verification before publishing
#   - three-marker policy for local / upload / policy success
#   - optional or required off-host S3 upload (S3_BACKUP_BUCKET;
#     REQUIRE_OFFSITE_BACKUP=true makes offsite a precondition of policy
#     success; supports R2 via S3_BACKUP_ENDPOINT)
#   - local retention cleanup
#
# Requires bash (process substitution below). In the production compose
# `backup` service this runs inside the payverge-backup image (alpine +
# bash + postgresql18-client + aws-cli).

DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-5432}"
DB_USER="${DB_USER:-postgres}"
DB_PASSWORD="${DB_PASSWORD:-}"
DB_NAME="${DB_NAME:-payverge}"
BACKUP_DIR="${BACKUP_DIR:-./backups}"
RETENTION_DAYS="${RETENTION_DAYS:-7}"
S3_BACKUP_BUCKET="${S3_BACKUP_BUCKET:-}"
S3_BACKUP_PREFIX="${S3_BACKUP_PREFIX:-db}"
S3_BACKUP_ENDPOINT="${S3_BACKUP_ENDPOINT:-}"
REQUIRE_OFFSITE_BACKUP="${REQUIRE_OFFSITE_BACKUP:-false}"

TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
BACKUP_FILE="${BACKUP_DIR}/${DB_NAME}_${TIMESTAMP}.sql.gz"
TMP_FILE="${BACKUP_FILE}.tmp"
# Three-marker contract:
#   .last-local-success  — local dump verified + published
#   .last-upload-success — S3 upload (and optional head-object) succeeded
#   .last-success        — policy success (monitor consumes this)
LOCAL_MARKER_FILE="${BACKUP_DIR}/.last-local-success"
UPLOAD_MARKER_FILE="${BACKUP_DIR}/.last-upload-success"
POLICY_MARKER_FILE="${BACKUP_DIR}/.last-success"

# Export password for pg_dump
export PGPASSWORD="${DB_PASSWORD}"

# Never leave a partial .tmp behind, whatever the exit path.
cleanup() {
  rm -f "${TMP_FILE}"
}
trap cleanup EXIT

# Atomically write marker content (render to temp, then mv into place).
write_marker() {
  local dest="$1"
  shift
  local marker_tmp="${dest}.tmp.$$"
  # Content is passed as remaining args joined by the printf format in caller
  # via stdin to keep multi-line payloads simple:
  cat > "${marker_tmp}"
  mv "${marker_tmp}" "${dest}"
}

build_aws_args() {
  AWS_ARGS=()
  if [ -n "${S3_BACKUP_ENDPOINT}" ]; then
    AWS_ARGS+=(--endpoint-url "${S3_BACKUP_ENDPOINT}")
  fi
}

echo "=== Payverge Database Backup ==="
echo "Host:      ${DB_HOST}:${DB_PORT}"
echo "Database:  ${DB_NAME}"
echo "Backup to: ${BACKUP_FILE}"
echo "Retention: ${RETENTION_DAYS} days"
echo "Offsite:   REQUIRE_OFFSITE_BACKUP=${REQUIRE_OFFSITE_BACKUP}"
echo ""

# Ensure backup directory exists
mkdir -p "${BACKUP_DIR}"

# When offsite is required, fail fast BEFORE dumping so a host-disk-only
# dump is never reported as durable and no markers are written.
if [ "${REQUIRE_OFFSITE_BACKUP}" = "true" ]; then
  if [ -z "${S3_BACKUP_BUCKET}" ]; then
    echo "ERROR: REQUIRE_OFFSITE_BACKUP=true but S3_BACKUP_BUCKET is empty."
    exit 1
  fi
  if ! command -v aws >/dev/null 2>&1; then
    echo "ERROR: REQUIRE_OFFSITE_BACKUP=true but the aws CLI is not installed in this image."
    exit 1
  fi
  build_aws_args
  echo "[$(date '+%H:%M:%S')] Verifying offsite bucket s3://${S3_BACKUP_BUCKET}..."
  # ${AWS_ARGS[@]+...} keeps `set -u` happy on bash 3.2 when the array is empty.
  if ! aws ${AWS_ARGS[@]+"${AWS_ARGS[@]}"} s3api head-bucket --bucket "${S3_BACKUP_BUCKET}"; then
    echo "ERROR: head-bucket failed for s3://${S3_BACKUP_BUCKET}. Offsite is not usable."
    exit 1
  fi
fi

echo "[$(date '+%H:%M:%S')] Starting backup..."

# Dump to a .tmp file first; only a verified, complete dump is renamed to
# the final name. pipefail makes a pg_dump failure abort the script even
# though gzip is the last command in the pipe.
pg_dump \
  --host="${DB_HOST}" \
  --port="${DB_PORT}" \
  --username="${DB_USER}" \
  --dbname="${DB_NAME}" \
  --no-owner \
  --no-privileges \
  | gzip > "${TMP_FILE}"

# Verify backup integrity — empty or tiny backups indicate a problem
BACKUP_SIZE_BYTES=$(stat -f%z "${TMP_FILE}" 2>/dev/null || stat -c%s "${TMP_FILE}" 2>/dev/null)
if [ "${BACKUP_SIZE_BYTES}" -lt 100 ]; then
  echo "ERROR: Backup file is suspiciously small (${BACKUP_SIZE_BYTES} bytes). pg_dump may have failed."
  exit 1
fi

# Verify the gzip stream is complete and uncorrupted before publishing.
if ! gzip -t "${TMP_FILE}"; then
  echo "ERROR: Backup gzip integrity check failed for ${TMP_FILE}."
  exit 1
fi

# Atomic publish: rename within the same filesystem.
mv "${TMP_FILE}" "${BACKUP_FILE}"

BACKUP_SIZE=$(ls -lh "${BACKUP_FILE}" | awk '{print $5}')
echo "[$(date '+%H:%M:%S')] Backup complete: ${BACKUP_FILE} (${BACKUP_SIZE})"

# Local success marker — dump verified and published.
printf 'timestamp=%s\nfile=%s\nsize_bytes=%s\n' \
  "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  "${BACKUP_FILE}" \
  "${BACKUP_SIZE_BYTES}" | write_marker "${LOCAL_MARKER_FILE}"

# Off-host upload.
# - REQUIRE_OFFSITE_BACKUP=true: already validated bucket/cli/head-bucket above.
# - bucket set (optional mode): attempt upload; failure exits nonzero and does
#   NOT write policy .last-success (local marker already written).
# - bucket unset + optional: local success IS policy success; never invoke aws.
if [ -n "${S3_BACKUP_BUCKET}" ]; then
  if ! command -v aws >/dev/null 2>&1; then
    echo "ERROR: S3_BACKUP_BUCKET is set but the aws CLI is not installed in this image."
    exit 1
  fi
  build_aws_args
  S3_OBJECT_KEY="${S3_BACKUP_PREFIX}/$(basename "${BACKUP_FILE}")"
  S3_TARGET="s3://${S3_BACKUP_BUCKET}/${S3_OBJECT_KEY}"
  echo "[$(date '+%H:%M:%S')] Uploading to ${S3_TARGET}..."
  # ${AWS_ARGS[@]+...} keeps `set -u` happy on bash 3.2 when the array is empty.
  aws ${AWS_ARGS[@]+"${AWS_ARGS[@]}"} s3 cp "${BACKUP_FILE}" "${S3_TARGET}" --only-show-errors

  # Optional existence check — confirm the object is durable off-host.
  if ! aws ${AWS_ARGS[@]+"${AWS_ARGS[@]}"} s3api head-object \
      --bucket "${S3_BACKUP_BUCKET}" \
      --key "${S3_OBJECT_KEY}" >/dev/null; then
    echo "ERROR: head-object failed for ${S3_TARGET}. Upload not confirmed."
    exit 1
  fi

  printf 'timestamp=%s\nobject=%s\n' \
    "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    "${S3_TARGET}" | write_marker "${UPLOAD_MARKER_FILE}"
  echo "[$(date '+%H:%M:%S')] Upload complete."

  # Policy success only after verified offsite upload when a bucket is configured.
  printf 'timestamp=%s\nfile=%s\nsize_bytes=%s\nobject=%s\n' \
    "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    "${BACKUP_FILE}" \
    "${BACKUP_SIZE_BYTES}" \
    "${S3_TARGET}" | write_marker "${POLICY_MARKER_FILE}"
else
  echo "[$(date '+%H:%M:%S')] S3_BACKUP_BUCKET not set — skipping off-host upload."
  # Local-only mode: local success is policy success.
  printf 'timestamp=%s\nfile=%s\nsize_bytes=%s\n' \
    "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    "${BACKUP_FILE}" \
    "${BACKUP_SIZE_BYTES}" | write_marker "${POLICY_MARKER_FILE}"
fi

# Clean up old backups
echo "[$(date '+%H:%M:%S')] Removing backups older than ${RETENTION_DAYS} days..."
DELETED_COUNT=0
while IFS= read -r old_file; do
  echo "  Deleting: $(basename "${old_file}")"
  rm -f "${old_file}"
  DELETED_COUNT=$((DELETED_COUNT + 1))
done < <(find "${BACKUP_DIR}" -name "${DB_NAME}_*.sql.gz" -type f -mtime +"${RETENTION_DAYS}" 2>/dev/null)

echo "[$(date '+%H:%M:%S')] Cleaned up ${DELETED_COUNT} old backup(s)."
echo ""
echo "=== Backup Complete ==="
