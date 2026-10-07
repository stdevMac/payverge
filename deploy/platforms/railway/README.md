# Payverge on Railway

**Status: untested.** These steps follow Railway's documented features
(Docker image services, reference variables, private networking, volumes);
nobody has run them end to end yet. Please open an issue with what you hit.

Railway templates are built in Railway's UI, not from a file in the repo, so
this page lists the exact services and variables to create. Read
[docs/self-hosting/platforms.md](../../../docs/self-hosting/platforms.md) for
the limits that apply to every platform template.

## Services

Create one project with three services. The names matter: the reference
variables and private hostnames below use them.

| Service | Source | Public domain |
|---|---|---|
| `Postgres` | Railway's PostgreSQL database | none |
| `backend` | Docker image `ghcr.io/stdevmac/payverge-backend:X.Y.Z` | **none** |
| `frontend` | Docker image `ghcr.io/stdevmac/payverge-frontend:X.Y.Z` | yes, port 3000 |

Only the frontend gets a domain. It proxies `/api/v1/*` and `/media/*` to the
backend over the private network, so the whole app is one origin.

`X.Y.Z` is one release tag for both images (the newest
[release](https://github.com/stdevMac/payverge/releases), not `latest`, so a
redeploy never moves one service to a newer release than the other); change
both together to upgrade.

## Postgres

Use Railway's database as is. The backend's `DB_*` variables reference its
user, database and password, so changing them needs no other edit.

## backend

No custom start command: the image's entrypoint is the server, which takes
its database location from the `DB_*` variables below when no `--db-*` flag
is given, and production mode from `APP_ENV`. If you renamed the `Postgres`
service, change `Postgres` in the reference variables to match.

**Volume**: mount one at `/data/storage` (uploaded images and files).
Railway mounts volumes as root while the image runs as an unprivileged
user, so also set `RAILWAY_RUN_UID=0` (Railway's documented switch for this),
or switch to `STORAGE_DRIVER=s3` and skip the volume
([storage.md](../../../docs/self-hosting/storage.md)).

**Healthcheck path**: `/api/v1/health/live`.

**Variables**:

| Variable | Value |
|---|---|
| `APP_ENV` | `production` |
| `PORT` | `8080` |
| `PUBLIC_URL` | `https://${{frontend.RAILWAY_PUBLIC_DOMAIN}}` (or your custom domain) |
| `DB_HOST` | `${{Postgres.RAILWAY_PRIVATE_DOMAIN}}` |
| `DB_PORT` | `5432` |
| `DB_USER` | `${{Postgres.PGUSER}}` |
| `DB_NAME` | `${{Postgres.PGDATABASE}}` |
| `DB_PASSWORD` | `${{Postgres.PGPASSWORD}}` |
| `DB_SSLMODE` | `disable` (private network; the server's default is `require`) |
| `JWT_SECRET_KEY` | output of `openssl rand -base64 48` |
| `PLUGIN_SECRET_KEY` | output of `openssl rand -hex 32` (keep it: it encrypts payment credentials) |
| `ADMIN_EMAIL` | the first admin's email |
| `ADMIN_PASSWORD` | a long passphrase |
| `REGISTRATION_MODE` | `invite` |
| `EDGE` | `none` |
| `STORAGE_DRIVER` | `local` |
| `STORAGE_DIR` | `/data/storage` |
| `RAILWAY_RUN_UID` | `0` (only with the volume, see above) |
| `TRUSTED_PROXIES` | `fc00::/7,10.0.0.0/8` |

Set `TRUSTED_PROXIES` explicitly. Unset, the backend trusts loopback only, so
every request would carry the frontend's address instead of the visitor's.
`fc00::/7` covers Railway's IPv6 private network (`fd12::/16`); `10.0.0.0/8`
covers its IPv4 private network.

Email, AI and payment variables are optional; add the ones you need from
[deploy/.env.example](../../.env.example).

## frontend

**Networking**: generate a domain on port `3000`.

**Healthcheck path**: `/api/health`.

**Variables**:

| Variable | Value |
|---|---|
| `PORT` | `3000` |
| `PUBLIC_URL` | `https://${{RAILWAY_PUBLIC_DOMAIN}}` (same value as the backend's) |
| `BACKEND_INTERNAL_URL` | `http://${{backend.RAILWAY_PRIVATE_DOMAIN}}:8080` |
| `EDGE` | `none` |

`FRONTEND_TRUSTED_PROXIES` is left empty on purpose: Railway does not
document the address range its edge uses to reach your container. Empty is
safe (no client can forge its IP) but every visitor then shares the edge's
address for the backend's per-IP rate limits. If you find the range in your
logs and Railway confirms it is only their edge, set it there; see
[frontend-config.md, "Client IP trust"](../../../docs/self-hosting/frontend-config.md#client-ip-trust).

## Order and first boot

1. Deploy `Postgres`, then `backend`; wait for its healthcheck.
2. Deploy `frontend`.
3. Open the frontend domain and sign in with `ADMIN_EMAIL` / `ADMIN_PASSWORD`.

Keep exactly one replica of `backend` and `frontend`
([why](../../../docs/self-hosting/platforms.md#known-limits)).
