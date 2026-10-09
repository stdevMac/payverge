#!/usr/bin/env bash
# Local/perf-environment capacity sampler. It never mutates the stack.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
perf_root="$(cd "${script_dir}/.." && pwd)"
compose_file="${COMPOSE_FILE:-${perf_root}/staging/docker-compose.staging-perf.yml}"
compose_env_file="${COMPOSE_ENV_FILE:-${perf_root}/staging/.env}"
sample_seconds="${SAMPLE_SECONDS:-5}"
duration_seconds="${DURATION_SECONDS:-60}"
output_dir="${OUTPUT_DIR:-${TMPDIR:-/tmp}/payverge-perf-observe-$$}"
output_file="${output_dir}/capacity-observations.csv"
base_url="${BASE_URL:-http://127.0.0.1:8080}"

for command_name in docker curl awk sed; do
  command -v "${command_name}" >/dev/null || {
    echo "missing required command: ${command_name}" >&2
    exit 1
  }
done
[[ "${sample_seconds}" =~ ^[1-9][0-9]*$ ]] || { echo "SAMPLE_SECONDS must be a positive integer" >&2; exit 1; }
[[ "${duration_seconds}" =~ ^[1-9][0-9]*$ ]] || { echo "DURATION_SECONDS must be a positive integer" >&2; exit 1; }

mkdir -p "${output_dir}"
compose_args=(-f "${compose_file}")
[[ -f "${compose_env_file}" ]] && compose_args=(--env-file "${compose_env_file}" "${compose_args[@]}")
printf '%s\n' 'timestamp,cpu_percent,memory_bytes,disk_percent,postgres_open_connections,postgres_waiting_locks,postgres_lock_wait_ms,go_goroutines,background_job_lag_seconds,queue_depth,sse_dropped' > "${output_file}"

metric_sum() {
  local metric_name="$1" metrics_body="$2"
  awk -v name="${metric_name}" '$1 ~ ("^" name "($|\\{)") { total += $NF } END { printf "%.0f", total + 0 }' <<<"${metrics_body}"
}

deadline=$((SECONDS + duration_seconds))
while (( SECONDS < deadline )); do
  metrics_args=(--fail --silent --show-error --max-time 5)
  if [[ -n "${METRICS_TOKEN:-}" ]]; then
    metrics_args+=(--header "Authorization: Bearer ${METRICS_TOKEN}")
  fi
  metrics_body="$(curl "${metrics_args[@]}" "${base_url}/metrics")"

  stats="$(docker compose "${compose_args[@]}" stats --no-stream --format '{{.Service}},{{.CPUPerc}},{{.MemUsage}}' backend)"
  cpu_percent="$(awk -F',' '{ gsub(/%/, "", $2); print $2 + 0 }' <<<"${stats}")"
  # cgroup memory is authoritative and unit-free; fall back to zero if the
  # engine does not expose the v2 path inside this container.
  memory_bytes="$(docker compose "${compose_args[@]}" exec -T backend sh -c 'cat /sys/fs/cgroup/memory.current 2>/dev/null || echo 0')"
  backend_disk_percent="$(docker compose "${compose_args[@]}" exec -T backend sh -c "df -P /app/data | awk 'NR==2 {gsub(/%/, \"\", \\$5); print \\$5}'")"
  postgres_disk_percent="$(docker compose "${compose_args[@]}" exec -T postgres sh -c "df -P /var/lib/postgresql | awk 'NR==2 {gsub(/%/, \"\", \\$5); print \\$5}'")"
  disk_percent="$(awk -v app="${backend_disk_percent}" -v db="${postgres_disk_percent}" 'BEGIN { print (app > db ? app : db) }')"

  read -r postgres_open_connections postgres_waiting_locks postgres_lock_wait_ms < <(
    docker compose "${compose_args[@]}" exec -T postgres psql -U "${DB_USER:-payverge}" -d "${DB_NAME:-payverge}" -At -F' ' -c "
      SELECT
        (SELECT count(*) FROM pg_stat_activity WHERE datname = current_database()),
        (SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'),
        COALESCE((SELECT max(EXTRACT(EPOCH FROM (clock_timestamp() - query_start)) * 1000)::bigint
                  FROM pg_stat_activity
                  WHERE datname = current_database() AND wait_event_type = 'Lock'), 0);"
  )

  go_goroutines="$(metric_sum go_goroutines "${metrics_body}")"
  background_job_lag_seconds="$(awk -v a="$(metric_sum payverge_fiscal_delivery_oldest_age_seconds "${metrics_body}")" -v b="$(metric_sum payverge_payment_reconciliation_oldest_pending_seconds "${metrics_body}")" 'BEGIN { print (a > b ? a : b) }')"
  queue_depth=$((
    $(metric_sum payverge_plugin_notification_queue_depth "${metrics_body}") +
    $(metric_sum payverge_fiscal_jobs_queue_depth "${metrics_body}") +
    $(metric_sum payverge_fiscal_delivery_pending "${metrics_body}")
  ))
  sse_dropped="$(metric_sum payverge_sse_dropped_events_total "${metrics_body}")"

  printf '%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s\n' \
    "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "${cpu_percent}" "${memory_bytes}" "${disk_percent}" \
    "${postgres_open_connections}" "${postgres_waiting_locks}" "${postgres_lock_wait_ms}" \
    "${go_goroutines}" "${background_job_lag_seconds}" "${queue_depth}" "${sse_dropped}" >> "${output_file}"

  remaining=$((deadline - SECONDS))
  (( remaining <= 0 )) && break
  (( remaining < sample_seconds )) && sleep "${remaining}" || sleep "${sample_seconds}"
done

echo "capacity observations: ${output_file}"
