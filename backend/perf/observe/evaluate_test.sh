#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "${tmp_dir}"' EXIT
header='timestamp,cpu_percent,memory_bytes,disk_percent,postgres_open_connections,postgres_waiting_locks,postgres_lock_wait_ms,go_goroutines,background_job_lag_seconds,queue_depth,sse_dropped'

printf '%s\n%s\n' "${header}" '2026-08-01T00:00:00Z,50,500000000,40,20,0,0,200,5,2,0' > "${tmp_dir}/clean.csv"
bash "${script_dir}/evaluate.sh" "${tmp_dir}/clean.csv" >/dev/null

printf '%s\n%s\n' "${header}" '2026-08-01T00:00:00Z,90,500000000,40,20,1,250,2000,120,200,1' > "${tmp_dir}/over.csv"
if bash "${script_dir}/evaluate.sh" "${tmp_dir}/over.csv" >/dev/null 2>&1; then
  echo "evaluate_test FAIL: over-limit fixture passed" >&2
  exit 1
fi

if bash "${script_dir}/evaluate.sh" "${tmp_dir}/missing.csv" >/dev/null 2>&1; then
  echo "evaluate_test FAIL: missing observations passed" >&2
  exit 1
fi

echo "evaluate_test PASS"
