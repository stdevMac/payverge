import type { BriefingPlay, DirectorResponse } from "@/api/directorConsole";

/**
 * #241 — never paint revenue advice that the operator cannot audit.
 *
 * A claim is grounded only when the same text carries a real figure (money or
 * a counted operational unit) AND a date range. "Dinner revenue is tracking
 * above the 30-day median" has a period but no dollars/covers — that is not
 * enough data. We never invent $ figures to fill the gap.
 */

const REVENUE_SUBJECT =
  /\b(revenue|sales|ingresos|ventas)\b/i;

const REVENUE_PERIOD =
  /\b(tonight|today|this week|this month|30-day|median|tracking|esta noche|hoy|esta semana|este mes|mediana|superan)\b/i;

const OPERATIONAL_SUBJECT =
  /\b(mesa|mesas|table|tables|cocina|kitchen|mozo|waiter|caja|cash drawer|cash register|plato|platos|best[- ]?sellers?|más vendid|combo|bundle|offer|promo|margen|margin|slow[- ]?mov|moviendo poco)\b/i;

const MONEY =
  /(?:[$€£¥]|USD|EUR|GBP|ARS|AED|MXN|BRL|CLP|COP|PEN|UYU)\s*[\d.,]+|[\d.,]+\s*(?:[$€£¥]|USD|EUR|GBP|ARS|AED|dollars?|euros?|pesos?)/i;

const COUNTS =
  /\b\d+[.,]?\d*\s*%|\b\d+[.,]?\d*\s*(?:orders?|covers?|bills?|tickets?|checks?|cheques?|pedidos?|cubiertos|cuentas)\b/i;

export function structuredAdviceText(
  structured: Pick<
    DirectorResponse,
    "summary" | "diagnosis" | "evidence" | "expected_impact"
  >,
): string {
  return [
    structured.summary,
    structured.diagnosis,
    ...(structured.evidence || []),
    structured.expected_impact,
  ]
    .filter((part): part is string => typeof part === "string" && part.trim().length > 0)
    .join("\n");
}

/** True when the text cites a money amount or a counted operational figure. */
export function hasGroundedRevenueRange(text: string): boolean {
  const blob = text.trim();
  if (!blob) return false;
  return MONEY.test(blob) || COUNTS.test(blob);
}

export function isRevenueAdvice(text: string): boolean {
  return REVENUE_SUBJECT.test(text) && REVENUE_PERIOD.test(text);
}

/** True when the copy talks about revenue but cannot cite figures + a range. */
export function shouldHideUngroundedRevenueAdvice(text: string): boolean {
  // Best-seller / floor / margin / promo copy is operational, not a revenue
  // briefing. Hiding it behind "not enough data" contradicts sibling threads
  // that already analyzed those dishes.
  if (OPERATIONAL_SUBJECT.test(text)) {
    return false;
  }
  return isRevenueAdvice(text) && !hasGroundedRevenueRange(text);
}

/** A play may render dollar figures only when every cited number is present. */
export function playHasGroundedFigures(play: BriefingPlay): boolean {
  if (!play.item_name.trim()) return false;
  if (!Number.isFinite(play.monthly_impact)) return false;
  if (play.kind === "reprice_up") {
    return (
      play.current_price != null &&
      play.suggested_price != null &&
      Number.isFinite(play.current_price) &&
      Number.isFinite(play.suggested_price)
    );
  }
  return true;
}
