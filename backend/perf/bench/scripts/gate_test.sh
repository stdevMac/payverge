#!/usr/bin/env bash
# gate_test.sh — fixture tests for gate.sh
# Usage: bash gate_test.sh
# Exits 0 and prints "gate_test PASS" when all assertions succeed.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GATE="${SCRIPT_DIR}/gate.sh"
TESTDATA="${SCRIPT_DIR}/testdata"

fail() {
  echo "gate_test FAIL: $*" >&2
  exit 1
}

# ── test 1: clean CSV should exit 0 ─────────────────────────────────────────
echo "--- test 1: clean.csv should exit 0 ---"
if bash "${GATE}" "${TESTDATA}/clean.csv"; then
  echo "  PASS: clean.csv → exit 0 (expected)"
else
  fail "clean.csv exited non-zero, expected exit 0"
fi

# ── test 2: regress CSV should exit 1 ───────────────────────────────────────
echo "--- test 2: regress.csv should exit 1 ---"
if bash "${GATE}" "${TESTDATA}/regress.csv"; then
  fail "regress.csv exited 0, expected exit 1"
else
  echo "  PASS: regress.csv → exit 1 (expected)"
fi

# ── test 3: B/op-only regression (time ~) should exit 1 ─────────────────────
echo "--- test 3: bop_regress.csv (B/op +30% significant, time ~) should exit 1 ---"
if bash "${GATE}" "${TESTDATA}/bop_regress.csv"; then
  fail "bop_regress.csv exited 0, expected exit 1 (B/op regression)"
else
  echo "  PASS: bop_regress.csv → exit 1 (expected)"
fi

# ── test 4: non-significant large delta should exit 0 ───────────────────────
echo "--- test 4: nonsig.csv (+40% delta, p=0.40 not significant) should exit 0 ---"
if bash "${GATE}" "${TESTDATA}/nonsig.csv"; then
  echo "  PASS: nonsig.csv → exit 0 (expected)"
else
  fail "nonsig.csv exited non-zero, expected exit 0 (p≥0.05 should be ignored)"
fi

echo ""
echo "gate_test PASS"
