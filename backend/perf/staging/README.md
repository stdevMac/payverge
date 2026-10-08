# staging-perf: API benchmark target stack

Docker-compose bring-up for a perf-tuned backend + Postgres pair you can point
k6 at. This is scaffolding, not provisioning — the operator supplies the VM.

Compose file: `docker-compose.staging-perf.yml`. Env template: `.env.example`.

## 1. Provision a VM

Minimum spec: **4 vCPU, 8GB RAM, 50GB disk**, Docker + docker compose installed.
The compose contract allocates 2 CPU / 2 GiB to each service, caps PostgreSQL
at 100 connections, and keeps the application's existing 25-connection pool.

- AWS: `c6i.xlarge`
- GCP: `n2-standard-4`
- DigitalOcean: 4 vCPU / 8GB droplet

Open inbound TCP `8080` from the load-generator host (or all of `0.0.0.0/0` if
you fully trust the network — the backend has no public surface yet).

## 2. Copy compose + env to the host

```bash
ssh user@perf-host "mkdir -p ~/payverge-perf"
scp backend/perf/staging/docker-compose.staging-perf.yml user@perf-host:~/payverge-perf/docker-compose.yml
cp  backend/perf/staging/.env.example backend/perf/staging/.env  # then edit secrets
scp backend/perf/staging/.env user@perf-host:~/payverge-perf/.env
```

Or use the Makefile helpers: `PERF_HOST=user@perf-host make perf-staging-up`.

The compose file pulls `${PAYVERGE_IMAGE_PREFIX}-backend:${PAYVERGE_IMAGE_TAG}` (the release image name is `ghcr.io/stdevmac/payverge-backend`)
(`pull_policy: always`). No `perf-latest` tag is published from this
repository, so build the backend image, push it to a registry the VM can pull
from, and set both variables in `.env`:

```bash
docker build -t registry.example.com/payverge-backend:perf-latest backend
docker push registry.example.com/payverge-backend:perf-latest
# .env: PAYVERGE_IMAGE_PREFIX=registry.example.com/payverge
```

## 3. Bring up

```bash
ssh user@perf-host "cd ~/payverge-perf && docker compose --env-file .env up -d"
```

Watch logs: `make perf-staging-logs PERF_HOST=user@perf-host`.

## 4. Seed perf fixtures

The seed CLI runs locally and writes to the staging-perf Postgres. If Postgres
isn't reachable from your machine, open an SSH tunnel first:

```bash
ssh -L 5432:postgres:5432 user@perf-host
# in another shell:
DATABASE_URL='postgres://payverge:PASSWORD@127.0.0.1:5432/payverge?sslmode=disable' \
  make perf-staging-seed
```

## 5. Run k6 against it

```bash
make k6-build
make k6-burst BASE_URL=http://perf-host.example.com:8080 \
  PERF_EXTERNAL_CONFIRM=isolated-perf
```

Launch D5 proof uses `backend/perf/k6/Makefile`: `burst`, `soak_1h`, and
`candidate_5x` all require an explicit isolated-perf confirmation. Run the
observer alongside k6 and enforce its CSV afterward:

```bash
OUTPUT_DIR=/tmp/payverge-perf-results DURATION_SECONDS=3600 \
  bash backend/perf/observe/capture.sh
bash backend/perf/observe/evaluate.sh \
  /tmp/payverge-perf-results/capacity-observations.csv
```

## 6. Tear down

```bash
make perf-staging-down PERF_HOST=user@perf-host
# Or, to also wipe Postgres data: ssh user@perf-host "cd ~/payverge-perf && docker compose down -v"
```

## Safety

- **`--env-file .env` is not optional.** Compose only injects variables that
  appear in the compose file; without `--env-file` you'll get missing-var
  errors for `JWT_SECRET_KEY` / `PPROF_TOKEN`.
- **`PERF_STUB_PROVIDERS=true` is forward-looking.** The backend doesn't honor
  it yet; it's set so a future task can flip Stripe/MercadoPago/etc. into stub
  mode without re-deploying. Stripe keys in `.env` must already be test-mode.
- **Production rollout is out of scope.** This stack is for perf testing only.
  Do not point real DNS at it, do not put real credentials in `.env`.
- **Rate limits stay enabled.** The stack pins the global per-source limit to
  1,200 requests/minute and the SSE source/business ceilings to 30/200.
  Five-times proof needs distributed load-generator IPs, not disabled guards.
- **`docker compose down -v` wipes Postgres data.** The Makefile uses `down`
  without `-v` by default; add `-v` manually when you want a clean slate.
- **Backend listens on `0.0.0.0:8080`** (production binds 127.0.0.1). Lock
  inbound `8080` to the load-generator host's IP via security group / firewall.
