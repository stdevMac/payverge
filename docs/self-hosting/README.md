# Self-hosting Payverge

This is the operator manual for running your own Payverge instance with the
Docker Compose stack in [`deploy/`](../../deploy/README.md). The
[deploy README](../../deploy/README.md) is the command reference that ships
with every release; these pages explain the choices behind it.

## Which page do I need?

| I want to... | Read |
|---|---|
| Install Payverge on a server, or try it on my laptop | [install.md](install.md) |
| Know what a setting in `.env` does | [configuration.md](configuration.md) |
| Upgrade to a new release, or roll back | [upgrades.md](upgrades.md) |
| Make sure my data survives a dead disk | [backups.md](backups.md) |
| Let restaurants take card or USDC payments | [payments.md](payments.md) |
| Run behind Cloudflare, nginx or a load balancer | [reverse-proxies.md](reverse-proxies.md) |
| Harden the instance before real customers use it | [security.md](security.md) |
| Fix something that is broken | [troubleshooting.md](troubleshooting.md) |
| Send email (invites, receipts, password resets) | [email.md](email.md) |
| Store uploads in S3 or MinIO instead of a local volume | [storage.md](storage.md) |
| Turn on the AI waiter and assistants, or run my own model | [ai.md](ai.md) |
| Message guests over WhatsApp | [whatsapp.md](whatsapp.md) |
| Manage platform admins and demo data from the command line | [admin.md](admin.md) |
| Run a public demo that anyone can try and that resets nightly | [public-demo.md](public-demo.md) |
| Run on Coolify, Dokploy, Render or Railway instead of the compose stack | [platforms.md](platforms.md) |
| Run the frontend outside the stack (PaaS, custom build) | [frontend-config.md](frontend-config.md) |

## The short path

1. [Install](install.md): DNS, firewall, `bash install.sh`.
2. Go through the [security checklist](security.md#before-you-go-live).
3. Configure [email](email.md), then [payments](payments.md).
4. Copy backups off the server and [restore one](backups.md#restore-drill).
5. Pin a version and [upgrade](upgrades.md) on your schedule.

## How the stack fits together

```text
internet ─▶ Caddy (80/443, TLS) ─┬─▶ frontend (Next.js) ─▶ backend
                                 └─▶ backend (Go, /api/v1/*) ─▶ Postgres (internal network)
                                      backup job ─▶ ./backups ─▶ rclone (optional, off-site)
```

Everything an instance needs is in one directory (`/opt/payverge` or
`~/payverge`): `.env`, the compose and Caddy files, and `backups/`. Data lives
in Docker volumes. Only Caddy is reachable from the network.

Commands on these pages run from that directory. A note next to a command
says when it was run while these pages were written (the installer dry run,
the compose file validation, the backup and restore round trip). Every other
command needs a server, a domain or provider accounts and was **not run**
for these pages; it comes from the code and the deploy files.
