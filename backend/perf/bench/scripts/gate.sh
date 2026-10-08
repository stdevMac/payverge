#!/usr/bin/env bash
# gate.sh — benchstat-aware performance regression gate.
#
# Usage: gate.sh <benchstat.csv>
#
# Reads a benchstat CSV comparison file (produced via `benchstat -format csv
# old.txt new.txt`) and exits 1 when ANY metric (sec/op, B/op, allocs/op)
# shows a statistically significant (p < 0.05) regression >= THRESHOLD_PCT.
#
# Rows where "vs base" is "~" are skipped (no significant change detected by
# benchstat). geomean / header rows (empty P column) are also skipped.
#
# Environment:
#   THRESHOLD_PCT  — regression threshold in percent (default: 15)

set -euo pipefail

CSV="${1:?usage: gate.sh <benchstat.csv>}"
THRESHOLD_PCT="${THRESHOLD_PCT:-15}"

if [ ! -f "${CSV}" ]; then
  echo "::error::gate.sh: file not found: ${CSV}" >&2
  exit 1
fi

# CSV format produced by `benchstat -format csv` (benchstat v0.0.1+):
#
#   goos: linux
#   goarch: amd64
#   pkg: example
#   ,old,,new,,,
#   ,sec/op,CI,sec/op,CI,vs base,P
#   Foo-8,1e-06,∞,1.228e-06,∞,+22.80%,p=0.008 n=5
#   geomean,7.07e-07,,7.95e-07,,+12.47%,
#
# Key columns (1-indexed, comma-separated):
#   $1  — benchmark name (empty on header/geomean rows)
#   $6  — "vs base": "~", "+22.80%", "-5.00%", or empty (geomean/header)
#   $7  — P value: "p=0.008 n=5" or empty (geomean/header)
#
# Rules:
#   - Skip rows where $6 == "~"          (benchstat says no significant change)
#   - Skip rows where $7 is empty        (geomean / section header / metadata)
#   - Skip rows where delta is negative  (improvement, not regression)
#   - Skip rows where p >= 0.05          (not statistically significant)
#   - FAIL if delta% >= THRESHOLD_PCT AND p < 0.05

found_regression=0

while IFS= read -r line; do
  # Skip blank lines and non-CSV metadata lines (goos:, goarch:, pkg:)
  [[ -z "${line}" ]] && continue
  [[ "${line}" =~ ^(goos|goarch|pkg): ]] && continue

  # Use awk to parse this line; output "FAIL name delta_pct p_val" if it's a regression candidate
  result=$(awk -v threshold="${THRESHOLD_PCT}" -F',' '
  {
    name   = $1
    vs     = $6
    p_col  = $7

    # Skip header/geomean rows: empty P column
    if (p_col == "") next

    # Skip "~" rows (benchstat no-significant-change marker)
    if (vs == "~") next

    # Extract the numeric delta percentage from "vs base" field
    # Examples: "+22.80%", "-5.00%", "+0.25%"
    delta_str = vs
    gsub(/%/, "", delta_str)
    delta = delta_str + 0

    # Only care about regressions (positive delta)
    if (delta < 0) next
    if (delta < threshold) next

    # Extract p-value from P column: "p=0.008 n=5" or "p=1.000 n=5"
    # Use POSIX-compatible sub() — works on macOS awk and gawk alike.
    p_val = p_col
    if (match(p_val, /p=[0-9.]+/)) {
      sub(/.*p=/, "", p_val)   # strip everything before the digits
      sub(/[^0-9.].*/, "", p_val)  # strip " n=5" suffix and anything after
      p = p_val + 0
    } else {
      # Cannot parse p-value; treat as significant (fail-closed)
      p = 0
    }

    # Fail when p < 0.05 (statistically significant)
    if (p >= 0.05) next

    print "REGRESSION", name, delta, p
  }
  ' <<< "${line}")

  if [[ -n "${result}" ]]; then
    read -r _ bench_name delta_pct p_val <<< "${result}"
    echo "::error::Perf regression >= ${THRESHOLD_PCT}% (significant) detected: ${bench_name} +${delta_pct}% (p=${p_val})"
    found_regression=1
  fi
done < "${CSV}"

if [ "${found_regression}" -eq 1 ]; then
  exit 1
fi

echo "gate OK — no significant regressions >= ${THRESHOLD_PCT}% detected in ${CSV}"
