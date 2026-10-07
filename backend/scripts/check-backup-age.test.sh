#!/bin/bash
set -uo pipefail

# Test harness for backend/scripts/check-backup-age.sh.
# Stubs docker, aws, and curl on a temp PATH. No real Docker/network.
# Works on macOS bash 3.2 and Linux.
#
# Usage: bash backend/scripts/check-backup-age.test.sh
# Exit 0 + "ALL TESTS PASSED" on success; exit 1 on any failure.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CHECK_SCRIPT="${SCRIPT_DIR}/check-backup-age.sh"

FAILURES=0

assert() {
  local desc="$1"
  local cond="$2" # "0" means pass
  if [ "${cond}" = "0" ]; then
    echo "  ok    - ${desc}"
  else
    echo "  FAIL  - ${desc}"
    FAILURES=$((FAILURES + 1))
  fi
}

# ISO-8601 UTC for an epoch (portable: GNU date first, then BSD).
iso_from_epoch() {
  local epoch="$1"
  date -u -d "@${epoch}" +%Y-%m-%dT%H:%M:%SZ 2>/dev/null \
    || date -u -r "${epoch}" +%Y-%m-%dT%H:%M:%SZ 2>/dev/null
}

NOW_EPOCH=$(date -u +%s)
FRESH_TS=$(iso_from_epoch "${NOW_EPOCH}")
# Default thresholds are 28800; stale = now - threshold - 3600
STALE_EPOCH=$((NOW_EPOCH - 28800 - 3600))
STALE_TS=$(iso_from_epoch "${STALE_EPOCH}")
# Future > 300s skew
FUTURE_EPOCH=$((NOW_EPOCH + 600))
FUTURE_TS=$(iso_from_epoch "${FUTURE_EPOCH}")

make_sandbox() {
  SANDBOX="$(mktemp -d "${TMPDIR:-/tmp}/check-backup-age.XXXXXX")"
  MOCK_BIN="${SANDBOX}/bin"
  FIXTURES="${SANDBOX}/fixtures"
  TEXTFILE_DIR="${SANDBOX}/textfile"
  CURL_LOG="${SANDBOX}/curl-invocations.log"
  CURL_COUNT_FILE="${SANDBOX}/curl-count"
  AWS_LOG="${SANDBOX}/aws-invocations.log"
  DOCKER_LOG="${SANDBOX}/docker-invocations.log"
  mkdir -p "${MOCK_BIN}" "${FIXTURES}" "${TEXTFILE_DIR}"
  echo 0 > "${CURL_COUNT_FILE}"

  # Default fixture content (overwritten per test)
  printf 'timestamp=%s\nfile=/backups/payverge_test.sql.gz\nsize_bytes=1234\n' "${FRESH_TS}" \
    > "${FIXTURES}/.last-local-success"
  printf 'timestamp=%s\nobject=s3://example-db-backups/db/payverge_test.sql.gz\n' "${FRESH_TS}" \
    > "${FIXTURES}/.last-upload-success"

  # Mock docker: cat fixture for the marker path in the command line.
  # Empty file content (or missing fixture) yields empty stdout.
  cat > "${MOCK_BIN}/docker" <<EOF
#!/bin/bash
echo "\$@" >> "${DOCKER_LOG}"
# Find /backups/<marker> in argv
marker=""
for arg in "\$@"; do
  case "\$arg" in
    /backups/*)
      marker="\${arg#/backups/}"
      ;;
  esac
done
if [ -z "\${marker}" ]; then
  exit 0
fi
fixture="${FIXTURES}/\${marker}"
if [ -f "\${fixture}" ]; then
  cat "\${fixture}"
  exit 0
fi
# Missing fixture → empty output (marker absent)
exit 0
EOF
  chmod +x "${MOCK_BIN}/docker"

  # Mock aws: head-object success/fail via MOCK_AWS_HEAD_OBJECT_FAIL
  cat > "${MOCK_BIN}/aws" <<EOF
#!/bin/bash
echo "\$@" >> "${AWS_LOG}"
if echo "\$*" | grep -q 's3api head-object'; then
  if [ "\${MOCK_AWS_HEAD_OBJECT_FAIL:-0}" = "1" ]; then
    echo "head-object failed (mock)" >&2
    exit 1
  fi
  exit 0
fi
exit 0
EOF
  chmod +x "${MOCK_BIN}/aws"

  # Mock curl: one counter bump per invocation (message body may contain
  # newlines, so we must not count log lines). Record a single-line summary.
  cat > "${MOCK_BIN}/curl" <<EOF
#!/bin/bash
# Count invocations (not lines in argv — alert text is multi-line)
count=\$(cat "${CURL_COUNT_FILE}" 2>/dev/null || echo 0)
echo \$((count + 1)) > "${CURL_COUNT_FILE}"
# Flatten argv to one log line for inspection (tr newlines → spaces)
printf '%s\n' "\$*" | tr '\n' ' ' >> "${CURL_LOG}"
echo >> "${CURL_LOG}"
if [ "\${MOCK_CURL_FAIL:-0}" = "1" ]; then
  echo "curl failed (mock)" >&2
  exit 1
fi
exit 0
EOF
  chmod +x "${MOCK_BIN}/curl"
}

run_check() {
  # Args: extra env assignments
  : > "${CURL_LOG}" 2>/dev/null || true
  : > "${AWS_LOG}" 2>/dev/null || true
  : > "${DOCKER_LOG}" 2>/dev/null || true
  echo 0 > "${CURL_COUNT_FILE}"
  env PATH="${MOCK_BIN}:${PATH}" \
    BACKUP_VOLUME=test_backup_data \
    TELEGRAM_ESCALATION_BOT_TOKEN=test-token \
    TELEGRAM_ESCALATION_CHAT_ID=test-chat \
    "$@" \
    bash "${CHECK_SCRIPT}" > "${SANDBOX}/stdout.log" 2> "${SANDBOX}/stderr.log"
}

# Count Telegram POSTs (one curl invocation = one alert attempt)
curl_post_count() {
  if [ ! -f "${CURL_COUNT_FILE}" ]; then
    echo 0
    return
  fi
  cat "${CURL_COUNT_FILE}" | tr -d ' \n'
}

alert_body() {
  # Reconstruct roughly: curl log has --data-urlencode text=...
  if [ ! -f "${CURL_LOG}" ]; then
    echo ""
    return
  fi
  # The text is url-encoded in the log as separate args; dump full log for grep
  cat "${CURL_LOG}" 2>/dev/null
  # Also include script stdout (ALERT: lines)
  cat "${SANDBOX}/stdout.log" 2>/dev/null
  cat "${SANDBOX}/stderr.log" 2>/dev/null
}

# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------

echo "=== Test 1: both markers fresh + remote object present → exit 0, no alert ==="
make_sandbox
run_check
RC=$?
assert "exits 0" "$([ ${RC} -eq 0 ] && echo 0 || echo 1)"
assert "no curl (no alert)" "$([ "$(curl_post_count)" = "0" ] && echo 0 || echo 1)"
assert "stdout mentions OK" "$(grep -q 'OK:' "${SANDBOX}/stdout.log" 2>/dev/null && echo 0 || echo 1)"
assert "aws head-object attempted" "$(grep -q 's3api head-object' "${AWS_LOG}" 2>/dev/null && echo 0 || echo 1)"
rm -rf "${SANDBOX}"

echo "=== Test 2: local fresh, upload STALE → exit 1, alert mentions upload ==="
make_sandbox
printf 'timestamp=%s\nobject=s3://example-db-backups/db/payverge_test.sql.gz\n' "${STALE_TS}" \
  > "${FIXTURES}/.last-upload-success"
run_check
RC=$?
assert "exits 1" "$([ ${RC} -eq 1 ] && echo 0 || echo 1)"
assert "exactly one curl POST" "$([ "$(curl_post_count)" = "1" ] && echo 0 || echo 1)"
BODY=$(alert_body)
assert "alert mentions upload staleness" "$(echo "${BODY}" | grep -qi 'upload' && echo 0 || echo 1)"
assert "alert mentions age/threshold" "$(echo "${BODY}" | grep -qE 'old|threshold|stale' && echo 0 || echo 1)"
rm -rf "${SANDBOX}"

echo "=== Test 3: upload fresh, local STALE → exit 1, alert mentions local ==="
make_sandbox
printf 'timestamp=%s\nfile=/backups/payverge_test.sql.gz\nsize_bytes=1234\n' "${STALE_TS}" \
  > "${FIXTURES}/.last-local-success"
run_check
RC=$?
assert "exits 1" "$([ ${RC} -eq 1 ] && echo 0 || echo 1)"
assert "exactly one curl POST" "$([ "$(curl_post_count)" = "1" ] && echo 0 || echo 1)"
BODY=$(alert_body)
assert "alert mentions local staleness" "$(echo "${BODY}" | grep -qi 'local' && echo 0 || echo 1)"
rm -rf "${SANDBOX}"

echo "=== Test 4: missing .last-local-success (empty docker output) → problem ==="
make_sandbox
rm -f "${FIXTURES}/.last-local-success"
run_check
RC=$?
assert "exits 1" "$([ ${RC} -eq 1 ] && echo 0 || echo 1)"
BODY=$(alert_body)
assert "alert mentions missing local marker" "$(echo "${BODY}" | grep -qiE 'local|last-local-success' && echo 0 || echo 1)"
assert "alert mentions no/missing marker" "$(echo "${BODY}" | grep -qiE 'no |missing|not found' && echo 0 || echo 1)"
rm -rf "${SANDBOX}"

echo "=== Test 5: malformed timestamp (no timestamp= line) → problem ==="
make_sandbox
printf 'file=/backups/x.sql.gz\nsize_bytes=1\n' > "${FIXTURES}/.last-local-success"
run_check
RC=$?
assert "exits 1" "$([ ${RC} -eq 1 ] && echo 0 || echo 1)"
BODY=$(alert_body)
assert "alert mentions malformed / no timestamp" "$(echo "${BODY}" | grep -qiE 'malformed|timestamp' && echo 0 || echo 1)"
rm -rf "${SANDBOX}"

echo "=== Test 6: future timestamp (> now+300s) → problem ==="
make_sandbox
printf 'timestamp=%s\nfile=/backups/payverge_test.sql.gz\nsize_bytes=1234\n' "${FUTURE_TS}" \
  > "${FIXTURES}/.last-local-success"
run_check
RC=$?
assert "exits 1" "$([ ${RC} -eq 1 ] && echo 0 || echo 1)"
BODY=$(alert_body)
assert "alert mentions future timestamp" "$(echo "${BODY}" | grep -qi 'future' && echo 0 || echo 1)"
rm -rf "${SANDBOX}"

echo "=== Test 7: remote object missing (head-object fails) → exit 1 ==="
make_sandbox
run_check MOCK_AWS_HEAD_OBJECT_FAIL=1
RC=$?
assert "exits 1" "$([ ${RC} -eq 1 ] && echo 0 || echo 1)"
assert "exactly one curl POST" "$([ "$(curl_post_count)" = "1" ] && echo 0 || echo 1)"
BODY=$(alert_body)
assert "alert mentions remote object missing/inaccessible" "$(echo "${BODY}" | grep -qiE 'remote|missing/inaccessible|inaccessible' && echo 0 || echo 1)"
assert "head-object was attempted" "$(grep -q 's3api head-object' "${AWS_LOG}" 2>/dev/null && echo 0 || echo 1)"
rm -rf "${SANDBOX}"

echo "=== Test 8: wrong/empty volume (docker returns nothing for both) → problem ==="
make_sandbox
rm -f "${FIXTURES}/.last-local-success" "${FIXTURES}/.last-upload-success"
run_check
RC=$?
assert "exits 1" "$([ ${RC} -eq 1 ] && echo 0 || echo 1)"
BODY=$(alert_body)
assert "alert mentions local missing" "$(echo "${BODY}" | grep -qiE 'local' && echo 0 || echo 1)"
assert "alert mentions upload missing" "$(echo "${BODY}" | grep -qiE 'upload' && echo 0 || echo 1)"
# Single aggregated alert
assert "exactly one curl POST (aggregated)" "$([ "$(curl_post_count)" = "1" ] && echo 0 || echo 1)"
rm -rf "${SANDBOX}"

echo "=== Test 9: Telegram delivery FAILURE while problem exists → exit 2 + metrics ==="
make_sandbox
rm -f "${FIXTURES}/.last-local-success"
run_check MOCK_CURL_FAIL=1 NODE_EXPORTER_TEXTFILE_DIR="${TEXTFILE_DIR}"
RC=$?
assert "exits 2" "$([ ${RC} -eq 2 ] && echo 0 || echo 1)"
PROM="${TEXTFILE_DIR}/payverge_backup_check.prom"
assert ".prom file written" "$([ -f "${PROM}" ] && echo 0 || echo 1)"
assert "alert_delivered is 0" "$(grep -qE 'payverge_backup_check_alert_delivered 0$' "${PROM}" 2>/dev/null && echo 0 || echo 1)"
assert "local success is 0" "$(grep -qE 'payverge_backup_check_success\{type="local"\} 0' "${PROM}" 2>/dev/null && echo 0 || echo 1)"
assert "last_run_timestamp present" "$(grep -q 'payverge_backup_check_last_run_timestamp ' "${PROM}" 2>/dev/null && echo 0 || echo 1)"
rm -rf "${SANDBOX}"

echo "=== Test 10: alerting not configured while problem exists → exit 2 ==="
make_sandbox
rm -f "${FIXTURES}/.last-local-success"
# Override tokens to empty (must pass after defaults in run_check — use env that clears)
env PATH="${MOCK_BIN}:${PATH}" \
  BACKUP_VOLUME=test_backup_data \
  TELEGRAM_ESCALATION_BOT_TOKEN= \
  TELEGRAM_ESCALATION_CHAT_ID= \
  bash "${CHECK_SCRIPT}" > "${SANDBOX}/stdout.log" 2> "${SANDBOX}/stderr.log"
RC=$?
assert "exits 2" "$([ ${RC} -eq 2 ] && echo 0 || echo 1)"
assert "no curl when not configured" "$([ ! -f "${CURL_LOG}" ] || [ "$(curl_post_count)" = "0" ] && echo 0 || echo 1)"
rm -rf "${SANDBOX}"

echo "=== Test 11: healthy run writes metrics with success=1 when textfile dir set ==="
make_sandbox
run_check NODE_EXPORTER_TEXTFILE_DIR="${TEXTFILE_DIR}"
RC=$?
assert "exits 0" "$([ ${RC} -eq 0 ] && echo 0 || echo 1)"
PROM="${TEXTFILE_DIR}/payverge_backup_check.prom"
assert ".prom file written" "$([ -f "${PROM}" ] && echo 0 || echo 1)"
assert "local success 1" "$(grep -qE 'payverge_backup_check_success\{type="local"\} 1' "${PROM}" && echo 0 || echo 1)"
assert "upload success 1" "$(grep -qE 'payverge_backup_check_success\{type="upload"\} 1' "${PROM}" && echo 0 || echo 1)"
assert "remote_object success 1" "$(grep -qE 'payverge_backup_check_success\{type="remote_object"\} 1' "${PROM}" && echo 0 || echo 1)"
assert "alert_delivered 1" "$(grep -qE 'payverge_backup_check_alert_delivered 1$' "${PROM}" && echo 0 || echo 1)"
rm -rf "${SANDBOX}"

echo "=== Test 12: MAX_LOCAL_AGE_SECONDS sets the local threshold; MAX_AGE_SECONDS is not read ==="
make_sandbox
# The default 8h threshold already passes FRESH; a removed alias set tiny must
# change nothing.
run_check MAX_AGE_SECONDS=0
RC=$?
assert "exits 0 (MAX_AGE_SECONDS ignored)" "$([ ${RC} -eq 0 ] && echo 0 || echo 1)"
rm -rf "${SANDBOX}"
make_sandbox
printf 'timestamp=%s\nfile=/backups/x.sql.gz\nsize_bytes=1\n' "${STALE_TS}" \
  > "${FIXTURES}/.last-local-success"
run_check MAX_LOCAL_AGE_SECONDS=100
RC=$?
assert "exits 1 with MAX_LOCAL_AGE_SECONDS=100" "$([ ${RC} -eq 1 ] && echo 0 || echo 1)"
BODY=$(alert_body)
assert "alert mentions local / threshold 100" "$(echo "${BODY}" | grep -qi 'local' && echo "${BODY}" | grep -q '100' && echo 0 || echo 1)"
rm -rf "${SANDBOX}"

echo "=== Test 13: S3_BACKUP_ENDPOINT passed to aws head-object ==="
make_sandbox
run_check S3_BACKUP_ENDPOINT=https://accountid.r2.cloudflarestorage.com
RC=$?
assert "exits 0" "$([ ${RC} -eq 0 ] && echo 0 || echo 1)"
assert "aws got --endpoint-url" "$(grep -q -- '--endpoint-url https://accountid.r2.cloudflarestorage.com' "${AWS_LOG}" 2>/dev/null && echo 0 || echo 1)"
rm -rf "${SANDBOX}"

echo "=== Test 14: both local and upload stale → single alert names both ==="
make_sandbox
printf 'timestamp=%s\nfile=/backups/x.sql.gz\nsize_bytes=1\n' "${STALE_TS}" \
  > "${FIXTURES}/.last-local-success"
printf 'timestamp=%s\nobject=s3://example-db-backups/db/x.sql.gz\n' "${STALE_TS}" \
  > "${FIXTURES}/.last-upload-success"
run_check
RC=$?
assert "exits 1" "$([ ${RC} -eq 1 ] && echo 0 || echo 1)"
assert "exactly one curl POST" "$([ "$(curl_post_count)" = "1" ] && echo 0 || echo 1)"
BODY=$(alert_body)
assert "alert mentions local" "$(echo "${BODY}" | grep -qi 'local' && echo 0 || echo 1)"
assert "alert mentions upload" "$(echo "${BODY}" | grep -qi 'upload' && echo 0 || echo 1)"
rm -rf "${SANDBOX}"

echo ""
if [ "${FAILURES}" -gt 0 ]; then
  echo "${FAILURES} ASSERTION(S) FAILED"
  exit 1
fi
echo "ALL TESTS PASSED"
