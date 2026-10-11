# Self-hosting Payverge

This directory runs a complete Payverge instance on one server with Docker
Compose. It needs no third-party account: TLS comes from Let's Encrypt (or
Caddy's own CA for a local trial), uploads stay on a local volume, email is
written to the log until you configure a provider, and AI features stay off
until you point them at a model endpoint.

| Service | What it does |
|---|---|
| `caddy` | The only service with published ports (80, 443, 443/udp). Gets and renews the certificate, sends `/api/v1/*` and `/media/*` to the backend and everything else to the frontend. |
| `frontend` | Next.js. It reads its settings when it starts, so one image works for any domain. |
| `backend` | Go API. Applies database migrations when it starts. |
| `postgres` | PostgreSQL 18 on an internal network with no route out and no published port. |
| `backup` | Profile `backup`, on by default. Nightly database dump plus an archive of the uploads. |

Optional profiles: `backup-offsite` (copies backups to any rclone remote),
`minio` (S3-compatible storage on the same host), `restore` (one-shot
restore job).

Contents: [Requirements](#requirements) ·
[Install](#install) · [Local trial](#local-trial) ·
[Configuration](#configuration) · [Admin access](#admin-access) ·
[Upgrades](#upgrades) · [Backups](#backups) · [Restore](#restore) ·
[Off-site copies](#off-site-copies) · [MinIO](#minio) ·
[Cloudflare](#cloudflare) · [Build from source](#build-from-source) ·
[Metrics and logs](#metrics-and-logs) ·
[Troubleshooting](#troubleshooting) · [Uninstall](#uninstall)

## Requirements

- A Linux server with 2 CPUs, 4 GB of RAM and 20 GB of disk. Building the
  images yourself needs about 8 GB of RAM; pulling them does not.
- Docker Engine with the Compose plugin, version 2.20 or newer (`docker compose
  version`). See https://docs.docker.com/engine/install/.
- A domain name whose A (and, if the server has IPv6, AAAA) record points at
  the server. Ports 80 and 443 must be reachable from the internet so that
  Let's Encrypt can verify the domain.

## Install

Download the installer, read it, run it:

```sh
curl -fsSLO https://github.com/stdevMac/payverge/releases/latest/download/install.sh
less install.sh
bash install.sh
```

It asks for three things: the domain, the admin email, and whether to load
demo restaurants. Then it:

1. downloads the deploy files of the release and checks them against the
   release's `SHA256SUMS` (and its cosign signature, when cosign is installed);
2. writes `.env` (mode 600) with new random secrets and a generated admin
   password;
3. runs `docker compose up -d` and waits for `/api/v1/health/ready`;
4. checks the backend and the frontend through Caddy, at the public URL;
5. prints the URL, the admin email and the password. The password is shown
   **once** and then removed from `.env`, so save it in your password manager.
   If step 4 failed, it then says which check failed and exits non-zero.

As root it installs into `/opt/payverge`; otherwise into `~/payverge`. For an
unattended install, pass everything as flags:

```sh
bash install.sh --yes --domain pos.example.com --admin-email you@example.com \
  --acme-email you@example.com --version 1.2.3 --dir /opt/payverge
```

| Flag | Meaning |
|---|---|
| `--domain NAME` | Public host name, or `localhost` for a trial. |
| `--admin-email EMAIL` | The first platform admin. |
| `--acme-email EMAIL` | Contact address for the Let's Encrypt account (optional). |
| `--demo` / `--no-demo` | Seed demo restaurants owned by the admin on the first start. |
| `--dir DIR` | Install directory. |
| `--version V` | Release to install or upgrade to (`1.2.3`, `v1.2.3` or `latest`). The installer pins the exact version in `.env`. |
| `--http-port [ADDR:]N`, `--https-port [ADDR:]N` | Host ports, optionally with a bind address (`127.0.0.1:8443`). A `localhost` trial binds to `127.0.0.1` unless you give another address. Set `COMPOSE_PROJECT_NAME` in the environment to name the compose project; the installer saves it to `.env`. |
| `--yes` | Never prompt; fail when a required value is missing. |
| `--dry-run` | Print what would happen and change nothing. |
| `--force` | Rewrite an existing `.env`. The old file is kept as `.env.bak-<time>`, and the database, JWT and plugin secrets are carried over. |
| `--build` | Build the images from the clone instead of pulling them (see [Build from source](#build-from-source)). |
| `--no-start` | Write the files and `.env` but do not start anything. |
| `--no-backup` | Upgrade without taking a backup set first (see [Upgrades](#upgrades)). |
| `--require-signature` | Verify `SHA256SUMS` with cosign before installing a release. Refuse to install if cosign is missing or the signature does not check. |

Each flag can also come from the environment: `PAYVERGE_DOMAIN`,
`PAYVERGE_ADMIN_EMAIL`, `PAYVERGE_ACME_EMAIL`, `PAYVERGE_DEMO`, `PAYVERGE_DIR`,
`PAYVERGE_INSTALL_VERSION`, `PAYVERGE_HTTP_PORT`, `PAYVERGE_HTTPS_PORT`,
`PAYVERGE_YES`, `PAYVERGE_DRY_RUN`, `PAYVERGE_FORCE`, `PAYVERGE_BUILD`,
`PAYVERGE_NO_BACKUP` and `PAYVERGE_REQUIRE_SIGNATURE`. Four more have no flag:
`PAYVERGE_READY_TIMEOUT` (seconds to wait for the health check, default 600),
`PAYVERGE_PROBE_ATTEMPTS` (tries, 5 seconds apart, of the public check,
default 24), `PAYVERGE_DOCKER_TIMEOUT`
(seconds before an unresponsive Docker daemon counts as down, default 30) and
`PAYVERGE_RELEASES_URL` (a mirror laid out like GitHub releases).

When a step fails, the installer stops and names the line and the command
that failed, and nothing after it runs. If the containers do not start, it
shows their state and the last log lines first. Fix the cause and run it
again.

Running the installer again is safe. It keeps `.env`, keeps the pinned
version unless you pass `--version`, and starts whatever is not running.
Upgrading replaces the files that come with a release (`docker-compose.yml`,
the Caddy files, `backup/`, this README). Keep your own changes in `.env` or
in a `docker-compose.override.yml`. Compose merges that file automatically
only while `COMPOSE_FILE` is unset. When `.env` sets `COMPOSE_FILE` (as
`--build` does), add the override to the list yourself:
`COMPOSE_FILE=docker-compose.yml:docker-compose.build.yml:docker-compose.override.yml`.

### From a clone

```sh
git clone https://github.com/stdevMac/payverge.git
cd payverge
./deploy/install.sh
```

Run from a clone, the installer works in place in `deploy/`. With no
`--version` and no `--build`, it pins `PAYVERGE_VERSION` to the newest
published release and never writes `latest`. `--build`, or an existing `.env`
whose `COMPOSE_FILE` lists `docker-compose.build.yml`, pins
`PAYVERGE_VERSION=local` (the images are `payverge-*:local`). A re-run keeps
the version already in `.env`, except a leftover `local` when nothing is
being built, which is resolved to a release. With `--dir` it copies the
deploy files somewhere else first. To upgrade a release install, run its own
copy: `bash /opt/payverge/install.sh --version 1.3.0` (an install from before
the first public release fetches the new installer first; see
[PostgreSQL 15 to 18](#postgresql-15-to-18)). Before the first release is
published, there is nothing to pin: the installer says so and points at
`--build` or `--version`.

### By hand

```sh
cp .env.example .env
chmod 600 .env
$EDITOR .env          # fill in the "Required" block
docker compose up -d
```

### On a managed platform

Coolify, Dokploy, Render and Railway templates (untested on the platforms so
far) are in [platforms/README.md](platforms/README.md). How they differ from
this stack: [docs/self-hosting/platforms.md](../docs/self-hosting/platforms.md).

## Local trial

```sh
bash install.sh --domain localhost --admin-email you@example.com --demo
```

From a clone, run `./deploy/install.sh` with the same flags (plus `--build`
to build the images from the checkout, see [Build from source](#build-from-source)).

Caddy issues the certificate for `localhost` (and for `*.localhost`, `*.local`,
`*.internal`, `*.home.arpa` and bare IP addresses) from its own local CA, so
browsers warn until you trust that CA:

```sh
docker compose cp caddy:/data/caddy/pki/authorities/local/root.crt ./payverge-local-ca.crt
```

Import `payverge-local-ca.crt` into the system or browser trust store, or
accept the warning once. A `localhost` trial publishes Caddy on `127.0.0.1`
only, so other machines on your network cannot reach it; pass
`--https-port 0.0.0.0:443` to open it up. If ports 80 and 443 are already in
use, pass `--http-port 8080 --https-port 8443`. The installer then also sets
`PUBLIC_URL=https://localhost:8443`, which is the address to open. Open it
directly: Caddy's HTTP-to-HTTPS redirect on the HTTP port does not know about
the moved HTTPS port and sends the browser to `https://localhost/`.

## Configuration

Everything lives in `.env` next to `docker-compose.yml`. The "Required" block
at the top is all a working instance needs; every other setting is a commented
example in `.env.example`. After an edit, apply it with:

```sh
docker compose up -d
```

Compose recreates only the services whose settings changed.

> [!WARNING]
> Never change `DB_PASSWORD`, `JWT_SECRET_KEY` or `PLUGIN_SECRET_KEY` on a running
> install. The database keeps the old password, every session ends, and payment
> credentials encrypted with the old plugin key can no longer be read.

In `.env`, `$` starts a variable. Write a literal `$` as `$$`, or put the
value in single quotes.

| Topic | Guide |
|---|---|
| First admin, signups (`REGISTRATION_MODE`), demo data | [docs/self-hosting/admin.md](https://github.com/stdevMac/payverge/blob/main/docs/self-hosting/admin.md) |
| Email: SMTP, Resend, Postmark, or log only | [docs/self-hosting/email.md](https://github.com/stdevMac/payverge/blob/main/docs/self-hosting/email.md) |
| AI: OpenRouter, Ollama or any OpenAI-compatible endpoint, budgets | [docs/self-hosting/ai.md](https://github.com/stdevMac/payverge/blob/main/docs/self-hosting/ai.md) |
| Uploads: local volume, S3, R2, MinIO | [docs/self-hosting/storage.md](https://github.com/stdevMac/payverge/blob/main/docs/self-hosting/storage.md) |
| Frontend settings: network, analytics, maintenance mode | [docs/self-hosting/frontend-config.md](https://github.com/stdevMac/payverge/blob/main/docs/self-hosting/frontend-config.md) |
| WhatsApp (needs a source build) | [docs/self-hosting/whatsapp.md](https://github.com/stdevMac/payverge/blob/main/docs/self-hosting/whatsapp.md) |

Those guides were written for the repository's development compose and
sometimes say "the root `.env`". For this stack it is the `.env` in this
directory.

The backend checks its configuration when it starts. A missing or invalid
setting stops it with an error that names the variable:

```sh
docker compose logs backend | tail -n 30
```

## Admin access

The installer creates the admin account from `ADMIN_EMAIL` and the generated
password on the first start. The backend never overwrites an existing
account's password from `.env`, so use the command line to set a new one. The
image has no shell, so pipe the password in:

```sh
# Prompts without echoing, then sets the password.
read -rs NEW_PASSWORD && printf '%s' "$NEW_PASSWORD" | docker compose exec -T backend \
  /app/server admin reset-password --email you@example.com --password-stdin
unset NEW_PASSWORD
```

Passwords need at least 12 characters and at least 5 different characters.
Common defaults such as `changeme` are refused.

```sh
# Another platform admin:
printf '%s' "$PASSWORD" | docker compose exec -T backend \
  /app/server admin create --email ops@example.com --password-stdin

# Demo restaurants after the fact, or wipe and re-seed them:
docker compose exec -T backend /app/server demo seed --owner-email you@example.com
docker compose exec -T backend /app/server demo reset --owner-email you@example.com
```

## Upgrades

The installer pins the image version in `.env` (`PAYVERGE_VERSION=1.2.3`).
To upgrade:

```sh
bash install.sh --version 1.3.0
```

Before it changes anything, the installer takes a backup set with the old
version (`docker compose run --rm backup once`). It does so whenever the
version changes, whenever the version is `latest`, and on every `--build`. If
that backup fails, it stops and leaves the old files, version and containers
as they were. Then it installs the new release files, sets the new version,
and runs `docker compose pull` and `docker compose up -d`, then recreates the
`caddy` container (`docker compose up -d --force-recreate --no-deps caddy`) so
it loads the new Caddyfile. It prints the name of the pre-upgrade set and the
exact commands that restore it. `--no-backup` skips the backup; only use it
when you have just taken one. It does not skip keeping the images of an
install from before the first public release under `VERSION-pg15`.

By hand, take the backup yourself first, then set `PAYVERGE_VERSION` in `.env`
and pull and start:

```sh
docker compose run --rm backup once
# set PAYVERGE_VERSION=1.3.0 in .env
docker compose pull
docker compose up -d
docker compose up -d --force-recreate --no-deps caddy   # load a changed Caddyfile
```

Read the release notes first, because a release can add a required setting.

The backend applies new database migrations when it starts. Migrations only
run forward, and an older backend refuses to start on a newer schema. **To
roll back, restore the backup you took before the upgrade**, with
`PAYVERGE_VERSION` set to the old version. Name that set explicitly (the
installer printed it): a nightly set taken after the upgrade already has the
new schema.

```sh
docker compose stop backend frontend
# set PAYVERGE_VERSION back to the old version in .env
ls backups/daily              # the set taken before the upgrade, e.g. 20261003T091500Z
docker compose --profile restore run --rm restore daily/20261003T091500Z --yes
docker compose up -d
```

Anything written to the database after the upgrade is lost in a rollback.

### PostgreSQL 15 to 18

Every public release runs PostgreSQL 18 and keeps the database in the
`pgdata` volume. Installs from before the first public release ran
PostgreSQL 15 in the `db` volume. To upgrade one of those, first keep a copy
of the deploy files it runs (releases before 1.0.0 cannot be downloaded
again, and that copy is the rollback; the installer also tags the cached
images as `<version>-pg15`, because a pre-release install pinned to 1.0.0
shares that tag with the release), then fetch the new installer rather
than running the old local `install.sh`, which has no PostgreSQL 18 guard:

```sh
curl -fsSLO https://github.com/stdevMac/payverge/releases/latest/download/install.sh
bash install.sh --version 1.0.0
```

When `db` still holds PostgreSQL 15 data, `install.sh` stops before starting
the stack and the postgres service waits with a message instead of creating
an empty database. Move the data once:

```sh
./upgrade-postgres.sh --dry-run   # the plan; changes nothing
./upgrade-postgres.sh             # dump with 15, restore into 18, compare row counts, start
```

The `db` volume is never removed, and the dump and row counts stay in
`backups/pg18-upgrade-<time>/`. To roll back, run `docker compose down`, put
back the deploy files you kept with `PAYVERGE_VERSION` set to the kept tag
the installer printed, and `docker compose up -d`: PostgreSQL 15
runs on `db` again, and what was written on 18 is lost. A backend that
reports `postgres major version X does not match the genesis baseline's Y`
is running against the other major's data. Details:
[docs/self-hosting/upgrades.md](../docs/self-hosting/upgrades.md#postgresql-18).

### Verifying images

The release workflow signs the backend and frontend images with a keyless
cosign signature tied to `.github/workflows/release.yml`. `docker compose pull`
does not check it, and `PAYVERGE_VERSION` is a tag, which can be moved. To
check what you run, install [cosign](https://docs.sigstore.dev/cosign/system_config/installation/)
and verify each image before starting it:

```sh
for image in backend frontend; do
  cosign verify "ghcr.io/stdevmac/payverge-$image:1.3.0" \
    --certificate-oidc-issuer https://token.actions.githubusercontent.com \
    --certificate-identity https://github.com/stdevMac/payverge/.github/workflows/release.yml@refs/heads/main
done
```

The output names the digest that was signed (`"docker-manifest-digest"`). To
make Compose run exactly that image even if the tag later moves, pin the
digest in `docker-compose.override.yml`:

```yaml
services:
  backend:
    image: ghcr.io/stdevmac/payverge-backend:1.3.0@sha256:<digest from cosign>
  frontend:
    image: ghcr.io/stdevmac/payverge-frontend:1.3.0@sha256:<digest from cosign>
```

The pin wins over `PAYVERGE_VERSION`, so update or remove it on every upgrade,
including one done with `install.sh --version`.

The installer checks the release tarball against `SHA256SUMS`. Both files
come from the same release, so the checksum alone catches a damaged download,
not a replaced one. Releases also publish `SHA256SUMS.sigstore.json`, a
keyless cosign bundle for `SHA256SUMS` (`cosign sign-blob --bundle`). When
`cosign` is on `PATH`, the installer verifies that bundle against the release
workflow (`https://github.com/stdevMac/payverge/.github/workflows/release.yml@refs/heads/main`,
issuer `https://token.actions.githubusercontent.com`) before trusting the
checksums. Without cosign it checks only the checksum and says how to require
the signature. `--require-signature` (`PAYVERGE_REQUIRE_SIGNATURE=1`) refuses
to install unless that check succeeds. Install cosign from
https://docs.sigstore.dev/cosign/system_config/installation/.

## Backups

The `backup` profile is on by default (`COMPOSE_PROFILES=backup` in `.env`).
Every night at `BACKUP_TIME` (03:30, in `TZ`, UTC by default) it writes a set
to `./backups`:

```text
backups/
  daily/20261003T033000Z/       one set per night; the newest 7 are kept
    db.dump                     pg_dump, custom format
    storage.tar.gz              uploaded files (local storage driver)
    minio.tar.gz                MinIO data, only when the minio profile is used
    MANIFEST                    version, time, sizes
    SHA256SUMS
  weekly/2026-W40/              first set of each ISO week; 4 kept
  monthly/2026-10/              first set of each month; 6 kept
```

Weekly and monthly sets are hard links to a daily set, so they use no extra
disk space until the daily set is pruned. Change the retention with
`BACKUP_KEEP_DAILY`, `BACKUP_KEEP_WEEKLY` and `BACKUP_KEEP_MONTHLY`, and the
location with `BACKUP_DIR`.

The job also takes a set as soon as it starts with an empty `backups/daily`.
On a fresh install it first waits up to 10 minutes for the backend to create
the database schema. A set of a database with no schema stays in `daily/` and
never becomes a weekly or monthly set.

```sh
docker compose run --rm backup once     # take a set now
docker compose logs backup              # last runs
ls backups/daily
```

The job runs as root inside its container. When the installer runs as a
normal user it sets `BACKUP_UID` and `BACKUP_GID` in `.env` to that user, and
the job hands each set to them, so you can read and copy `backups/` without
`sudo`. Set them yourself for any other owner.

The database dump is consistent: `pg_dump` reads one snapshot while the
application keeps running. The uploads and the MinIO data are copied file by
file while the application keeps running too, so a file written during the
copy can be missing from the set. When copying a volume fails (a file changed
or vanished under `tar`), the job tries again up to `BACKUP_ARCHIVE_ATTEMPTS`
times (3). If it still fails, the set is written without that archive, its
`MANIFEST` says `archives_failed=storage` (or `minio`), it does not become a
weekly or monthly set, and the job exits with an error. For an exact copy of
the MinIO data, stop MinIO while the set is taken:

```sh
docker compose stop minio
docker compose run --rm backup once
docker compose start minio
```

The sets contain the whole database, including customer data and encrypted
payment credentials. `.env` is **not** in them: keep a copy of `.env` in your
password manager, because restoring onto a new server needs the same
`PLUGIN_SECRET_KEY`.

A backup that only lives on the same disk is not a backup. Copy `backups/`
somewhere else: see [Off-site copies](#off-site-copies).

With `STORAGE_DRIVER=s3` and an external bucket, the uploads are not in the
sets. Back up the bucket with your provider's own versioning or replication.

## Restore

A restore **replaces** the database and the uploads with the chosen set.

```sh
docker compose stop backend frontend                    # also minio, if you use it
docker compose --profile restore run --rm restore latest --yes
docker compose up -d
```

Instead of `latest`, name a set: `daily/20261003T033000Z`, `weekly/2026-W40`
or `monthly/2026-10`. Add `--skip-storage` to restore only the database.

The job checks the set against its `SHA256SUMS` first, then refuses to run
while anything else is connected to the database. It runs `pg_restore` into a
temporary database (`payverge_restore`) and unpacks the uploads (and MinIO
data) into a `.payverge-restore-staging` directory inside each volume. Only
when all of that succeeded does it rename the restored database into place and
swap the volume contents; a corrupt dump or archive leaves the live data as it
was. The volumes need free space for a second copy of the set while it runs.
The backend must be the version in the set's `MANIFEST`, or newer.

### Moving to a new server

1. On the new server, install the same version with `--no-start`, then
   replace the new `.env` with your saved one.
2. Copy the backup set into `backups/daily/` there.
3. Start only the database, restore, then start everything:

   ```sh
   docker compose up -d postgres
   docker compose --profile restore run --rm restore latest --yes
   docker compose up -d
   ```

4. Point DNS at the new server.

Try this once on a spare machine before you need it. Until you have restored a
set, you have no evidence that your backups work.

## Off-site copies

The `backup-offsite` profile runs [rclone](https://rclone.org) every hour to
copy `backups/` to any rclone remote: S3, B2, an SFTP server, another machine
of yours, and so on.

```sh
mkdir -p rclone
docker run --rm -it -v "$PWD/rclone:/config/rclone" rclone/rclone:1.75.1 config
```

Then set the following in `.env` and run `docker compose up -d`:

```sh
COMPOSE_PROFILES=backup,backup-offsite
RCLONE_REMOTE=offsite:payverge-backups
# copy (default) keeps sets that were pruned here; sync mirrors the pruning.
RCLONE_MODE=copy
```

`rclone/rclone.conf` holds the remote's credentials; keep it mode 600. Use a
remote that encrypts (rclone `crypt`) if the destination is not yours.

## MinIO

Uploads go to a local volume by default, which is the simplest and is backed
up with everything else. To use S3-compatible storage without a cloud
account, enable the bundled MinIO. `install.sh` already wrote a
`MINIO_ROOT_PASSWORD`; MinIO refuses to start with an empty one. Generate any
password you set by hand with `openssl rand -hex 24`, then set in `.env`:

```sh
COMPOSE_PROFILES=backup,minio
STORAGE_DRIVER=s3
MINIO_ROOT_PASSWORD=<generated>
S3_ENDPOINT=http://minio:9000
S3_PROTECTED_ENDPOINT=http://minio:9000
S3_FORCE_PATH_STYLE=true
S3_BUCKET=payverge-public
S3_PROTECTED_BUCKET=payverge-protected
AWS_ACCESS_KEY=payverge-app
AWS_SECRET_KEY=<generated>
AWS_PROTECTED_ACCESS_KEY=payverge-app
AWS_PROTECTED_SECRET_KEY=<same as AWS_SECRET_KEY>
```

Start it with `docker compose up -d`. `minio-init` creates both buckets with
anonymous access off, and creates the `payverge-app` user so that the backend
never holds the root credentials. Files are served to browsers through the
backend at `/media/...`, so MinIO is never exposed. The nightly backup
includes the MinIO volume.

Switching drivers does not move files that were already uploaded. Choose the
driver before you upload menus and photos, or see the storage guide.

MinIO is AGPL-3.0 and runs here as its own unmodified program.

## Cloudflare

To put Cloudflare's proxy (the orange cloud) in front of the server:

1. Set SSL/TLS mode to **Full (strict)**.
2. Get the first certificate before you turn the proxy on: keep the DNS record
   "DNS only" (grey cloud) until `https://your-domain` works, then switch it to
   proxied. Caddy renews in the background while the certificate is still
   valid. If you prefer, leave the proxy on and turn off "Always Use HTTPS"
   instead, since Caddy already redirects HTTP to HTTPS.
3. Set `EDGE=cloudflare` in `.env` and run `docker compose up -d`.

With `EDGE=cloudflare`, Caddy uses `Caddyfile.cloudflare`. It takes the client
address from `CF-Connecting-IP`, and only from Cloudflare's published ranges in
`cloudflare-cidrs.caddy`. Without that setting, every visitor would appear to
come from a Cloudflare address, and per-client rate limits would then hit
everyone at once. A new release refreshes the ranges.

### Other proxies and load balancers

For another proxy or load balancer in front of Caddy, keep `EDGE=none` and set
`TRUSTED_PROXY_CIDRS` to the addresses it connects from, and nothing broader:

```sh
TRUSTED_PROXY_CIDRS=10.0.0.0/24
```

The value is a list of IPv4 or IPv6 addresses or CIDRs separated by spaces, not
commas, and quoted when it has more than one entry
(`TRUSTED_PROXY_CIDRS="10.0.0.0/24 fd00::/64"`). `install.sh` refuses any other
format and any range broader than `/8` (IPv4) or `/16` (IPv6), because
Caddy splices the value into its own configuration and an over-broad range
(a `/0`, or two `/1` halves) would trust every client's `X-Forwarded-For`.

Such a proxy usually appends the address it saw to the `X-Forwarded-For` the
client sent. Caddy therefore reads that header from the right and takes the
first address that is not in `TRUSTED_PROXY_CIDRS`, so a value the client wrote
itself is never used. A proxy that replaces the header works the same way.

Whatever the edge, Caddy passes exactly one client address to the apps, in
`X-Forwarded-For`, and removes any `X-Real-IP`, `CF-Connecting-IP`,
`True-Client-IP`, `X-Client-IP` and `Forwarded` header a client sent.

Behind Caddy, the backend and the frontend trust that one address because the
compose file gives the edge network a fixed subnet, `EDGE_SUBNET` (default
`172.30.0.0/24`), and sets `TRUSTED_PROXIES` and `FRONTEND_TRUSTED_PROXIES` to
it (the backend also trusts loopback). No other private range is trusted.
Neither app publishes a host port, so only Caddy (and, for the backend, the
frontend's proxy) can reach them. The binaries alone trust less: loopback only
for the backend and nothing for the frontend. If `172.30.0.0/24` is already
used on the host (a second Payverge stack, a VPN), set another `EDGE_SUBNET`
in `.env` before the first start; changing it later means `docker compose
down` first, so the network is recreated.

### Behind another web server

When another web server on the host already owns ports 80 and 443 (nginx,
Apache, Traefik), let it keep them: it terminates TLS for the domain and
forwards plain HTTP to Caddy on a loopback port.

1. Add to `.env`, keeping `EDGE=none` (the default) and leaving `PUBLIC_URL`
   unset, since visitors still open `https://DOMAIN`:

   ```sh
   TLS_TERMINATION=proxy
   HTTP_PORT=127.0.0.1:8080
   HTTPS_PORT=127.0.0.1:8443
   ```

   With `TLS_TERMINATION=proxy`, Caddy serves `http://DOMAIN` without a
   certificate and without a redirect to HTTPS, and still adds the security
   headers. The `127.0.0.1:` prefix keeps both ports off the network, so the
   only way in is through your web server; `install.sh` refuses to start in
   this mode while either port lacks a bind address (write `0.0.0.0:PORT` if
   the web server really is on another host). Nothing answers behind
   `HTTPS_PORT` in this mode, but Compose still publishes it, so give it any
   free port. To install this way, run the installer with `--no-start`, edit
   `.env`, then run `./install.sh` again to start.

2. Have your web server forward the domain to `http://127.0.0.1:8080`. It
   must keep the `Host` header (otherwise Caddy answers with an empty page),
   append the client address to `X-Forwarded-For`, pass responses through
   unbuffered (the dashboards are live event streams), wait up to 300 seconds
   for AI requests, and accept 15 MB uploads. With nginx:

   ```nginx
   server {
       listen 443 ssl;
       server_name pay.example.com;
       # ssl_certificate and ssl_certificate_key: your existing certificate

       client_max_body_size 15m;
       location / {
           proxy_pass http://127.0.0.1:8080;
           proxy_http_version 1.1;
           proxy_set_header Host $host;
           proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
           proxy_buffering off;
           proxy_read_timeout 300s;
       }
   }
   ```

   Let your web server redirect HTTP to HTTPS for the domain as well; Caddy
   no longer does.

3. Tell Caddy to trust that server. Docker hands the connections it accepts on
   `127.0.0.1:8080` to Caddy from the gateway address of the `payverge_edge`
   network (`<project>_edge` when `COMPOSE_PROJECT_NAME` is set;
   `docker network ls --filter label=com.docker.compose.network=edge` prints the exact name). With the stack running,
   print that address:

   ```sh
   docker network inspect payverge_edge --format '{{range .IPAM.Config}}{{.Gateway}}{{end}}'
   ```

   Set it in `.env`, for example `TRUSTED_PROXY_CIDRS=172.18.0.1/32`, and run
   `docker compose up -d`. Without this, every visitor appears to come from
   that address and shares one rate limit. After `docker compose down`, Docker
   can give the network another range, so check the address again after the
   next start.

If your web server runs on another machine, publish `HTTP_PORT` on an address
it can reach, and set `TRUSTED_PROXY_CIDRS` to the address it connects from.

### HSTS

Caddy sends `Strict-Transport-Security: max-age=31536000` for the domain
itself and replaces the value the apps send. It does not add
`includeSubDomains`, because that would force HTTPS on every other host under
your domain. Once all of them serve HTTPS, opt in with
`HSTS_POLICY=max-age=31536000; includeSubDomains` in `.env`.

### Rate limiting

The backend limits API requests per client address. Caddy here is the stock
image and adds no limits of its own, so page requests to the frontend are not
rate limited. Put a CDN or WAF with rate limiting (Cloudflare, for example) in
front of an instance that is exposed to heavy or abusive traffic.

## Build from source

From a clone, build the images instead of pulling them:

```sh
./deploy/install.sh --build
```

This sets `COMPOSE_FILE=docker-compose.yml:docker-compose.build.yml` and
`PAYVERGE_VERSION=local` in `.env`, so every later `docker compose` command
builds `payverge-backend:local` and `payverge-frontend:local` from
`../backend` and `../frontend`. The first build takes 10 to 30 minutes and
about 8 GB of RAM.
After `git pull`, run `docker compose up -d --build`.

WhatsApp messaging uses a GPL-3.0 library and is left out of the published
images. To include it, set `GO_TAGS=whatsapp` in `.env` and build from
source; the resulting backend image is then distributed under GPL-3.0 terms.
See [docs/self-hosting/whatsapp.md](https://github.com/stdevMac/payverge/blob/main/docs/self-hosting/whatsapp.md).

## Metrics and logs

```sh
docker compose ps                  # health of each service
docker compose logs -f backend     # follow one service
```

Logs are rotated at 10 MB, keeping 5 files per service.

Prometheus metrics are served by the backend at `http://backend:8080/metrics`.
That path is not routed through Caddy, so scrape it from a container on the
`payverge_edge` network (`<project>_edge` when `COMPOSE_PROJECT_NAME` is set; `docker network ls --filter label=com.docker.compose.network=edge` prints the exact name), with `METRICS_TOKEN` set in
`.env` as the bearer token.

`monitoring/alerts.yml` holds Prometheus alerting rules for the failures that
need a person: the backend down or not ready, payment webhooks failing, an
AFIP CAE without its receipt, and fiscal receipts or emails dead-lettered.
Add it to your Prometheus `rule_files:`; the comment at its top names the scrape
jobs it expects. `promtool test rules monitoring/alerts.test.yml` checks it.

The database superuser (`postgres`) has no password and only connects from
inside its container: `docker compose exec postgres psql -U postgres`. The
backend, backups and restores use `DB_USER`, which owns the database but is
not a superuser.

## Troubleshooting

**The certificate is not issued.** Check `docker compose logs caddy`. The
usual causes are a DNS record that does not point at this server yet, ports 80
and 443 that are closed in a cloud firewall or by `ufw`, or a Cloudflare proxy
switched on before the first certificate (see [Cloudflare](#cloudflare)).
Caddy keeps retrying with back-off, so after you fix the cause, wait or run
`docker compose restart caddy`.

**The backend keeps restarting.** `docker compose logs backend` names the
setting it rejected. After fixing `.env`, run `docker compose up -d`.

**A setting in `.env` seems to be ignored.** Docker Compose prefers a variable
exported in your shell over the same name in `.env`. If your shell exports, for
example, `OPENROUTER_API_KEY`, `DOMAIN` or `EMAIL_PROVIDER` (from a profile or
another project), that value reaches the containers instead. Check with
`docker compose config | grep NAME`, then `unset` it or run
`env -u NAME docker compose up -d`.

**"Database migration failed" or "Schema verification failed" after a
downgrade.** The image is older than the data. Set `PAYVERGE_VERSION` back to
the version you upgraded to, or restore a backup taken with the older version
([Upgrades](#upgrades)).

**"postgres major version X does not match the genesis baseline's Y".** The
backend and the database are on different PostgreSQL majors: 15 against 18
means the data still needs `./upgrade-postgres.sh`; 18 against 15 means an
image from before 1.0.0 is running on upgraded data. See
[PostgreSQL 15 to 18](#postgresql-15-to-18).

**Every request comes from the same IP.** When Docker forwards a connection
through its userland proxy instead of its iptables rules (IPv6 without
`ip6tables`, or a host where Docker manages no iptables), Caddy sees Docker's
gateway address instead of the client's. Rate limits and the audit log then
treat all visitors as one. Set `"ip6tables": true` (and, optionally,
`"userland-proxy": false`) in `/etc/docker/daemon.json` and restart Docker, or
publish only an A record. Behind Cloudflare, use `EDGE=cloudflare`.

**Port 80 or 443 is already in use.** Stop the other web server, or let it
terminate TLS and forward to Caddy over plain HTTP (see
[Behind another web server](#behind-another-web-server)). For a trial only,
`--http-port 8080 --https-port 8443` moves Caddy to other ports; a public
certificate still needs 80 and 443.

**The installer says a database volume already exists.** An earlier install
left its data behind, but its `.env` is gone. Put the old `.env` back, or
delete the old data (see [Uninstall](#uninstall)).

## Uninstall

```sh
docker compose down          # stop and remove the containers; data is kept
```

To delete everything, including the database, the uploads and the
certificates, add `-v`. This cannot be undone, so take a backup and copy it
off the server first:

```sh
docker compose --profile '*' down -v --rmi all
```

Then remove the install directory, which holds `.env` and `backups/`.
