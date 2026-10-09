#!/usr/bin/env bash
set -uo pipefail

# App-host-independent off-site freshness contract. AWS is fully mocked.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CHECK_SCRIPT="${SCRIPT_DIR}/check-offsite-backup.sh"
FAILURES=0

assert() {
  local desc="$1" result="$2"
  if [ "$result" = 0 ]; then echo "  ok    - ${desc}"; else
    echo "  FAIL  - ${desc}"
    FAILURES=$((FAILURES + 1))
  fi
}

make_sandbox() {
  SANDBOX="$(mktemp -d "${TMPDIR:-/tmp}/offsite-backup-test.XXXXXX")"
  MOCK_BIN="${SANDBOX}/bin"
  AWS_LOG="${SANDBOX}/aws.log"
  EVIDENCE_FILE="${SANDBOX}/freshness.json"
  METRICS_FILE="${SANDBOX}/freshness.prom"
  mkdir -p "${MOCK_BIN}"
  : >"${AWS_LOG}"
  cat >"${MOCK_BIN}/aws" <<EOF
#!/usr/bin/env bash
echo "\$*" >>"${AWS_LOG}"
if echo "\$*" | grep -q 's3api list-objects-v2'; then
  if [ "\${MOCK_EMPTY:-0}" = 1 ]; then exit 0; fi
  printf 'db/payverge_latest.sql.gz\t%s\t4096\t"mock-etag"\n' "\${MOCK_LAST_MODIFIED}"
  exit 0
fi
if echo "\$*" | grep -q 's3api head-object'; then
  [ "\${MOCK_HEAD_FAIL:-0}" != 1 ]
  exit \$?
fi
exit 1
EOF
  chmod +x "${MOCK_BIN}/aws"
}

run_check() {
  env PATH="${MOCK_BIN}:/bin:/usr/bin" \
    S3_BACKUP_BUCKET=example-db-backups S3_BACKUP_PREFIX=db \
    NOW_EPOCH=1785582000 MAX_OFFSITE_BACKUP_AGE_SECONDS=14400 \
    EVIDENCE_FILE="${EVIDENCE_FILE}" METRICS_FILE="${METRICS_FILE}" \
    "$@" bash "${CHECK_SCRIPT}" >"${SANDBOX}/stdout" 2>"${SANDBOX}/stderr"
}

echo "=== Test 1: absent off-site backup fails closed with evidence ==="
make_sandbox
run_check MOCK_EMPTY=1
RC=$?
assert "exits nonzero" "$([ "$RC" -ne 0 ] && echo 0 || echo 1)"
assert "failure evidence exists" "$([ -f "${EVIDENCE_FILE}" ] && grep -q '"result": "fail"' "${EVIDENCE_FILE}" && echo 0 || echo 1)"
assert "head-object not called without an object" "$(! grep -q 'head-object' "${AWS_LOG}" && echo 0 || echo 1)"
rm -rf "${SANDBOX}"

echo "=== Test 2: stale off-site object fails before the six-hour RPO ==="
make_sandbox
# 2026-08-01T00:00:00Z is 11 hours older than NOW_EPOCH.
run_check MOCK_LAST_MODIFIED=2026-08-01T00:00:00Z
RC=$?
assert "exits nonzero" "$([ "$RC" -ne 0 ] && echo 0 || echo 1)"
assert "reports stale age and threshold" "$(grep -Eqi 'stale|age|14400' "${SANDBOX}/stderr" && echo 0 || echo 1)"
assert "freshness metric is zero" "$(grep -q '^payverge_offsite_backup_fresh 0$' "${METRICS_FILE}" && echo 0 || echo 1)"
rm -rf "${SANDBOX}"

echo "=== Test 3: fresh object passes and exports object identity ==="
make_sandbox
# 2026-08-01T10:00:00Z is one hour older than NOW_EPOCH.
run_check MOCK_LAST_MODIFIED=2026-08-01T10:00:00Z
RC=$?
assert "exits zero" "$([ "$RC" -eq 0 ] && echo 0 || echo 1)"
assert "head-object verifies newest key" "$(grep -q 'head-object.*--key db/payverge_latest.sql.gz' "${AWS_LOG}" && echo 0 || echo 1)"
assert "evidence exports object key" "$(grep -q 'db/payverge_latest.sql.gz' "${EVIDENCE_FILE}" && echo 0 || echo 1)"
assert "evidence exports ETag" "$(grep -q 'mock-etag' "${EVIDENCE_FILE}" && echo 0 || echo 1)"
assert "freshness metric is one" "$(grep -q '^payverge_offsite_backup_fresh 1$' "${METRICS_FILE}" && echo 0 || echo 1)"
rm -rf "${SANDBOX}"

echo "=== Test 4: head-object failure fails closed ==="
make_sandbox
run_check MOCK_LAST_MODIFIED=2026-08-01T10:00:00Z MOCK_HEAD_FAIL=1
RC=$?
assert "exits nonzero" "$([ "$RC" -ne 0 ] && echo 0 || echo 1)"
assert "freshness metric is zero" "$(grep -q '^payverge_offsite_backup_fresh 0$' "${METRICS_FILE}" && echo 0 || echo 1)"
rm -rf "${SANDBOX}"

echo
if [ "${FAILURES}" -ne 0 ]; then
  echo "${FAILURES} ASSERTION(S) FAILED"
  exit 1
fi
echo "ALL TESTS PASSED"
