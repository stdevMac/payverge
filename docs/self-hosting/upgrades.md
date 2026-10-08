# Upgrades

Payverge ships as versioned images (`ghcr.io/stdevmac/payverge-backend` and
`-frontend`) plus a deploy bundle per release. The version your instance runs
is `PAYVERGE_VERSION` in `.env`. The command reference is in
[deploy/README.md](../../deploy/README.md#upgrades).

## Choosing a version policy

- **Pinned (recommended).** The installer writes the exact version, for
  example `PAYVERGE_VERSION=1.2.3`. Nothing changes until you upgrade.
- **`latest`.** Every `docker compose pull` moves you to the newest release,
  including its database migrations. Fine for a trial, risky for a restaurant
  in service.

## Before you upgrade

1. Read the release notes for every version between yours and the target. A
   release can add a required setting or change a default.
2. Upgrade outside service hours. The backend restarts and runs migrations;
   expect a minute or two of downtime.
3. Make sure a backup will be taken (the installer does it) and that the
   previous night's set was copied off the server.

## Upgrading

```sh
cd /opt/payverge                 # the install directory
bash install.sh --version 1.3.0
```

The installer takes a backup set with the old version first, and stops with
everything unchanged if that backup fails. It then replaces the release files
(`docker-compose.yml`, the Caddy files, `backup/`, `install.sh`, `README.md`),
pins the new version, pulls and starts. Your `.env` and any
`docker-compose.override.yml` are kept.

By hand, or for a build-from-source install, see
[deploy/README.md](../../deploy/README.md#upgrades) and
[Build from source](../../deploy/README.md#build-from-source).

After the upgrade:

```sh
docker compose ps                         # every service healthy
docker compose logs --since 10m backend | grep -iE 'migrat|error|fatal'
curl -fsS https://pay.example.com/api/v1/health/ready
```

None of these commands were run for this page; they need a released version
to upgrade from and to.

## Rolling back

Migrations only go forward, and an older backend refuses to start on a newer
schema. So a rollback is a restore of the set taken **before** the upgrade,
with `PAYVERGE_VERSION` set back to the old version. Anything written after
the upgrade is lost. The exact commands are in
[deploy/README.md](../../deploy/README.md#upgrades); name the set explicitly
(`daily/<time>`), because `latest` may already be a post-upgrade set.

## PostgreSQL 18

Releases from this one on run PostgreSQL 18; earlier ones ran PostgreSQL 15.
A major version cannot read the other's data files, and the backend refuses
to start on any major other than the one its schema baseline was taken on
(`postgres major version 15 does not match the genesis baseline's 18`).
Moving the data is a one-time step.

PostgreSQL 18 also keeps its data in a different place: the `pgdata` volume,
mounted at `/var/lib/postgresql` (the server's data directory is
`/var/lib/postgresql/18/docker`). Your PostgreSQL 15 data stays in the old
`db` volume, which the new release mounts read-only and never changes.

**What happens if you just upgrade.** `install.sh` takes its usual
pre-upgrade backup, puts the new release files in place, sees the
PostgreSQL 15 data and stops before starting anything, naming
`./upgrade-postgres.sh`. A plain `docker compose up -d` with the new files
does not create an empty database either: the postgres service
(`postgres/pg18-guard.sh`) logs what to do and waits, its healthcheck stays
red, and the backend never starts.

**Upgrading the data.** Outside service hours, in the install directory:

```sh
./upgrade-postgres.sh --dry-run   # what it will do; changes nothing
./upgrade-postgres.sh             # asks before stopping Payverge
```

It:

1. stops the stack (`docker compose down`; every volume is kept);
2. starts PostgreSQL 15 on the `db` volume, records the row count of every
   table in every database, and dumps the whole cluster with `pg_dumpall`
   (roles with their passwords, databases, owners and grants) into
   `backups/pg18-upgrade-<time>/`;
3. starts PostgreSQL 18 on a new `pgdata` volume, restores the dump, runs
   `ANALYZE`, and compares the row counts with step 2. Any difference, or
   any restore error other than the expected "role already exists" for the
   superuser, stops it: the new volume is removed and the stack is left
   stopped with the `db` volume as it was;
4. starts Payverge (`--no-start` skips this).

Expect a few minutes per GB of database. The dump needs free disk space
in `backups/` of roughly the size of the data, compressed. Custom edits to
`pg_hba.conf` or `postgresql.conf` inside the old volume are not carried
over; put settings in `docker-compose.override.yml` (`command: postgres -c
...`) instead.

**Rolling back.** The PostgreSQL 15 data is still in the `db` volume, and
the dump is in `backups/pg18-upgrade-<time>/`. To go back:

```sh
docker compose down
bash install.sh --version <previous release>   # runs PostgreSQL 15 on db again
```

Anything written on PostgreSQL 18 after the upgrade is lost: the rollback
runs on the `db` volume as it was when the upgrade dumped it. From then on
the `pgdata` copy is stale in turn, since it misses what is written on 15.
The upgrade records a checksum of the 15 cluster's `global/pg_control` in
`pgdata`, so once 15 has run again, the PostgreSQL 18 release refuses to
start on that copy (`install.sh` stops, and the postgres service logs why
and waits) instead of silently serving older data. Before trying the upgrade
again, remove the stale copy: `docker volume rm <project>_pgdata` (the
project is `payverge` unless `COMPOSE_PROJECT_NAME` says otherwise), then
run `./upgrade-postgres.sh`.

**Afterwards.** Once the instance has run well on PostgreSQL 18 for a while,
free the space the old data takes: `docker volume rm <project>_db`. Backup
sets taken on PostgreSQL 15 still restore into 18 (`pg_restore` reads older
dumps).

**Other setups.**

- *Coolify / Dokploy* (`deploy/platforms/coolify`): the volume keeps its name
  and moves to `/var/lib/postgresql`. PostgreSQL 18 refuses to start on the
  15 data at its root (nothing is overwritten). Dump with the old release
  first (`pg_dumpall`), then empty the volume, deploy, and restore the dump
  with `psql`.
- *Managed databases* (Render, Railway, your own server): upgrade the
  database to PostgreSQL 18 with the provider's tool before deploying this
  release. On Render, `postgresMajorVersion: "18"` in `render.yaml` only
  applies when the Blueprint creates the database; an existing database
  keeps its version until you upgrade it from the Render dashboard (or
  dump, create a new 18 database and restore). Until then the backend
  refuses to start (`postgres major version 15 does not match the genesis
  baseline's 18`).
- *Development* (root `docker-compose.yml`): the `postgres_data` volume moves
  to `/var/lib/postgresql`; a throwaway dev database is simplest to
  recreate.

## Keeping your changes across upgrades

- Settings belong in `.env`.
- Service changes belong in `docker-compose.override.yml`. Compose reads it
  automatically unless `.env` sets `COMPOSE_FILE` (as `--build` does), in
  which case add it to that list.
- Do not edit `docker-compose.yml` or the Caddy files in place: an upgrade
  overwrites them.

## Security releases

Watch the repository's releases to hear about security fixes. Releases with a
security fix say so in their notes; upgrade to them promptly. See
[security.md](security.md).
