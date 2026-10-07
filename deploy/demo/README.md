# Public demo bundle

Files that turn a self-host install into the public demo at
`demo.payverge.io`: one-click sign-in, a banner, outbound side effects off,
and a nightly reset to a pristine snapshot. The full runbook (sizing, DNS,
install, rotation, AI budget, teardown) is
[`docs/self-hosting/public-demo.md`](../../docs/self-hosting/public-demo.md).

| File | What it does |
|---|---|
| `docker-compose.demo.yml` | Overlay: `DEMO_MODE`, `DEMO_DATA`, closed signup, email to the log, no outbound channels or card providers, AI off unless `DEMO_AI_API_KEY` (AI image generation stays refused either way), resource limits, Caddy `site.d` mount. |
| `caddy/demo.caddy` | Imported inside the site block: `X-Robots-Tag: noindex`. |
| `reset-demo.sh` | `--snapshot` once, then nightly: restore into a scratch DB, swap, restore uploads and space-scan artifacts, health-check, roll back on failure. `--maintenance` resets and keeps the site offline for a re-snapshot. `--check` is a dry preflight and staleness watchdog. |
| `systemd/payverge-demo-reset.{service,timer}` | Runs the reset at 03:00 UTC. |
| `systemd/payverge-demo-reset-check.{service,timer}` | Hourly watchdog: fails when the last good reset (or the snapshot, before the first reset) is over 26 hours old. |
| `cron.example` | The same with cron. |

Quick start, from `deploy/` in a clone:

```bash
COMPOSE_PROJECT_NAME=payverge-demo ./install.sh --domain demo.payverge.io --demo --no-start
# Layer the demo overlay on every docker compose command in this directory.
# Edit the key rather than appending, and keep docker-compose.build.yml when
# install.sh --build already put it there.
cur=$(sed -n 's/^COMPOSE_FILE=//p' .env)
sed -i.bak '/^COMPOSE_FILE=/d' .env && rm -f .env.bak
printf 'COMPOSE_FILE=%s:demo/docker-compose.demo.yml\n' "${cur:-docker-compose.yml}" >> .env
docker compose up -d --wait
demo/reset-demo.sh --snapshot     # once the showroom is seeded
demo/reset-demo.sh --check
```

The snapshot lands in `demo/pristine/` (gitignored, mode 600). Re-take it
after an upgrade or after rotating the admin password, or the reset puts the
old one back. Once a snapshot exists, `--snapshot` refuses to dump the live
demo: run `demo/reset-demo.sh --maintenance` (the site stays offline), make
the change, then `demo/reset-demo.sh --snapshot`.
