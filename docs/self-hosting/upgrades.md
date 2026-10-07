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
