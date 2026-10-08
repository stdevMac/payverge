#!/usr/bin/env bash
# Reproducible local k6 build with the community SSE extension.
set -euo pipefail

output="${1:-/tmp/payverge-k6}"
xk6_version="v1.4.8"
k6_version="v1.8.0"
sse_version="v0.1.12"

[[ "${output}" == /* ]] || { echo "k6 output path must be absolute" >&2; exit 1; }
mkdir -p "$(dirname "${output}")"
go run "go.k6.io/xk6/cmd/xk6@${xk6_version}" build "${k6_version}" \
  --with "github.com/phymbert/xk6-sse@${sse_version}" \
  --output "${output}"
"${output}" version
