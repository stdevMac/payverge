#!/bin/bash
# Payverge model benchmark — production-faithful.
#
# Each surface loads the REAL backend prompt assets through
# lib/prod_prompts.js (system prompt + a genuine user turn) and runs the two
# candidate models with that surface's production params. Per-surface results
# are written to results/<surface>.json and rolled up into one scoreboard.
#
# Usage:
#   ./evals/promptfoo/runner.sh               # run all surfaces + scoreboard
#   ./evals/promptfoo/runner.sh waiter-ordering   # run one surface
#   ./evals/promptfoo/runner.sh scoreboard    # re-print scoreboard from last run
#   ./evals/promptfoo/runner.sh view          # open the promptfoo web dashboard

set -e
cd "$(dirname "$0")/../.."
ROOT="$(pwd)"
CFG="$ROOT/evals/promptfoo/configs"
RES="$ROOT/evals/promptfoo/results"
mkdir -p "$RES"

if [[ -n "${PROMPTFOO_BIN:-}" ]]; then
  read -r -a PROMPTFOO_CMD <<< "$PROMPTFOO_BIN"
else
  PROMPTFOO_CMD=(npx --yes promptfoo@0.120.19)
fi

SURFACES=(waiter-ordering waiter-concierge director wizard guardrails assistant-v2)

run_one() {
  local s="$1"
  local report
  local status=0
  echo "── $s ─────────────────────────────────────────────"
  report="$(mktemp)"
  "${PROMPTFOO_CMD[@]}" eval -c "$CFG/$s.yaml" --no-cache --output "$RES/$s.json" >"$report" 2>&1 || status=$?
  grep -iE "Results:|error|invalid" "$report" || true
  rm -f "$report"
  return "$status"
}

case "${1:-run}" in
  view)
    "${PROMPTFOO_CMD[@]}" view -y
    ;;
  scoreboard)
    node "$ROOT/evals/promptfoo/lib/scoreboard.cjs"
    ;;
  run)
    echo "=== Payverge Model Benchmark (gemini vs gpt-4o-mini) ==="
    for s in "${SURFACES[@]}"; do run_one "$s"; done
    node "$ROOT/evals/promptfoo/lib/scoreboard.cjs"
    echo "Run './evals/promptfoo/runner.sh view' for the per-test dashboard."
    ;;
  *)
    # single named surface
    if [[ " ${SURFACES[*]} " == *" $1 "* ]]; then
      run_one "$1"
      node "$ROOT/evals/promptfoo/lib/scoreboard.cjs"
    else
      echo "Unknown surface '$1'. Valid: ${SURFACES[*]}"
      exit 1
    fi
    ;;
esac
