/**
 * Sample-check math for Settings → Payments fee preview (#266).
 *
 * Mirrors backend `money.BillTotalsCents`: tax and service fee are each a
 * percentage of subtotal (half-up to the cent), then added. Inclusive toggles
 * do not change bill math today — they only affect how menu prices are labeled
 * — so the preview always shows additive totals and surfaces the inclusive
 * flags as display notes.
 */

export const SAMPLE_CHECK_SUBTOTAL = 100;

export interface SampleCheckRates {
  tax_rate: number;
  service_fee_rate: number;
  tax_inclusive: boolean;
  service_inclusive: boolean;
}

export interface SampleCheckPreview {
  subtotal: number;
  tax: number;
  serviceFee: number;
  total: number;
  taxInclusive: boolean;
  serviceInclusive: boolean;
}

function percentageMajor(amount: number, ratePercent: number): number {
  if (!Number.isFinite(amount) || amount < 0) return 0;
  if (!Number.isFinite(ratePercent)) return 0;
  const cents = Math.round(amount * 100);
  const basisPoints = Math.round(ratePercent * 100);
  // Integer half-up in cents (matches money.PercentageCents), then to major units.
  const resultCents = Math.trunc((cents * basisPoints + 5000) / 10000);
  return resultCents / 100;
}

export function previewSampleCheck(
  rates: SampleCheckRates,
  subtotal: number = SAMPLE_CHECK_SUBTOTAL,
): SampleCheckPreview {
  const safeSubtotal =
    Number.isFinite(subtotal) && subtotal >= 0 ? subtotal : 0;
  const tax = percentageMajor(safeSubtotal, rates.tax_rate);
  const serviceFee = percentageMajor(safeSubtotal, rates.service_fee_rate);
  return {
    subtotal: safeSubtotal,
    tax,
    serviceFee,
    total: safeSubtotal + tax + serviceFee,
    taxInclusive: Boolean(rates.tax_inclusive),
    serviceInclusive: Boolean(rates.service_inclusive),
  };
}

export function formatSampleCheckMoney(amount: number): string {
  const safe = Number.isFinite(amount) ? amount : 0;
  return `$${safe.toFixed(2)}`;
}
