#!/bin/bash
set -euo pipefail

# Backup staleness monitor for Payverge production.
# Intended to run from host cron (see crontab line below). Reads the
# .last-local-success and .last-upload-success markers written by
# backup-db.sh out of the backup_data volume, verifies the remote S3
# object still exists, and alerts via the Telegram escalation bot when
# any check fails. All failures are aggregated into ONE alert.
#
# Requires: docker, curl, aws CLI (for remote head-object), GNU date
# (production host is Linux).
#
# Environment (usually sourced from the repo root .env):
#   TELEGRAM_ESCALATION_BOT_TOKEN  bot token for alerts (required to alert)
#   TELEGRAM_ESCALATION_CHAT_ID    chat to alert (required to alert)
#   BACKUP_VOLUME                  docker volume name (default: payverge_backup_data;
#                                  confirm with `docker volume ls | grep backup_data`)
#   MAX_LOCAL_AGE_SECONDS          local marker staleness threshold (default 28800 = 8h)
#   MAX_UPLOAD_AGE_SECONDS         upload marker staleness threshold (default 28800)
#   S3_BACKUP_ENDPOINT             optional (e.g. R2); passed to aws as --endpoint-url
#   NODE_EXPORTER_TEXTFILE_DIR     optional; when set, writes payverge_backup_check.prom
#
# Exit codes:
#   0 — all checks healthy
#   1 — at least one problem AND alert was delivered
#   2 — at least one problem AND alert delivery failed OR alerting not configured
#       (operator will NOT be notified)
#
# Crontab (as the deploy user; adjust the checkout path):
#   17 * * * * cd /path/to/payverge && set -a && . ./.env && set +a && ./backend/scripts/check-backup-age.sh >> /var/log/payverge-backup-check.log 2>&1

MAX_LOCAL_AGE_SECONDS="${MAX_LOCAL_AGE_SECONDS:-28800}"
MAX_UPLOAD_AGE_SECONDS="${MAX_UPLOAD_AGE_SECONDS:-28800}"
BACKUP_VOLUME="${BACKUP_VOLUME:-payverge_backup_data}"
TELEGRAM_ESCALATION_BOT_TOKEN="${TELEGRAM_ESCALATION_BOT_TOKEN:-}"
TELEGRAM_ESCALATION_CHAT_ID="${TELEGRAM_ESCALATION_CHAT_ID:-}"
S3_BACKUP_ENDPOINT="${S3_BACKUP_ENDPOINT:-}"
NODE_EXPORTER_TEXTFILE_DIR="${NODE_EXPORTER_TEXTFILE_DIR:-}"

# Future-skew allowance (seconds): reject timestamps more than this far ahead.
FUTURE_SKEW_SECONDS=300

# Check result flags (1 = ok, 0 = failed)
LOCAL_OK=0
UPLOAD_OK=0
REMOTE_OK=0
ALERT_DELIVERED=0

# Accumulated problem descriptions (newline-joined for the single alert).
PROBLEMS=""

add_problem() {
  local msg="$1"
  if [ -z "${PROBLEMS}" ]; then
    PROBLEMS="${msg}"
  else
    PROBLEMS="${PROBLEMS}
${msg}"
  fi
}

# Parse ISO-8601 UTC timestamp (e.g. 2026-04-21T12:00:00Z) to epoch seconds.
# Primary path: GNU date (production). Fallback: BSD date for local macOS runs.
parse_timestamp_epoch() {
  local ts="$1"
  local epoch=""
  epoch=$(date -d "${ts}" +%s 2>/dev/null || true)
  if [ -z "${epoch}" ]; then
    epoch=$(date -j -u -f "%Y-%m-%dT%H:%M:%SZ" "${ts}" +%s 2>/dev/null || true)
  fi
  echo "${epoch}"
}

alert() {
  local message="$1"
  echo "ALERT: ${message}"
  if [ -n "${TELEGRAM_ESCALATION_BOT_TOKEN}" ] && [ -n "${TELEGRAM_ESCALATION_CHAT_ID}" ]; then
    if curl -sS -m 15 -X POST \
      "https://api.telegram.org/bot${TELEGRAM_ESCALATION_BOT_TOKEN}/sendMessage" \
      --data-urlencode "chat_id=${TELEGRAM_ESCALATION_CHAT_ID}" \
      --data-urlencode "text=🚨 Payverge backup alert: ${message}" \
      > /dev/null; then
      ALERT_DELIVERED=1
    else
      echo "WARNING: failed to deliver Telegram alert"
      ALERT_DELIVERED=0
    fi
  else
    echo "WARNING: TELEGRAM_ESCALATION_BOT_TOKEN / TELEGRAM_ESCALATION_CHAT_ID not set — alert not delivered"
    ALERT_DELIVERED=0
  fi
}

read_marker() {
  local name="$1"
  docker run --rm -v "${BACKUP_VOLUME}:/backups:ro" alpine:3.20 cat "/backups/${name}" 2>/dev/null || true
}

# Validate a marker body for freshness. Sets globals via named result vars is awkward
# in bash — echoes "ok" or adds problems and returns 1.
# Usage: check_marker_freshness <label> <marker_body> <max_age_seconds>
# Prints epoch of timestamp on stdout when parseable; return 0 if fresh, 1 otherwise.
# Problems are always recorded via add_problem when failing.
check_marker_freshness() {
  local label="$1"
  local body="$2"
  local max_age="$3"
  local marker_file="$4"

  if [ -z "${body}" ]; then
    add_problem "${label}: no ${marker_file} marker found in volume ${BACKUP_VOLUME}"
    return 1
  fi

  local last_ts
  last_ts=$(echo "${body}" | sed -n 's/^timestamp=//p' | head -1)
  if [ -z "${last_ts}" ]; then
    add_problem "${label}: ${marker_file} is malformed (no timestamp= line)"
    return 1
  fi

  local last_epoch
  last_epoch=$(parse_timestamp_epoch "${last_ts}")
  if [ -z "${last_epoch}" ]; then
    add_problem "${label}: could not parse timestamp '${last_ts}' from ${marker_file}"
    return 1
  fi

  local now_epoch
  now_epoch=$(date -u +%s)
  local skew=$((last_epoch - now_epoch))
  if [ "${skew}" -gt "${FUTURE_SKEW_SECONDS}" ]; then
    add_problem "${label}: ${marker_file} timestamp is in the future (${last_ts}, ${skew}s ahead) — corrupt or clock skew"
    return 1
  fi

  local age=$((now_epoch - last_epoch))
  # Negative age (slightly ahead, within skew) is fine.
  if [ "${age}" -gt "${max_age}" ]; then
    add_problem "${label}: last success is ${age}s old (threshold ${max_age}s). Marker: ${last_ts}. Check: docker compose logs backup"
    return 1
  fi

  echo "OK ${label}: ${last_ts} (${age}s ago, threshold ${max_age}s)"
  return 0
}

parse_s3_object() {
  # stdin: marker body. stdout: "bucket key" or empty on failure.
  local object_uri
  object_uri=$(sed -n 's/^object=//p' | head -1)
  if [ -z "${object_uri}" ]; then
    return 1
  fi
  # Expect s3://bucket/key...
  case "${object_uri}" in
    s3://*/*)
      local rest="${object_uri#s3://}"
      local bucket="${rest%%/*}"
      local key="${rest#*/}"
      if [ -z "${bucket}" ] || [ -z "${key}" ] || [ "${bucket}" = "${rest}" ]; then
        return 1
      fi
      echo "${bucket} ${key}"
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

check_remote_object() {
  local upload_body="$1"

  if [ -z "${upload_body}" ]; then
    add_problem "remote: cannot verify object — .last-upload-success missing (no object= line)"
    return 1
  fi

  local parsed
  if ! parsed=$(echo "${upload_body}" | parse_s3_object); then
    add_problem "remote: .last-upload-success missing or malformed object=s3://bucket/key line"
    return 1
  fi

  local bucket key
  bucket=$(echo "${parsed}" | awk '{print $1}')
  key=$(echo "${parsed}" | awk '{print $2}')

  local aws_args=()
  if [ -n "${S3_BACKUP_ENDPOINT}" ]; then
    aws_args+=(--endpoint-url "${S3_BACKUP_ENDPOINT}")
  fi

  if ! command -v aws >/dev/null 2>&1; then
    add_problem "remote: aws CLI not available — cannot verify s3://${bucket}/${key}"
    return 1
  fi

  # ${aws_args[@]+...} keeps set -u happy when the array is empty (bash 3.2).
  if ! aws ${aws_args[@]+"${aws_args[@]}"} s3api head-object \
      --bucket "${bucket}" \
      --key "${key}" >/dev/null 2>&1; then
    add_problem "remote: backup object missing/inaccessible (s3://${bucket}/${key})"
    return 1
  fi

  echo "OK remote: s3://${bucket}/${key} exists"
  return 0
}

write_prometheus_metrics() {
  if [ -z "${NODE_EXPORTER_TEXTFILE_DIR}" ]; then
    return 0
  fi
  if [ ! -d "${NODE_EXPORTER_TEXTFILE_DIR}" ]; then
    echo "WARNING: NODE_EXPORTER_TEXTFILE_DIR=${NODE_EXPORTER_TEXTFILE_DIR} is not a directory — skipping metrics"
    return 0
  fi

  local now_epoch
  now_epoch=$(date -u +%s)
  local prom_file="${NODE_EXPORTER_TEXTFILE_DIR}/payverge_backup_check.prom"
  local tmp_file="${prom_file}.tmp.$$"

  cat > "${tmp_file}" <<EOF
# HELP payverge_backup_check_success Whether the last backup check passed (1) or failed (0) per check type
# TYPE payverge_backup_check_success gauge
payverge_backup_check_success{type="local"} ${LOCAL_OK}
payverge_backup_check_success{type="upload"} ${UPLOAD_OK}
payverge_backup_check_success{type="remote_object"} ${REMOTE_OK}
# HELP payverge_backup_check_last_run_timestamp Unix timestamp of the last monitor run
# TYPE payverge_backup_check_last_run_timestamp gauge
payverge_backup_check_last_run_timestamp ${now_epoch}
# HELP payverge_backup_check_alert_delivered 1 if no problems or Telegram alert was delivered; 0 if problems exist and alert was not delivered
# TYPE payverge_backup_check_alert_delivered gauge
payverge_backup_check_alert_delivered ${ALERT_DELIVERED}
EOF
  mv "${tmp_file}" "${prom_file}"
}

# --- main -------------------------------------------------------------------

LOCAL_MARKER=$(read_marker ".last-local-success")
UPLOAD_MARKER=$(read_marker ".last-upload-success")

if check_marker_freshness "local" "${LOCAL_MARKER}" "${MAX_LOCAL_AGE_SECONDS}" ".last-local-success"; then
  LOCAL_OK=1
fi

if check_marker_freshness "upload" "${UPLOAD_MARKER}" "${MAX_UPLOAD_AGE_SECONDS}" ".last-upload-success"; then
  UPLOAD_OK=1
fi

if check_remote_object "${UPLOAD_MARKER}"; then
  REMOTE_OK=1
fi

if [ -z "${PROBLEMS}" ]; then
  # Healthy: no alert needed; treat as "delivered" so a green scrape is not alarming.
  ALERT_DELIVERED=1
  write_prometheus_metrics
  echo "OK: local, upload, and remote object checks all passed (local threshold ${MAX_LOCAL_AGE_SECONDS}s, upload threshold ${MAX_UPLOAD_AGE_SECONDS}s)"
  exit 0
fi

# Aggregate every problem into a single Telegram POST.
AGG_MSG="backup check failures:
${PROBLEMS}
volume=${BACKUP_VOLUME}"
alert "${AGG_MSG}"

# If we have problems, ALERT_DELIVERED is set by alert(); metrics use that value.
write_prometheus_metrics

if [ "${ALERT_DELIVERED}" -eq 1 ]; then
  exit 1
fi
exit 2
