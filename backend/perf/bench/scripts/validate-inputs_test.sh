#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "${tmp_dir}"' EXIT

for i in 1 2 3 4 5; do
  printf 'BenchmarkFlow%d-8 10 100 ns/op 10 B/op 1 allocs/op\n' "${i}"
done > "${tmp_dir}/baseline.txt"
cp "${tmp_dir}/baseline.txt" "${tmp_dir}/candidate.txt"
bash "${script_dir}/validate-inputs.sh" "${tmp_dir}/baseline.txt" "${tmp_dir}/candidate.txt" >/dev/null

printf 'BenchmarkOther-8 10 100 ns/op 10 B/op 1 allocs/op\n' > "${tmp_dir}/unrelated.txt"
if MINIMUM_BENCHMARKS=1 bash "${script_dir}/validate-inputs.sh" "${tmp_dir}/baseline.txt" "${tmp_dir}/unrelated.txt" >/dev/null 2>&1; then
  echo "validate-inputs_test FAIL: unrelated inputs passed" >&2
  exit 1
fi
if bash "${script_dir}/validate-inputs.sh" "${tmp_dir}/missing.txt" "${tmp_dir}/candidate.txt" >/dev/null 2>&1; then
  echo "validate-inputs_test FAIL: missing baseline passed" >&2
  exit 1
fi

echo "validate-inputs_test PASS"
