#!/usr/bin/env bash
# Run a command in backend/ with the hermetic test environment.
#
#   scripts/ci/backend-env.sh go test -short ./...
#
# Backend tests read URLs, provider keys and mode switches from the process
# environment. A runner or developer shell that happens to export one of them
# changes test behaviour (redirect allow-lists, production mode, live AI
# calls). This wrapper drops every such variable, sets a deterministic,
# test-only encryption key, and then runs the command from backend/.
#
# The unset list must match scripts/verify-backend.sh; the workflow contract
# test TestBackendEnvMatchesVerifyBackend compares the two.
set -euo pipefail

if [[ $# -eq 0 ]]; then
  echo "usage: $0 <command> [args...]   (runs in backend/ with a scrubbed env)" >&2
  exit 2
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# Shell and runner URLs must not choose test behavior.
unset FRONTEND_URL BASE_URL NEXT_PUBLIC_BASE_URL APP_BASE_URL
unset ALLOWED_REDIRECT_DOMAINS APP_DOMAIN API_DOMAIN COOKIE_DOMAIN DOMAIN
unset ENV APP_ENV NODE_ENV NEXT_PUBLIC_ENV
unset GOOGLE_TRANSLATE_API_KEY GEMINI_API_KEY OPENROUTER_API_KEY FIRECRAWL_API_KEY LIFI_API_KEY
unset NANOBANANA_API_KEY PUBLIC_URL STORAGE_DRIVER EMAIL_PROVIDER REGISTRATION_MODE LLM_BASE_URL LLM_API_KEY
# Encryption tests must not inherit a developer or production key. Use an
# obvious, deterministic 32-byte test-only value after scrubbing the shell.
export PLUGIN_SECRET_KEY=payverge-hermetic-test-key-00000
export GO_ENV=test

cd "$repo_root/backend"
exec "$@"
