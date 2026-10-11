---
name: setup-and-deploy
description: Install a self-hosted Payverge server with Docker Compose and Caddy, create the first platform admin, optionally load demo data, and verify the instance is healthy. Use when someone wants to deploy, install or stand up Payverge on a VPS, a home server or a restaurant LAN.
---

# Set up and deploy Payverge

The goal is a running instance at the operator's URL, with one platform admin
who can sign in and a green `/api/v1/health/ready`.

## 0. Ask before doing anything

Ask the operator for these. Do not guess them.

1. **Where it runs**: a Linux host they control, reached over SSH, with
   Docker Engine and the Compose plugin. Check with `docker compose version`.
2. **How guests reach it.** Either a domain whose A/AAAA record points at
   the host, with ports 80 and 443 open (Caddy gets the certificate
   automatically), or a LAN-only install on a fixed address such as
   `http://192.168.1.20:3000`. LAN setup is covered in
   `docs/self-hosting/frontend-config.md` under "LAN installs".
   Also ask whether Cloudflare proxies the domain (the orange cloud). If it
   does, set `EDGE=cloudflare` in `.env`: the `deploy/` compose file then
   runs Caddy with `deploy/Caddyfile.cloudflare`, and "Cloudflare" in
   `deploy/README.md` gives the TLS mode and certificate order. Otherwise
   leave `EDGE` unset (the default is `none`). Leave `TRUSTED_PLATFORM`
   blank either way: in production a non-empty value stops startup
   (`proxy.platform.unexpected` under `EDGE=none`,
   `cloudflare.platform.unexpected` under `EDGE=cloudflare`).
3. **The admin email.**
4. **Demo restaurants**: yes or no.
5. **Optional integrations.** All of them can wait, and none is needed for
   a first boot: an LLM provider (`docs/self-hosting/ai.md`), an email
   provider (`docs/self-hosting/email.md`), S3 storage
   (`docs/self-hosting/storage.md`).

Never ask the operator to paste a password, key or token into the chat.
Secrets are generated on the server, or typed by the operator into a prompt
or editor that you do not read back.

## 1. Pick the install path

Look at the release the operator is installing:

```bash
ls deploy/ 2>/dev/null   # deploy/docker-compose.yml, Caddyfile, .env.example, install.sh
```

- `deploy/` exists: use path A (installer) or path B (by hand).
- `deploy/` does not exist (an older checkout): use path C.

## Path A: the installer

```bash
curl -fsSL https://github.com/stdevMac/payverge/releases/latest/download/install.sh -o install.sh
less install.sh          # read it before running it; it is also in the repo as deploy/install.sh
bash install.sh
```

The installer checks Docker, asks for the domain, the admin email and
whether to load demo data, generates the secrets with `openssl rand`, writes
`.env` with mode 600, starts the stack and waits for
`/api/v1/health/ready`. It prints the URL and the admin credentials **once**.

Run it in an interactive terminal on the host, because it prompts. Tell the
operator to store the printed password in their password manager. Do not
copy it into the conversation or into a file.

## Path B: by hand with `deploy/`

```bash
git clone https://github.com/stdevMac/payverge.git
cd payverge/deploy
cp .env.example .env && chmod 600 .env
```

1. Fill the required block at the top of `deploy/.env.example`: the domain,
   `ADMIN_EMAIL`, `ADMIN_PASSWORD` and the three generated secrets. Each
   secret has its own format, and production preflight rejects the wrong
   one, so use the command listed next to it in `deploy/.env.example`:

   | Variable | Command | Rule the backend checks |
   |---|---|---|
   | `DB_PASSWORD` | `openssl rand -hex 24` | Applied when the database volume is first created; changing it later locks the backend out |
   | `JWT_SECRET_KEY` | `openssl rand -base64 48` | At least 32 characters (`jwt.secret.short`) |
   | `PLUGIN_SECRET_KEY` | `openssl rand -hex 32` | 64 hex characters, base64 of 32 bytes, or exactly 32 bytes (`plugin_secret.invalid` otherwise) |

   Write them straight into `.env` on the host, so no secret is printed,
   copied into the conversation, put on a command line (where `ps` and shell
   history can see it) or left in a world-readable backup file. The script
   below generates the same formats as the commands above, runs under
   `umask 077` and replaces `.env` atomically:

   ```bash
   (umask 077 && python3 - <<'PY'
   import base64, os, secrets
   values = {
       "DB_PASSWORD": secrets.token_hex(24),
       "JWT_SECRET_KEY": base64.b64encode(secrets.token_bytes(48)).decode(),
       "PLUGIN_SECRET_KEY": secrets.token_hex(32),
   }
   with open(".env") as f:
       lines = f.readlines()
   seen = set()
   for i, line in enumerate(lines):
       key = line.split("=", 1)[0]
       if "=" in line and key in values:
           lines[i] = f"{key}={values[key]}\n"
           seen.add(key)
   missing = sorted(set(values) - seen)
   if missing:
       raise SystemExit("not found in .env: " + ", ".join(missing))
   with open(".env.tmp", "w") as f:
       f.writelines(lines)
   os.chmod(".env.tmp", 0o600)
   os.replace(".env.tmp", ".env")
   print("generated " + ", ".join(sorted(values)))
   PY
   )
   ```

   Run it once, before the first boot: it overwrites existing values, and a
   new `DB_PASSWORD` or `PLUGIN_SECRET_KEY` on a running install locks the
   backend out of its database or makes stored provider credentials
   unreadable. Every other variable is optional: each one is a commented
   example in `deploy/.env.example`, and "Configuration" in
   `deploy/README.md` links the topic guides.
2. The password policy is 12 or more characters and at most 72 bytes
   (`docs/self-hosting/admin.md`).
3. Start the stack and watch it come up:

   ```bash
   docker compose up -d
   docker compose ps
   docker compose logs -f backend    # Ctrl-C once the server is listening
   ```

4. Demo data: set `DEMO_DATA=true` before the first boot, or seed later with
   `docker compose exec -T backend /app/server demo seed`.

The defaults need no third-party account: files go on a local volume,
email goes to the backend log, and AI stays off until a provider is set.

## Path C: older checkout without `deploy/`

The root `docker-compose.yml` builds from source and runs the backend in
development mode: it passes no `--production` flag and defaults
`APP_ENV=development`. To serve a restaurant from it, set
`APP_ENV=production` (or `ENV=production`) in `.env`. Production preflight
then refuses to start until the values it checks are set: at least the JWT
secret, the plugin key and the email settings. Releases before the
self-host preflight also require `RPC_URL`, `TRUSTED_PROXIES` and the AI
budgets.

```bash
cp .env.example .env && chmod 600 .env    # then fill every value marked replace_with_*
docker compose --env-file .env up -d --build
```

If the backend keeps restarting, read `docker compose logs backend`. Each
missing or unsafe value is logged as
`PRODUCTION PREFLIGHT — <code> [<component>]: <message>`, for example
`jwt.secret.missing`, and then the process exits. Fix every listed value and
run `up -d` again.

## 2. First admin

- With `ADMIN_EMAIL` and `ADMIN_PASSWORD` set, the first boot creates the
  admin. The step is idempotent, and it never overwrites the password of an
  existing account.
- **After the first successful boot, remove `ADMIN_PASSWORD` from `.env`**
  and run `docker compose up -d` again.
- Without the env vars, the operator creates the admin themselves. The
  password goes on stdin:

  ```bash
  read -rs ADMIN_PW
  printf '%s' "$ADMIN_PW" | docker compose exec -T backend \
    /app/server admin create --email you@example.com --password-stdin
  unset ADMIN_PW
  ```

- Who else may sign up is set by `REGISTRATION_MODE`. See
  `docs/self-hosting/admin.md`.

## 3. Verify

```bash
curl -fsS https://pos.example.com/api/v1/health/live
curl -fsS https://pos.example.com/api/v1/health/ready | jq
curl -fsS https://pos.example.com/api/v1/instance | jq '.features'
```

- `ready` answers `{"status":"ready"}` (HTTP 200) or `{"status":"not ready"}`
  (HTTP 503). The per-component breakdown is returned only with
  `Authorization: Bearer $HEALTH_DETAIL_TOKEN`, and only when that variable
  reaches the backend container. `deploy/docker-compose.yml` forwards it;
  the root `docker-compose.yml` does not (see "Forward a variable the
  compose file does not name" in `.claude/skills/troubleshoot/SKILL.md`).
  Without it, read `docker compose logs backend`.
- `features` shows what is switched on (AI, email, and so on).
- Sign in at `https://pos.example.com/dashboard` with the admin email.
  `https://pos.example.com/` redirects there until a venue is published,
  then serves that venue to guests. Platform admin tools are at `/admin`.

## 4. Hand over

Tell the operator:

- the URL and which admin email to use;
- that `ADMIN_PASSWORD` was removed from `.env`, or that they still need to
  remove it;
- how to back up. Check first: `docker compose config --profiles` prints
  only profile names. If it lists `backup` (the `deploy/` compose file has
  one), nightly sets land in `./backups` and `docker compose run --rm backup
  once` takes one now; see "Backups" in `deploy/README.md`. Otherwise take a
  `pg_dump` of the database plus a copy of the storage volume, as in step 2
  of `.claude/skills/upgrade/SKILL.md`, and see
  `backend/scripts/backup-db.sh` and `infra/backup/README.md` for scheduled
  dumps. Either way, copy the backups off the host;
- the next steps: `.claude/skills/configure-restaurant/SKILL.md` for the
  first restaurant, and the "Enable the in-app AI" section of
  `docs/agents/README.md` for AI.

## Do not

- Publish Postgres or the backend port on a public interface. In
  `deploy/docker-compose.yml` only Caddy publishes 80 and 443.
- Commit `.env`, or paste its contents anywhere.
- Run `docker compose down -v`. It deletes the database volume.
