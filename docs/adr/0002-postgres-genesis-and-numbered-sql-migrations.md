# 0002. A genesis baseline plus numbered SQL migrations own the schema

- Status: Accepted
- Date: 2026-10-03 (records a decision already in effect)

## Context

The schema was once built by a mix of SQL files, GORM `AutoMigrate` and
"ensure" helpers that ran at startup. The result depended on the order code
ran in and on what was already in the database, so it was hard to say what
schema a given version should have, or to prove that a database had it.
Replaying hundreds of historical migrations on every fresh install was also
slow.

## Decision

Postgres 18 is the only supported database, and exactly two things may
change its schema:

1. **A genesis baseline**
   ([backend/schema/genesis/](../../backend/schema/genesis/)): a
   deterministic, schema-only dump at a known migration head, with its
   SHA-256 recorded. It is applied only to an empty database, in one
   transaction under an advisory lock.
2. **Numbered SQL migrations**
   ([backend/migrations/](../../backend/migrations/)), applied with
   golang-migrate, each with an up and a down file.

At startup the backend classifies the database (empty, versioned, or
neither), applies the baseline or pending migrations, and refuses to serve
unless `schema_migrations` is clean and at the newest version it ships (no
row while that version is 0, the baseline itself). A database with tables
but no migration ledger is refused, not guessed at.

For the open-source release the history was squashed: the baseline is the
whole schema at version 0 and `backend/migrations/` starts empty, so the
first later change is `000001`. No production data predated the release.

Source-level tests fail the build if production code adds an `AutoMigrate`
call or startup DDL.

## Consequences

- **Easier:** every install on a given version has the same schema. Fresh
  installs are fast. A failed bootstrap leaves an empty database.
- **Easier:** schema changes are reviewable SQL, and recent down files are
  rehearsed against a real Postgres (`integration_postgres` tests).
- **Harder:** a contributor must write SQL and keep the GORM model in step
  by hand. The model does not create anything.
- **Harder:** code that relies on Postgres features (row locks,
  `SKIP LOCKED`, partial indexes) needs Postgres tests; the fast SQLite tests
  do not prove it.
- **Accepted:** no other database engines.

See [architecture/data-and-migrations.md](../architecture/data-and-migrations.md).
