# 0003. Store money as `int64` cents; send dollars on the wire

- Status: Accepted
- Date: 2026-10-03 (records a decision already in effect)

## Context

Bills are added up, taxed, split between guests, tipped, partly paid,
refunded and reported to tax authorities. Floating-point arithmetic drifts
by fractions of a cent across those steps, and a split that does not add up
exactly leaves a bill unpayable or overpaid. Payment providers count in each
currency's minor unit, which is not always a hundredth.

## Decision

- **Storage:** every settled amount (bill totals, payments, tips, split
  shares, ledger entries, payroll) is an `int64` of major units times 100,
  for every currency.
- **Arithmetic:** totals go through
  [money/bill_totals.go](../../backend/internal/money/bill_totals.go):
  percentages are computed to a thousandth of a percent and rounded half-up
  once. Splits allocate integer cents and give the remainder to the last
  share, so parts always sum to the whole.
- **Wire format:** JSON carries **dollars** as a number. Custom
  `MarshalJSON` methods in
  [models_json.go](../../backend/internal/database/models_json.go) convert
  on the way out; parsers convert back on the way in. The few fields that
  carry cents say so in their name (`amount_cents`).
- **Provider boundary:**
  [money/minorunits.go](../../backend/internal/money/minorunits.go) converts
  stored cents to the currency's real minor unit (zero-decimal and
  three-decimal currencies included) before a provider call, and back after.
- **Other units:** AI spend uses integer micro-USD; USDC uses integer
  micro-USDC (six decimals). The rule is the same: integers in the ledger.

## Consequences

- **Easier:** totals and splits are exact and testable. Concurrent payments
  compare and add integers under a row lock.
- **Harder:** two units meet at the API edge. Every handler and every client
  must know that the database holds cents and JSON holds dollars, and must
  not convert twice (printer payloads are a known trap).
- **Accepted debt:** menu and line-item prices are still `float64` dollars,
  converted to cents when they enter a total.

See [architecture/money-flow.md](../architecture/money-flow.md).
