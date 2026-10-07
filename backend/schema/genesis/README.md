# Genesis schema baseline

This directory holds a **schema-only** SQL baseline equal to what a **fresh
Postgres 15** database looks like after Payverge's production startup
reconciliation — the same sequence as `cmd/app/main.go` and
`database.ReconcileReferenceSchema`.

## What it is

| File | Purpose |
|------|---------|
| `current_schema.sql` | `pg_dump --schema-only` of the reconciled reference DB (no data rows) |
| `version.json` | Migration HEAD, content SHA-256, schema fingerprint SHA-256, Postgres major, generator path |
| `README.md` | This document |

**Current baseline**

- Migration HEAD: **0 (no numbered migrations; the baseline is the whole schema)**
- Schema SHA-256: `023d7fafefeb106b37b119f0e33fa8de69a961d5a4b2f71094a0643245b6a83c`
- Schema fingerprint SHA-256: `64c40f26c9f478ad3beece8263cec52761da3806bd92599a98349c7b8185b71a` (startup refuses a live schema
  whose `database.SchemaFingerprintSHA` differs, or a different Postgres major)
- Postgres major: **15**
- Generator: `backend/scripts/generate-genesis-schema.sh`

The dump is **schema-only** (no `INSERT` rows). Runtime-owned schemas such as
**whatsmeow** tables are created when those subsystems connect; they are not
part of this baseline (see the fresh-DB artifact allowlist).

Seed rows a fresh install needs (the `runtime_controls` launch defaults) are
written by `seedGenesisRuntimeControls` during bootstrap, not by this file.

## How it was generated

```bash
bash backend/scripts/generate-genesis-schema.sh
```

That script:

1. Starts an isolated `postgres:15-alpine` container on `127.0.0.1` + a free port.
2. Runs `go run ./cmd/genesisgen` which calls `database.ReconcileReferenceSchema`
   (embedded genesis → pending numbered migrations → read-only verification)
   and refuses to continue if `schema_migrations.dirty` is true.
3. Dumps with **in-container** `pg_dump` 15 (`--schema-only --no-owner --no-privileges`).
4. Normalizes volatile dump headers and trailing whitespace for determinism.
5. Writes `current_schema.sql`, `version.json`, and this README.

## Regenerating after new migrations

1. Land the new `backend/migrations/NNNNNN_*.up.sql` / `.down.sql` pair. The
   generator derives `MIGRATION_HEAD` from the newest file in that directory.
2. Re-run:

   ```bash
   bash backend/scripts/generate-genesis-schema.sh
   ```

3. Confirm determinism:

   ```bash
   bash backend/scripts/generate-genesis-schema.test.sh
   ```

4. Commit the three artifacts together with the migration that changed the schema.

Do **not** hand-edit `current_schema.sql` — always regenerate.
