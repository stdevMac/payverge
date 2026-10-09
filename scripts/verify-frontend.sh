#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# A production shell must not leak URLs or dependency-pruning flags into the
# clean-install verification run.
unset FRONTEND_URL BASE_URL NEXT_PUBLIC_BASE_URL PUBLIC_URL
unset NODE_ENV NPM_CONFIG_PRODUCTION
export CI=true
export NEXT_STRICT_BUILD=1
# Explicit public build contract. These are synthetic HTTPS values, not inherited
# operator state; missing/malformed production configuration is tested separately.
export NEXT_PUBLIC_API_URL=https://api.verify.invalid/api/v1
export NEXT_PUBLIC_PUBLIC_URL=https://verify.invalid
export NEXT_PUBLIC_RPC_URL=https://rpc.verify.invalid
export NEXT_PUBLIC_NETWORK=base
export NEXT_PUBLIC_SUPPORT_EMAIL=support@verify.invalid

cd "$repo_root/frontend"

npm ci
npm run lint -- --max-warnings 0
npm run typecheck
npm exec -- jest --watchman=false --runInBand --silent --forceExit
npm run i18n:check
npm run i18n:validate
npm run build
