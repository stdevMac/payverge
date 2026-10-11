#!/usr/bin/env bash
# Enforces system limits in capacity-contract.json against capture.sh output.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
contract="${CONTRACT_FILE:-${script_dir}/../capacity-contract.json}"
csv="${1:?usage: evaluate.sh capacity-observations.csv}"
command -v jq >/dev/null || { echo "jq is required" >&2; exit 1; }
[[ -s "${csv}" ]] || { echo "capacity observations are absent: ${csv}" >&2; exit 1; }
[[ "$(wc -l < "${csv}")" -gt 1 ]] || { echo "capacity observations contain no samples" >&2; exit 1; }

max_column() {
  local column="$1"
  awk -F',' -v wanted="${column}" '
    NR == 1 { for (i=1; i<=NF; i++) if ($i == wanted) col=i; next }
    col == 0 { exit 2 }
    $col + 0 > max { max=$col + 0 }
    END { if (col == 0) exit 2; print max + 0 }
  ' "${csv}"
}

failures=0
check_max() {
  local column="$1" limit_key="$2" actual limit
  actual="$(max_column "${column}")"
  limit="$(jq -er ".limits.${limit_key}" "${contract}")"
  if ! awk -v actual="${actual}" -v limit="${limit}" 'BEGIN { exit !(actual <= limit) }'; then
    echo "FAIL ${column}: max=${actual} limit=${limit}" >&2
    failures=$((failures + 1))
  else
    echo "PASS ${column}: max=${actual} limit=${limit}"
  fi
}

check_max cpu_percent cpu_percent
check_max memory_bytes memory_bytes
check_max disk_percent disk_percent
check_max postgres_open_connections postgres_open_connections
check_max postgres_waiting_locks postgres_waiting_locks
check_max postgres_lock_wait_ms postgres_lock_wait_ms
check_max go_goroutines goroutines
check_max background_job_lag_seconds background_job_lag_seconds
check_max queue_depth queue_depth

(( failures == 0 )) || exit 1
echo "capacity system limits PASS"
