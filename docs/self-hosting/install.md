# Installing Payverge

The supported way to run Payverge is the Docker Compose stack in
[`deploy/`](../../deploy/README.md): Caddy (TLS), the frontend, the backend,
Postgres and a nightly backup job. This page covers what to prepare and the
choices you make while installing. The step-by-step reference stays in
[deploy/README.md](../../deploy/README.md#install).

## 1. Prepare the server

- Linux with 2 CPUs, 4 GB of RAM and 20 GB of disk (8 GB of RAM if you build
  the images yourself).
- Docker Engine with the Compose plugin 2.20 or newer. Check with
  `docker compose version`.
- `curl` and `openssl` (the installer uses both).

### DNS

Create an `A` record for the host name you will use (for example
`pay.example.com`) pointing at the server's public IPv4 address, and an
`AAAA` record if the server has IPv6. Wait until `dig +short pay.example.com`
returns the server's address from outside before you install: Let's Encrypt
checks it during the first start.

Publish an `AAAA` record only if IPv6 works end to end on the host, including
Docker's `ip6tables`; otherwise every IPv6 visitor arrives from Docker's
gateway address ([troubleshooting.md](troubleshooting.md#every-visitor-has-the-same-ip)).

### Ports and firewall

| Port | Protocol | From | Why |
|---|---|---|---|
| 80 | TCP | anywhere | Let's Encrypt HTTP challenge, and the redirect to HTTPS. |
| 443 | TCP | anywhere | The site. |
| 443 | UDP | anywhere | HTTP/3. Optional; browsers fall back to TCP. |
| 22 | TCP | your addresses | SSH. |

Nothing else needs to be open: Postgres, the backend and the frontend are not
published on the host. Open the ports in your cloud provider's firewall
(security group) **and** on the host, for example with `ufw`:

```sh
sudo ufw allow OpenSSH
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw allow 443/udp
sudo ufw enable
```

Docker writes its own iptables rules for published ports, which `ufw` does
not see. That does not matter here, because only Caddy publishes ports.

## 2. Run the installer

```sh
curl -fsSLO https://github.com/stdevMac/payverge/releases/latest/download/install.sh
less install.sh
bash install.sh
```

It asks for the domain, the admin email and whether to load demo restaurants,
then verifies the release files against `SHA256SUMS`, writes `.env` with new
random secrets, starts the stack and waits until the backend is ready. It
prints the admin password **once**; save it.

To see what it would do first (run for these docs from a clone, with Docker
stopped; it printed every copy, the `.env` keys it would write with secrets
hidden, and the `docker compose pull` / `up` it would run):

```sh
bash install.sh --dry-run --yes --domain pay.example.com --admin-email you@example.com
```

### Installer options

Every option is a flag or an environment variable. The flags are explained in
[deploy/README.md](../../deploy/README.md#install).

| Flag | Environment | Default |
|---|---|---|
| `--domain NAME` | `PAYVERGE_DOMAIN` | asked |
| `--admin-email EMAIL` | `PAYVERGE_ADMIN_EMAIL` | asked |
| `--acme-email EMAIL` | `PAYVERGE_ACME_EMAIL` | none |
| `--demo` / `--no-demo` | `PAYVERGE_DEMO` | asked |
| `--dir DIR` | `PAYVERGE_DIR` | `/opt/payverge` as root, else `~/payverge`; in place when run from a clone |
| `--version V` | `PAYVERGE_INSTALL_VERSION` | latest release, then pinned in `.env` |
| `--http-port [ADDR:]N` | `PAYVERGE_HTTP_PORT` | `80` (`127.0.0.1:80` for a `localhost` trial) |
| `--https-port [ADDR:]N` | `PAYVERGE_HTTPS_PORT` | `443` (`127.0.0.1:443` for a `localhost` trial) |
| `-y`, `--yes` | `PAYVERGE_YES` | prompt |
| `--dry-run` | `PAYVERGE_DRY_RUN` | off |
| `--force` | `PAYVERGE_FORCE` | off |
| `--build` | `PAYVERGE_BUILD` | off |
| `--no-start` | – | off |
| `--no-backup` | `PAYVERGE_NO_BACKUP` | off |
| `--require-signature` | `PAYVERGE_REQUIRE_SIGNATURE` | off: a downloaded release is checked against `SHA256SUMS`, and that file's cosign signature is checked only when `cosign` is installed. With the flag, a missing `cosign` stops the installer before the download, and a failed signature stops it before any release file is installed. |
| – | `PAYVERGE_READY_TIMEOUT` | `600`: seconds to wait for `/api/v1/health/ready` |
| – | `PAYVERGE_DOCKER_TIMEOUT` | `30`: seconds before an unresponsive Docker daemon counts as down |
| – | `PAYVERGE_RELEASES_URL` | `https://github.com/stdevMac/payverge/releases`: a mirror laid out like GitHub releases (`download/vX.Y.Z/...`), for air-gapped or pinned installs |

`PAYVERGE_VERSION` is not an installer option: it is the image tag the
installer writes into `.env` ([upgrades.md](upgrades.md)).

## Without the installer

From a clone or an unpacked release, in `deploy/`:

```sh
cp .env.example .env
chmod 600 .env
$EDITOR .env        # the "Required" block: DOMAIN, ADMIN_EMAIL, ADMIN_PASSWORD and three secrets
docker compose up -d
docker compose logs -f backend    # until it says it is listening
```

Generate each secret with `openssl rand`, as noted next to it in
`.env.example` and [configuration.md](configuration.md#required). The admin
account is created on the first start from `ADMIN_EMAIL` and
`ADMIN_PASSWORD`; remove `ADMIN_PASSWORD` from `.env` afterwards.

For this page, `docker compose config --quiet` was run against the stock
`deploy/docker-compose.yml` with a minimal `.env` (`DOMAIN`, `ADMIN_EMAIL` and the three
secrets), and passed. Starting the stack was not run.

## Trying it on your laptop

```sh
bash install.sh --domain localhost --admin-email you@example.com --demo
```

Caddy signs `localhost` with its own local CA; trust it as described in
[deploy/README.md](../../deploy/README.md#local-trial), or accept the browser
warning. The trial listens on `127.0.0.1` only. Use
`--http-port 8080 --https-port 8443` if 80 and 443 are taken,
then open `https://localhost:8443`. Emails are written to the backend log
instead of being sent until you configure a provider ([email.md](email.md)).

## First login

1. Open `https://pay.example.com/dashboard` and sign in with the admin email
   and the printed password. If you lost it, reset it from the command line
   ([deploy/README.md](../../deploy/README.md#admin-access)). Until a venue
   is published, `https://pay.example.com/` sends you there too.
2. Create your restaurant, or open a demo restaurant if you loaded them.
   Once its public page is published, `/` serves it to guests. With several
   published venues `/` lists them; set `PRIMARY_VENUE` to pin one
   ([configuration.md](configuration.md)). Platform admin tools stay at
   `/admin`.
3. Invite the people who will run restaurants. Sign-ups need an invite code
   by default (`REGISTRATION_MODE=invite`).
4. Set up email ([email.md](email.md)) so invites, receipts and password
   resets go out.
5. Work through [security.md](security.md#before-you-go-live) before taking
   real orders.

## Next

- Everything you can set in `.env`: [configuration.md](configuration.md).
- Payments: [payments.md](payments.md).
- Behind Cloudflare or another web server: [reverse-proxies.md](reverse-proxies.md).
- Backups: [backups.md](backups.md).
