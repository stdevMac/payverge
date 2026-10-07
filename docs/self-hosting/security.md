# Security checklist

Payverge stores customer data and the payment credentials restaurants
connect. This page lists what the operator of an instance is responsible
for. To report a vulnerability in Payverge itself, see
[SECURITY.md](../../.github/SECURITY.md).

## Before you go live

- [ ] `.env` is mode 600, owned by the user that runs Compose, and not in any
      git repository. The installer creates it that way.
- [ ] A copy of `.env` is in your password manager. `PLUGIN_SECRET_KEY`
      decrypts the stored payment credentials; a restored database is useless
      without it ([backups.md](backups.md)).
- [ ] The admin password the installer printed is saved, and was not reused.
- [ ] `REGISTRATION_MODE` is `invite` (the default) or `closed`, unless you
      really want anyone on the internet to open a restaurant account.
- [ ] Only ports 80 and 443 (and 443/udp for HTTP/3) are open to the internet
      ([install.md](install.md#ports-and-firewall)). SSH is limited to your
      addresses or keys only.
- [ ] Client IPs resolve correctly ([reverse-proxies.md](reverse-proxies.md#checking-it)).
      Otherwise rate limits on login and payments do not protect anything.
- [ ] Backups run, are copied off the server, and you have restored one
      ([backups.md](backups.md)).
- [ ] You get release notes, for example by watching releases on the
      repository, and upgrade when a security fix ships ([upgrades.md](upgrades.md)).

## What is exposed

In `deploy/docker-compose.yml`, Caddy is the only service with published
ports. The backend and frontend listen only on the `payverge_edge` Docker
network (`<project>_edge` when `COMPOSE_PROJECT_NAME` is set); Postgres is on the internal `db` network with no route to the
internet and no published port. Do not add `ports:` to them in an override
file: the backend then sees connections that did not pass Caddy's header
cleaning.

Caddy routes `/api/v1/*`, `/media/*` and the AI routes to the backend and
everything else to the frontend. Paths outside `/api/v1/` on the backend,
such as `/metrics` and `/internal/_pprof_snapshot`, are therefore not
reachable from the internet at all; the tokens below are a second layer.

## Secrets

| Secret | Rotate by | Effect of rotating |
|---|---|---|
| `JWT_SECRET_KEY` | new value, `docker compose up -d` | Everyone is signed out. Unsubscribe links and crypto quotes signed with it stop working unless `UNSUBSCRIBE_TOKEN_SECRET` and `CRYPTO_QUOTE_SECRET` are set separately. |
| `PLUGIN_SECRET_KEY` | **Do not change it.** | Stored payment credentials become unreadable; restaurants must reconnect every provider. |
| `DB_PASSWORD` | `ALTER USER` in Postgres first, then `.env` | Only applied when the database is first created: changing `.env` alone locks the backend out. |
| Provider webhook secrets | `NAME_PREVIOUS` overlap ([payments.md](payments.md#webhook-secrets)) | None, with the overlap. |
| `METRICS_TOKENS` | prepend the new token, drop the old one later | None. |

The installer generates every secret with `openssl rand`. If you write
`.env` by hand, do the same; in production mode the backend refuses a
`JWT_SECRET_KEY` shorter than 32 characters or one published in the
repository (examples, compose defaults).

## Metrics, health and profiling

All are off or minimal by default.

| Endpoint | Gate | Notes |
|---|---|---|
| `GET /api/v1/health/live` | none | Returns `{"status":"alive"}` while the process runs. Safe for any uptime monitor. |
| `GET /api/v1/health/ready` | `HEALTH_DETAIL_TOKEN` for the detail | Without a valid token it returns only `{"status":"ready"}` (200) or `{"status":"not ready"}` (503), so public probes cannot fingerprint the instance. With `Authorization: Bearer <token>` it shows each dependency. A `?token=` query parameter is not accepted. |
| `GET /metrics` | `METRICS_TOKEN` / `METRICS_TOKENS` | Prometheus, `Authorization: Bearer <token>`. In production the route is not registered at all without a token. Not routed by Caddy; scrape it from a container on `payverge_edge` ([deploy/README.md](../../deploy/README.md#metrics-and-logs)). |
| `GET /internal/_pprof_snapshot` | `PPROF_TOKEN` in `X-Pprof-Token` | Go profiling. Not registered unless `PPROF_TOKEN` is set. Leave it unset except while debugging a performance problem, then remove it. |

Generate tokens with `openssl rand -hex 32`.

## Admin MCP API

Setting `PAYVERGE_ADMIN_MCP_TOKEN_SHA256` (or the clear-text
`PAYVERGE_ADMIN_MCP_TOKEN`) turns on an API that lets an AI assistant run
platform-admin actions. Leave it off unless you use it. If you do: store only
the SHA-256 hash in `.env`, restrict callers with
`PAYVERGE_ADMIN_MCP_ALLOWED_IPS`, and rotate the token if a machine that held
it is lost.

The token works only on a fixed set of diagnostic routes: health, fiscal
summary and jobs, fiscal job requeue, error logs, failed webhooks with
acknowledge, and business detail. Every other admin route returns 403 for it.
Requeueing a `failed_permanent` fiscal job needs an explicit
`allow_permanent` flag.

## Debug switches

`CLIENT_IP_DEBUG` and `PPROF_TOKEN` are for one-off use. Remove them from `.env`
once done ([configuration.md](configuration.md#admin-mcp-debug-and-maintenance-switches)).

## Data you hold

Backups contain the whole database: guest names, emails and phone numbers,
orders, and encrypted payment credentials. Copy them off the server only to
storage you control or encrypt with rclone `crypt`
([deploy/README.md](../../deploy/README.md#off-site-copies)). Depending on
where you and your restaurants operate, you may be a data processor under
privacy law; the retention settings in
[configuration.md](configuration.md#retention-and-background-workers) control
how long logs and transcripts are kept.
