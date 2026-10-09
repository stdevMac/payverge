# Running a public demo

This page sets up a public demo like `demo.payverge.io`. Anyone can open it,
enter a seeded restaurant with one click and change things. Every night at
03:00 UTC the database and the uploads go back to a pristine snapshot.

A demo is the normal self-host stack plus three things:

- **`DEMO_MODE=true`** (backend). It turns on the one-click sign-in, the
  banner and the deny list described below. It needs `DEMO_DATA=true`, and
  the backend refuses to start without it.
- **The overlay [`deploy/demo/docker-compose.demo.yml`](../../deploy/demo/docker-compose.demo.yml).**
  It sets the demo environment, turns outbound channels off, sets resource
  limits and adds `noindex` at the edge.
- **The reset**:
  [`deploy/demo/reset-demo.sh`](../../deploy/demo/reset-demo.sh), run by a
  systemd timer at 03:00 UTC.

Do not turn on `DEMO_MODE` on an instance with real restaurants. It hands a
session to anyone who asks.

## What visitors get

- **The demo venue at `/`.** The overlay sets `PRIMARY_VENUE` to the core
  showroom venue's slug (`bodegon-mesa-larga`, from its seeded name; the
  demo freezes `custom_url`, so it cannot drift). Visitors land on its
  storefront. Override it with `DEMO_PRIMARY_VENUE` in `.env`. If the value
  matches no published storefront, `/` falls back to the venue directory.
  The owner dashboard is at `/dashboard`.
- **A banner on every page** (storefront, dashboard, staff and admin):
  "Public demo — anyone can see changes, everything resets nightly at 03:00
  UTC". Its **Enter as owner or staff** link opens `/dashboard?auth=signin`,
  and it links to GitHub and to the install guide. Its "Try as a guest"
  panel lists the demo venues' storefronts and table links, with a QR code
  for each table, so a visitor can order from a phone with no login.
- **Sign-in buttons** in the sign-in dialog, which `/dashboard` opens on its
  own in demo mode: **Enter demo as Owner**,
  **Enter demo as Staff (kitchen)** and **Enter demo as Staff (waiter)**.
  They call `POST /api/v1/auth/demo/login`, which answers 404 unless
  `DEMO_MODE` is on. No password is published, and none exists.
  - The owner is `owner@demo.payverge.invalid`. It is a non-admin account
    that owns the demo venues and cannot receive mail.
  - The staff buttons sign in seeded staff of the core venue.
  - A demo owner session is valid only while `DEMO_MODE` is on.
- **`/admin` is unreachable** for demo sessions. The platform admin
  (`ADMIN_EMAIL`) is a separate account, and only you know its password.
- **`/api/v1/instance`** reports `demo: {enabled, mode, reset_utc}`, and
  `registration_mode: "closed"`.

## What the demo refuses

Everything a restaurant does day to day stays writable: menu, tables,
orders, bills, the kitchen display, reservations, CRM and accounting. The
nightly reset puts it back.

The backend refuses the actions below with `403` and
`{"code": "DEMO_MODE_FORBIDDEN", "params": {"kind": ...}}`, and the message
names the action. The list is
[`backend/internal/demomode/guard.go`](../../backend/internal/demomode/guard.go).
Every write under `/api/v1/auth/` and `/api/v1/admin/` is refused by
default. The only exceptions are the demo login, login, logout and token
refresh. Wallet sign-in (SIWE) is refused because it creates
users. The nightly reset uses the CLI, not the admin API.

Two tests in `backend/cmd/app/demo_guard_routes_test.go` keep the list
honest:

- Every entry matches a wired route.
- Every write route in a sensitive group is either refused or listed in a
  reviewed allowlist with a reason. The groups are account and session,
  staff, integrations, wallets and payouts, outbound URLs, uploads, admin,
  fiscal and subscription. A new route in one of these groups that is in
  neither place fails the test.

| Kind | Refused |
|---|---|
| Account | Any `/auth/` write except demo login, login, logout and refresh (wallet sign-in creates users), changing passwords or email addresses, the demo profile, deleting the account, exporting data, email preferences, wallet linking, creating accounts or venues |
| Staff access | Inviting users, managing and accepting invitations, staff roles, permissions, PINs and access, staff email sign-in |
| Venue | Deleting the venue |
| Storefront | Changing the venue name, logo, address, phone, website, social links, `custom_url`, banner and gallery images, the QR logo (venue default, per table and "apply to all") and the settlement and tipping wallets. `UpdateBusiness` is an allowlist: only rates, toggles, currency and time zone, AI settings, QR colors, and the description, welcome, about and AI-instruction texts may change. A field added later is refused until someone reviews it. Free text is capped at 500 characters, on every route that writes it: business settings, hospitality, the "Why choose us" features and menu translation overrides |
| Admin | Every `/admin/` write |
| CRM | Guest customer-account signup |
| AI images | AI image generation and enhancement |
| Integrations | Every `/plugins/` write: payment provider credentials, Stripe and MercadoPago OAuth, Telegram. Also WhatsApp and Google Business connections |
| Payments | Online card checkout, card terminals, QR charges, crypto payments and on-chain refunds. Paying at the counter works: the guest asks to pay, and the owner confirms it as cash or card on the bill's payments page. The waiter and kitchen roles cannot list pending counter requests. No money moves. Every showroom venue boots with its cash drawer open, so guests can pay before anyone has opened Caja |
| Outbound | Emailed receipts, the manual onboarding form, push, printers and test prints, fiscal (ARCA) e-invoicing, inbound provider webhooks |
| Uploads | Every upload and attachment. The message says that uploads are off in the public demo |
| Size | Any write body over 256 KB, which bounds JSON blobs such as menus, settings and layouts |

Two other limits apply:

- **Writes are rate-limited.** Each client IP (each IPv6 /64) gets
  `DEMO_WRITE_RATE_PER_MIN` state-changing requests per minute (default 30).
  Past that, the backend answers `429` with `Retry-After: 60`. The limiter
  tracks at most 50,000 clients at once; a new client beyond that is refused
  with the same `429` until older budgets refill.
- **Some settings are forced** whatever `.env` says:
  - Email always goes to the log provider, so nothing is delivered.
  - Signup is `closed`.
- **Demo sessions are shared and scoped.** Every visitor who picks the
  same role signs in as the same showroom owner or staff member. A replayed
  refresh token ends only the session it belongs to, never the other
  visitors' sessions. Switching `DEMO_MODE` off ends every one-click owner
  and staff session at its next request.

The demo banner shows a fixed venue label, never the editable venue name.
Guest forms that collect personal data (reservations, delivery checkout,
and customer accounts) show a notice: the demo is public,
so do not enter real personal data.

### Not covered (known gaps)

- **Menu content.** Menu item names and descriptions, and menu item image
  URLs, are free text. Only the 256 KB body cap bounds them, and they show
  on the public storefront until the next reset. Watch it, and run the
  reset by hand if you need to (or more often than nightly, with the
  timer's `OnCalendar=`).
- **Table names** are free text, up to 64 characters.
- **Space-scan** connect, status and apply-review, and the AI chat routes
  stay open. They do nothing harmful without uploads or an AI key.

## Sizing

| | Minimum | Comfortable |
|---|---|---|
| vCPU | 2 | 4 |
| RAM | 4 GB | 8 GB |
| Disk | 30 GB | 60 GB |

The overlay caps the backend at 1 GB, the frontend at 768 MB and Postgres
at 768 MB, with one CPU each. The cap variables are `DEMO_BACKEND_MEM`,
`DEMO_BACKEND_CPUS` and the same pair for the frontend and Postgres. Each
reset keeps the previous database for one night, so plan disk for about
three copies of the seeded database (a few hundred MB).

Use a host of its own. Do not use the host of a real instance: the
demo is meant to be poked at.

## DNS and firewall

- Create an `A` record (and `AAAA`, if the host has IPv6) for
  `demo.payverge.io` pointing at the host.
- Open TCP 80 and 443 and UDP 443. Caddy obtains the certificate itself.
- Behind Cloudflare, set `EDGE=cloudflare` as in
  [reverse-proxies.md](reverse-proxies.md).

## Install

Run this from a clone of the repository. The downloaded installer does not
ship `deploy/demo/`.

```bash
git clone https://github.com/stdevMac/payverge.git /opt/payverge-src
cd /opt/payverge-src/deploy

COMPOSE_PROJECT_NAME=payverge-demo \
  ./install.sh --domain demo.payverge.io --admin-email you@example.com --demo --no-start

# Layer the demo overlay on every docker compose command in this directory.
# Edit the key rather than appending, and keep docker-compose.build.yml when
# install.sh --build already put it there.
cur=$(sed -n 's/^COMPOSE_FILE=//p' .env)
sed -i.bak '/^COMPOSE_FILE=/d' .env && rm -f .env.bak
printf 'COMPOSE_FILE=%s:demo/docker-compose.demo.yml\n' "${cur:-docker-compose.yml}" >> .env

docker compose up -d --wait
```

`--demo` sets `DEMO_DATA=true`. The overlay sets `DEMO_MODE=true` as well.

The first boot seeds the showroom. Check it, then take the pristine snapshot
before anyone else can reach the site:

```bash
curl -s https://demo.payverge.io/api/v1/instance | grep -o '"demo":{[^}]*}'
# "demo":{"enabled":true,"mode":true,"reset_utc":"03:00"}

demo/reset-demo.sh --snapshot   # writes demo/pristine/ (gitignored, mode 600)
demo/reset-demo.sh --check      # preflight: demo project, snapshot checksums, postgres
```

`reset-demo.sh` refuses to touch any project whose effective config is not
the demo: the backend service itself must have `DEMO_MODE=true`. Running it
by accident on a real instance stops with exit code 78 and changes nothing.

Once a snapshot exists, `--snapshot` refuses to dump the live demo (exit
78). A snapshot taken while visitors can reach the site would bake their
edits (a phishing venue name, a link) into every future reset. Every later
snapshot goes through `--maintenance`; see
[Rotate the admin password](#rotate-the-admin-password).

## Enable the nightly reset

The units assume the deploy directory is `/opt/payverge`. If yours is
somewhere else, edit `WorkingDirectory`, `EnvironmentFile`,
`DEMO_COMPOSE_DIR` and `ExecStart` first.

```bash
sudo cp demo/systemd/payverge-demo-reset*.service demo/systemd/payverge-demo-reset*.timer /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now payverge-demo-reset.timer payverge-demo-reset-check.timer
systemctl list-timers 'payverge-demo-reset*'          # reset at 03:00 UTC, watchdog hourly
sudo systemctl start payverge-demo-reset.service      # one reset now, to prove it
journalctl -u payverge-demo-reset.service -n 50
```

The watchdog (`payverge-demo-reset-check`) runs `reset-demo.sh --check`
every hour. It fails, and posts to `DEMO_ALERT_URL`, when the last good
reset is older than `DEMO_RESET_MAX_AGE_HOURS` (default 26). That catches a
stopped timer, a run killed halfway, or a reset that keeps failing, so
visitor content never silently outlives the night.

Optional settings go in `/opt/payverge/demo/reset.env`:

```bash
DEMO_HEALTH_URL=https://demo.payverge.io/api/v1/health/ready   # also check through the edge
DEMO_ALERT_URL=https://ntfy.sh/<private-topic>                 # POSTed one line on failure
DEMO_HEALTH_TIMEOUT=300
DEMO_RESET_MAX_AGE_HOURS=26                                    # watchdog threshold
```

If you do not use systemd, use cron instead:
[`demo/cron.example`](../../deploy/demo/cron.example).

### What a reset does

1. **Lock and verify.** It takes a lock (`demo/pristine/.lock`, holding
   the run's PID and start time) and checks the snapshot against its
   `SHA256SUMS`. A lock left by a run that no longer exists (killed, or a
   reboot) is broken automatically and logged. If a live run holds it, the
   new run exits 75, which counts as a failure, and it raises an `ALERT`
   when the last good reset is already too old.
2. **Restore into a scratch database.** It restores the snapshot into
   `<db>_reset_new` while the demo keeps serving. A bad dump fails here,
   and nothing changes.
3. **Swap.** It stops the backend and the frontend, then swaps the
   databases: `<db>` becomes `<db>_previous` and `<db>_reset_new` becomes
   `<db>`. It moves the uploads and the space-scan artifacts
   (`/app/data/space-scans` in the backend data volume) aside and unpacks
   the snapshot's copies.
4. **Start and health-check.** It starts the stack and waits until the
   backend and frontend container health checks pass. It also checks
   `/api/v1/health/ready` from inside the network, plus `DEMO_HEALTH_URL`
   when you set one.
5. **Finish or roll back.**
   - On success, the moved-aside files are deleted, `<db>_previous` stays
     until the next night for forensics, and `demo/pristine/last-ok` is
     stamped for the watchdog.
   - On any failure after step 3, the swap is reversed and the uploads and
     space-scan artifacts are put back. The stack starts on the previous state, an `ALERT` line goes
     to the journal (and to `DEMO_ALERT_URL`), and the script exits with
     code 1. If the rollback is not healthy either, the exit code is 2.

Downtime is the stop/start window, usually under a minute. All sessions end
at the reset, because they live in the database.

In scope: the database, the uploads volume and the space-scan artifacts.
Out of scope: the generated plugin key in the backend data volume (it must
survive, or stored plugin secrets become unreadable) and object storage. The
overlay forces `STORAGE_DRIVER=local`, so the demo writes nothing to MinIO
or S3. If you change that, those writes outlive every reset.

### Uptime

Point an external monitor (Uptime Kuma, Better Stack, a cron `curl`) at
`https://demo.payverge.io/api/v1/health/ready`.

- Expect a short gap at 03:00 UTC. Silence alerts for 03:00–03:10 UTC, or
  raise the failure threshold.
- Alert separately on a failed `payverge-demo-reset.service` or
  `payverge-demo-reset-check.service`. Either set `OnFailure=` in the units
  or use `DEMO_ALERT_URL`.

## Rotate the admin password

In demo mode the app refuses password changes for every account, the
platform admin included. Use the CLI instead. The snapshot holds the admin
account as it was when you took it, so re-take it straight after, or the
next reset brings the old password back.

Re-take it from a clean state with the site offline. `--maintenance` resets
to the current snapshot and leaves the edge (Caddy) stopped, so no visitor
can change anything in between. `--snapshot` then takes the new snapshot and
brings the edge back:

```bash
demo/reset-demo.sh --maintenance
openssl rand -base64 24 | tee /dev/stderr | \
  docker compose exec -T backend /app/server admin reset-password --email you@example.com --password-stdin
demo/reset-demo.sh --snapshot
```

`--snapshot` refuses if the edge was started in between (for example by a
`docker compose up -d`): run `--maintenance` again. Only if you have checked
the live content by hand, `DEMO_SNAPSHOT_FROM_LIVE=1 demo/reset-demo.sh
--snapshot` skips this check.

Store the printed password in your password manager. [admin.md](admin.md)
has the other `admin` commands.

Re-take the snapshot after each upgrade too, the same way: upgrade, then
`--maintenance`, then `--snapshot`. A reset onto an older snapshot
works, because the backend migrates it forward on boot, but it repeats that
migration every night.

A demo installed from before the first public release runs PostgreSQL 15.
Upgrade it as in [upgrades.md](upgrades.md#postgresql-18), and before
re-taking the snapshot on PostgreSQL 18, copy `demo/pristine/` aside (for
example `cp -a demo/pristine demo/pristine.pg15`). PostgreSQL 15's
`pg_restore` cannot read a dump taken by 18, so a rollback to 15 needs the
old snapshot back in `demo/pristine/`.

## AI: off, or on with a budget

**Off (the default).** The overlay empties `LLM_API_KEY` and
`OPENROUTER_API_KEY`, so a key in the main `.env` never reaches the demo.
Every AI surface shows its built-in "AI is not configured" state. It is
honest about the missing model and does not fake answers.

**On, with a budget.** Image generation stays off either way. Put a dedicated key in `.env`:

```bash
DEMO_AI_API_KEY=sk-or-...          # a key used ONLY by the demo, with a provider-side spend limit
# DEMO_AI_BASE_URL=https://...     # only for a non-OpenRouter, OpenAI-compatible endpoint
DEMO_AI_DAILY_BUDGET_USD=1         # per-venue and instance-wide daily ceiling (default 1)
DEMO_AI_GUEST_BUDGET_USD=0.5       # guest AI waiter pool (default 0.5)
DEMO_AI_MESSAGES_PER_IP=20         # AI waiter messages per IP per day (default 20)
```

Then run `docker compose up -d backend`.

- **The key is a secret.** It belongs to whoever owns the demo. Set a hard
  monthly cap at the provider as well, because the in-app ceilings reset
  daily.
- **Never commit it.** It goes only in `.env`, which is mode 600 and
  untracked.

## Teardown

```bash
sudo systemctl disable --now payverge-demo-reset.timer payverge-demo-reset-check.timer
sudo rm /etc/systemd/system/payverge-demo-reset{,-check}.{service,timer}
sudo systemctl daemon-reload
docker compose down --volumes      # in the deploy directory: this project only
rm -rf demo/pristine
```

Then remove the DNS record.

## Tested

The flow on this page was run end to end against a throwaway compose project
on `127.0.0.1`:

- install with the overlay
- `--snapshot`
- owner and guest changes
- a guest at a table link: order, split equally, pay one share at the
  counter, the owner confirms both shares, the bill closes as paid
- the fail-safes: a non-demo project, a corrupt snapshot and a tampered
  snapshot each leave the live demo untouched
- `reset-demo.sh`
- the change is gone, the stack is healthy and the cash drawers are open
  again
- a database change and a space-scan file left by a "visitor" are gone
  after a reset
- `--snapshot` on the live demo is refused (exit 78), also after
  `--maintenance` when the edge was started again; `--maintenance` then
  `--snapshot` works, the new snapshot carries the space-scan artifacts and
  the edge comes back
- a forced health failure rolls back: the previous database (a marker
  table) and the previous space-scan files are back
- the lock: a dead holder's lock and a reused PID's lock are broken; a live
  holder makes a second run exit 75, with an `ALERT` once the last good
  reset is older than 26 hours
- `--check`: passes while a run is in progress, fails on a stale
  `last-ok`, and falls back to the snapshot's age when there is no stamp

The systemd units, DNS, ACME and the Cloudflare edge were not run. They
need a server and a domain.
