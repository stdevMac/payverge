# Versioned migrations

Numbered, paired SQL migrations (`NNNNNN_name.up.sql` / `NNNNNN_name.down.sql`)
applied by golang-migrate after the genesis baseline
(`backend/schema/genesis/current_schema.sql`). The runner reads only
`*.up.sql` / `*.down.sql`; this README is ignored by it.

The genesis baseline is the whole schema at version 0: the open-source
release squashed every earlier migration into it, so this directory holds no
numbered migration yet and the first schema change after the release is
`000001`. An empty database bootstraps straight from the baseline.
`TestRollbackRehearsal_EachMigration` walks every `.down.sql` here (none yet)
and rolls them forward again.

Rules:

- Every schema change is a new numbered migration. Never edit an applied
  migration and **never renumber** existing files: deployed databases record
  the applied version in `schema_migrations`, so a renumber would skip or
  re-run real migrations.
- Numbers only increase. A gap in the sequence is permanent and harmless:
  golang-migrate moves from one existing version to the next.
- After adding a migration, regenerate the baseline with
  `bash backend/scripts/generate-genesis-schema.sh` and commit the migration
  pair and the regenerated `backend/schema/genesis/` files together.
