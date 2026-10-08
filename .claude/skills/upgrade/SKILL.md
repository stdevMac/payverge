---
name: upgrade
description: Upgrade a self-hosted Payverge server to a newer release by reading the release notes, backing up the database and uploaded files, pulling or building the new version, watching the startup migrations, verifying the instance, and rolling back by restoring the backup if needed. Use when someone asks to update, upgrade or patch their Payverge instance, or when the backend stops starting after an upgrade.
---

# Upgrade a Payverge server

Three facts shape every step:

- **The backend migrates the database when it starts.** `RunMigrations` in
  `backend/cmd/app/main.go` applies the pending files from
  `backend/migrations/` and logs `Migrations completed (version: N)`.
- **It then refuses to start unless the schema is exactly at the newest
  migration it ships** (`VerifySchemaAtVersion`). An older binary will not
  run against a newer database.
- **So migrations only go forward.** Rolling back after an upgrade means
  restoring the backup you take in step 2. Without that backup, there is no
  way back.

Never run `docker compose down -v`. It deletes the database volume.

## 0. Ask first

- Which server, and whether guests are using it right now. The upgrade
  restarts the backend, so schedule it outside service hours.
- The current version and the target version.
- Where the backups will go, and whether the disk has room (`df -h`).

Ask again before each step that restarts, upgrades or restores a production
server. Run the commands from the directory that holds the compose file:
`deploy/` for an image-based install, or the repository root for a
source checkout. With the root `docker-compose.yml`, add `--env-file .env`
to every `docker compose` command.

## 1. Read the release notes

Read `docs/CHANGELOG.md` and the GitHub release page for every version
between the current one and the target. Look for new required variables,
renamed or removed variables, manual steps and breaking changes. Tell the
operator which ones apply to their server before you go on.

Record where you start:

```bash
docker compose images                      # image tags (image-based install)
git describe --tags --always               # source checkout
docker compose exec -T postgres sh -c \
  'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atc "SELECT version, dirty FROM schema_migrations"'
```

The `sh -c '...'` form uses the database container's own `POSTGRES_USER`
and `POSTGRES_DB`, so you never need to open `.env`.

## 2. Back up

Take a backup every time, even for a patch release.

Run this as one command. `pipefail` makes a failed `pg_dump` fail the
whole line instead of leaving a small, valid-looking `.gz` behind:

```bash
set -o pipefail
mkdir -p backups && chmod 700 backups
BACKUP="backups/payverge_$(date +%Y%m%d_%H%M%S).sql.gz"
docker compose exec -T postgres sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB"' | gzip > "$BACKUP" \
  && gunzip -t "$BACKUP" \
  && echo "backup: $BACKUP  tables: $(gunzip -c "$BACKUP" | grep -c '^CREATE TABLE')" \
  || { echo "BACKUP FAILED"; rm -f "$BACKUP"; }
```

The table count should be in the hundreds. `BACKUP FAILED` or a count of
zero means there is no usable backup. Stop there.

Write down the exact file name it prints and tell the operator. Shell
variables such as `$BACKUP` do not survive between separate commands (an
agent's shell starts fresh for each call), so the rollback in step 6 names
the file literally.

- **Uploaded files.** With the default local storage, back up the storage
  volume as well, following "Backup and restore" in
  `docs/self-hosting/storage.md`. Database rows point at stored objects, so
  keep the two backups together. With S3 storage, the bucket keeps the
  files.
- If `docker compose config --profiles` lists `backup` (the `deploy/`
  compose file has one), its nightly sets in `./backups` are useful, and
  `docker compose run --rm backup once` takes a fresh set that includes the
  uploaded files; see "Backups" in `deploy/README.md`. Either way, make sure
  a backup exists from just before the upgrade.
- Copy the backup off the host if you can. For automated off-host dumps, see
  `backend/scripts/backup-db.sh` and `infra/backup/README.md`.

## 3. Upgrade

Image-based install (`deploy/`). The images are pinned by
`PAYVERGE_VERSION` in `.env` (the installer writes the exact release, for
example `1.2.3`), so a bare `docker compose pull` fetches the same release
again. Move the pin first. Either let the installer do it, which also
refreshes the release files, then pulls and restarts:

```bash
bash install.sh --version X.Y.Z
```

or set `PAYVERGE_VERSION=X.Y.Z` in `.env` with an editor and run:

```bash
docker compose pull
docker compose up -d
```

`PAYVERGE_VERSION=latest` follows the newest release on every pull. When
`deploy/` lives in a git clone and you upgrade by hand, check out the
matching tag first (`git fetch --tags && git checkout vX.Y.Z`), so the
compose file matches the images. See "Upgrades" in `deploy/README.md`.
Confirm the new tag with `docker compose images` before you go on.

Source checkout (the root `docker-compose.yml`):

```bash
git status --short            # local changes? Stop and ask before going on.
git fetch --tags
git checkout vX.Y.Z           # or: git pull --ff-only, to follow main
docker compose --env-file .env up -d --build
```

### Crossing PostgreSQL 15 to 18

Releases from the PostgreSQL 18 one on keep the database in the `pgdata`
volume; older ones kept PostgreSQL 15 data in `db`. When the operator moves
across that line (`docker compose logs postgres` shows "this install's
database is PostgreSQL 15", or `install.sh` stopped naming
`./upgrade-postgres.sh`), the data has to be moved once. In the install
directory, with the operator's go-ahead (Payverge is offline while it runs):

```bash
./upgrade-postgres.sh --dry-run
./upgrade-postgres.sh --yes
```

It dumps with PostgreSQL 15, restores into 18 on a fresh `pgdata` volume,
compares the row count of every table and starts the stack. Tell the
operator where the dump went (`backups/pg18-upgrade-<time>/`). It never
touches the `db` volume: that is the rollback (`bash install.sh --version
<previous>`). Never remove `db` unless the operator asks, after the instance
has run well on 18. Full procedure: `docs/self-hosting/upgrades.md`,
"PostgreSQL 18".

## 4. Watch it start

```bash
docker compose logs -f backend      # Ctrl-C once the server is listening
```

Wait for `Migrations completed (version: N)`. If the backend restarts
instead, match the log line:

| Log line | Meaning | Action |
|---|---|---|
| `PRODUCTION PREFLIGHT — <code> [<component>]: <message>` | A required setting is missing or unsafe, usually a new one from the release notes | Set it in `.env`, then run `up -d` again |
| `database is in dirty state at migration version N` | A migration stopped halfway | Follow `docs/runbooks/migration-dirty-recovery.md`, or roll back (step 6) |
| `Database migration failed: ...` | A migration statement failed, or the database is at a version this binary does not ship (an older binary on a newer database) | Do not restart in a loop. If the error names a version above the release's newest file in `backend/migrations/`, run the newer release or roll back. Otherwise use the runbook or roll back. |
| `postgres major version 15 does not match the genesis baseline's 18` | The database is still PostgreSQL 15 | Move it: "Crossing PostgreSQL 15 to 18" above |
| `Schema verification failed: VerifySchemaAtVersion: ...` | After migrating, the schema is not exactly at the newest version this binary ships | Run the release that matches the database, or roll back |

## 5. Verify

```bash
curl -fsS https://pos.example.com/api/v1/health/live
curl -fsS https://pos.example.com/api/v1/health/ready
docker compose ps
```

- `ready` answers `{"status":"ready"}`. The per-component detail needs
  `Authorization: Bearer $HEALTH_DETAIL_TOKEN`, as described in
  `.claude/skills/troubleshoot/SKILL.md`.
- Sign in to the dashboard and open one table page (`/t/<table_code>`).
- With the MCP tools, `payverge_instance_status` and, with an admin token,
  `payverge_health_snapshot`.
- If the operator runs `tools/payverge-admin-mcp` from a clone, update that
  clone too. A tool that answers `not_found` for a whole route means the
  backend and the MCP server are from different releases.

Tell the operator the version they are on now, the migration version, and
where the backup is.

## 6. Roll back

Roll back only with the operator's go-ahead. The restore replaces every
change made since the backup, including orders and payments taken after the
upgrade. Say this plainly before you start.

1. Stop the app and keep the failed state for the bug report:

   ```bash
   set -o pipefail
   docker compose stop backend frontend
   docker compose exec -T postgres sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB"' \
     | gzip > "backups/failed_upgrade_$(date +%Y%m%d_%H%M%S).sql.gz"
   ```

2. Go back to the previous version: set `PAYVERGE_VERSION` in `.env` back
   to the release you recorded in step 1 (image-based install), or
   `git checkout <previous tag>` (source checkout).
3. Restore the database from the backup taken in step 2. Ask the operator
   to confirm the exact file name first (`ls -l backups/`), and put it in
   the command below. Do not rely on a `$BACKUP` variable from an earlier
   command: if it is empty, the database is dropped and nothing is restored.
   Run the whole block as one command. It checks the file before it drops
   anything:

   ```bash
   set -o pipefail
   BACKUP=backups/payverge_YYYYMMDD_HHMMSS.sql.gz   # the file from step 2
   test -s "$BACKUP" && gunzip -t "$BACKUP" \
     && [ "$(gunzip -c "$BACKUP" | grep -c '^CREATE TABLE')" -gt 0 ] \
     && docker compose exec -T postgres sh -c \
       'dropdb -U "$POSTGRES_USER" --force "$POSTGRES_DB" && createdb -U "$POSTGRES_USER" "$POSTGRES_DB"' \
     && gunzip -c "$BACKUP" | docker compose exec -T postgres sh -c \
       'psql -v ON_ERROR_STOP=1 -q -U "$POSTGRES_USER" -d "$POSTGRES_DB"' \
     && echo "RESTORE OK" \
     || echo "RESTORE FAILED: do not start the backend"
   ```

   If it prints `RESTORE FAILED` after the drop, the database is empty or
   partly restored. Do not start the backend, because it would create a
   fresh empty schema. Fix the cause and run the block again.

4. Restore the storage volume if you backed it up (see
   `docs/self-hosting/storage.md`).
5. Start the stack with `docker compose up -d` (with a source checkout, add
   `--env-file .env --build`). Then verify as in step 5.

To rehearse a restore without touching the live database, use
`backend/scripts/restore-db.sh`. It only restores into an isolated scratch
database on a loopback address.

## Do not

- Skip the backup because "it is only a patch".
- Drop or restore the database through a variable set in an earlier
  command. Name the backup file in the same command.
- Edit `schema_migrations` by hand, except as the dirty-state runbook
  describes.
- Run `docker compose down -v`, or delete the `postgres` volume.
- Print `.env`, or paste its contents into the conversation.
