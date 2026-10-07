# Troubleshooting

Start with these two commands in the install directory; most problems name
themselves there.

```sh
docker compose ps                       # which service is unhealthy or restarting
docker compose logs --tail 100 backend  # or caddy, frontend, postgres, backup
```

More cases, with their fixes, are in
[deploy/README.md](../../deploy/README.md#troubleshooting).

## Installation

**The installer stops with "the backend was not ready after 600s".** The first
start downloads about 1 GB of images and runs the database setup. On a slow
link, raise the limit with `PAYVERGE_READY_TIMEOUT=1200` and run the
installer again. Otherwise read `docker compose logs backend`.

**The certificate is not issued.** DNS does not point at the server yet, port
80 or 443 is closed in the cloud firewall or `ufw`, or a Cloudflare proxy was
switched on before the first certificate. See
[install.md](install.md#dns) and [install.md](install.md#ports-and-firewall).

**Port 80 or 443 is already in use.** Another web server owns them. Put
Payverge behind it ([reverse-proxies.md](reverse-proxies.md)), or for a trial
use `--http-port 8080 --https-port 8443`.

## The backend does not start

The backend checks its configuration at boot and exits with a message naming
each setting it rejects (for example `JWT_SECRET_KEY must be at least 32
characters`). Fix `.env` and run `docker compose up -d`. The settings are in
[configuration.md](configuration.md).

**"Database migration failed" or "Schema verification failed".** The image is
older than the database, usually after a rollback without a restore. See
[upgrades.md](upgrades.md#rolling-back).

**Password authentication failed for the database.** `DB_PASSWORD` in `.env`
was changed after the database was created. Put the old value back.

## Signing in

**Lost the admin password.** Reset it from the command line
([deploy/README.md](../../deploy/README.md#admin-access)). Changing
`ADMIN_PASSWORD` in `.env` does nothing once the account exists.

**Everyone is signed out after a restart.** `JWT_SECRET_KEY` changed.

**No invite, verification or password-reset emails arrive.** With no
`EMAIL_PROVIDER` the emails are only written to the backend log
(`docker compose logs backend | grep -i email`). Set up a provider:
[email.md](email.md).

## Every visitor has the same IP

Symptoms: "too many requests" errors for many people at once, or the audit log
shows one `172.x` address for everyone.

1. Check how the IP resolves ([reverse-proxies.md](reverse-proxies.md#checking-it)).
2. Behind Cloudflare, set `EDGE=cloudflare`. Behind another proxy, set
   `TRUSTED_PROXY_CIDRS` to its address.
3. If you replaced the compose defaults or run the backend outside the bundled
   stack, make sure `TRUSTED_PROXIES` (backend) and `FRONTEND_TRUSTED_PROXIES`
   (frontend) include the proxy's address; look for a one-time `[SECURITY]`
   line about `X-Forwarded-For` in the backend log
   ([reverse-proxies.md](reverse-proxies.md#trusted_proxies-on-the-backend)).
4. If Caddy itself sees Docker's gateway address, Docker is forwarding through
   its userland proxy (common with IPv6 and no `ip6tables`). Set
   `"ip6tables": true` in `/etc/docker/daemon.json` and restart Docker, or
   remove the `AAAA` record.

## Payments

**Stripe or PayPal is missing or inactive for restaurants.** In production
both stay off until `PAYMENT_PROVIDER_STRIPE_ENABLED=true` or
`PAYMENT_PROVIDER_PAYPAL_ENABLED=true` is set and the backend restarted. See
[payments.md](payments.md#production-activation).

**A provider says the webhook failed.** `503` means no webhook secret is
configured for that provider, `401` that the secret is wrong. See
[payments.md](payments.md#webhook-secrets).

**"Connect Stripe" or "Connect MercadoPago" does nothing or errors.** The
platform OAuth app is not configured (the backend logs `OAuth not configured`
at boot), or its redirect URI does not match
[payments.md](payments.md#urls-the-providers-call) exactly.

**Restaurants must reconnect every provider after a restore or a move.**
`PLUGIN_SECRET_KEY` differs from the one the backup was taken with. Put the
original `.env` back.

**USDC payments stay pending.** The backend RPC is rate-limited or down; use
a private `RPC_URL` ([payments.md](payments.md#usdc-on-base)).

## Uploads and images

**Images upload but do not show.** With `STORAGE_DRIVER=s3`, check
`S3_PUBLIC_BASE_URL` and that `MEDIA_ORIGINS` contains its origin. See
[storage.md](storage.md).

**Uploads fail behind your own web server.** It limits the body size; allow
15 MB (`client_max_body_size 15m` in nginx).

## A setting is on that you never set

Docker Compose fills `${VAR}` in `docker-compose.yml` from your shell
environment first and from `.env` second. A variable exported in the shell
that runs `docker compose` (for example `OPENROUTER_API_KEY` from your
profile) reaches the backend even when `.env` does not mention it, so AI can
show as on with no key in `.env`. Check with `docker compose config | grep
<NAME>`, then `unset` it in that shell or set it explicitly in `.env`.

## Live dashboards stop updating

The kitchen and table views use server-sent events. A proxy that buffers
responses or closes idle connections breaks them: disable buffering
(`proxy_buffering off` in nginx) and allow 300-second reads. Cloudflare passes
them through unchanged.

## Backups

**The backup job failed.** `docker compose logs backup`. A set with
`archives_failed=` in its `MANIFEST` misses that volume because files kept
changing during the copy; the next night usually succeeds. Disk full is the
other common cause: `df -h` and lower `BACKUP_KEEP_DAILY`.

**Restore refuses to run.** Something is still connected to the database.
Stop the backend and frontend first: `docker compose stop backend frontend`.

## Asking for help

When you open an issue, include the Payverge version (`PAYVERGE_VERSION`),
`docker compose version`, `docker compose ps`, and the relevant log lines.
Remove domains, emails and anything from `.env` first.
