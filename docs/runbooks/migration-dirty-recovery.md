# Migration Dirty-State Recovery Runbook

**When to use this runbook:** The backend starts up and immediately exits with an error like:

```
database is in dirty state at migration version 90 — the migration partially
applied and did not complete; manual recovery is required before the server can
start. See docs/runbooks/migration-dirty-recovery.md for step-by-step instructions.
```

A "dirty" migration means a migration's SQL began executing but did not finish
(e.g. the process was killed mid-run, the database ran out of disk space, or a
statement failed after earlier statements in the same file had already committed).
The `schema_migrations` table records the version as dirty and the backend refuses
to start to prevent serving traffic against a partially-migrated schema.

**This is a manual recovery step — do not restart the server in a loop expecting it to self-heal.**

---

## 1. Identify the dirty version

The error message includes the version number. You can also confirm directly:

```sql
SELECT version, dirty FROM schema_migrations;
```

Expected output showing the problem:

```
 version | dirty
---------+-------
      90 | t
```

## 2. Inspect the failed migration file

Find the migration file for that version:

```bash
ls backend/migrations/ | grep '^000090'
# e.g. 000090_add_business_plugin_health.up.sql
```

Open the `.up.sql` file and read every statement. You need to know:
- Which statements ran successfully before the failure.
- Which statement failed (check Postgres logs or the error from the previous deploy).
- Which statements never ran.

## 3. Determine what state the schema is actually in

Connect to the database and inspect the relevant tables/columns:

```sql
-- Check if the table/column from the migration exists:
\d table_name

-- Or query information_schema:
SELECT column_name
FROM information_schema.columns
WHERE table_name = 'your_table'
ORDER BY ordinal_position;
```

Compare what exists against what the migration file expects. This tells you
how far the migration got before it failed.

## 4. Make the schema consistent

You have two options depending on whether the migration is idempotent:

### Option A: Apply the missing statements manually

If the migration file is idempotent (uses `IF NOT EXISTS`, `IF EXISTS`, etc.),
you can re-run the remaining statements directly:

```sql
-- Connect as the migration user, then run the missing DDL:
ALTER TABLE businesses ADD COLUMN IF NOT EXISTS plugin_last_ok_at TIMESTAMPTZ;
-- ... etc.
```

### Option B: Roll back partial changes

If the migration cannot be re-applied cleanly, undo what already ran and let
`golang-migrate` apply the full `.up.sql` cleanly on next start:

```sql
-- Undo what the failed migration partially applied:
ALTER TABLE businesses DROP COLUMN IF EXISTS plugin_last_ok_at;
-- ... (reverse the statements that did execute)
```

## 5. Force-clear the dirty flag

Once the schema is in a consistent state (either fully applied or fully
reverted), tell `golang-migrate` the correct version to resume from.

**If you applied the migration manually (Option A):**
```bash
# Mark version 90 as cleanly applied so golang-migrate skips it on next start:
migrate -database "$DATABASE_URL" -path backend/migrations force 90
```

**If you rolled back (Option B):**
```bash
# Revert schema_migrations to the last clean version (e.g. 89) so
# golang-migrate will re-run the migration from scratch on next start:
migrate -database "$DATABASE_URL" -path backend/migrations force 89
```

You can also do this via raw SQL if the `migrate` CLI is not available:

```sql
-- Option A: mark as cleanly applied at version 90
UPDATE schema_migrations SET dirty = false WHERE version = 90;

-- Option B: revert to version 89 (remove the dirty row, insert a clean one)
DELETE FROM schema_migrations;
INSERT INTO schema_migrations (version, dirty) VALUES (89, false);
```

> **Warning:** `UPDATE schema_migrations SET dirty = false` without fixing the
> underlying schema inconsistency will allow the server to start against a
> partially-migrated database, which may cause data corruption or runtime errors.
> Always verify the schema state in step 3 before clearing the dirty flag.

## 6. Restart the backend

With the dirty flag cleared and the schema in a consistent state, the backend
will start normally. On startup it will either:
- Skip the migration (if you forced it to version 90, Option A), or
- Re-apply the migration from scratch (if you reverted to 89, Option B).

```bash
docker compose --env-file .env up -d backend
docker compose --env-file .env logs -f backend
```

Watch for `Migrations completed (version: NN)` in the log output.

## 7. Verify

After the backend starts:
1. Hit `/api/v1/health/ready` — it must return `200 OK`.
2. Run a smoke test against the affected feature.
3. Confirm `schema_migrations` shows the expected clean version:
   ```sql
   SELECT version, dirty FROM schema_migrations;
   -- Should show dirty = false
   ```

## Prevention

- Each migration file should be written to be idempotent where possible
  (`IF NOT EXISTS`, `IF EXISTS`, `ON CONFLICT DO NOTHING`).
- Run new migrations against a staging environment before production.
- Never kill the backend process during startup when migrations are running.
- The `golang-migrate` library does not wrap each migration file in a
  transaction by default — if transactional DDL is important, begin/commit
  explicitly in the `.up.sql` file.

## Related documentation

- `backend/migrations/` — canonical migration files
- `backend/internal/database/migrate.go` — `RunMigrations` and `decideMigrationStartup`
