#!/bin/bash
# One-off: run every promptfoo config with ONLY the gemini provider
# (drops gpt-4o-mini via --filter-providers). Fresh (--no-cache) for a true
# current-output snapshot. Per-config JSON → results/gemini-only/, summary → SUMMARY.txt
set -u
cd "$(dirname "$0")" || exit 1
OUT="results/gemini-only"
mkdir -p "$OUT"
SUMMARY="$OUT/SUMMARY.txt"
: > "$SUMMARY"

# Same pinned CLI as runner.sh; PROMPTFOO_BIN overrides (e.g. a local install).
if [[ -n "${PROMPTFOO_BIN:-}" ]]; then
  read -r -a PROMPTFOO_CMD <<< "$PROMPTFOO_BIN"
else
  PROMPTFOO_CMD=(npx --yes promptfoo@0.120.19)
fi

CONFIGS=(
  waiter-ordering waiter-ordering-full waiter-concierge director wizard guardrails
  extraction
  helpfulness-director helpfulness-waiter-ordering helpfulness-waiter-concierge helpfulness-wizard
  director-setup-mode director-proposes-not-applies
  redteam-director redteam-guardrails redteam-waiter-ordering redteam-waiter-concierge redteam-wizard
)

for c in "${CONFIGS[@]}"; do
  echo "── running $c ──"
  line=$("${PROMPTFOO_CMD[@]}" eval -c "configs/$c.yaml" \
      --filter-providers 'gemini-2.5-flash' --no-cache \
      --output "$OUT/$c.json" 2>&1 | grep -iE "Results:" | tail -1)
  [ -z "$line" ] && line="(no Results line — see run log; likely error)"
  printf '%-34s %s\n' "$c" "$line" | tee -a "$SUMMARY"
done

echo ""
echo "===== GEMINI-2.5-FLASH-ONLY SCOREBOARD ====="
cat "$SUMMARY"
echo "done"
