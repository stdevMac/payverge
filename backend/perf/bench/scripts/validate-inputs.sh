#!/usr/bin/env bash
# Rejects absent, empty, or unrelated benchmark inputs before benchstat.
set -euo pipefail

baseline="${1:?usage: validate-inputs.sh baseline.txt candidate.txt}"
candidate="${2:?usage: validate-inputs.sh baseline.txt candidate.txt}"
minimum_benchmarks="${MINIMUM_BENCHMARKS:-5}"
minimum_overlap_percent="${MINIMUM_OVERLAP_PERCENT:-80}"

for file in "${baseline}" "${candidate}"; do
  [[ -s "${file}" ]] || { echo "benchmark input is absent or empty: ${file}" >&2; exit 1; }
done

tmp_dir="$(mktemp -d)"
trap 'rm -rf "${tmp_dir}"' EXIT
extract_names() {
  awk '$1 ~ /^Benchmark/ { name=$1; sub(/-[0-9]+$/, "", name); print name }' "$1" | sort -u
}
extract_names "${baseline}" > "${tmp_dir}/baseline.names"
extract_names "${candidate}" > "${tmp_dir}/candidate.names"

baseline_count="$(wc -l < "${tmp_dir}/baseline.names")"
candidate_count="$(wc -l < "${tmp_dir}/candidate.names")"
if (( baseline_count < minimum_benchmarks || candidate_count < minimum_benchmarks )); then
  echo "benchmark inputs need at least ${minimum_benchmarks} unique benchmarks; baseline=${baseline_count} candidate=${candidate_count}" >&2
  exit 1
fi

overlap_count="$(comm -12 "${tmp_dir}/baseline.names" "${tmp_dir}/candidate.names" | wc -l)"
overlap_percent=$((overlap_count * 100 / candidate_count))
if (( overlap_percent < minimum_overlap_percent )); then
  echo "benchmark overlap ${overlap_percent}% is below ${minimum_overlap_percent}% (baseline=${baseline_count} candidate=${candidate_count})" >&2
  exit 1
fi

echo "benchmark inputs valid: baseline=${baseline_count} candidate=${candidate_count} overlap=${overlap_percent}%"
