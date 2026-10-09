#!/usr/bin/env bash
set -euo pipefail
umask 077

# Check the newest backup directly in object storage. This script has no Docker,
# volume, or production-host dependency and is intended for an external runner.

S3_BACKUP_BUCKET="${S3_BACKUP_BUCKET:-}"
S3_BACKUP_PREFIX="${S3_BACKUP_PREFIX:-db}"
S3_BACKUP_ENDPOINT="${S3_BACKUP_ENDPOINT:-}"
MAX_OFFSITE_BACKUP_AGE_SECONDS="${MAX_OFFSITE_BACKUP_AGE_SECONDS:-14400}"
NOW_EPOCH="${NOW_EPOCH:-$(date -u +%s)}"
EVIDENCE_FILE="${EVIDENCE_FILE:-backup-freshness.json}"
METRICS_FILE="${METRICS_FILE:-backup-freshness.prom}"

OBJECT_KEY=""
OBJECT_TIMESTAMP=""
OBJECT_EPOCH=""
OBJECT_AGE_SECONDS=""
OBJECT_SIZE_BYTES=""
OBJECT_ETAG=""

die_usage() {
  echo "ERROR: $*" >&2
  exit 64
}

[ -n "${S3_BACKUP_BUCKET}" ] || die_usage "S3_BACKUP_BUCKET is required"
[[ "${MAX_OFFSITE_BACKUP_AGE_SECONDS}" =~ ^[0-9]+$ ]] || die_usage "MAX_OFFSITE_BACKUP_AGE_SECONDS must be numeric"
[[ "${NOW_EPOCH}" =~ ^[0-9]+$ ]] || die_usage "NOW_EPOCH must be numeric"
command -v aws >/dev/null 2>&1 || die_usage "aws CLI is required"
command -v python3 >/dev/null 2>&1 || die_usage "python3 is required"

mkdir -p "$(dirname "${EVIDENCE_FILE}")" "$(dirname "${METRICS_FILE}")"

write_outputs() {
  local result="$1" reason="$2" fresh="$3"
  local metrics_tmp="${METRICS_FILE}.tmp.$$"
  cat >"${metrics_tmp}" <<EOF
# HELP payverge_offsite_backup_fresh Whether the newest off-site backup exists, is readable, and is within the freshness threshold.
# TYPE payverge_offsite_backup_fresh gauge
payverge_offsite_backup_fresh ${fresh}
# HELP payverge_offsite_backup_age_seconds Age of the newest off-site backup in seconds, or -1 when unavailable.
# TYPE payverge_offsite_backup_age_seconds gauge
payverge_offsite_backup_age_seconds ${OBJECT_AGE_SECONDS:--1}
# HELP payverge_offsite_backup_check_timestamp Unix timestamp of this external check.
# TYPE payverge_offsite_backup_check_timestamp gauge
payverge_offsite_backup_check_timestamp ${NOW_EPOCH}
EOF
  mv "${metrics_tmp}" "${METRICS_FILE}"

  python3 - "${EVIDENCE_FILE}" "${result}" "${reason}" \
    "${S3_BACKUP_BUCKET}" "${OBJECT_KEY}" "${OBJECT_TIMESTAMP}" \
    "${OBJECT_AGE_SECONDS}" "${OBJECT_SIZE_BYTES}" "${OBJECT_ETAG}" \
    "${MAX_OFFSITE_BACKUP_AGE_SECONDS}" "${NOW_EPOCH}" <<'PY'
import json
import sys
from datetime import datetime, timezone
from pathlib import Path

(path, result, reason, bucket, key, timestamp, age, size, etag,
 threshold, checked) = sys.argv[1:]
payload = {
    "result": result,
    "reason": reason,
    "bucket": bucket,
    "object_key": key or None,
    "object_timestamp": timestamp or None,
    "object_age_seconds": int(age) if age else None,
    "object_size_bytes": int(size) if size.isdigit() else None,
    "object_etag": etag or None,
    "max_age_seconds": int(threshold),
    "checked_at_epoch": int(checked),
    "checked_at_utc": datetime.fromtimestamp(int(checked), timezone.utc).isoformat().replace("+00:00", "Z"),
}
target = Path(path)
tmp = target.with_name(target.name + ".tmp")
tmp.write_text(json.dumps(payload, indent=2, sort_keys=True) + "\n", encoding="utf-8")
tmp.replace(target)
PY
}

fail_check() {
  local reason="$1"
  write_outputs fail "${reason}" 0
  echo "ERROR: ${reason}" >&2
  exit 1
}

AWS_ARGS=()
if [ -n "${S3_BACKUP_ENDPOINT}" ]; then
  AWS_ARGS+=(--endpoint-url "${S3_BACKUP_ENDPOINT}")
fi

if ! LISTING="$(aws ${AWS_ARGS[@]+"${AWS_ARGS[@]}"} s3api list-objects-v2 \
  --bucket "${S3_BACKUP_BUCKET}" \
  --prefix "${S3_BACKUP_PREFIX%/}/" \
  --query 'sort_by(Contents,&LastModified)[-1].[Key,LastModified,Size,ETag]' \
  --output text)"; then
  fail_check "could not list off-site backup objects"
fi

if [ -z "${LISTING}" ] || [ "${LISTING}" = "None" ]; then
  fail_check "no off-site backup object exists under ${S3_BACKUP_PREFIX%/}/"
fi

IFS=$'\t' read -r OBJECT_KEY OBJECT_TIMESTAMP OBJECT_SIZE_BYTES OBJECT_ETAG <<<"${LISTING}"
if [ -z "${OBJECT_KEY}" ] || [ -z "${OBJECT_TIMESTAMP}" ] || [ "${OBJECT_KEY}" = "None" ]; then
  fail_check "could not parse newest off-site backup identity"
fi

if ! OBJECT_EPOCH="$(python3 - "${OBJECT_TIMESTAMP}" <<'PY'
import sys
from datetime import datetime
value = sys.argv[1].strip().replace("Z", "+00:00")
print(int(datetime.fromisoformat(value).timestamp()))
PY
)"; then
  fail_check "could not parse off-site backup timestamp '${OBJECT_TIMESTAMP}'"
fi

OBJECT_AGE_SECONDS=$((NOW_EPOCH - OBJECT_EPOCH))
if [ "${OBJECT_AGE_SECONDS}" -lt 0 ]; then
  fail_check "off-site backup timestamp is in the future"
fi

if ! aws ${AWS_ARGS[@]+"${AWS_ARGS[@]}"} s3api head-object \
  --bucket "${S3_BACKUP_BUCKET}" --key "${OBJECT_KEY}" >/dev/null; then
  fail_check "newest off-site backup is not readable: ${OBJECT_KEY}"
fi

if [ "${OBJECT_AGE_SECONDS}" -gt "${MAX_OFFSITE_BACKUP_AGE_SECONDS}" ]; then
  fail_check "off-site backup is stale: age=${OBJECT_AGE_SECONDS}s threshold=${MAX_OFFSITE_BACKUP_AGE_SECONDS}s object=${OBJECT_KEY}"
fi

write_outputs pass "fresh off-site backup verified" 1
echo "OK: off-site backup ${OBJECT_KEY} age=${OBJECT_AGE_SECONDS}s threshold=${MAX_OFFSITE_BACKUP_AGE_SECONDS}s"
