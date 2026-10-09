# Self-hosting: the platform admin, signups and demo data

This page covers the operator side of a fresh install:

- creating the first platform admin;
- resetting a password from the server, with no email provider needed;
- deciding who may sign up (`REGISTRATION_MODE`);
- optional demo restaurants (`DEMO_DATA`).

None of it needs a third-party account.

Every command below runs **inside the backend container**. The image is
distroless (it has no shell), so the maintenance commands live in the server
binary itself: `/app/server <group> <command>`.

## 1. First admin

Set both variables in the install directory's `.env` before the first `docker compose up`. That file is the one `deploy/install.sh` writes (`deploy/.env` when running from a checkout), not the repository root `.env`:

```bash
ADMIN_EMAIL=you@example.com
ADMIN_PASSWORD='<a-long-unique-passphrase>'
```

On boot the backend creates that account inside one transaction. It holds a
Postgres advisory lock, so replicas that start together cannot race. The
account gets:

- the platform `admin` role, and
- an email/password login that is already verified.

You can sign in immediately at your `PUBLIC_URL`. Creating your first restaurant
from that account never needs an invite.

The boot is idempotent:

| State at boot | What happens |
|---|---|
| Neither variable set | Nothing. The log warns once if no platform admin exists yet. |
| Only `ADMIN_EMAIL` set, and that address is already a platform admin | Nothing to do. This is the normal state after `install.sh` removes the generated password; the log says `already exists; ADMIN_PASSWORD is not needed`. |
| Only one of the two set otherwise | Skipped, with a warning naming the missing variable. |
| Account does not exist | Created as admin with the given password. |
| Account exists and is already a platform admin | Left alone. **The password is never overwritten**, so a password you changed later survives restarts. The log says `ADMIN_PASSWORD ignored`. |
| Account exists but is not an admin | **Taken over** (see below), then promoted. The log says `took it over`. |
| Account exists, is not an admin, and has a wallet linked | Refused. Nothing is changed and the log says so. Pick an email no account uses yet. |
| A deleted account (or a leftover login with no account) still holds this email | Its old email/password login is removed, then the row above that matches applies. The log says `removed … login(s) … left by a deleted account`. The deleted account itself stays in place for deletion review. |
| A different, active account holds an email/password login for this email | Refused. Nothing is changed, because removing that login would lock its owner out. Pick another email. |
| Password fails the policy | Not created. The log names the rule that failed. The server still starts. |

**Why an existing account is taken over, not just promoted.** Anyone can
register an email they do not own (for example while `REGISTRATION_MODE=open`,
or before you set `ADMIN_*`). Whoever did so chose that account's password and
may have linked their own Google login to it. Even a verified address proves
little, because you may have clicked the verification mail yourself. Promoting
such an account as-is would hand them the platform admin. So when `ADMIN_EMAIL`
(or `admin create`) names an existing account that is not yet an admin, the
backend does all of this in one transaction:

1. sets its email/password login to `ADMIN_PASSWORD` and marks it verified;
2. clears any pending password-reset or verification link;
3. unlinks every other sign-in method, such as Google. A Google account with
   the same verified address re-links itself the next time you use it;
4. **signs out every existing session** and clears the failed-login lockout;
5. promotes it to `admin`.

Restaurants and other data on the account stay where they are. Accounts with a
wallet linked are refused instead of taken over: wallet sign-in and restaurant
ownership can both hang off that wallet, so the backend will not clear it, and
it will not leave it attached to an admin. Once an account is an admin, later
boots leave it alone (first row of the table).

**Password policy** (the same for `ADMIN_PASSWORD`, `admin create` and
`admin reset-password`):

- at least 12 characters, at most 72 bytes (the bcrypt limit);
- not a known default such as `password123`, `adminadmin` or `payverge123`.
  The comparison ignores case and separators;
- not containing a template placeholder such as `changeme`, `replaceme` or
  `yourpassword`;
- at least 5 distinct characters.

After the first successful boot, **remove `ADMIN_PASSWORD` from `.env`**. The
account and its password hash live in the database. While the account stays an
admin, nothing reads the variable again except to log that it was ignored. If
the variable is still set and the account is ever demoted, the next boot takes
it over again. Keep `ADMIN_EMAIL` if you want `DEMO_DATA` and `demo seed` to
default to that owner.

### Admin without environment variables

If you would rather not put a password in `.env` at all, start the stack without
`ADMIN_*` and create the admin afterwards. The password goes on stdin, so it
never appears in `ps`, in the shell history of the container, or in compose
files:

```bash
printf '%s' "$ADMIN_PW" | docker compose exec -T backend \
  /app/server admin create --email you@example.com --password-stdin
```

`admin create` follows the same rules as the boot. If the email belongs to an
existing admin, nothing changes; use `admin reset-password` to set a new
password. If it belongs to a non-admin account, that account is taken over as
described above. If a wallet is linked to that account, the command changes
nothing and exits with code 1.

## 2. Resetting a password (locked out, forgot it, staff left)

```bash
printf '%s' "$NEW_PASSWORD" | docker compose exec -T backend \
  /app/server admin reset-password --email you@example.com --password-stdin
```

This works for any active account, not only admins. It:

1. sets the email/password login, creating it if the account only used Google
   or a wallet, and marks the email verified;
2. **signs out every existing session** for the account;
3. clears the failed-login lockout for that email;
4. removes any email/password login for the same email that a deleted account
   left behind. Sign-in picks the oldest login for an email, so such a leftover
   would otherwise block the new password. If a different, active account holds
   that login, the command changes nothing and exits with code 1.

Always use `-T` with `docker compose exec` when you pipe stdin. Without it,
compose allocates a TTY and the piped password is not delivered. The command
reads one line from stdin. If you type the password instead of piping it, your
terminal echoes it, so prefer `read -rs NEW_PASSWORD` and then the `printf`
pipe above.

The command refuses to run (exit code 2) when:

- no password arrives on stdin;
- the password fails the policy; or
- a password is passed as an argument. This is so it does not end up in
  `/proc` or in shell history.

## 3. Command reference

```text
/app/server admin create          --email EMAIL --password-stdin
/app/server admin reset-password  --email EMAIL --password-stdin
/app/server invite create         [--uses N] [--days D] [--name TEXT]
/app/server demo seed             [--owner-email EMAIL]
/app/server demo reset            (--owner-email EMAIL | --all)
/app/server settings image-limits [--daily N] [--monthly-alert N]
/app/server <group> --help
```

How the commands behave:

- They connect to the same database as the running server. They **never start
  HTTP, schedulers or workers, and never run migrations**. They refuse to touch
  a database whose schema is not at this binary's migration head, so start the
  server once (it migrates on boot) before using them on a new install or after
  an upgrade.
- They find the database the same way the server does. The order is:
  `--db-host/--db-port/--db-user/--db-name/--db-sslmode`, then the `DB_*`
  environment variables, then the flags of the running server process (PID 1 in
  the container). Inside `docker compose exec` you normally pass nothing. The
  database password is read from `DB_PASSWORD` only, never from a flag.
- Exit codes: `0` on success, `1` when the operation failed, `2` on a usage
  error. Usage errors never echo argument values back.

## 4. Who may sign up: `REGISTRATION_MODE`

| Value | Signup |
|---|---|
| `invite` (default) | A new operator account needs a valid invite code (see [Minting invites](#minting-invites)). |
| `open` | Anyone can create an account. No invite is consumed. |
| `closed` | No new accounts at all, by email/password, Google or wallet. Existing users still sign in. |

The rules apply to every signup path on the backend: email/password, Google and
wallet (SIWE).

The signup form reads the mode from `GET /api/v1/platform/registration-mode`:

- `open` shows the normal form;
- `invite` requires the invite code field;
- `closed` replaces the form with a "sign in instead" notice.

An unknown or misspelled value never opens signups by accident:

- in production mode (`--production`, `ENV=production` or `APP_ENV=production`)
  the production preflight fails with `registration_mode.invalid` and the
  backend **refuses to start**;
- outside production it is treated as **closed**, and the boot log shows an
  error.

The bootstrap admin (`ADMIN_EMAIL` or `admin create`) is never subject to the
registration mode.

### Minting invites

Mint a code from the server:

```bash
docker compose exec -T backend /app/server invite create
docker compose exec -T backend /app/server invite create --uses 5 --days 14 --name "Opening week"
```

| Flag | Default | Meaning |
|---|---|---|
| `--uses` | `1` | How many accounts may sign up with this code (1 to 10000). |
| `--days` | `30` | Days until the code expires (1 to 365). |
| `--name` | `Self-hosted invite` | Label shown in the batch history. |
| `--owner` | `ADMIN_EMAIL`, else `operator` | Who is accountable for the batch. |
| `--reason` | a fixed note | Why the batch exists; kept in the audit trail. |

The command prints the code. **It is shown exactly once.** Only its hash is
stored. When the backend has `PUBLIC_URL` set, the command also prints a
signup link: your site's `/dashboard?invite_code=CODE` (the operator sign-in), which makes the signup
form fill the code in. Send the code or the link to the people you invite. If
`REGISTRATION_MODE` is not `invite`, the command still mints the code but warns
that signup does not ask for one.

The batch and its audit event are the same as the ones the admin API creates,
with `created_by` set to `server-cli`. The admin UI has no invite screen yet.
A signed-in platform admin can also mint codes with
`POST /api/v1/admin/runtime-controls/invite-batches`, using the JSON fields
`name`, `cohort_cap`, `owner`, `reason` and `expires_at` (RFC 3339). That
endpoint rejects requests from an origin the backend does not trust.

The batch history and audit trail are listed at
`GET /api/v1/admin/runtime-controls` and
`GET /api/v1/admin/runtime-controls/audit`.

## 5. Demo restaurants: `DEMO_DATA`

| Variable | Default | Effect |
|---|---|---|
| `DEMO_DATA` | `false` | When `true`, the first boot seeds the demo restaurants (menus, tables, sample orders) owned by the `ADMIN_EMAIL` admin, or by the oldest platform admin if `ADMIN_EMAIL` is unset. Later boots keep it idempotent and do not duplicate anything. The seeded businesses are flagged `is_demo`. |
| `ADMIN_DEMO_AUTOMATION_ENABLED` | unset (off) | Hosted-demo behaviour: keep a demo for **every** admin and run the periodic demo-activity job. Leave it unset on a self-hosted install. `true` turns it on, and `false` also stops the activity job that `DEMO_DATA=true` would otherwise run. |

On a default install neither variable is set. No demo jobs run, and nothing
needs object storage.

You can do the same thing on demand:

```bash
# Seed (idempotent) for ADMIN_EMAIL, or name the owner explicitly
docker compose exec -T backend /app/server demo seed
docker compose exec -T backend /app/server demo seed --owner-email you@example.com

# Wipe and re-seed the demo restaurants: one owner, or every demo instance
docker compose exec -T backend /app/server demo reset --owner-email you@example.com
docker compose exec -T backend /app/server demo reset --all
```

Some limits apply to the demo commands:

- The demo owner must be a platform admin.
- `demo reset` only removes rows that the demo seeder created, so your real
  restaurants are never touched.
- The seeder currently creates two demo restaurants per owner.
- Demo menu photos ship inside the server and are served from your public
  storage. The server uploads any missing photo when it starts with
  `DEMO_DATA=true`; `demo seed` and `demo reset` do not upload them. If you
  seed from the command line on an instance that never booted with demo data,
  the restaurants still work, but some images may not load.

## Taking cash: open the cash register first

With no payment provider configured, guests settle in cash or at the counter.
Recording a cash payment on a bill needs an open cash-register session for the
venue; without one the API answers `409 cash_session_required` ("Open a cash
register session before recording cash"). Card and other manual methods are
not gated.

From the dashboard: open the cash-register tab and start a session (enter the
opening float), then record the cash payment on the bill.

From the API (owner or a staff role with `cash_register:operate`):

```bash
# 1. Open a session for business 1 (amounts are dollars, not cents)
curl -X POST "$PUBLIC_URL/api/v1/inside/businesses/1/cash-register/sessions" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"opening_float": 100, "opening_note": "start of shift"}'

# 2. Check it is open
curl "$PUBLIC_URL/api/v1/inside/businesses/1/cash-register/current" \
  -H "Authorization: Bearer $TOKEN"
```

Close the session at the end of the shift with
`POST /api/v1/inside/businesses/{id}/cash-register/sessions/{sessionId}/close`.

## Wallet sign-in on one backend replica

Wallet (Sign-In with Ethereum) challenges are stateless HMAC nonces whose key
is random per process, and spent nonces are remembered in memory until they
expire (5 minutes). Two consequences:

- With several backend replicas, route each client to one replica (sticky
  sessions), or wallet sign-in fails; a restart invalidates challenges that
  were issued but not yet signed.
- The spent-nonce set is capped at 500,000 entries. When it is full of live
  entries, the backend forgets the entries closest to expiry and from then on
  treats every challenge expiring at or before the latest forgotten one as
  expired. A spent nonce therefore still cannot be reused, and new sign-ins
  keep working; only a challenge that was issued but not yet signed and is
  about as old as the forgotten ones has to be requested again. Every entry
  costs a valid signature, so reaching the cap needs well over 1,000
  successful wallet sign-ins per second.
