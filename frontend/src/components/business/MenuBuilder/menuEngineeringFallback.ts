/**
 * #834 — Menu Engineering opens on `week`, and a venue whose last recognized
 * payment predates Monday gets an empty panel while its Accounting tab shows
 * five figures of billed income. The panel is not wrong — the ISO week really
 * is empty — but "No recognized sales in this period" next to $18,237.98 reads
 * as a lie, and "try a wider period" makes the operator guess which one.
 *
 * These helpers pick the one wider window worth probing and decide when it is
 * worth probing at all, so the empty state can name the period that HAS the
 * sales and hand over a one-tap switch instead of a shrug.
 */
import type { MenuEngineeringReport } from "@/api/menuEngineering";

export type MePeriod = "day" | "week" | "month";

/**
 * The widest window the endpoint offers. Probing only this one is sufficient:
 * month ⊇ week ⊇ day, so if any preset has recognized sales, month does.
 * That bounds the fallback at exactly one extra request.
 */
export const WIDEST_ME_PERIOD: MePeriod = "month";

/**
 * True when the report is a definite "nothing sold in this window".
 *
 * Keyed on the backend's `has_sales` flag, never on empty dishes — a venue can
 * have zero costed dishes and plenty of sales. `undefined` (an older backend
 * that predates the flag) is deliberately NOT treated as no-sales: an unknown
 * is not evidence, and inventing a fallback offer from one would be the same
 * class of guess this issue is about.
 */
export function reportHasNoSales(
  report: MenuEngineeringReport | null,
): boolean {
  return report != null && report.has_sales === false;
}

/**
 * True when the selected window came back empty and a wider one still exists
 * to check. Already sitting on `month` means there is nothing wider to offer.
 */
export function shouldProbeWiderPeriod(
  report: MenuEngineeringReport | null,
  period: MePeriod,
): boolean {
  return period !== WIDEST_ME_PERIOD && reportHasNoSales(report);
}

/**
 * The period to offer in the empty state, or null when there is nothing
 * honest to offer — the probe failed, or the wider window is just as empty.
 */
export function widerPeriodWithSales(
  probe: MenuEngineeringReport | null,
): MePeriod | null {
  return probe != null && probe.has_sales === true ? WIDEST_ME_PERIOD : null;
}
