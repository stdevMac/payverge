#!/usr/bin/env bash
# Automated off-host backup restore drill for Payverge.
#
# Downloads the newest object under s3://${S3_BACKUP_BUCKET}/${S3_BACKUP_PREFIX}/,
# restores it into a throwaway Postgres container bound to 127.0.0.1 on a random
# free port, verifies schema + cheap invariants, writes evidence, and ALWAYS
# destroys scratch resources (container + download dir) via EXIT trap.
#
# NEVER points at a production DB host — only the scratch container.
#
# Environment:
#   S3_BACKUP_BUCKET     (required) backup bucket name
#   S3_BACKUP_PREFIX     (default: db)
#   S3_BACKUP_ENDPOINT   (optional; --endpoint-url for R2/etc.)
#   PG_IMAGE             (default: postgres:18-alpine)
#   BACKEND_IMAGE        (required) immutable candidate image@sha256:<digest>
#   SOURCE_SHA           (required) exact candidate Git SHA
#   PROOF_WORKFLOW_RUN_ID / PROOF_WORKFLOW_RUN_URL (required)
#   PROOF_EXECUTION_ENVIRONMENT (must be github_hosted)
#   PROOF_RUNNER_LABEL   (must be ubuntu-latest)
#   EVIDENCE_DIR         (default: mktemp -d)
#   MIN_TABLES           (default: 100)
#   SCRATCH_DIR          (optional override; default mktemp -d)
#   CONTAINER_NAME       (optional override; default pv-restore-drill-$$-<rand>)
#
# Exit 0 only when download + gzip integrity + restore + sanitization + ALL
# verifications pass.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

S3_BACKUP_BUCKET="${S3_BACKUP_BUCKET:-}"
S3_BACKUP_PREFIX="${S3_BACKUP_PREFIX:-db}"
S3_BACKUP_ENDPOINT="${S3_BACKUP_ENDPOINT:-}"
PG_IMAGE="${PG_IMAGE:-postgres:18-alpine}"
BACKEND_IMAGE="${BACKEND_IMAGE:-}"
SOURCE_SHA="${SOURCE_SHA:-}"
PROOF_WORKFLOW_RUN_ID="${PROOF_WORKFLOW_RUN_ID:-}"
PROOF_WORKFLOW_RUN_URL="${PROOF_WORKFLOW_RUN_URL:-}"
PROOF_EXECUTION_ENVIRONMENT="${PROOF_EXECUTION_ENVIRONMENT:-}"
PROOF_RUNNER_LABEL="${PROOF_RUNNER_LABEL:-}"
MIN_TABLES="${MIN_TABLES:-100}"
MAX_RTO_SECONDS="${MAX_RTO_SECONDS:-1800}"
BACKEND_WAIT_SECONDS="${BACKEND_WAIT_SECONDS:-120}"
EVIDENCE_DIR_SET="${EVIDENCE_DIR+x}"
EVIDENCE_DIR="${EVIDENCE_DIR:-}"

PG_USER="payverge"
PG_PASSWORD="drill_password"
PG_DB="payverge"
PG_HOST="127.0.0.1"

SCRATCH_DIR="${SCRATCH_DIR:-}"
CONTAINER_NAME="${CONTAINER_NAME:-}"
BACKEND_CONTAINER_NAME=""
NETWORK_NAME=""
HOST_PORT=""
BACKEND_HOST_PORT=""
OBJECT_KEY=""
OBJECT_TS=""
OBJECT_EPOCH=""
OBJECT_DIGEST=""
OBJECT_SIZE_BYTES=""
GZIP_INTEGRITY="fail"
OFFSITE_HEAD_VERIFICATION="fail"
DOWNLOAD_START_EPOCH=""
VERIFY_DONE_EPOCH=""
MIGRATION_VERSION=""
MIGRATION_DIRTY=""
TABLE_COUNT=""
COUNT_BUSINESSES=""
COUNT_BILLS=""
COUNT_PAYMENTS=""
NEGATIVE_BILL_AMOUNT_COUNT=""
RPO_SECONDS=""
RTO_SECONDS=""
BACKEND_IMAGE_NAME=""
BACKEND_IMAGE_DIGEST=""
BACKEND_LIVE_PROBE="fail"
BACKEND_READY_PROBE="fail"
AUTH_RESULT="fail"
AUTH_LOGIN_HTTP_STATUS="null"
AUTH_LOGIN_SUCCESS="false"
AUTH_SESSION_ISSUED="false"
AUTHENTICATED_REQUEST_HTTP_STATUS="null"
AUTH_PRINCIPAL_MATCH="false"
VERIFY_FAILURES=""
RESULT="fail"
SANITIZED="false"
EVIDENCE_BASE=""

# ---------------------------------------------------------------------------
# Cleanup — ALWAYS destroy scratch container + download dir (success or fail).
# Evidence directory is intentionally retained.
# ---------------------------------------------------------------------------
cleanup() {
  # Never alter the process exit status: ignore errors; EXIT trap must not fail.
  set +e
  if [ -n "${BACKEND_CONTAINER_NAME}" ]; then
    docker rm -f "${BACKEND_CONTAINER_NAME}" >/dev/null 2>&1 || true
  fi
  if [ -n "${CONTAINER_NAME}" ]; then
    docker rm -f "${CONTAINER_NAME}" >/dev/null 2>&1 || true
  fi
  if [ -n "${NETWORK_NAME}" ]; then
    docker network rm "${NETWORK_NAME}" >/dev/null 2>&1 || true
  fi
  if [ -n "${SCRATCH_DIR}" ] && [ -d "${SCRATCH_DIR}" ]; then
    rm -rf "${SCRATCH_DIR}"
  fi
}
trap cleanup EXIT

die() {
  echo "ERROR: $*" >&2
  exit 1
}

record_verify_fail() {
  local msg="$1"
  echo "VERIFY FAIL: ${msg}" >&2
  if [ -z "${VERIFY_FAILURES}" ]; then
    VERIFY_FAILURES="${msg}"
  else
    VERIFY_FAILURES="${VERIFY_FAILURES}; ${msg}"
  fi
}

build_aws_base() {
  AWS_BASE=(aws)
  if [ -n "${S3_BACKUP_ENDPOINT}" ]; then
    AWS_BASE+=(--endpoint-url "${S3_BACKUP_ENDPOINT}")
  fi
}

# Portable free port on 127.0.0.1.
find_free_port() {
  if command -v python3 >/dev/null 2>&1; then
    python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1]); s.close()'
    return 0
  fi
  if command -v python >/dev/null 2>&1; then
    python -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1]); s.close()'
    return 0
  fi
  # Fallback: high ephemeral-ish range ($$ + $RANDOM avoid Math.random).
  echo $((49152 + ($$ + ${RANDOM:-0}) % 16000))
}

# Parse "YYYY-MM-DD HH:MM:SS" (aws s3 ls) to epoch seconds (UTC).
parse_s3_ls_epoch() {
  local date_part="$1"
  local time_part="$2"
  local epoch=""
  epoch=$(date -u -d "${date_part} ${time_part}" +%s 2>/dev/null || true)
  if [ -z "${epoch}" ]; then
    epoch=$(date -j -u -f "%Y-%m-%d %H:%M:%S" "${date_part} ${time_part}" +%s 2>/dev/null || true)
  fi
  echo "${epoch}"
}

format_epoch_iso() {
  local epoch="$1" iso=""
  iso=$(date -u -d "@${epoch}" +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || true)
  if [ -z "${iso}" ]; then
    iso=$(date -j -u -r "${epoch}" +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || true)
  fi
  echo "${iso}"
}

sha256_file() {
  local file="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "${file}" | awk '{print $1}'
    return
  fi
  shasum -a 256 "${file}" | awk '{print $1}'
}

# Trim whitespace / newlines from psql -t output.
trim() {
  # shellcheck disable=SC2001
  echo "$1" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//'
}

psql_scratch() {
  # All DB traffic goes only to the scratch container on 127.0.0.1:$HOST_PORT.
  PGPASSWORD="${PG_PASSWORD}" psql \
    --host="${PG_HOST}" \
    --port="${HOST_PORT}" \
    --username="${PG_USER}" \
    --dbname="${PG_DB}" \
    --set ON_ERROR_STOP=1 \
    "$@"
}

psql_scalar() {
  local sql="$1"
  local out
  out=$(psql_scratch -t -A -c "${sql}" 2>/dev/null || true)
  trim "${out}"
}

is_dirty_false() {
  local v
  v=$(echo "$1" | tr '[:upper:]' '[:lower:]')
  case "${v}" in
    f|false|0) return 0 ;;
    *) return 1 ;;
  esac
}

json_escape() {
  # Minimal JSON string escape for evidence fields (no nested objects).
  printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

sql_escape() {
  printf '%s' "$1" | sed "s/'/''/g"
}

seed_restore_admin() {
  local repo_root auth_email auth_password auth_hash email_sql hash_sql
  repo_root="$(cd "${SCRIPT_DIR}/../.." && pwd)"
  auth_email="restore-drill-${PROOF_WORKFLOW_RUN_ID}@example.invalid"
  if ! auth_password="$(openssl rand -hex 24)"; then
    record_verify_fail "scratch authentication principal credential generation failed"
    return 1
  fi
  if ! auth_hash="$(cd "${repo_root}/backend" && go run ./cmd/hash-password -- "${auth_password}")"; then
    record_verify_fail "scratch authentication principal password hashing failed"
    return 1
  fi
  if [ -z "${auth_hash}" ] || [[ "${auth_hash}" != \$2a\$* && "${auth_hash}" != \$2b\$* ]]; then
    record_verify_fail "scratch authentication principal password hashing failed"
    return 1
  fi
  email_sql="$(sql_escape "${auth_email}")"
  hash_sql="$(sql_escape "${auth_hash}")"
  if ! psql_scratch >/dev/null <<SQL
BEGIN;
DELETE FROM user_auths
 WHERE provider = 'email'
   AND LOWER(provider_user_id) = LOWER('${email_sql}');
DELETE FROM users WHERE LOWER(email) = LOWER('${email_sql}');
INSERT INTO users (
  email, name, role, auth_method, email_verified, email_enabled,
  news_enabled, updates_enabled, transactional_enabled, security_enabled,
  reports_enabled, statistics_enabled, created_at, updated_at
) VALUES (
  '${email_sql}', 'Restore Drill', 'admin', 'email', true, true,
  true, true, true, true, true, true, NOW(), NOW()
);
INSERT INTO user_auths (
  user_id, provider, password_hash, provider_user_id, email_verified,
  created_at, updated_at
)
SELECT id, 'email', '${hash_sql}', '${email_sql}', true, NOW(), NOW()
  FROM users WHERE LOWER(email) = LOWER('${email_sql}');
DO \$\$
DECLARE seeded_count integer;
BEGIN
  SELECT count(*) INTO seeded_count
    FROM users u
    JOIN user_auths a ON a.user_id = u.id
   WHERE LOWER(u.email) = LOWER('${email_sql}')
     AND u.role = 'admin'
     AND u.email_verified = true
     AND a.provider = 'email'
     AND a.email_verified = true
     AND a.password_hash LIKE '\$2%';
  IF seeded_count <> 1 THEN
    RAISE EXCEPTION 'restore drill admin seed incomplete';
  END IF;
END
\$\$;
COMMIT;
SQL
  then
    record_verify_fail "scratch authentication principal seed failed"
    return 1
  fi
  AUTH_SMOKE_EMAIL="${auth_email}"
  AUTH_SMOKE_PASSWORD="${auth_password}"
  return 0
}

wait_for_backend_probe() {
  local path="$1" elapsed=0 running
  while [ "${elapsed}" -lt "${BACKEND_WAIT_SECONDS}" ]; do
    if curl -fsS --max-time 5 "http://127.0.0.1:${BACKEND_HOST_PORT}${path}" >/dev/null 2>&1; then
      return 0
    fi
    running="$(docker inspect --format='{{.State.Running}}' "${BACKEND_CONTAINER_NAME}" 2>/dev/null || true)"
    if [ "${running}" != "true" ]; then
      return 1
    fi
    sleep 1
    elapsed=$((elapsed + 1))
  done
  return 1
}

emit_candidate_diagnostics() {
  echo "candidate backend state (sanitized):" >&2
  docker inspect \
    --format 'status={{.State.Status}} exit_code={{.State.ExitCode}} oom_killed={{.State.OOMKilled}} error={{.State.Error}}' \
    "${BACKEND_CONTAINER_NAME}" >&2 || true
  echo "candidate backend startup log tail (sanitized):" >&2
  docker logs --tail 200 "${BACKEND_CONTAINER_NAME}" 2>&1 \
    | sed 's/drill_password/[REDACTED]/g' >&2 || true
}

run_candidate_auth_smoke() {
  local login_body me_body login_code me_code session_value
  if ! seed_restore_admin; then
    return 1
  fi
  BACKEND_HOST_PORT="$(find_free_port)"
  if ! docker pull "${BACKEND_IMAGE}" >/dev/null; then
    record_verify_fail "immutable candidate backend image pull failed"
    return 1
  fi
  # Startup requires an RPC URL even outside production. Keep recovery fully
  # isolated: the client may initialize, but no real chain endpoint is reachable.
  # The deployed legacy image reads S3 values directly from flags. Its fixed
  # "secret" canaries below are deliberately non-credentials and can reach only
  # loopback; no production or GitHub secret is ever copied into container argv.
  if ! docker run -d \
    --name "${BACKEND_CONTAINER_NAME}" \
    --network "${NETWORK_NAME}" \
    -p "127.0.0.1:${BACKEND_HOST_PORT}:8080" \
    -e ENV=development \
    -e APP_ENV=development \
    -e JWT_SECRET_KEY=restore_drill_ephemeral_jwt_secret_2026 \
    -e FROM_EMAIL=restore-drill@example.invalid \
    -e FROM_EMAIL_UPDATES=restore-drill@example.invalid \
    "${BACKEND_IMAGE}" \
    --db-host "${CONTAINER_NAME}" \
    --db-port 5432 \
    --db-user "${PG_USER}" \
    --db-password "${PG_PASSWORD}" \
    --db-name "${PG_DB}" \
    --db-sslmode disable \
    --rpc-url http://127.0.0.1:1 \
    --s3-bucket restore-drill-public \
    --aws-access-key restore-drill \
    --aws-secret-key restore-drill-nonsecret-canary \
    --s3-endpoint http://127.0.0.1:1 \
    --s3-protected-bucket restore-drill-protected \
    --aws-protected-access-key restore-drill \
    --aws-protected-secret-key restore-drill-nonsecret-canary \
    --s3-protected-endpoint http://127.0.0.1:1 >/dev/null; then
    record_verify_fail "immutable candidate backend failed to start"
    return 1
  fi
  if wait_for_backend_probe "/api/v1/health/live"; then
    BACKEND_LIVE_PROBE="pass"
  else
    emit_candidate_diagnostics
    record_verify_fail "candidate backend live probe timed out"
    return 1
  fi
  if wait_for_backend_probe "/api/v1/health/ready"; then
    BACKEND_READY_PROBE="pass"
  else
    emit_candidate_diagnostics
    record_verify_fail "candidate backend ready probe timed out"
    return 1
  fi

  login_body="${SCRATCH_DIR}/login-response.json"
  me_body="${SCRATCH_DIR}/me-response.json"
  if ! login_code=$(curl -sS --max-time 15 -o "${login_body}" -w '%{http_code}' \
    -H 'Content-Type: application/json' \
    -d "{\"email\":\"${AUTH_SMOKE_EMAIL}\",\"password\":\"${AUTH_SMOKE_PASSWORD}\"}" \
    "http://127.0.0.1:${BACKEND_HOST_PORT}/api/v1/auth/login"); then
    record_verify_fail "candidate backend login request failed"
    return 1
  fi
  AUTH_LOGIN_HTTP_STATUS="${login_code}"
  if [ "${login_code}" != "200" ] || ! jq -e '.success == true and (.token | type == "string" and length > 0)' "${login_body}" >/dev/null 2>&1; then
    record_verify_fail "candidate backend login did not issue a successful session"
    return 1
  fi
  AUTH_LOGIN_SUCCESS="true"
  AUTH_SESSION_ISSUED="true"
  session_value="$(jq -r '.token' "${login_body}")"
  if ! me_code=$(curl -sS --max-time 15 -o "${me_body}" -w '%{http_code}' \
    -H "Authorization: Bearer ${session_value}" \
    "http://127.0.0.1:${BACKEND_HOST_PORT}/api/v1/auth/me"); then
    unset session_value
    record_verify_fail "candidate backend authenticated request failed"
    return 1
  fi
  AUTHENTICATED_REQUEST_HTTP_STATUS="${me_code}"
  if [ "${me_code}" != "200" ] || ! jq -e --arg email "${AUTH_SMOKE_EMAIL}" '.email == $email' "${me_body}" >/dev/null 2>&1; then
    unset session_value
    record_verify_fail "candidate backend authenticated principal did not match"
    return 1
  fi
  AUTH_PRINCIPAL_MATCH="true"
  AUTH_RESULT="pass"
  unset session_value AUTH_SMOKE_PASSWORD AUTH_SMOKE_EMAIL
  rm -f "${login_body}" "${me_body}"
  return 0
}

write_evidence() {
  local utc_stamp
  utc_stamp=$(date -u +"%Y%m%dT%H%M%SZ")
  mkdir -p "${EVIDENCE_DIR}"
  EVIDENCE_BASE="${EVIDENCE_DIR}/restore-drill-${utc_stamp}"

  local failures_json="null"
  if [ -n "${VERIFY_FAILURES}" ]; then
    failures_json="\"$(json_escape "${VERIFY_FAILURES}")\""
  fi

  cat > "${EVIDENCE_BASE}.json" <<EOF
{
  "schema_version": 2,
  "source_sha": "$(json_escape "${SOURCE_SHA}")",
  "workflow_run_id": "$(json_escape "${PROOF_WORKFLOW_RUN_ID}")",
  "workflow_run_url": "$(json_escape "${PROOF_WORKFLOW_RUN_URL}")",
  "result": "$(json_escape "${RESULT}")",
  "sanitized": ${SANITIZED},
  "object_key": "$(json_escape "${OBJECT_KEY}")",
  "object_timestamp": "$(json_escape "${OBJECT_TS}")",
  "object_digest": "sha256:$(json_escape "${OBJECT_DIGEST}")",
  "object_size_bytes": ${OBJECT_SIZE_BYTES:-null},
  "gzip_integrity": "$(json_escape "${GZIP_INTEGRITY}")",
  "offsite_head_verification": "$(json_escape "${OFFSITE_HEAD_VERIFICATION}")",
  "rpo_seconds": ${RPO_SECONDS:-null},
  "duration_seconds": ${RTO_SECONDS:-null},
  "rto_seconds": ${RTO_SECONDS:-null},
  "migration_version": $(if [ -n "${MIGRATION_VERSION}" ]; then echo "\"$(json_escape "${MIGRATION_VERSION}")\""; else echo null; fi),
  "migration_dirty": $(if [ -n "${MIGRATION_DIRTY}" ]; then echo "\"$(json_escape "${MIGRATION_DIRTY}")\""; else echo null; fi),
  "table_count": ${TABLE_COUNT:-null},
  "counts": {
    "businesses": ${COUNT_BUSINESSES:-null},
    "bills": ${COUNT_BILLS:-null},
    "payments": ${COUNT_PAYMENTS:-null},
    "bills_negative_total_amount": ${NEGATIVE_BILL_AMOUNT_COUNT:-null}
  },
  "min_tables": ${MIN_TABLES},
  "recovery_origin": {
    "execution_environment": "$(json_escape "${PROOF_EXECUTION_ENVIRONMENT}")",
    "runner_label": "$(json_escape "${PROOF_RUNNER_LABEL}")",
    "backup_source": "offsite_object_storage",
    "restore_target": "ephemeral_loopback_postgres",
    "production_host_contacted": false,
    "production_database_contacted": false
  },
  "candidate_backend": {
    "image": "$(json_escape "${BACKEND_IMAGE_NAME}")",
    "digest": "$(json_escape "${BACKEND_IMAGE_DIGEST}")",
    "live_probe": "$(json_escape "${BACKEND_LIVE_PROBE}")",
    "ready_probe": "$(json_escape "${BACKEND_READY_PROBE}")"
  },
  "authentication_smoke": {
    "result": "$(json_escape "${AUTH_RESULT}")",
    "method": "email_password",
    "login_http_status": ${AUTH_LOGIN_HTTP_STATUS},
    "login_success": ${AUTH_LOGIN_SUCCESS},
    "session_issued": ${AUTH_SESSION_ISSUED},
    "authenticated_request_http_status": ${AUTHENTICATED_REQUEST_HTTP_STATUS},
    "principal_match": ${AUTH_PRINCIPAL_MATCH},
    "response_body_retained": false
  },
  "verify_failures": ${failures_json},
  "generated_at_utc": "$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
}
EOF

  cat > "${EVIDENCE_BASE}.md" <<EOF
# Restore drill evidence

| Field | Value |
|---|---|
| Result | ${RESULT} |
| Sanitized (PII scrubbed) | ${SANITIZED} |
| Object key | ${OBJECT_KEY} |
| Object timestamp | ${OBJECT_TS} |
| RPO (seconds) | ${RPO_SECONDS:-n/a} |
| Duration / RTO (seconds) | ${RTO_SECONDS:-n/a} |
| Migration version | ${MIGRATION_VERSION:-n/a} |
| Migration dirty | ${MIGRATION_DIRTY:-n/a} |
| Table count (public) | ${TABLE_COUNT:-n/a} |
| businesses count | ${COUNT_BUSINESSES:-n/a} |
| bills count | ${COUNT_BILLS:-n/a} |
| payments count | ${COUNT_PAYMENTS:-n/a} |
| bills with total_amount < 0 | ${NEGATIVE_BILL_AMOUNT_COUNT:-n/a} |
| MIN_TABLES | ${MIN_TABLES} |
| Candidate backend | ${BACKEND_IMAGE_NAME}@${BACKEND_IMAGE_DIGEST} |
| Source SHA | ${SOURCE_SHA} |
| Workflow run | ${PROOF_WORKFLOW_RUN_URL} |
| Scratch host port | ${HOST_PORT:-n/a} |
| Container name | ${CONTAINER_NAME} |
| Backend live / ready | ${BACKEND_LIVE_PROBE} / ${BACKEND_READY_PROBE} |
| Authentication smoke | ${AUTH_RESULT} |
| Verify failures | ${VERIFY_FAILURES:-none} |
| Generated (UTC) | $(date -u +"%Y-%m-%dT%H:%M:%SZ") |

Non-sensitive aggregates and pass/fail assertions only. Authentication
credentials, session material, response bodies, and restored rows are not retained.
Scratch resources (container + download dir) are destroyed on EXIT.
EOF

  echo "Evidence written:"
  echo "  ${EVIDENCE_BASE}.json"
  echo "  ${EVIDENCE_BASE}.md"
}

# ---------------------------------------------------------------------------
# Preconditions
# ---------------------------------------------------------------------------
if [ -z "${S3_BACKUP_BUCKET}" ]; then
  die "S3_BACKUP_BUCKET is required"
fi
if ! [[ "${BACKEND_IMAGE}" =~ ^[^[:space:]@]+@sha256:[a-f0-9]{64}$ ]]; then
  die "BACKEND_IMAGE must be an immutable image@sha256:<64 lowercase hex> reference"
fi
BACKEND_IMAGE_NAME="${BACKEND_IMAGE%@sha256:*}"
BACKEND_IMAGE_DIGEST="${BACKEND_IMAGE##*@}"
if ! [[ "${SOURCE_SHA}" =~ ^[a-f0-9]{40}$ ]]; then
  die "SOURCE_SHA must be an exact 40-character lowercase Git SHA"
fi
if ! [[ "${PROOF_WORKFLOW_RUN_ID}" =~ ^[1-9][0-9]*$ ]]; then
  die "PROOF_WORKFLOW_RUN_ID must be a positive GitHub Actions run ID"
fi
if ! [[ "${PROOF_WORKFLOW_RUN_URL}" =~ ^https://github\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+/actions/runs/${PROOF_WORKFLOW_RUN_ID}$ ]]; then
  die "PROOF_WORKFLOW_RUN_URL must be the GitHub Actions URL of run ${PROOF_WORKFLOW_RUN_ID}"
fi
if [ "${PROOF_EXECUTION_ENVIRONMENT}" != "github_hosted" ] || [ "${PROOF_RUNNER_LABEL}" != "ubuntu-latest" ]; then
  die "restore proof must run on the GitHub-hosted ubuntu-latest recovery runner"
fi
if ! [[ "${MAX_RTO_SECONDS}" =~ ^[0-9]+$ ]] || [ "${MAX_RTO_SECONDS}" -gt 1800 ]; then
  die "MAX_RTO_SECONDS must be an integer no greater than 1800"
fi

if [ -z "${EVIDENCE_DIR_SET}" ] || [ -z "${EVIDENCE_DIR}" ]; then
  EVIDENCE_DIR=$(mktemp -d "${TMPDIR:-/tmp}/pv-restore-evidence.XXXXXX")
fi
mkdir -p "${EVIDENCE_DIR}"

if [ -z "${SCRATCH_DIR}" ]; then
  SCRATCH_DIR=$(mktemp -d "${TMPDIR:-/tmp}/pv-restore-drill.XXXXXX")
else
  mkdir -p "${SCRATCH_DIR}"
fi

if [ -z "${CONTAINER_NAME}" ]; then
  # Unique name without Math.random: $$ + $RANDOM + mktemp suffix fragment.
  _rand_suffix="${RANDOM:-0}$$"
  _tmp_frag=$(mktemp -u XXXXXX 2>/dev/null || echo "x$$")
  CONTAINER_NAME="pv-restore-drill-$$-${_rand_suffix}-${_tmp_frag}"
fi
BACKEND_CONTAINER_NAME="${CONTAINER_NAME}-backend"
NETWORK_NAME="${CONTAINER_NAME}-network"

CHECKED_OUT_SHA=$(git rev-parse HEAD 2>/dev/null || echo "unknown")
if [ "${CHECKED_OUT_SHA}" != "${SOURCE_SHA}" ]; then
  die "SOURCE_SHA does not match the checked-out candidate"
fi

build_aws_base

echo "=== Payverge backup restore drill ==="
echo "Bucket:   s3://${S3_BACKUP_BUCKET}/${S3_BACKUP_PREFIX}/"
echo "PG image: ${PG_IMAGE}"
echo "Evidence: ${EVIDENCE_DIR}"
echo "Scratch:  ${SCRATCH_DIR}"
echo "Container:${CONTAINER_NAME}"
echo ""

# ---------------------------------------------------------------------------
# 1. Pick newest remote object
# ---------------------------------------------------------------------------
echo "[$(date -u +%H:%M:%S)] Listing remote backups..."
LISTING=$("${AWS_BASE[@]}" s3 ls "s3://${S3_BACKUP_BUCKET}/${S3_BACKUP_PREFIX}/" 2>/dev/null || true)
if [ -z "${LISTING}" ]; then
  die "no remote backup objects under s3://${S3_BACKUP_BUCKET}/${S3_BACKUP_PREFIX}/"
fi

# Newest by lexical sort of date+time columns (aws s3 ls format).
LATEST_LINE=$(echo "${LISTING}" | sort | tail -1)
OBJECT_KEY_BASENAME=$(echo "${LATEST_LINE}" | awk '{print $4}')
OBJECT_DATE=$(echo "${LATEST_LINE}" | awk '{print $1}')
OBJECT_TIME=$(echo "${LATEST_LINE}" | awk '{print $2}')
if [ -z "${OBJECT_KEY_BASENAME}" ] || [ -z "${OBJECT_DATE}" ] || [ -z "${OBJECT_TIME}" ]; then
  die "could not parse newest object from listing: ${LATEST_LINE}"
fi
OBJECT_KEY="${S3_BACKUP_PREFIX}/${OBJECT_KEY_BASENAME}"
OBJECT_TS="${OBJECT_DATE} ${OBJECT_TIME}"
OBJECT_EPOCH=$(parse_s3_ls_epoch "${OBJECT_DATE}" "${OBJECT_TIME}")
if [ -z "${OBJECT_EPOCH}" ]; then
  die "could not parse object timestamp '${OBJECT_TS}'"
fi
OBJECT_TS=$(format_epoch_iso "${OBJECT_EPOCH}")
if [ -z "${OBJECT_TS}" ]; then
  die "could not format object timestamp epoch '${OBJECT_EPOCH}'"
fi
echo "Newest object: s3://${S3_BACKUP_BUCKET}/${OBJECT_KEY} (${OBJECT_TS})"

if ! "${AWS_BASE[@]}" s3api head-object --bucket "${S3_BACKUP_BUCKET}" --key "${OBJECT_KEY}" >/dev/null; then
  die "off-site head verification failed for s3://${S3_BACKUP_BUCKET}/${OBJECT_KEY}"
fi
OFFSITE_HEAD_VERIFICATION="pass"

# ---------------------------------------------------------------------------
# 2. Download + gzip integrity
# ---------------------------------------------------------------------------
DOWNLOAD_START_EPOCH=$(date -u +%s)
LOCAL_FILE="${SCRATCH_DIR}/${OBJECT_KEY_BASENAME}"
echo "[$(date -u +%H:%M:%S)] Downloading..."
"${AWS_BASE[@]}" s3 cp "s3://${S3_BACKUP_BUCKET}/${OBJECT_KEY}" "${LOCAL_FILE}"
if [ ! -f "${LOCAL_FILE}" ]; then
  die "download failed — file missing: ${LOCAL_FILE}"
fi
if ! gzip -t "${LOCAL_FILE}"; then
  die "gzip integrity check failed (corrupt/truncated dump): ${LOCAL_FILE}"
fi
GZIP_INTEGRITY="pass"
OBJECT_SIZE_BYTES=$(wc -c < "${LOCAL_FILE}" | tr -d '[:space:]')
OBJECT_DIGEST=$(sha256_file "${LOCAL_FILE}")
if ! [[ "${OBJECT_SIZE_BYTES}" =~ ^[1-9][0-9]*$ ]] || ! [[ "${OBJECT_DIGEST}" =~ ^[a-f0-9]{64}$ ]]; then
  die "could not bind downloaded backup bytes to size and SHA-256 evidence"
fi
echo "gzip OK"

# ---------------------------------------------------------------------------
# 3. Isolated scratch Postgres on random free localhost port
# ---------------------------------------------------------------------------
echo "[$(date -u +%H:%M:%S)] Creating isolated recovery network..."
docker network create "${NETWORK_NAME}" >/dev/null
HOST_PORT=$(find_free_port)
if [ -z "${HOST_PORT}" ]; then
  die "could not allocate a free host port"
fi
echo "[$(date -u +%H:%M:%S)] Starting scratch Postgres on 127.0.0.1:${HOST_PORT} (${PG_IMAGE})..."
docker run -d \
  --name "${CONTAINER_NAME}" \
  --network "${NETWORK_NAME}" \
  -e POSTGRES_USER="${PG_USER}" \
  -e POSTGRES_PASSWORD="${PG_PASSWORD}" \
  -e POSTGRES_DB="${PG_DB}" \
  -p "127.0.0.1:${HOST_PORT}:5432" \
  "${PG_IMAGE}" >/dev/null

# Wait for readiness (host-side pg_isready → scratch only).
echo "[$(date -u +%H:%M:%S)] Waiting for pg_isready..."
ready=0
i=0
while [ "${i}" -lt 60 ]; do
  if pg_isready -h "${PG_HOST}" -p "${HOST_PORT}" -U "${PG_USER}" -d "${PG_DB}" >/dev/null 2>&1; then
    ready=1
    break
  fi
  i=$((i + 1))
  sleep 1
done
if [ "${ready}" -ne 1 ]; then
  die "scratch Postgres not ready within 60s on 127.0.0.1:${HOST_PORT}"
fi
echo "scratch postgres ready"

# ---------------------------------------------------------------------------
# 4. Restore into fresh scratch DB
# ---------------------------------------------------------------------------
echo "[$(date -u +%H:%M:%S)] Restoring dump (ON_ERROR_STOP=1)..."
if ! gunzip -c "${LOCAL_FILE}" | PGPASSWORD="${PG_PASSWORD}" psql \
  --host="${PG_HOST}" \
  --port="${HOST_PORT}" \
  --username="${PG_USER}" \
  --dbname="${PG_DB}" \
  --set ON_ERROR_STOP=1 >/dev/null; then
  die "restore failed (psql ON_ERROR_STOP)"
fi
echo "restore complete"

# ---------------------------------------------------------------------------
# 4.5 Sanitize — scrub PII/secrets from the clone BEFORE it is verified/used.
# A restored production dump carries real customer/staff PII, wallet/financial
# identifiers, auth secrets, provider tokens, and PII-bearing free-text. The
# drill clone must never retain any of it (Priority-4 gate (e): sanitized clone).
# ON_ERROR_STOP=1 means a missing table/column aborts loudly rather than leaving
# an unscrubbed column.
# ---------------------------------------------------------------------------
SANITIZE_SQL="${SCRIPT_DIR}/sanitize-clone.sql"
echo "[$(date -u +%H:%M:%S)] Sanitizing clone (PII/secret scrub)..."
if [ ! -f "${SANITIZE_SQL}" ]; then
  die "sanitize script missing: ${SANITIZE_SQL}"
fi
if ! psql_scratch -f "${SANITIZE_SQL}" >/dev/null; then
  die "sanitization scrub failed (clone NOT safe to use)"
fi
SANITIZED="true"
echo "clone sanitized"

# ---------------------------------------------------------------------------
# 5. Verify (record failures, then exit nonzero if any)
# ---------------------------------------------------------------------------
echo "[$(date -u +%H:%M:%S)] Verifying..."

# Post-scrub PII-absence assertion: every identity email must be redacted to the
# reserved .invalid TLD. A nonzero count means the scrub left routable PII behind.
PII_LEAK=$(psql_scalar "SELECT (SELECT count(*) FROM users WHERE email NOT LIKE '%@example.invalid') + (SELECT count(*) FROM customers WHERE email NOT LIKE '%@example.invalid') + (SELECT count(*) FROM staff WHERE email NOT LIKE '%@example.invalid');")
if [ -z "${PII_LEAK}" ]; then
  record_verify_fail "could not verify PII scrub (identity email TLD check)"
elif [ "${PII_LEAK}" != "0" ]; then
  record_verify_fail "PII scrub incomplete: ${PII_LEAK} identity emails still routable"
fi
echo "pii scrub check: ${PII_LEAK:-error} unredacted identity emails"

# schema_migrations: at most one row, dirty=false. Zero rows is a head-0
# genesis baseline (one with no numbered migrations).
MIG_COUNT=$(psql_scalar "SELECT count(*) FROM schema_migrations;")
case "${MIG_COUNT}" in
  0)
    MIGRATION_VERSION="0"
    MIGRATION_DIRTY="f"
    ;;
  1)
    MIG_ROW=$(psql_scalar "SELECT version || '|' || dirty::text FROM schema_migrations LIMIT 1;")
    if [ -n "${MIG_ROW}" ]; then
      MIGRATION_VERSION="${MIG_ROW%%|*}"
      MIGRATION_DIRTY="${MIG_ROW#*|}"
    else
      MIGRATION_VERSION=""
      MIGRATION_DIRTY=""
    fi
    ;;
  *)
    record_verify_fail "schema_migrations row count expected 0 or 1, got '${MIG_COUNT}'"
    MIGRATION_VERSION=""
    MIGRATION_DIRTY=""
    ;;
esac

if [ -z "${MIGRATION_VERSION}" ]; then
  record_verify_fail "could not read schema_migrations.version"
fi
if ! is_dirty_false "${MIGRATION_DIRTY}"; then
  record_verify_fail "schema_migrations.dirty expected false, got '${MIGRATION_DIRTY}'"
fi
echo "migration: version=${MIGRATION_VERSION} dirty=${MIGRATION_DIRTY}"

# public table count
TABLE_COUNT=$(psql_scalar "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public';")
if [ -z "${TABLE_COUNT}" ] || [ "${TABLE_COUNT}" -lt "${MIN_TABLES}" ] 2>/dev/null; then
  record_verify_fail "public table count ${TABLE_COUNT:-empty} < MIN_TABLES=${MIN_TABLES}"
fi
echo "tables: ${TABLE_COUNT} (min ${MIN_TABLES})"

# Cheap invariants + recorded aggregates (counts only)
NEGATIVE_BILL_AMOUNT_COUNT=$(psql_scalar "SELECT count(*) FROM bills WHERE total_amount < 0;")
if [ -z "${NEGATIVE_BILL_AMOUNT_COUNT}" ]; then
  record_verify_fail "could not query bills.total_amount invariant"
elif [ "${NEGATIVE_BILL_AMOUNT_COUNT}" != "0" ]; then
  record_verify_fail "bills with total_amount < 0 expected 0, got ${NEGATIVE_BILL_AMOUNT_COUNT}"
fi

COUNT_BUSINESSES=$(psql_scalar "SELECT count(*) FROM businesses;")
COUNT_BILLS=$(psql_scalar "SELECT count(*) FROM bills;")
COUNT_PAYMENTS=$(psql_scalar "SELECT count(*) FROM payments;")

for pair in "businesses:${COUNT_BUSINESSES}" "bills:${COUNT_BILLS}" "payments:${COUNT_PAYMENTS}"; do
  label="${pair%%:*}"
  val="${pair#*:}"
  if [ -z "${val}" ]; then
    record_verify_fail "${label} count missing"
  elif ! [ "${val}" -ge 0 ] 2>/dev/null; then
    record_verify_fail "${label} count not non-negative: '${val}'"
  fi
done
echo "counts: businesses=${COUNT_BUSINESSES} bills=${COUNT_BILLS} payments=${COUNT_PAYMENTS}"
echo "invariant bills total_amount<0: ${NEGATIVE_BILL_AMOUNT_COUNT}"

echo "[$(date -u +%H:%M:%S)] Booting immutable candidate backend and running authentication smoke..."
if ! run_candidate_auth_smoke; then
  : # The helper records a sanitized failure reason for the evidence artifact.
fi

VERIFY_DONE_EPOCH=$(date -u +%s)
NOW_EPOCH="${VERIFY_DONE_EPOCH}"
RPO_SECONDS=$((NOW_EPOCH - OBJECT_EPOCH))
RTO_SECONDS=$((VERIFY_DONE_EPOCH - DOWNLOAD_START_EPOCH))
echo "RPO=${RPO_SECONDS}s  RTO/duration=${RTO_SECONDS}s"
if [ "${RTO_SECONDS}" -gt "${MAX_RTO_SECONDS}" ]; then
  record_verify_fail "restore RTO ${RTO_SECONDS}s exceeded ${MAX_RTO_SECONDS}s"
fi

# ---------------------------------------------------------------------------
# 6–7. Evidence
# ---------------------------------------------------------------------------
if [ -z "${VERIFY_FAILURES}" ]; then
  RESULT="pass"
else
  RESULT="fail"
fi

write_evidence

if [ -n "${VERIFY_FAILURES}" ]; then
  die "verification failed: ${VERIFY_FAILURES}"
fi

echo ""
echo "=== Restore drill PASSED ==="
exit 0
