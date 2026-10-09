#!/usr/bin/env bash
set -uo pipefail

# Safety contract for restore-db.sh. All database commands are mocked; this
# test never contacts PostgreSQL and never performs a real restore.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESTORE_SCRIPT="${SCRIPT_DIR}/restore-db.sh"
FAILURES=0

assert() {
  local desc="$1" result="$2"
  if [ "$result" = 0 ]; then
    echo "  ok    - ${desc}"
  else
    echo "  FAIL  - ${desc}"
    FAILURES=$((FAILURES + 1))
  fi
}

make_sandbox() {
  SANDBOX="$(mktemp -d "${TMPDIR:-/tmp}/restore-db-test.XXXXXX")"
  MOCK_BIN="${SANDBOX}/bin"
  PSQL_LOG="${SANDBOX}/psql.log"
  mkdir -p "${MOCK_BIN}"
  : >"${PSQL_LOG}"
  printf '%s\n' '-- mock pg dump' 'CREATE TABLE restored(id bigint);' | gzip -c >"${SANDBOX}/valid.sql.gz"
  printf '%s' 'truncated-not-gzip' >"${SANDBOX}/corrupt.sql.gz"

  cat >"${MOCK_BIN}/psql" <<EOF
#!/usr/bin/env bash
echo "\$*" >>"${PSQL_LOG}"
if [[ "\$*" == *"information_schema.tables"* ]]; then echo 1; fi
if [[ "\$*" != *" -c "* && "\$*" != *" -f "* ]]; then
  cat >/dev/null 2>&1 || true
fi
exit 0
EOF
  cat >"${MOCK_BIN}/sleep" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
  chmod +x "${MOCK_BIN}"/*
}

run_restore() {
  env PATH="${MOCK_BIN}:/bin:/usr/bin" "$@" bash "${RESTORE_SCRIPT}" "${BACKUP_FILE}" \
    >"${SANDBOX}/stdout" 2>"${SANDBOX}/stderr"
}

echo "=== Test 1: default production-like target is refused before psql ==="
make_sandbox
BACKUP_FILE="${SANDBOX}/valid.sql.gz"
run_restore RESTORE_SCOPE=isolated RESTORE_CONFIRM='DROP localhost:5432/payverge'
RC=$?
assert "restore exits nonzero" "$([ "$RC" -ne 0 ] && echo 0 || echo 1)"
assert "psql was never called" "$([ ! -s "${PSQL_LOG}" ] && echo 0 || echo 1)"
assert "refusal names isolated target policy" "$(grep -Eqi 'isolated|refus' "${SANDBOX}/stderr" && echo 0 || echo 1)"
rm -rf "${SANDBOX}"

echo "=== Test 2: remote host is refused even with exact confirmation ==="
make_sandbox
BACKUP_FILE="${SANDBOX}/valid.sql.gz"
run_restore DB_HOST=db.production.internal DB_NAME=payverge_restore_drill \
  RESTORE_SCOPE=isolated RESTORE_CONFIRM='DROP db.production.internal:5432/payverge_restore_drill'
RC=$?
assert "restore exits nonzero" "$([ "$RC" -ne 0 ] && echo 0 || echo 1)"
assert "psql was never called" "$([ ! -s "${PSQL_LOG}" ] && echo 0 || echo 1)"
rm -rf "${SANDBOX}"

echo "=== Test 3: corrupt/truncated gzip is refused before DROP ==="
make_sandbox
BACKUP_FILE="${SANDBOX}/corrupt.sql.gz"
run_restore DB_HOST=127.0.0.1 DB_NAME=payverge_restore_drill \
  RESTORE_SCOPE=isolated RESTORE_CONFIRM='DROP 127.0.0.1:5432/payverge_restore_drill'
RC=$?
assert "restore exits nonzero" "$([ "$RC" -ne 0 ] && echo 0 || echo 1)"
assert "psql was never called" "$([ ! -s "${PSQL_LOG}" ] && echo 0 || echo 1)"
assert "stderr reports integrity failure" "$(grep -Eqi 'gzip|corrupt|integrity|truncated' "${SANDBOX}/stderr" && echo 0 || echo 1)"
rm -rf "${SANDBOX}"

echo "=== Test 4: confirmation must exactly bind host, port, and database ==="
make_sandbox
BACKUP_FILE="${SANDBOX}/valid.sql.gz"
run_restore DB_HOST=127.0.0.1 DB_PORT=55432 DB_NAME=payverge_restore_drill \
  RESTORE_SCOPE=isolated RESTORE_CONFIRM='DROP 127.0.0.1:5432/payverge_restore_drill'
RC=$?
assert "restore exits nonzero" "$([ "$RC" -ne 0 ] && echo 0 || echo 1)"
assert "psql was never called" "$([ ! -s "${PSQL_LOG}" ] && echo 0 || echo 1)"
rm -rf "${SANDBOX}"

echo "=== Test 5: isolated loopback target with exact confirmation restores ==="
make_sandbox
BACKUP_FILE="${SANDBOX}/valid.sql.gz"
run_restore DB_HOST=127.0.0.1 DB_PORT=55432 DB_NAME=payverge_restore_drill \
  RESTORE_SCOPE=isolated RESTORE_CONFIRM='DROP 127.0.0.1:55432/payverge_restore_drill'
RC=$?
assert "restore exits zero" "$([ "$RC" -eq 0 ] && echo 0 || echo 1)"
assert "DROP targets only confirmed database" "$(grep -q 'DROP DATABASE IF EXISTS.*payverge_restore_drill' "${PSQL_LOG}" && echo 0 || echo 1)"
assert "restore uses ON_ERROR_STOP" "$(grep -q 'ON_ERROR_STOP=1' "${PSQL_LOG}" && echo 0 || echo 1)"
assert "scratch clone is sanitized" "$(grep -q 'sanitize-clone.sql' "${PSQL_LOG}" && echo 0 || echo 1)"
rm -rf "${SANDBOX}"

echo
if [ "${FAILURES}" -ne 0 ]; then
  echo "${FAILURES} ASSERTION(S) FAILED"
  exit 1
fi
echo "ALL TESTS PASSED"
