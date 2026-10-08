# Backups and restore

The deploy stack backs itself up every night. This page explains what is in
a backup, what is not, and how to prove you can restore one. The command
reference is in [deploy/README.md](../../deploy/README.md#backups).

## What is covered

| Data | In the nightly set? | Notes |
|---|---|---|
| Database | yes, `db.dump` | Consistent `pg_dump` snapshot, taken while the app runs. |
| Uploads (local storage) | yes, `storage.tar.gz` | Menus, logos, photos, documents. |
| Bundled MinIO | yes, `minio.tar.gz` | Only with the `minio` profile. |
| An external S3 bucket | **no** | Use the provider's versioning or replication ([storage.md](storage.md)). |
| `.env` | **no** | Keep it in your password manager. Without `PLUGIN_SECRET_KEY` a restored database cannot decrypt payment credentials. |
| Caddy certificates | no | Caddy gets new ones on a new server. |
| `rclone/rclone.conf` | no | Keep it with `.env`. |

Sets land in `./backups` (`BACKUP_DIR`) at `BACKUP_TIME` (03:30 UTC by
default), with 7 daily, 4 weekly and 6 monthly sets kept. The variables are
in [configuration.md](configuration.md#deploy-stack-only).

## The three rules

1. **Copy sets off the server.** A backup on the same disk dies with it. Turn
   on the `backup-offsite` profile
   ([deploy/README.md](../../deploy/README.md#off-site-copies)).
2. **Watch it.** `docker compose logs backup` shows each run; a failed run
   exits non-zero and writes `archives_failed=` in the set's `MANIFEST`. Check
   that `ls backups/daily` gains a set every day, or alert on the newest
   set's age from your monitoring.
3. **Restore one.** Until you have restored a set, you do not know that your
   backups work. Do the drill below after installing and after any change to
   storage.

## Restore drill

This restores the newest set into a separate, throwaway Compose project on
any machine with Docker, so production is never touched. Copy the newest set
and your `.env` there first.

```sh
mkdir drill && cd drill
cp -R /opt/payverge/. .             # or unpack the same release's deploy files
mkdir -p backups/daily && cp -R /path/to/copied/set backups/daily/
docker compose -p payverge-drill up -d --wait postgres
docker compose -p payverge-drill --profile restore run --rm restore latest --yes
docker compose -p payverge-drill exec -T postgres \
  sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "SELECT count(*) FROM businesses;"'
docker compose -p payverge-drill --profile '*' down -v    # delete the drill
```

The count should match production on the day of the set. To click through
the restored data, also run `docker compose -p payverge-drill up -d` with
`HTTP_PORT=127.0.0.1:8080`, `HTTPS_PORT=127.0.0.1:8443` and
`EDGE_SUBNET=172.30.1.0/24` (the live stack holds the default subnet) set in
the drill's `.env`, and remove it the same way.

What `restore` does: it verifies `SHA256SUMS`, refuses to run while anything
else is connected to the database, drops and re-creates the database, runs
`pg_restore`, and replaces the uploads (and MinIO) volume. `--skip-storage`
restores only the database. The backend must be the version in the set's
`MANIFEST`, or newer.

### Evidence for this page

- **Run:** `scripts/ci/deploy-backup_test.sh`, the CI test of
  `deploy/backup/backup.sh` and `restore.sh`, against a throwaway PostgreSQL
  server on 127.0.0.1. It takes a set (database, uploads, MinIO data), checks
  the `MANIFEST`, checksums and weekly/monthly promotion, restores it, and
  compares the database and both volumes with the originals. It also checks
  that restore refuses a set with bad checksums and refuses while another
  client is connected. It ended with `all checks passed (PostgreSQL 14.22)`.
- **Not run:** the full Compose drill (backup service, `restore` service and
  the real `postgres:18` image in containers). Docker was not available when
  these docs were written. A script that runs it end to end against a
  throwaway project (marker row and uploaded file, backup, delete, restore,
  compare) is kept with the release work and is to be run before the docs are
  published.

## Rolling back an upgrade

A restore is also how you roll back a release: see
[upgrades.md](upgrades.md#rolling-back).

## Moving to a new server

See [deploy/README.md](../../deploy/README.md#moving-to-a-new-server). Install
the same version with `--no-start`, put your saved `.env` in place, copy the
set, restore, then start and point DNS at the new server.
