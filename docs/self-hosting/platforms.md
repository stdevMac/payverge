# Self-hosting: managed platforms (Coolify, Dokploy, Render, Railway)

The supported install is the Docker Compose stack in `deploy/` with Caddy in
front ([deploy/README.md](../../deploy/README.md)). For people who already run
a PaaS, `deploy/platforms/` has starting points:

| Platform | Template | Status |
|---|---|---|
| Coolify v4 | `deploy/platforms/coolify/docker-compose.yml` | untested on Coolify; `docker compose config` passes |
| Dokploy | the same compose file, secrets set by hand | untested on Dokploy |
| Render | `deploy/platforms/render/render.yaml` (Blueprint) | untested on Render; passes Render's published JSON schema; one manual step, start-command form unverified |
| Railway | `deploy/platforms/railway/README.md` (UI steps) | untested |

"Untested" means exactly that: the files follow each platform's documentation
and the Payverge env contract, but nobody has deployed them yet. Reports and
fixes are welcome.

Setup steps per platform: [deploy/platforms/README.md](../../deploy/platforms/README.md).

## How the templates differ from the compose stack

| | Compose stack | Platform templates |
|---|---|---|
| TLS | Caddy (ACME) | the platform's proxy |
| `/api/v1/*`, `/media/*` | Caddy routes them to the backend | the Next server proxies them (`BACKEND_INTERNAL_URL`) |
| Public services | Caddy only | the frontend only; the backend has no domain |
| Uploads | `storage` volume | a volume/disk at `STORAGE_DIR`, or `STORAGE_DRIVER=s3` |
| Backups | `backup` profile | the platform's database backups; uploads are yours to back up |

The proxy contract (SSE streaming, 15 MB request bodies, 300 s upstream
timeout, header rules) is in
[frontend-config.md](frontend-config.md#same-origin-proxy).

## Known limits

**Run one replica of the backend and one of the frontend.** Several pieces
of state live in the backend process's memory:

- the per-IP rate limiters (`backend/internal/middleware/rate_limiter.go`);
- wallet sign-in challenges (`backend/internal/logic/challenge.go`): the
  signing key is random per process, so a challenge issued by one replica
  fails on another, and redeemed nonces are tracked per process;
- the realtime SSE hubs (`backend/internal/events/hub.go`,
  `table_hub.go`, `backend/internal/aiwaiterevents/hub.go`): an event
  published on one replica never reaches clients connected to another;
- with `STORAGE_DRIVER=local`, the upload directory itself.

Scaling out needs shared stores for these first; it is not supported today.
Disks on Render and volumes on Railway pin a service to one instance anyway.

**No Caddy edge.** What the compose stack's Caddy adds and you do not get
here:

- the edge default headers in `deploy/payverge.caddy`; the Next app and the
  backend set their own security headers (including HSTS from
  `frontend/next.config.mjs`), so this is a second layer, not the only one;
- Caddy's response compression and its long upstream timeouts. The
  platform's own proxy timeouts apply instead: if it closes idle or long
  connections, SSE streams reconnect and very slow uploads can fail;
- the bundled Caddy does not rate-limit requests either; all rate limiting is
  in the backend, in memory (see above).

**Client IPs come through the platform's proxy.** The backend rate-limits by
client IP. On these platforms the chain is: platform proxy → Next proxy →
backend. Two settings make the real client IP reach the backend:

- `FRONTEND_TRUSTED_PROXIES` on the frontend must cover the platform proxy's
  private address, so the Next proxy keeps its `X-Forwarded-For` chain.
  The Coolify template trusts the private ranges (the frontend publishes no
  host port there, so only containers on the stack network can reach it);
  the Render template trusts `10.0.0.0/8`; Railway leaves it empty because
  its edge range is undocumented.
- `TRUSTED_PROXIES` on the backend must cover the frontend **and** the
  platform proxy. Left empty, the backend trusts loopback only
  (`127.0.0.0/8,::1`), which covers neither the frontend container nor the
  platform proxy, so operators must set `TRUSTED_PROXIES` explicitly to
  cover the frontend and the platform proxy's private range (e.g. the
  platform's private network CIDR) in the platform's environment. Without
  it, every request appears to come from the frontend container and all
  visitors share one rate-limit bucket.

When `FRONTEND_TRUSTED_PROXIES` misses the platform proxy, nothing can be
forged, but every visitor shares one rate-limit bucket. Details and a worked
example: [frontend-config.md, "Client IP trust"](frontend-config.md#client-ip-trust).

**The backend reads its database location from the environment.** The
server's `--db-host`, `--db-port`, `--db-user`, `--db-name` and
`--db-sslmode` flags default to `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_NAME`
and `DB_SSLMODE`, and the password comes from `DB_PASSWORD`, so the Railway
steps and the Render blueprint set variables and keep the image's own start
command. `DB_SSLMODE` defaults to `require`; set it for a private-network
database without TLS.

**Volume ownership.** The backend image runs as an unprivileged user and
writes uploads to `STORAGE_DIR`. Docker named volumes (Coolify, Dokploy)
inherit the image's ownership. Railway volumes are mounted as root, hence
`RAILWAY_RUN_UID=0` in its steps; whether a Render disk is writable by the
image's user has not been checked. If uploads fail with a permission error,
use `STORAGE_DRIVER=s3` ([storage.md](storage.md)).

**Pin a version.** The templates use the `latest` image tag for a first
try. Pin a release tag before relying on an install: the backend runs
migrations on boot and refuses to start against a schema newer than its own.
