#!/usr/bin/env bash
# Run the "Trattoria Bella Vista" showcase seed against the running stack.
#
# Usage:
#   ./scripts/seed-showcase.sh                                  # compose, placeholder owner
#   ./scripts/seed-showcase.sh --owner-email=you@example.com    # attach to existing user
#   DSN=postgres://... ./scripts/seed-showcase.sh --host        # against a host DB
#
# The owner-email user must already exist in the `users` table (register the
# account first via /business/register or OAuth). If the lookup fails the
# seed logs a warning and continues with the placeholder owner — you can
# re-run later after the user is created.
#
# The script is idempotent — re-running it is a no-op for rows that already
# exist. To regenerate from scratch:
#   docker compose exec postgres psql -U payverge -d payverge \
#     -c "DELETE FROM businesses WHERE business_id = 'showcase-bellavista';"
#
# Then re-run this script.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

# Pull out --host so it doesn't get passed to docker compose / go run.
HOST_MODE=0
ARGS=()
for arg in "$@"; do
  if [[ "$arg" == "--host" ]]; then
    HOST_MODE=1
  else
    ARGS+=("$arg")
  fi
done

if [[ "$HOST_MODE" -eq 1 ]]; then
  if [[ -z "${DSN:-}" ]]; then
    echo "DSN env var required when using --host. Example:" >&2
    echo "  DSN='postgres://payverge:payverge_password@localhost:5432/payverge?sslmode=disable' ./scripts/seed-showcase.sh --host --owner-email=you@example.com" >&2
    exit 1
  fi
  cd backend
  exec go run ./perf/seed/showcase --dsn="$DSN" "${ARGS[@]}"
fi

if [[ ! -f .env ]]; then
  echo "Missing .env at repo root — copy .env.example to .env first." >&2
  exit 1
fi

# `--profile seed` activates the showcase-seed service which is gated off
# from regular `docker compose up`. `--rm` cleans up after the one-shot.
exec docker compose --env-file .env --profile seed run --rm showcase-seed "${ARGS[@]}"
