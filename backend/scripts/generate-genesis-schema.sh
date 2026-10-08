#!/usr/bin/env bash
# generate-genesis-schema.sh — produce a deterministic schema-only genesis baseline
# equal to the current fresh-DB startup reconciliation (ReconcileReferenceSchema).
#
# Artifacts:
#   backend/schema/genesis/current_schema.sql
#   backend/schema/genesis/version.json
#   backend/schema/genesis/README.md
#
# Usage (from repo root or anywhere):
#   bash backend/scripts/generate-genesis-schema.sh
#
# Requires: Docker, Go, the pinned postgres image (GENESIS_POSTGRES_IMAGE).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKEND_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
MIGRATIONS_DIR="${BACKEND_DIR}/migrations"
OUT_DIR="${BACKEND_DIR}/schema/genesis"
SCHEMA_OUT="${OUT_DIR}/current_schema.sql"
VERSION_OUT="${OUT_DIR}/version.json"
README_OUT="${OUT_DIR}/README.md"

# HEAD is the newest numbered migration in backend/migrations, or 0 when the
# directory holds none (the genesis baseline is then the whole schema and the
# schema_migrations ledger stays empty until 000001 lands).
MIGRATION_HEAD="$(find "${MIGRATIONS_DIR}" -maxdepth 1 -name '[0-9]*.up.sql' -exec basename {} \; 2>/dev/null \
  | sed -E 's/^0*([0-9]+)_.*/\1/' | sort -n | tail -n 1)"
MIGRATION_HEAD="${MIGRATION_HEAD:-0}"
if [ "${MIGRATION_HEAD}" = "0" ]; then
  MIGRATION_HEAD_LABEL="0 (no numbered migrations; the baseline is the whole schema)"
else
  HEAD_PADDED="$(printf '%06d' "${MIGRATION_HEAD}")"
  MIGRATION_HEAD_LABEL="${HEAD_PADDED} ("'`'"backend/migrations/${HEAD_PADDED}_*.up.sql"'`'")"
fi
POSTGRES_MAJOR=18
# The PostgreSQL build deploy/docker-compose.yml runs, pinned by digest so the
# baseline is dumped by a known server. scripts/ci/workflowcontract fails when
# the two differ: bump this line with the deploy compose pin.
GENESIS_POSTGRES_IMAGE="${GENESIS_POSTGRES_IMAGE:-postgres:18.6-alpine@sha256:77f585114c32fbca283dc835b0596f4e52b51b4c6662d7810b2f4084f60a1873}"
GENERATOR_PATH="backend/scripts/generate-genesis-schema.sh"

# --- free port on 127.0.0.1 ---
pick_free_port() {
  python3 - <<'PY'
import socket
s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
s.bind(("127.0.0.1", 0))
print(s.getsockname()[1])
s.close()
PY
}

wait_for_target_db() {
  local container_name="$1"
  local ready=0
  for _ in $(seq 1 60); do
    if docker exec "${container_name}" psql -U payverge -d payverge -v ON_ERROR_STOP=1 -c 'SELECT 1' >/dev/null 2>&1; then
      ready=1
      break
    fi
    sleep 1
  done
  if [ "${ready}" -ne 1 ]; then
    echo "error: postgres target database did not become ready in time" >&2
    exit 1
  fi
}

PORT="$(pick_free_port)"
CONTAINER_NAME="${GENESIS_CONTAINER_PREFIX:-payverge-genesis}-$$-${PORT}-$(date +%s)"
DSN="postgres://payverge:genesis@127.0.0.1:${PORT}/payverge?sslmode=disable"

cleanup() {
  docker rm -f "${CONTAINER_NAME}" >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "Starting isolated ${GENESIS_POSTGRES_IMAGE} on 127.0.0.1:${PORT} (${CONTAINER_NAME})..."
docker run -d --name "${CONTAINER_NAME}" \
  -e POSTGRES_USER=payverge \
  -e POSTGRES_PASSWORD=genesis \
  -e POSTGRES_DB=payverge \
  -p "127.0.0.1:${PORT}:5432" \
  "$GENESIS_POSTGRES_IMAGE" >/dev/null

echo "Waiting for target database..."
wait_for_target_db "${CONTAINER_NAME}"

echo "Reconciling reference schema via genesisgen..."
FINGERPRINT_OUT="$(mktemp)"
trap 'rm -f "${FINGERPRINT_OUT}"; cleanup' EXIT
(
  cd "${BACKEND_DIR}"
  go run ./cmd/genesisgen \
    -dsn="${DSN}" \
    -migrations="${MIGRATIONS_DIR}" \
    -fingerprint-out="${FINGERPRINT_OUT}"
)
FINGERPRINT_SHA="$(tr -d '[:space:]' < "${FINGERPRINT_OUT}")"
if [ "${#FINGERPRINT_SHA}" -ne 64 ]; then
  echo "error: genesisgen did not write a schema fingerprint" >&2
  exit 1
fi

# If we got here, schema_migrations is clean (genesisgen verified dirty=false).
# Dump schema-only from INSIDE the container (host pg_dump may be older).
echo "Dumping schema with in-container pg_dump..."
RAW_DUMP="$(mktemp)"
NORM_DUMP="$(mktemp)"
trap 'rm -f "${RAW_DUMP}" "${NORM_DUMP}" "${FINGERPRINT_OUT}"; cleanup' EXIT

docker exec "${CONTAINER_NAME}" \
  pg_dump --schema-only --no-owner --no-privileges -U payverge payverge \
  > "${RAW_DUMP}"

# Normalize deterministically: strip volatile dump metadata, collapse trailing WS.
# pg_dump 15+ emits random \restrict / \unrestrict tokens per invocation — drop them.
# Keep all structural SQL stable.
sed -e '/^-- Dumped /d' \
    -e '/^-- Started on/d' \
    -e '/^-- Completed on/d' \
    -e '/^\\restrict /d' \
    -e '/^\\unrestrict /d' \
    -e 's/[[:space:]]*$//' \
    "${RAW_DUMP}" > "${NORM_DUMP}"

mkdir -p "${OUT_DIR}"
cp "${NORM_DUMP}" "${SCHEMA_OUT}"

if command -v shasum >/dev/null 2>&1; then
  SCHEMA_SHA="$(shasum -a 256 "${SCHEMA_OUT}" | awk '{print $1}')"
elif command -v sha256sum >/dev/null 2>&1; then
  SCHEMA_SHA="$(sha256sum "${SCHEMA_OUT}" | awk '{print $1}')"
else
  echo "error: need shasum or sha256sum" >&2
  exit 1
fi

# version.json — single-quoted keys in task text; emit standard JSON.
cat > "${VERSION_OUT}" <<EOF
{
  "migration_head": ${MIGRATION_HEAD},
  "schema_sha256": "${SCHEMA_SHA}",
  "fingerprint_sha256": "${FINGERPRINT_SHA}",
  "postgres_major": ${POSTGRES_MAJOR},
  "generator": "${GENERATOR_PATH}"
}
EOF

cat > "${README_OUT}" <<EOF
# Genesis schema baseline

This directory holds a **schema-only** SQL baseline equal to what a **fresh
Postgres 18** database looks like after Payverge's production startup
reconciliation — the same sequence as \`cmd/app/main.go\` and
\`database.ReconcileReferenceSchema\`.

## What it is

| File | Purpose |
|------|---------|
| \`current_schema.sql\` | \`pg_dump --schema-only\` of the reconciled reference DB (no data rows) |
| \`version.json\` | Migration HEAD, content SHA-256, schema fingerprint SHA-256, Postgres major, generator path |
| \`README.md\` | This document |

**Current baseline**

- Migration HEAD: **${MIGRATION_HEAD_LABEL}**
- Schema SHA-256: \`${SCHEMA_SHA}\`
- Schema fingerprint SHA-256: \`${FINGERPRINT_SHA}\` (startup refuses a live schema
  whose \`database.SchemaFingerprintSHA\` differs, or a different Postgres major)
- Postgres major: **${POSTGRES_MAJOR}**
- Generator: \`${GENERATOR_PATH}\`

The dump is **schema-only** (no \`INSERT\` rows). Runtime-owned schemas such as
**whatsmeow** tables are created when those subsystems connect; they are not
part of this baseline (see the fresh-DB artifact allowlist).

Seed rows a fresh install needs (the \`runtime_controls\` launch defaults) are
written by \`seedGenesisRuntimeControls\` during bootstrap, not by this file.

## How it was generated

\`\`\`bash
bash backend/scripts/generate-genesis-schema.sh
\`\`\`

That script:

1. Starts an isolated \`postgres:18-alpine\` container on \`127.0.0.1\` + a free port.
2. Runs \`go run ./cmd/genesisgen\` which calls \`database.ReconcileReferenceSchema\`
   (embedded genesis → pending numbered migrations → read-only verification)
   and refuses to continue if \`schema_migrations.dirty\` is true.
3. Dumps with **in-container** \`pg_dump\` 15 (\`--schema-only --no-owner --no-privileges\`).
4. Normalizes volatile dump headers and trailing whitespace for determinism.
5. Writes \`current_schema.sql\`, \`version.json\`, and this README.

## Regenerating after new migrations

1. Land the new \`backend/migrations/NNNNNN_*.up.sql\` / \`.down.sql\` pair. The
   generator derives \`MIGRATION_HEAD\` from the newest file in that directory.
2. Re-run:

   \`\`\`bash
   bash backend/scripts/generate-genesis-schema.sh
   \`\`\`

3. Confirm determinism:

   \`\`\`bash
   bash backend/scripts/generate-genesis-schema.test.sh
   \`\`\`

4. Commit the three artifacts together with the migration that changed the schema.

Do **not** hand-edit \`current_schema.sql\` — always regenerate.
EOF

echo ""
echo "Genesis schema written:"
echo "  ${SCHEMA_OUT}"
echo "  SHA-256: ${SCHEMA_SHA}"
echo "  version.json migration_head=${MIGRATION_HEAD}"
