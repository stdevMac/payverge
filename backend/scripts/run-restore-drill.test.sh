#!/bin/bash
set -uo pipefail

# Test harness for backend/scripts/run-restore-drill.sh.
# Stubs docker, aws, psql, pg_isready on a temp PATH. Uses real gzip for a
# tiny fixture (or corrupt bytes). No real Docker/network.
# Works on macOS bash 3.2 and Linux.
#
# Usage: bash backend/scripts/run-restore-drill.test.sh
# Exit 0 + "ALL TESTS PASSED" on success; exit 1 on any failure.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DRILL_SCRIPT="${SCRIPT_DIR}/run-restore-drill.sh"

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
  SANDBOX="$(mktemp -d "${TMPDIR:-/tmp}/restore-drill-test.XXXXXX")"
  MOCK_BIN="${SANDBOX}/bin"
  FIXTURES="${SANDBOX}/fixtures"
  EVIDENCE_DIR="${SANDBOX}/evidence"
  SCRATCH_DIR="${SANDBOX}/scratch"
  AWS_LOG="${SANDBOX}/aws-invocations.log"
  DOCKER_LOG="${SANDBOX}/docker-invocations.log"
  PSQL_LOG="${SANDBOX}/psql-invocations.log"
  PGISREADY_LOG="${SANDBOX}/pgisready-invocations.log"
  FIXED_CONTAINER="pv-restore-drill-test-$$"
  FIXED_BACKEND_CONTAINER="${FIXED_CONTAINER}-backend"
  FIXED_NETWORK="${FIXED_CONTAINER}-network"

  mkdir -p "${MOCK_BIN}" "${FIXTURES}" "${EVIDENCE_DIR}" "${SCRATCH_DIR}"
  : > "${AWS_LOG}"
  : > "${DOCKER_LOG}"
  : > "${PSQL_LOG}"
  : > "${PGISREADY_LOG}"

  # Tiny real gzip fixture: a few SQL comments (enough for gunzip | psql happy path).
  printf -- '-- mock dump\nSELECT 1;\n' | gzip -c > "${FIXTURES}/payverge_mock.sql.gz"
  printf 'not-a-gzip-payload' > "${FIXTURES}/corrupt.sql.gz"

  # Mock aws: list / download switchable via MOCK_AWS_EMPTY, MOCK_GZIP_CORRUPT
  cat > "${MOCK_BIN}/aws" <<EOF
#!/bin/bash
echo "\$@" >> "${AWS_LOG}"
if echo "\$*" | grep -q 's3 ls'; then
  if [ "\${MOCK_AWS_EMPTY:-0}" = "1" ]; then
    exit 0
  fi
  # aws s3 ls line: DATE TIME SIZE KEY
  echo "2026-04-21 12:00:00    1024 payverge_mock.sql.gz"
  exit 0
fi
if echo "\$*" | grep -q 's3 cp'; then
  # Last argv is destination path
  dest=""
  for a in "\$@"; do dest="\$a"; done
  if [ "\${MOCK_GZIP_CORRUPT:-0}" = "1" ]; then
    cp "${FIXTURES}/corrupt.sql.gz" "\$dest"
  else
    cp "${FIXTURES}/payverge_mock.sql.gz" "\$dest"
  fi
  exit 0
fi
exit 0
EOF
  chmod +x "${MOCK_BIN}/aws"

  # Mock docker: record argv; run succeeds; rm -f recorded for cleanup assert
  cat > "${MOCK_BIN}/docker" <<EOF
#!/bin/bash
echo "\$@" >> "${DOCKER_LOG}"
if [ "\$1" = "inspect" ] && echo "\$*" | grep -q '{{.State.Running}}'; then
  if [ "\${MOCK_BACKEND_EXITED:-0}" = "1" ]; then
    echo "false"
  else
    echo "true"
  fi
  exit 0
fi
if [ "\$1" = "run" ]; then
  echo "mock-container-id"
  exit 0
fi
if [ "\$1" = "rm" ]; then
  exit 0
fi
exit 0
EOF
  chmod +x "${MOCK_BIN}/docker"

  cat > "${MOCK_BIN}/go" <<'EOF'
#!/bin/bash
if [ "${MOCK_HASH_FAIL:-0}" = "1" ]; then
  exit 1
fi
echo '$2b$12$abcdefghijklmnopqrstuvabcdefghijklmnopqrstuvabcd'
EOF
  chmod +x "${MOCK_BIN}/go"

  cat > "${MOCK_BIN}/openssl" <<'EOF'
#!/bin/bash
echo 'mock-restore-password-never-retain'
EOF
  chmod +x "${MOCK_BIN}/openssl"

  # Mock curl: health probes plus login and authenticated /me checks. Response
  # bodies contain a canary session value so the harness can prove it is never
  # copied into evidence.
  cat > "${MOCK_BIN}/curl" <<'EOF'
#!/bin/bash
out=""
url=""
prev=""
for arg in "$@"; do
  if [ "$prev" = "-o" ]; then out="$arg"; fi
  prev="$arg"
  url="$arg"
done
if [ "${MOCK_CURL_SLEEP_SECONDS:-0}" -gt 0 ]; then
  sleep "${MOCK_CURL_SLEEP_SECONDS}"
fi
case "$url" in
  */health/live)
    [ "${MOCK_LIVE_FAIL:-0}" != "1" ]
    ;;
  */health/ready)
    [ "${MOCK_READY_FAIL:-0}" != "1" ]
    ;;
  */auth/login)
    if [ "${MOCK_LOGIN_STATUS:-200}" != "200" ]; then
      printf '{"success":false}' > "$out"
      printf '%s' "${MOCK_LOGIN_STATUS}"
    elif [ "${MOCK_LOGIN_MALFORMED:-0}" = "1" ]; then
      printf '{bad json' > "$out"
      printf '200'
    elif [ "${MOCK_LOGIN_EMPTY_SESSION:-0}" = "1" ]; then
      printf '{"success":true,"token":""}' > "$out"
      printf '200'
    else
      printf '{"success":true,"token":"mock-session-never-retain"}' > "$out"
      printf '200'
    fi
    ;;
  */auth/me)
    if [ "${MOCK_ME_STATUS:-200}" != "200" ]; then
      printf '{"error":"unauthorized"}' > "$out"
      printf '%s' "${MOCK_ME_STATUS}"
    elif [ "${MOCK_PRINCIPAL_MISMATCH:-0}" = "1" ]; then
      printf '{"email":"other@example.invalid"}' > "$out"
      printf '200'
    else
      printf '{"email":"restore-drill-30693253679@example.invalid"}' > "$out"
      printf '200'
    fi
    ;;
  *) exit 1 ;;
esac
EOF
  chmod +x "${MOCK_BIN}/curl"

  # Mock pg_isready: always ready
  cat > "${MOCK_BIN}/pg_isready" <<EOF
#!/bin/bash
echo "\$@" >> "${PGISREADY_LOG}"
exit 0
EOF
  chmod +x "${MOCK_BIN}/pg_isready"

  # Mock psql: restore (stdin) vs -c queries; switchable failure modes
  cat > "${MOCK_BIN}/psql" <<EOF
#!/bin/bash
# Flatten argv for log
printf '%s\n' "\$*" >> "${PSQL_LOG}"

# Sanitize path: psql -f .../sanitize-clone.sql (no -c, does NOT read stdin).
# Handle it before the stdin-drain restore branch so it never blocks.
for a in "\$@"; do
  case "\$a" in
    *sanitize-clone.sql)
      if [ "\${MOCK_SANITIZE_FAIL:-0}" = "1" ]; then
        echo "ERROR: mock sanitize failure" >&2
        exit 1
      fi
      exit 0
      ;;
  esac
done

has_c=0
sql=""
prev=""
for a in "\$@"; do
  if [ "\$prev" = "-c" ]; then
    sql="\$a"
    has_c=1
  fi
  prev="\$a"
done

# Restore path: no -c, reads stdin
if [ "\$has_c" -eq 0 ]; then
  if [ "\${MOCK_RESTORE_FAIL:-0}" = "1" ]; then
    echo "ERROR: mock restore failure" >&2
    # Drain stdin so the pipe does not SIGPIPE the producer oddly
    cat >/dev/null 2>&1 || true
    exit 1
  fi
  cat >/dev/null 2>&1 || true
  exit 0
fi

# Query path
# Dirty migration
if echo "\$sql" | grep -qi 'schema_migrations' && echo "\$sql" | grep -qi 'version'; then
  if [ "\${MOCK_DIRTY:-0}" = "1" ]; then
    echo "45|t"
    exit 0
  fi
  echo "45|f"
  exit 0
fi
if echo "\$sql" | grep -qi 'schema_migrations' && echo "\$sql" | grep -qi 'count'; then
  echo "\${MOCK_MIG_COUNT:-1}"
  exit 0
fi
if echo "\$sql" | grep -qi 'information_schema.tables'; then
  echo "150"
  exit 0
fi
if echo "\$sql" | grep -qi 'total_amount'; then
  if [ "\${MOCK_INVARIANT_FAIL:-0}" = "1" ]; then
    echo "3"
    exit 0
  fi
  echo "0"
  exit 0
fi
if echo "\$sql" | grep -qi 'FROM businesses'; then
  echo "12"
  exit 0
fi
if echo "\$sql" | grep -qi 'FROM bills'; then
  echo "340"
  exit 0
fi
if echo "\$sql" | grep -qi 'FROM payments'; then
  echo "280"
  exit 0
fi
echo "0"
exit 0
EOF
  chmod +x "${MOCK_BIN}/psql"
}

run_drill() {
  # Args: extra env assignments
  : > "${AWS_LOG}"
  : > "${DOCKER_LOG}"
  : > "${PSQL_LOG}"
  : > "${PGISREADY_LOG}"
  # Fresh scratch each run (script may rm -rf it)
  rm -rf "${SCRATCH_DIR}"
  mkdir -p "${SCRATCH_DIR}" "${EVIDENCE_DIR}"

  env PATH="${MOCK_BIN}:/bin:/usr/bin:/usr/local/bin" \
    S3_BACKUP_BUCKET=example-db-backups \
    S3_BACKUP_PREFIX=db \
    EVIDENCE_DIR="${EVIDENCE_DIR}" \
    SCRATCH_DIR="${SCRATCH_DIR}" \
    CONTAINER_NAME="${FIXED_CONTAINER}" \
    MIN_TABLES=100 \
    PG_IMAGE=postgres:18-alpine \
    BACKEND_IMAGE="ghcr.io/payverge/backend@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" \
    SOURCE_SHA="$(git rev-parse HEAD)" \
    PROOF_WORKFLOW_RUN_ID=30693253679 \
    PROOF_WORKFLOW_RUN_URL=https://github.com/example-org/payverge/actions/runs/30693253679 \
    PROOF_EXECUTION_ENVIRONMENT=github_hosted \
    PROOF_RUNNER_LABEL=ubuntu-latest \
    BACKEND_WAIT_SECONDS=1 \
    "$@" \
    bash "${DRILL_SCRIPT}" > "${SANDBOX}/stdout.log" 2> "${SANDBOX}/stderr.log"
}

assert_cleanup() {
  # Always: docker rm -f <container> logged AND scratch dir gone
  local cname="${FIXED_CONTAINER}"
  if [ -f "${DOCKER_LOG}" ] && grep -q "rm -f ${cname}" "${DOCKER_LOG}" 2>/dev/null; then
    assert "docker rm -f ${cname} ran" "0"
  elif [ -f "${DOCKER_LOG}" ] && grep -E "rm[[:space:]]+(-f[[:space:]]+)?${cname}|rm[[:space:]]+.*${cname}" "${DOCKER_LOG}" 2>/dev/null | grep -q .; then
    assert "docker rm -f ${cname} ran" "0"
  else
    # Also accept "rm" then "-f" then name as separate argv lines (we log one line)
    if [ -f "${DOCKER_LOG}" ] && grep -q "rm -f ${cname}\|rm -f  ${cname}" "${DOCKER_LOG}" 2>/dev/null; then
      assert "docker rm -f ${cname} ran" "0"
    else
      echo "  (docker log: $(tr '\n' '|' < "${DOCKER_LOG}" 2>/dev/null || true))"
      assert "docker rm -f ${cname} ran" "1"
    fi
  fi
  if grep -q "rm -f ${FIXED_BACKEND_CONTAINER}" "${DOCKER_LOG}" 2>/dev/null; then
    assert "candidate backend container cleanup ran" "0"
  else
    assert "candidate backend container cleanup ran" "1"
  fi
  if grep -q "network rm ${FIXED_NETWORK}" "${DOCKER_LOG}" 2>/dev/null; then
    assert "scratch network cleanup ran" "0"
  else
    assert "scratch network cleanup ran" "1"
  fi
  if [ ! -d "${SCRATCH_DIR}" ]; then
    assert "scratch dir destroyed" "0"
  else
    assert "scratch dir destroyed" "1"
  fi
}

# ---------------------------------------------------------------------------
# Failure modes
# ---------------------------------------------------------------------------

echo "=== Test 1: no remote object (empty listing) → nonzero + cleanup ==="
make_sandbox
run_drill MOCK_AWS_EMPTY=1
RC=$?
assert "exits nonzero" "$([ ${RC} -ne 0 ] && echo 0 || echo 1)"
assert_cleanup
# No container started if list failed early — cleanup still issues docker rm -f (harmless)
# Actually: if list fails before docker run, CONTAINER_NAME is still set, so trap still rm -f. Good.
rm -rf "${SANDBOX}"

echo "=== Test 2: corrupt gzip → nonzero + cleanup ==="
make_sandbox
run_drill MOCK_GZIP_CORRUPT=1
RC=$?
assert "exits nonzero" "$([ ${RC} -ne 0 ] && echo 0 || echo 1)"
# gzip fails after download, before docker — container name set, trap rm -f still runs
assert_cleanup
rm -rf "${SANDBOX}"

echo "=== Test 3: restore (psql) failure → nonzero + cleanup ==="
make_sandbox
run_drill MOCK_RESTORE_FAIL=1
RC=$?
assert "exits nonzero" "$([ ${RC} -ne 0 ] && echo 0 || echo 1)"
assert_cleanup
if grep -qE '(^|[[:space:]])run([[:space:]]|$)' "${DOCKER_LOG}" 2>/dev/null; then
  assert "docker run logged" "0"
else
  assert "docker run logged" "1"
fi
rm -rf "${SANDBOX}"

echo "=== Test 4: migration dirty=t → nonzero + cleanup ==="
make_sandbox
run_drill MOCK_DIRTY=1
RC=$?
assert "exits nonzero" "$([ ${RC} -ne 0 ] && echo 0 || echo 1)"
assert_cleanup
# Evidence should still be written on verify failure
EVIDENCE_JSON_COUNT=$(find "${EVIDENCE_DIR}" -name 'restore-drill-*.json' 2>/dev/null | wc -l | tr -d ' ')
EVIDENCE_MD_COUNT=$(find "${EVIDENCE_DIR}" -name 'restore-drill-*.md' 2>/dev/null | wc -l | tr -d ' ')
assert "evidence json written" "$([ "${EVIDENCE_JSON_COUNT}" -ge 1 ] && echo 0 || echo 1)"
assert "evidence md written" "$([ "${EVIDENCE_MD_COUNT}" -ge 1 ] && echo 0 || echo 1)"
if [ "${EVIDENCE_JSON_COUNT}" -ge 1 ]; then
  if grep -q '"result": "fail"' "${EVIDENCE_DIR}"/restore-drill-*.json 2>/dev/null; then
    assert "evidence result=fail" "0"
  else
    assert "evidence result=fail" "1"
  fi
fi
rm -rf "${SANDBOX}"

echo "=== Test 4b: empty schema_migrations (genesis version 0) → exit 0 ==="
make_sandbox
run_drill MOCK_MIG_COUNT=0
RC=$?
assert "exits zero" "$([ ${RC} -eq 0 ] && echo 0 || echo 1)"
assert_cleanup
if grep -q '"migration_version": "0"' "${EVIDENCE_DIR}"/restore-drill-*.json 2>/dev/null; then
  assert "evidence migration_version=0" "0"
else
  assert "evidence migration_version=0" "1"
fi
rm -rf "${SANDBOX}"

echo "=== Test 4c: two schema_migrations rows → nonzero ==="
make_sandbox
run_drill MOCK_MIG_COUNT=2
RC=$?
assert "exits nonzero" "$([ ${RC} -ne 0 ] && echo 0 || echo 1)"
assert_cleanup
rm -rf "${SANDBOX}"

echo "=== Test 5: failed invariant (negative amounts) → nonzero + cleanup ==="
make_sandbox
run_drill MOCK_INVARIANT_FAIL=1
RC=$?
assert "exits nonzero" "$([ ${RC} -ne 0 ] && echo 0 || echo 1)"
assert_cleanup
EVIDENCE_JSON_COUNT=$(find "${EVIDENCE_DIR}" -name 'restore-drill-*.json' 2>/dev/null | wc -l | tr -d ' ')
assert "evidence json written" "$([ "${EVIDENCE_JSON_COUNT}" -ge 1 ] && echo 0 || echo 1)"
rm -rf "${SANDBOX}"

echo "=== Test 6: happy path → exit 0 + evidence + cleanup ==="
make_sandbox
run_drill
RC=$?
assert "exits zero" "$([ ${RC} -eq 0 ] && echo 0 || echo 1)"
if [ ${RC} -ne 0 ]; then
  echo "  --- stdout ---"
  cat "${SANDBOX}/stdout.log" 2>/dev/null || true
  echo "  --- stderr ---"
  cat "${SANDBOX}/stderr.log" 2>/dev/null || true
fi
assert_cleanup
EVIDENCE_JSON_COUNT=$(find "${EVIDENCE_DIR}" -name 'restore-drill-*.json' 2>/dev/null | wc -l | tr -d ' ')
EVIDENCE_MD_COUNT=$(find "${EVIDENCE_DIR}" -name 'restore-drill-*.md' 2>/dev/null | wc -l | tr -d ' ')
assert "evidence json written" "$([ "${EVIDENCE_JSON_COUNT}" -ge 1 ] && echo 0 || echo 1)"
assert "evidence md written" "$([ "${EVIDENCE_MD_COUNT}" -ge 1 ] && echo 0 || echo 1)"
if [ "${EVIDENCE_JSON_COUNT}" -ge 1 ]; then
  JSON_FILE=$(find "${EVIDENCE_DIR}" -name 'restore-drill-*.json' | head -1)
  for field in schema_version source_sha workflow_run_id workflow_run_url object_key object_digest object_size_bytes gzip_integrity offsite_head_verification rpo_seconds duration_seconds rto_seconds migration_version table_count recovery_origin candidate_backend authentication_smoke; do
    if grep -q "\"${field}\"" "${JSON_FILE}" 2>/dev/null; then
      assert "json has ${field}" "0"
    else
      assert "json has ${field}" "1"
    fi
  done
  if grep -q '"result": "pass"' "${JSON_FILE}" 2>/dev/null; then
    assert "evidence result=pass" "0"
  else
    assert "evidence result=pass" "1"
  fi
  if grep -q '"sanitized": true' "${JSON_FILE}" 2>/dev/null; then
    assert "evidence sanitized=true" "0"
  else
    assert "evidence sanitized=true" "1"
  fi
  if grep -q '"schema_version": 2' "${JSON_FILE}" &&
     grep -q '"live_probe": "pass"' "${JSON_FILE}" &&
     grep -q '"ready_probe": "pass"' "${JSON_FILE}" &&
     grep -q '"authentication_smoke"' "${JSON_FILE}" &&
     grep -q '"session_issued": true' "${JSON_FILE}"; then
    assert "v2 candidate and authentication proof passes" "0"
  else
    assert "v2 candidate and authentication proof passes" "1"
  fi
  if grep -q 'mock-restore-password-never-retain\|mock-session-never-retain\|restore-drill-30693253679@example.invalid' "${JSON_FILE}"; then
    assert "evidence retains no authentication secret or principal" "1"
  else
    assert "evidence retains no authentication secret or principal" "0"
  fi
fi
# Scrub must have been invoked with the sanitize SQL file.
if grep -q 'sanitize-clone.sql' "${PSQL_LOG}" 2>/dev/null; then
  assert "sanitize scrub invoked (psql -f sanitize-clone.sql)" "0"
else
  assert "sanitize scrub invoked (psql -f sanitize-clone.sql)" "1"
fi
if grep -q -- '--rpc-url http://127.0.0.1:1' "${DOCKER_LOG}" 2>/dev/null; then
  assert "candidate uses an isolated non-routable RPC endpoint" "0"
else
  assert "candidate uses an isolated non-routable RPC endpoint" "1"
fi
if grep -q -- '-e FROM_EMAIL=restore-drill@example.invalid' "${DOCKER_LOG}" 2>/dev/null &&
   grep -q -- '-e FROM_EMAIL_UPDATES=restore-drill@example.invalid' "${DOCKER_LOG}" 2>/dev/null; then
  assert "candidate uses non-deliverable recovery sender addresses" "0"
else
  assert "candidate uses non-deliverable recovery sender addresses" "1"
fi
for s3_arg in \
  '--s3-bucket restore-drill-public' \
  '--aws-access-key restore-drill' \
  '--s3-endpoint http://127.0.0.1:1' \
  '--s3-protected-bucket restore-drill-protected' \
  '--aws-protected-access-key restore-drill' \
  '--s3-protected-endpoint http://127.0.0.1:1'
do
  if grep -q -- "${s3_arg}" "${DOCKER_LOG}" 2>/dev/null; then
    assert "candidate has isolated ${s3_arg%% *}" "0"
  else
    assert "candidate has isolated ${s3_arg%% *}" "1"
  fi
done
for s3_secret_arg in \
  '--aws-secret-key restore-drill-nonsecret-canary' \
  '--aws-protected-secret-key restore-drill-nonsecret-canary'
do
  if grep -q -- "${s3_secret_arg}" "${DOCKER_LOG}" 2>/dev/null; then
    assert "candidate uses fixed recovery-only ${s3_secret_arg%% *}" "0"
  else
    assert "candidate uses fixed recovery-only ${s3_secret_arg%% *}" "1"
  fi
done
if grep -q -- '-e AWS_SECRET_KEY=\|-e AWS_PROTECTED_SECRET_KEY=' "${DOCKER_LOG}" 2>/dev/null; then
  assert "candidate does not inject recovery S3 canaries through environment" "1"
else
  assert "candidate does not inject recovery S3 canaries through environment" "0"
fi
rm -rf "${SANDBOX}"

echo "=== Test 8a: backend live timeout emits sanitized startup diagnostics ==="
make_sandbox
run_drill MOCK_LIVE_FAIL=1 MOCK_BACKEND_EXITED=1
RC=$?
assert "exits nonzero" "$([ ${RC} -ne 0 ] && echo 0 || echo 1)"
assert_cleanup
if grep -q "inspect.*${FIXED_BACKEND_CONTAINER}" "${DOCKER_LOG}" 2>/dev/null; then
  assert "candidate container state inspected" "0"
else
  assert "candidate container state inspected" "1"
fi
if grep -q "logs --tail 200 ${FIXED_BACKEND_CONTAINER}" "${DOCKER_LOG}" 2>/dev/null; then
  assert "candidate startup logs emitted" "0"
else
  assert "candidate startup logs emitted" "1"
fi
if grep -q '{{.State.Running}}' "${DOCKER_LOG}" 2>/dev/null; then
  assert "live probe stops when the candidate container exits" "0"
else
  assert "live probe stops when the candidate container exits" "1"
fi
rm -rf "${SANDBOX}"

echo "=== Test 8: backend ready timeout → nonzero + fail evidence ==="
make_sandbox
run_drill MOCK_READY_FAIL=1
RC=$?
assert "exits nonzero" "$([ ${RC} -ne 0 ] && echo 0 || echo 1)"
assert_cleanup
if grep -q '"ready_probe": "fail"' "${EVIDENCE_DIR}"/restore-drill-*.json 2>/dev/null; then
  assert "ready failure recorded" "0"
else
  assert "ready failure recorded" "1"
fi
rm -rf "${SANDBOX}"

echo "=== Test 9: login rejection → nonzero + sanitized fail evidence ==="
make_sandbox
run_drill MOCK_LOGIN_STATUS=401
RC=$?
assert "exits nonzero" "$([ ${RC} -ne 0 ] && echo 0 || echo 1)"
assert_cleanup
if grep -q '"login_http_status": 401' "${EVIDENCE_DIR}"/restore-drill-*.json 2>/dev/null &&
   grep -q '"result": "fail"' "${EVIDENCE_DIR}"/restore-drill-*.json 2>/dev/null; then
  assert "login failure recorded without response" "0"
else
  assert "login failure recorded without response" "1"
fi
if grep -q 'mock-restore-password-never-retain\|mock-session-never-retain' "${EVIDENCE_DIR}"/restore-drill-* 2>/dev/null; then
  assert "failed evidence retains no authentication material" "1"
else
  assert "failed evidence retains no authentication material" "0"
fi
rm -rf "${SANDBOX}"

echo "=== Test 10: malformed or empty login session → nonzero ==="
for mode in MOCK_LOGIN_MALFORMED MOCK_LOGIN_EMPTY_SESSION; do
  make_sandbox
  run_drill "${mode}=1"
  RC=$?
  assert "${mode} exits nonzero" "$([ ${RC} -ne 0 ] && echo 0 || echo 1)"
  assert_cleanup
  rm -rf "${SANDBOX}"
done

echo "=== Test 11: authenticated principal mismatch → nonzero ==="
make_sandbox
run_drill MOCK_PRINCIPAL_MISMATCH=1
RC=$?
assert "exits nonzero" "$([ ${RC} -ne 0 ] && echo 0 || echo 1)"
assert_cleanup
if grep -q '"principal_match": false' "${EVIDENCE_DIR}"/restore-drill-*.json 2>/dev/null; then
  assert "principal mismatch recorded" "0"
else
  assert "principal mismatch recorded" "1"
fi
rm -rf "${SANDBOX}"

echo "=== Test 12: mutable backend image is refused before restore ==="
make_sandbox
run_drill BACKEND_IMAGE=ghcr.io/payverge/backend:latest
RC=$?
assert "exits nonzero" "$([ ${RC} -ne 0 ] && echo 0 || echo 1)"
if grep -q 's3 ls' "${AWS_LOG}" 2>/dev/null; then
  assert "mutable image rejected before backup access" "1"
else
  assert "mutable image rejected before backup access" "0"
fi
rm -rf "${SANDBOX}"

echo "=== Test 13: password hashing failure is fail-closed ==="
make_sandbox
run_drill MOCK_HASH_FAIL=1
RC=$?
assert "exits nonzero" "$([ ${RC} -ne 0 ] && echo 0 || echo 1)"
assert_cleanup
if grep -q 'password hashing failed' "${EVIDENCE_DIR}"/restore-drill-*.json 2>/dev/null; then
  assert "hashing failure recorded" "0"
else
  assert "hashing failure recorded" "1"
fi
rm -rf "${SANDBOX}"

echo "=== Test 14: restore exceeding configured RTO fails ==="
make_sandbox
run_drill MAX_RTO_SECONDS=0 MOCK_CURL_SLEEP_SECONDS=1
RC=$?
assert "exits nonzero" "$([ ${RC} -ne 0 ] && echo 0 || echo 1)"
assert_cleanup
if grep -q 'exceeded 0s' "${EVIDENCE_DIR}"/restore-drill-*.json 2>/dev/null; then
  assert "RTO breach recorded" "0"
else
  assert "RTO breach recorded" "1"
fi
rm -rf "${SANDBOX}"

echo "=== Test 7: sanitize scrub failure → nonzero + cleanup ==="
make_sandbox
run_drill MOCK_SANITIZE_FAIL=1
RC=$?
assert "exits nonzero" "$([ ${RC} -ne 0 ] && echo 0 || echo 1)"
assert_cleanup
# docker run happened (we got far enough to restore) but verify never passed
if grep -q 'sanitize-clone.sql' "${PSQL_LOG}" 2>/dev/null; then
  assert "sanitize scrub was attempted" "0"
else
  assert "sanitize scrub was attempted" "1"
fi
rm -rf "${SANDBOX}"

# ---------------------------------------------------------------------------
echo "=== Test 15: proof URL must name the same Actions run ==="
make_sandbox
run_drill PROOF_WORKFLOW_RUN_URL=https://github.com/example-org/payverge/actions/runs/1
RC=$?
assert "exits nonzero" "$([ ${RC} -ne 0 ] && echo 0 || echo 1)"
if grep -q 's3 ls' "${AWS_LOG}" 2>/dev/null; then
  assert "mismatched run URL rejected before backup access" "1"
else
  assert "mismatched run URL rejected before backup access" "0"
fi
rm -rf "${SANDBOX}"

echo ""
if [ "${FAILURES}" -eq 0 ]; then
  echo "ALL TESTS PASSED"
  exit 0
fi
echo "FAILURES: ${FAILURES}"
exit 1
