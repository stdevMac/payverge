/**
 * Branded types for money values. Prevents accidental mixing of dollar
 * floats and integer cents that caused several audit findings during
 * the 2026-04 Bill/Payment/Reservation int64-cents migration.
 *
 * Wire contract (see backend/internal/database/models_json.go):
 *   - Bill/Payment/Withdrawal/PaymentBreakdown/ReservationSettings money
 *     fields arrive as DOLLARS (float) over JSON via MarshalJSON helpers.
 *   - The backend DB stores int64 cents; MarshalJSON does the conversion.
 *   - Stripe-native payloads (e.g. Connect amounts) carry raw CENTS.
 *
 * Use `Dollars` for wire/display values, `Cents` for Stripe-native or
 * explicit-integer amounts. Conversions are explicit via `dollarsToCents` /
 * `centsToDollars` so mistakes surface at compile time.
 *
 * Example:
 *   const tip: Dollars = bill.tip_amount as Dollars;
 *   const tipCents: Cents = dollarsToCents(tip);     // explicit conversion
 *   const bad = tip + 100;                            // ✗ type error
 *   const good = (tip + (100 as Dollars)) as Dollars; // ✓ same brand
 */

declare const DOLLARS_BRAND: unique symbol;
declare const CENTS_BRAND: unique symbol;

export type Dollars = number & { readonly [DOLLARS_BRAND]: "dollars" };
export type Cents = number & { readonly [CENTS_BRAND]: "cents" };

/**
 * Tag a raw number as Dollars. Use at API deserialization boundaries to
 * opt into the brand, e.g. `asDollars(response.data.total_amount)`.
 * A runtime no-op.
 */
export function asDollars(value: number): Dollars {
  return value as Dollars;
}

/**
 * Round a dollar amount to cents precision. Binary float residue from
 * qty×price / rate conversions (e.g. 36 * 4.2 → 151.20000000000002) must
 * not surface on display totals (FIND-043 residual).
 */
export function roundDollars(value: number): Dollars {
  return (Math.round(value * 100) / 100) as Dollars;
}

export function dollarsToCents(d: Dollars): Cents {
  return Math.round((d as number) * 100) as Cents;
}

export function centsToDollars(c: Cents): Dollars {
  return ((c as number) / 100) as Dollars;
}

/**
 * Format a Dollars amount for display: `$45.50`, `$1,234.00`, etc.
 * Defaults to USD / en-US; pass `locale` / `currency` to override.
 */
export function formatDollars(
  d: Dollars,
  opts?: { locale?: string; currency?: string },
): string {
  const locale = opts?.locale ?? "en-US";
  const currency = opts?.currency ?? "USD";
  return new Intl.NumberFormat(locale, {
    style: "currency",
    currency,
  }).format(d as number);
}

/**
 * Sum a list of Dollars. Preserves the brand so callers don't have to
 * re-tag. Returns 0 Dollars for an empty list.
 */
export function sumDollars(values: readonly Dollars[]): Dollars {
  return values.reduce<number>(
    (acc, v) => acc + (v as number),
    0,
  ) as Dollars;
}
