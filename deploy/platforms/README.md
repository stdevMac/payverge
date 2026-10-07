# Platform templates

Starting points for running Payverge on a managed platform instead of the
Caddy-fronted [Docker Compose stack](../README.md). The compose stack in
`deploy/` is the supported path; these templates are **not tested on the real
platforms yet**. Each one is validated offline only, as listed below.

| Platform | File | Validation done | Tested on the platform |
|---|---|---|---|
| Coolify v4 | [coolify/docker-compose.yml](coolify/docker-compose.yml) | `docker compose config -q` | no |
| Dokploy | same file, variables set by hand | `docker compose config -q` | no |
| Render | [render/render.yaml](render/render.yaml) | Render's published JSON schema (`render.com/schema/render.yaml.json`) | no |
| Railway | [railway/README.md](railway/README.md) (UI steps) | n/a | no |

All of them share one shape:

- **One public origin.** Only the frontend gets a domain. The Next server
  proxies `/api/v1/*` and `/media/*` to the backend on the platform's private
  network (`BACKEND_INTERNAL_URL`), streaming SSE, with a 15 MB body limit
  and a 300 s upstream timeout
  ([frontend-config.md](../../docs/self-hosting/frontend-config.md)).
- **No Caddy.** The platform's proxy terminates TLS.
- **Persistent Postgres and uploads.** A volume or disk for `STORAGE_DIR`,
  or `STORAGE_DRIVER=s3`.
- **Generated secrets** through the platform where it can
  (Coolify magic variables, Render `generateValue`).
- **One replica each.**

Read [docs/self-hosting/platforms.md](../../docs/self-hosting/platforms.md)
before deploying: it lists what you lose compared with the compose stack and
how client IPs are trusted.

## Coolify

1. New resource → Docker Compose (empty) → paste
   `deploy/platforms/coolify/docker-compose.yml`, or point a Git-based
   resource at that path.
2. Set `ADMIN_EMAIL` in Environment Variables. Coolify fills in
   `SERVICE_URL_FRONTEND_3000`, the database password, `JWT_SECRET_KEY`,
   `PLUGIN_SECRET_KEY` and the admin password, and keeps them across deploys.
3. **Required:** give the `frontend` service an https domain (or set an https
   wildcard domain on the server). A default install generates
   `http://<id>.<ip>.sslip.io`, and the backend refuses to start on a
   non-loopback http `PUBLIC_URL` (preflight `public_url.insecure`).
   If you set `EMAIL_PROVIDER`, also set `FROM_EMAIL` and `FROM_EMAIL_UPDATES`
   (and for `resend`: `RESEND_API_KEY`, `EMAIL_ALLOWED_FROM_DOMAINS`);
   preflight refuses to start without them.
4. Deploy. Sign in with `ADMIN_EMAIL` and the generated
   `SERVICE_PASSWORD_ADMIN` (shown in Environment Variables).

## Dokploy

Create a Compose service from the same file. Dokploy has no magic variables,
so set the `SERVICE_*` names yourself in its Environment tab (the file's
header lists them with `openssl` commands), set `ADMIN_EMAIL`, and attach
your domain to the `frontend` service on port 3000.

## Render

New → Blueprint → this repository, and set **Blueprint path** to
`deploy/platforms/render/render.yaml` (Render reads `render.yaml` at the repo
root by default; the path is configurable at setup).

Render asks for the `sync: false` values (`PUBLIC_URL` on both services,
`ADMIN_EMAIL`, `ADMIN_PASSWORD`, and the optional `EMAIL_PROVIDER` /
`OPENROUTER_API_KEY`). `PUBLIC_URL` is the frontend's URL; on a first deploy
that is `https://payverge-frontend.onrender.com` or the suffixed name Render
assigns, so you may have to edit it once the service exists.

The backend reads its database connection from the `DB_*` variables the
Blueprint fills in from `payverge-db`, so there is no manual step. Both
images are pinned to the release the files come from (release-please bumps
the tag); to upgrade, change both tags together and redeploy.

Disks need a paid plan; the blueprint uses `starter` services and a
`basic-256mb` database. Change the plans to suit you.

## Railway

See [railway/README.md](railway/README.md).
