#!/bin/bash
set -uo pipefail

# Test harness for backend/scripts/backup-db.sh.
# Runs the real script against mocked pg_dump/aws binaries on PATH.
# No Docker, no network. Works on macOS bash 3.2 and Linux.
#
# Usage: bash backend/scripts/backup-db.test.sh
# Exit 0 + "ALL TESTS PASSED" on success; exit 1 on any failure.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKUP_SCRIPT="${SCRIPT_DIR}/backup-db.sh"

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

make_sandbox() {
  SANDBOX="$(mktemp -d "${TMPDIR:-/tmp}/backup-test.XXXXXX")"
  MOCK_BIN="${SANDBOX}/bin"
  BACKUP_DIR="${SANDBOX}/backups"
  AWS_LOG="${SANDBOX}/aws-invocations.log"
  mkdir -p "${MOCK_BIN}" "${BACKUP_DIR}"

  # Mock pg_dump: emits ~4KB of plausible SQL, or fails when MOCK_PG_DUMP_FAIL=1.
  cat > "${MOCK_BIN}/pg_dump" <<'EOF'
#!/bin/bash
if [ "${MOCK_PG_DUMP_FAIL:-0}" = "1" ]; then
  echo "pg_dump: error: connection to server failed (mock)" >&2
  exit 1
fi
echo "-- PostgreSQL database dump (mock)"
i=0
while [ $i -lt 100 ]; do
  echo "INSERT INTO mock_table (id, payload) VALUES ($i, 'xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx');"
  i=$((i + 1))
done
echo "-- PostgreSQL database dump complete"
EOF
  chmod +x "${MOCK_BIN}/pg_dump"

  # Mock aws: records argv to AWS_LOG. Supports:
  #   MOCK_AWS_FAIL=1            — fail s3 cp (upload)
  #   MOCK_AWS_HEAD_BUCKET_FAIL=1 — fail s3api head-bucket
  #   MOCK_AWS_HEAD_OBJECT_FAIL=1 — fail s3api head-object
  cat > "${MOCK_BIN}/aws" <<EOF
#!/bin/bash
echo "\$@" >> "${AWS_LOG}"
# Detect subcommand: "s3api head-bucket", "s3api head-object", "s3 cp"
if echo "\$*" | grep -q 's3api head-bucket'; then
  if [ "\${MOCK_AWS_HEAD_BUCKET_FAIL:-0}" = "1" ]; then
    echo "head-bucket failed (mock)" >&2
    exit 1
  fi
  exit 0
fi
if echo "\$*" | grep -q 's3api head-object'; then
  if [ "\${MOCK_AWS_HEAD_OBJECT_FAIL:-0}" = "1" ]; then
    echo "head-object failed (mock)" >&2
    exit 1
  fi
  exit 0
fi
if echo "\$*" | grep -q 's3 cp'; then
  if [ "\${MOCK_AWS_FAIL:-0}" = "1" ]; then
    echo "upload failed (mock)" >&2
    exit 1
  fi
  exit 0
fi
# Default: succeed for unknown aws invocations
exit 0
EOF
  chmod +x "${MOCK_BIN}/aws"
}

run_backup() {
  # Args are extra environment assignments, e.g. S3_BACKUP_BUCKET=b
  env PATH="${MOCK_BIN}:${PATH}" \
    DB_HOST=mockhost DB_PORT=5432 DB_USER=payverge DB_PASSWORD=pw DB_NAME=payverge \
    BACKUP_DIR="${BACKUP_DIR}" RETENTION_DAYS=7 \
    "$@" \
    bash "${BACKUP_SCRIPT}" > "${SANDBOX}/stdout.log" 2> "${SANDBOX}/stderr.log"
}

count_final_backups() {
  find "${BACKUP_DIR}" -name 'payverge_*.sql.gz' -type f | wc -l | tr -d ' '
}

count_tmp_files() {
  find "${BACKUP_DIR}" -name '*.tmp' -type f | wc -l | tr -d ' '
}

no_markers() {
  [ ! -f "${BACKUP_DIR}/.last-success" ] && \
  [ ! -f "${BACKUP_DIR}/.last-local-success" ] && \
  [ ! -f "${BACKUP_DIR}/.last-upload-success" ] && \
  [ ! -f "${BACKUP_DIR}/.last-upload" ]
}

# ---------------------------------------------------------------------------
# NEW contract tests (written first — must fail against the old script)
# ---------------------------------------------------------------------------

echo "=== Test A: REQUIRE_OFFSITE_BACKUP=true + empty bucket → fail-fast, no dump/markers ==="
make_sandbox
run_backup REQUIRE_OFFSITE_BACKUP=true S3_BACKUP_BUCKET=
RC=$?
assert "exits nonzero" "$([ ${RC} -ne 0 ] && echo 0 || echo 1)"
assert "no .last-success" "$([ ! -f "${BACKUP_DIR}/.last-success" ] && echo 0 || echo 1)"
assert "no .last-local-success" "$([ ! -f "${BACKUP_DIR}/.last-local-success" ] && echo 0 || echo 1)"
assert "no .last-upload-success" "$([ ! -f "${BACKUP_DIR}/.last-upload-success" ] && echo 0 || echo 1)"
assert "no final .sql.gz (fail-fast before dump)" "$([ "$(count_final_backups)" = "0" ] && echo 0 || echo 1)"
assert "aws never invoked" "$([ ! -f "${AWS_LOG}" ] && echo 0 || echo 1)"
rm -rf "${SANDBOX}"

echo "=== Test B: REQUIRE_OFFSITE_BACKUP=true + bucket set but aws missing from PATH ==="
make_sandbox
# Mock bin with only pg_dump (no aws). Restrict PATH so a real host aws cannot leak in.
rm -f "${MOCK_BIN}/aws"
env PATH="${MOCK_BIN}:/bin:/usr/bin" \
  DB_HOST=mockhost DB_PORT=5432 DB_USER=payverge DB_PASSWORD=pw DB_NAME=payverge \
  BACKUP_DIR="${BACKUP_DIR}" RETENTION_DAYS=7 \
  REQUIRE_OFFSITE_BACKUP=true S3_BACKUP_BUCKET=example-db-backups \
  bash "${BACKUP_SCRIPT}" > "${SANDBOX}/stdout.log" 2> "${SANDBOX}/stderr.log"
RC=$?
assert "exits nonzero" "$([ ${RC} -ne 0 ] && echo 0 || echo 1)"
assert "no markers" "$(no_markers && echo 0 || echo 1)"
assert "no final .sql.gz" "$([ "$(count_final_backups)" = "0" ] && echo 0 || echo 1)"
rm -rf "${SANDBOX}"

echo "=== Test C: REQUIRE_OFFSITE_BACKUP=true + head-bucket fails → no markers, no dump ==="
make_sandbox
run_backup REQUIRE_OFFSITE_BACKUP=true S3_BACKUP_BUCKET=example-db-backups MOCK_AWS_HEAD_BUCKET_FAIL=1
RC=$?
assert "exits nonzero" "$([ ${RC} -ne 0 ] && echo 0 || echo 1)"
assert "no markers" "$(no_markers && echo 0 || echo 1)"
assert "no final .sql.gz" "$([ "$(count_final_backups)" = "0" ] && echo 0 || echo 1)"
assert "head-bucket was attempted" "$(grep -q 's3api head-bucket' "${AWS_LOG}" 2>/dev/null && echo 0 || echo 1)"
if [ -f "${AWS_LOG}" ] && grep -q 's3 cp' "${AWS_LOG}" 2>/dev/null; then
  assert "s3 cp was NOT attempted" "1"
else
  assert "s3 cp was NOT attempted" "0"
fi
rm -rf "${SANDBOX}"

echo "=== Test D: REQUIRE_OFFSITE_BACKUP=true + upload fails → local marker only, dump kept ==="
make_sandbox
run_backup REQUIRE_OFFSITE_BACKUP=true S3_BACKUP_BUCKET=example-db-backups MOCK_AWS_FAIL=1
RC=$?
assert "exits nonzero" "$([ ${RC} -ne 0 ] && echo 0 || echo 1)"
assert "local dump kept" "$([ "$(count_final_backups)" = "1" ] && echo 0 || echo 1)"
assert ".last-local-success present" "$([ -f "${BACKUP_DIR}/.last-local-success" ] && echo 0 || echo 1)"
assert "no .last-upload-success" "$([ ! -f "${BACKUP_DIR}/.last-upload-success" ] && echo 0 || echo 1)"
assert "no .last-success (policy requires offsite)" "$([ ! -f "${BACKUP_DIR}/.last-success" ] && echo 0 || echo 1)"
assert "head-bucket ran (precheck)" "$(grep -q 's3api head-bucket' "${AWS_LOG}" 2>/dev/null && echo 0 || echo 1)"
assert "s3 cp was attempted" "$(grep -q 's3 cp' "${AWS_LOG}" 2>/dev/null && echo 0 || echo 1)"
rm -rf "${SANDBOX}"

echo "=== Test E: REQUIRE_OFFSITE_BACKUP=true + full success → all three markers ==="
make_sandbox
run_backup REQUIRE_OFFSITE_BACKUP=true S3_BACKUP_BUCKET=example-db-backups S3_BACKUP_PREFIX=db
RC=$?
assert "exits 0" "$([ ${RC} -eq 0 ] && echo 0 || echo 1)"
assert ".last-local-success present" "$([ -f "${BACKUP_DIR}/.last-local-success" ] && echo 0 || echo 1)"
assert ".last-upload-success present" "$([ -f "${BACKUP_DIR}/.last-upload-success" ] && echo 0 || echo 1)"
assert ".last-success present" "$([ -f "${BACKUP_DIR}/.last-success" ] && echo 0 || echo 1)"
assert "local marker has file= line" "$(grep -q '^file=' "${BACKUP_DIR}/.last-local-success" 2>/dev/null && echo 0 || echo 1)"
assert "local marker has size_bytes= line" "$(grep -q '^size_bytes=' "${BACKUP_DIR}/.last-local-success" 2>/dev/null && echo 0 || echo 1)"
assert "upload marker has object= line" "$(grep -q '^object=' "${BACKUP_DIR}/.last-upload-success" 2>/dev/null && echo 0 || echo 1)"
assert "policy marker has file= and size_bytes=" "$(grep -q '^file=' "${BACKUP_DIR}/.last-success" 2>/dev/null && grep -q '^size_bytes=' "${BACKUP_DIR}/.last-success" 2>/dev/null && echo 0 || echo 1)"
assert "durable policy marker exports offsite object identifier" "$(grep -q '^object=s3://example-db-backups/db/payverge_' "${BACKUP_DIR}/.last-success" 2>/dev/null && echo 0 || echo 1)"
# Policy mtime >= upload mtime (policy written after upload)
if [ -f "${BACKUP_DIR}/.last-success" ] && [ -f "${BACKUP_DIR}/.last-upload-success" ]; then
  # Compare epoch seconds; allow equal if same-second write
  POL_MT=$(stat -f%m "${BACKUP_DIR}/.last-success" 2>/dev/null || stat -c%Y "${BACKUP_DIR}/.last-success" 2>/dev/null)
  UP_MT=$(stat -f%m "${BACKUP_DIR}/.last-upload-success" 2>/dev/null || stat -c%Y "${BACKUP_DIR}/.last-upload-success" 2>/dev/null)
  assert ".last-success mtime >= .last-upload-success" "$([ "${POL_MT}" -ge "${UP_MT}" ] && echo 0 || echo 1)"
else
  assert ".last-success mtime >= .last-upload-success" "1"
fi
assert "aws called with s3 cp to s3://example-db-backups/db/" "$(grep -q 's3 cp' "${AWS_LOG}" 2>/dev/null && grep -q 's3://example-db-backups/db/payverge_' "${AWS_LOG}" 2>/dev/null && echo 0 || echo 1)"
assert "head-bucket precheck ran" "$(grep -q 's3api head-bucket' "${AWS_LOG}" 2>/dev/null && echo 0 || echo 1)"
rm -rf "${SANDBOX}"

echo "=== Test F: REQUIRE_OFFSITE_BACKUP unset/false + bucket unset → local is policy success ==="
make_sandbox
run_backup S3_BACKUP_BUCKET=
RC=$?
assert "exits 0" "$([ ${RC} -eq 0 ] && echo 0 || echo 1)"
assert ".last-local-success present" "$([ -f "${BACKUP_DIR}/.last-local-success" ] && echo 0 || echo 1)"
assert ".last-success present" "$([ -f "${BACKUP_DIR}/.last-success" ] && echo 0 || echo 1)"
assert "no .last-upload-success" "$([ ! -f "${BACKUP_DIR}/.last-upload-success" ] && echo 0 || echo 1)"
assert "aws never invoked" "$([ ! -f "${AWS_LOG}" ] && echo 0 || echo 1)"
rm -rf "${SANDBOX}"

# ---------------------------------------------------------------------------
# Preserved / updated existing coverage
# ---------------------------------------------------------------------------

echo "=== Test 1: successful local backup writes final file atomically + markers ==="
make_sandbox
run_backup S3_BACKUP_BUCKET=
RC=$?
assert "exits 0" "$([ ${RC} -eq 0 ] && echo 0 || echo 1)"
assert "exactly one final .sql.gz exists" "$([ "$(count_final_backups)" = "1" ] && echo 0 || echo 1)"
assert "no .tmp file left behind" "$([ "$(count_tmp_files)" = "0" ] && echo 0 || echo 1)"
assert "final file is valid gzip" "$(gzip -t "${BACKUP_DIR}"/payverge_*.sql.gz 2>/dev/null && echo 0 || echo 1)"
assert ".last-success marker written" "$([ -f "${BACKUP_DIR}/.last-success" ] && echo 0 || echo 1)"
assert ".last-local-success marker written" "$([ -f "${BACKUP_DIR}/.last-local-success" ] && echo 0 || echo 1)"
assert "policy marker contains timestamp= line" "$(grep -q '^timestamp=' "${BACKUP_DIR}/.last-success" 2>/dev/null && echo 0 || echo 1)"
assert "policy marker contains file= line" "$(grep -q '^file=' "${BACKUP_DIR}/.last-success" 2>/dev/null && echo 0 || echo 1)"
assert "aws never invoked when bucket unset" "$([ ! -f "${AWS_LOG}" ] && echo 0 || echo 1)"
rm -rf "${SANDBOX}"

echo "=== Test 2: pg_dump failure -> nonzero exit, no final file, no markers ==="
make_sandbox
run_backup MOCK_PG_DUMP_FAIL=1 S3_BACKUP_BUCKET=
RC=$?
assert "exits nonzero" "$([ ${RC} -ne 0 ] && echo 0 || echo 1)"
assert "no final .sql.gz published" "$([ "$(count_final_backups)" = "0" ] && echo 0 || echo 1)"
assert "no .tmp file left behind (trap cleanup)" "$([ "$(count_tmp_files)" = "0" ] && echo 0 || echo 1)"
assert "no .last-success marker" "$([ ! -f "${BACKUP_DIR}/.last-success" ] && echo 0 || echo 1)"
assert "no .last-local-success marker" "$([ ! -f "${BACKUP_DIR}/.last-local-success" ] && echo 0 || echo 1)"
rm -rf "${SANDBOX}"

echo "=== Test 3: S3 upload invoked with bucket/prefix + .last-upload-success ==="
make_sandbox
run_backup S3_BACKUP_BUCKET=example-db-backups S3_BACKUP_PREFIX=db
RC=$?
assert "exits 0" "$([ ${RC} -eq 0 ] && echo 0 || echo 1)"
assert "aws invoked" "$([ -f "${AWS_LOG}" ] && echo 0 || echo 1)"
assert "aws called with s3 cp" "$(grep -q 's3 cp' "${AWS_LOG}" 2>/dev/null && echo 0 || echo 1)"
assert "aws target is s3://example-db-backups/db/" "$(grep -q 's3://example-db-backups/db/payverge_' "${AWS_LOG}" 2>/dev/null && echo 0 || echo 1)"
assert ".last-upload-success marker written" "$([ -f "${BACKUP_DIR}/.last-upload-success" ] && echo 0 || echo 1)"
assert ".last-local-success present" "$([ -f "${BACKUP_DIR}/.last-local-success" ] && echo 0 || echo 1)"
assert ".last-success present (upload ok)" "$([ -f "${BACKUP_DIR}/.last-success" ] && echo 0 || echo 1)"
# Old .last-upload name must NOT be used
assert "old .last-upload name not written" "$([ ! -f "${BACKUP_DIR}/.last-upload" ] && echo 0 || echo 1)"
rm -rf "${SANDBOX}"

echo "=== Test 4: custom endpoint passed through to aws ==="
make_sandbox
run_backup S3_BACKUP_BUCKET=example-db-backups S3_BACKUP_ENDPOINT=https://accountid.r2.cloudflarestorage.com
RC=$?
assert "exits 0" "$([ ${RC} -eq 0 ] && echo 0 || echo 1)"
assert "aws called with --endpoint-url" "$(grep -q -- '--endpoint-url https://accountid.r2.cloudflarestorage.com' "${AWS_LOG}" 2>/dev/null && echo 0 || echo 1)"
rm -rf "${SANDBOX}"

echo "=== Test 5: aws upload failure (offsite optional) -> nonzero, local marker only ==="
# With REQUIRE_OFFSITE_BACKUP=false (default) and bucket set: dump succeeds,
# .last-local-success is written, but upload failure must NOT write .last-success
# (policy success requires a durable upload when bucket is configured).
make_sandbox
run_backup S3_BACKUP_BUCKET=example-db-backups MOCK_AWS_FAIL=1
RC=$?
assert "exits nonzero" "$([ ${RC} -ne 0 ] && echo 0 || echo 1)"
assert "local dump still exists (upload retries next cycle)" "$([ "$(count_final_backups)" = "1" ] && echo 0 || echo 1)"
assert ".last-local-success written (dump itself succeeded)" "$([ -f "${BACKUP_DIR}/.last-local-success" ] && echo 0 || echo 1)"
assert "no .last-success (upload failed)" "$([ ! -f "${BACKUP_DIR}/.last-success" ] && echo 0 || echo 1)"
assert "no .last-upload-success" "$([ ! -f "${BACKUP_DIR}/.last-upload-success" ] && echo 0 || echo 1)"
rm -rf "${SANDBOX}"

echo "=== Test G: head-object failure after cp → no policy/upload markers ==="
make_sandbox
run_backup REQUIRE_OFFSITE_BACKUP=true S3_BACKUP_BUCKET=example-db-backups MOCK_AWS_HEAD_OBJECT_FAIL=1
RC=$?
assert "exits nonzero" "$([ ${RC} -ne 0 ] && echo 0 || echo 1)"
assert "local dump kept" "$([ "$(count_final_backups)" = "1" ] && echo 0 || echo 1)"
assert ".last-local-success present" "$([ -f "${BACKUP_DIR}/.last-local-success" ] && echo 0 || echo 1)"
assert "no .last-upload-success" "$([ ! -f "${BACKUP_DIR}/.last-upload-success" ] && echo 0 || echo 1)"
assert "no .last-success" "$([ ! -f "${BACKUP_DIR}/.last-success" ] && echo 0 || echo 1)"
assert "head-object was attempted" "$(grep -q 's3api head-object' "${AWS_LOG}" 2>/dev/null && echo 0 || echo 1)"
rm -rf "${SANDBOX}"

echo ""
if [ "${FAILURES}" -gt 0 ]; then
  echo "${FAILURES} ASSERTION(S) FAILED"
  exit 1
fi
echo "ALL TESTS PASSED"
