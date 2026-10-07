# Zero-accounts acceptance

Proves a fresh self-host install runs a restaurant end to end with **no
third-party accounts**: no payment provider, no email provider (the `log`
provider), no S3 (local storage), no AI key, no analytics.

## Scripts

| File | Purpose |
|---|---|
| `zero-accounts.sh` | The acceptance flow. Compose mode (default) or `--target` mode. |
| `compose.acceptance.yml` | Overlay on `deploy/docker-compose.yml` + `deploy/docker-compose.build.yml`: tags images `payverge-*:oss-accept` and binds Caddy to `127.0.0.1` only. |
| `boot-native.sh` | Starts a natively built backend in `--production` mode with only generated secrets and a PostgreSQL connection. Used by the PR CI job. |

## Compose mode (weekly workflow, `.github/workflows/acceptance.yml`)

```bash
scripts/acceptance/zero-accounts.sh            # build, boot, test, tear down
scripts/acceptance/zero-accounts.sh --keep     # leave the stack running
scripts/acceptance/zero-accounts.sh --no-build # reuse payverge-*:oss-accept images
```

Needs Docker with Compose v2, `curl`, `jq`, `openssl`. Generates a throwaway
`.env` (`DOMAIN=localhost`, random admin/DB/JWT/plugin secrets,
`EMAIL_PROVIDER=log`, backup profile off) under a temp dir, runs compose with a
scrubbed environment (only `PATH`, `HOME` and the `DOCKER_*` client settings) so
variables exported in your shell cannot override that file, and uses a unique
project name (`ACCEPTANCE_PROJECT`, default `oss-acceptance-<pid>`). Caddy
listens on `127.0.0.1:${ACCEPTANCE_HTTP_PORT:-28080}` and
`127.0.0.1:${ACCEPTANCE_HTTPS_PORT:-28443}` (internal CA, so requests use
`curl -k`). Teardown (`down -v`) runs from a trap unless `--keep`; on failure
the backend log is printed first. Images are left tagged `:oss-accept` for
`--no-build` reruns.

## Target mode (PR CI job `boot-zero-accounts` in `ci.yml`)

```bash
export ACCEPTANCE_STATE=/tmp/payverge-acceptance DB_PASSWORD="$PGPASSWORD"  # empty PG15 db "payverge"
(cd backend && go build -o /tmp/payverge-backend ./cmd/app)
PAYVERGE_BIN=/tmp/payverge-backend scripts/acceptance/boot-native.sh start
set -a; source "$ACCEPTANCE_STATE/boot.env"; set +a
scripts/acceptance/zero-accounts.sh --target http://127.0.0.1:8080 \
  --backend-log "$ACCEPTANCE_STATE/backend.log" \
  --restart-cmd "scripts/acceptance/boot-native.sh restart"
scripts/acceptance/boot-native.sh stop
```

No images, no Docker-in-Docker. The frontend smoke is skipped in this mode.

## What each step proves

1. `GET /api/v1/health/ready` and `GET /api/v1/instance`: booted, no `billing_mode` field.
2. Admin login with the bootstrap `ADMIN_EMAIL`/`ADMIN_PASSWORD`.
3. Create a business: no plan step, no paywall.
4. Upload a menu photo and fetch the same bytes back via `/media/...` (local storage driver).
5. Menu category and item (with the photo), then a table.
6. Guest routes `/api/v1/guest/table/:code` and `/menu` show the item.
7. Staff SSE stream opened **before** a guest order; `order.created` must arrive.
8. The order is on the orders list.
9. Cash settlement: open a cash-register session, record a cash
   alternative payment, bill becomes `paid`.
10. Email: the `log` provider records deliveries after settlement with no
    failures. A guest receipt to an unbound address must be refused (403,
    `receipt_recipient_unbound`): the anti-relay rule, not a bug.
11. Frontend smoke (compose mode): `/` and `/es` serve the venue or the
    venue directory (200), or redirect to `/dashboard` while nothing is
    published; `/?invite_code=...` redirects to `/dashboard` keeping its
    query; `/pricing` is 404; `/dashboard`, `/staff/login` and
    `/t/<code>` return 200, embed `window.__PAYVERGE_ENV__` with this
    instance's `PUBLIC_URL`, and never mention `payverge.io`. There is no
    `/login` route in the frontend.
12. Upgrade: `compose up -d` again plus `compose restart` (or `--restart-cmd`)
    keeps the business, menu, order, bill and photo.
