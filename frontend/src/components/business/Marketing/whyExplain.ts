/**
 * S2 why-this-post explainability helpers.
 *
 * Prefer structured `why_factors` from the ranking API. Fall back to legacy
 * `why_data` prose only — never invent metrics, rank stories, or parse
 * free-form English into fake factor rows.
 */

import type { CampaignSuggestion, WhyFactor } from "@/api/marketing";

type WhyExplainMode = "factors" | "legacy" | "empty";

interface WhyExplainRow {
  /** Factor key (empty for legacy prose-only row). */
  key: string;
  /** Localized factor label (i18n). */
  label: string;
  /** Operator-facing value from the API (already formatted). */
  value: string;
}

export interface WhyExplainModel {
  mode: WhyExplainMode;
  /** Structured factor rows when mode === "factors". */
  rows: WhyExplainRow[];
  /** Legacy single-string explanation when mode === "legacy" (or empty). */
  legacyText: string;
}

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

const FACTOR_KEY_RE = /^[a-z][a-z0-9_]{0,63}$/;

/** Drop empty / malformed why_factor atoms so the card never renders junk. */
export function normalizeWhyFactors(
  factors: WhyFactor[] | null | undefined,
): WhyFactor[] {
  if (!Array.isArray(factors) || factors.length === 0) return [];
  const out: WhyFactor[] = [];
  const seen = new Set<string>();
  for (const f of factors) {
    if (!f || typeof f !== "object") continue;
    const key = typeof f.key === "string" ? f.key.trim() : "";
    const value = typeof f.value === "string" ? f.value.trim() : "";
    if (!key || !value) continue;
    if (!FACTOR_KEY_RE.test(key)) continue;
    if (seen.has(key)) continue;
    seen.add(key);
    out.push({
      key,
      value,
      ...(typeof f.weight === "number" && Number.isFinite(f.weight)
        ? { weight: f.weight }
        : {}),
    });
  }
  return out;
}

/**
 * Parse a pct_below_mean wire value into a whole percent 0–100.
 * Accepts "38%", "38", "0.38" (fraction of mean), rejecting non-numeric junk.
 *
 * The ranking engine stores a 0–1 fraction of *sales* below the mean of
 * occupied hour-slots — not visit traffic, and not an already-scaled percent.
 * Displaying the raw "100%" next to "Below average traffic" was the audited
 * dishonest pair ("Below average traffic 100%").
 */
function parsePctBelowMean(raw: string): number | null {
  const trimmed = raw.trim();
  if (!trimmed) return null;
  const bare = trimmed.endsWith("%") ? trimmed.slice(0, -1).trim() : trimmed;
  const n = Number(bare);
  if (!Number.isFinite(n) || n < 0) return null;
  // Fraction base (0–1 exclusive of 1? allow 1.0) → whole percent.
  // Values > 1 are already whole percents (e.g. "38" or "38%").
  if (n <= 1) return Math.round(n * 100);
  return Math.round(n);
}

function formatWhyFactorValue(
  key: string,
  value: string,
  t: Translate,
): string {
  // L4-22: BE emits numeric/flag-only values; FE owns unit/word rendering.
  if (key === "pct_below_mean") {
    const pct = parsePctBelowMean(value);
    if (pct == null) return value;
    return t("why.factors.pct_below_mean_value", { pct });
  }
  if (key === "weak_signal") {
    const n = Number(String(value).replace(/[^\d.-]/g, ""));
    if (!Number.isFinite(n)) return value;
    return t("why.factors.weak_signal_value", { count: Math.round(n) });
  }
  if (key === "qty_sold" || key === "marketable_lapsed" || key === "total_lapsed") {
    const n = Number(String(value).replace(/[^\d.-]/g, ""));
    if (!Number.isFinite(n)) return value;
    return t("why.factors.count_value", { count: Math.round(n) });
  }
  if (key === "velocity_gap") {
    const n = Number(String(value).replace(/[^\d.-]/g, ""));
    if (!Number.isFinite(n)) return value;
    return t("why.factors.velocity_gap_value", { count: Math.round(n) });
  }
  if (key === "photo_ready") {
    // Boolean flag ("1" / "true" / non-empty) → localized ready copy.
    const on =
      value === "1" ||
      value.toLowerCase() === "true" ||
      value === "yes";
    return on ? t("why.factors.photo_ready_value") : value;
  }
  if (key === "ending_soon") {
    const n = Number(String(value).replace(/[^\d.-]/g, ""));
    if (!Number.isFinite(n)) return value;
    return t("why.factors.ending_soon_value", { count: Math.round(n) });
  }
  if (key === "discount") {
    // Wire: "12.5%" (percentage) or bare money number (fixed).
    const trimmed = value.trim();
    if (trimmed.endsWith("%")) {
      const bare = trimmed.slice(0, -1).trim();
      const n = Number(bare);
      if (!Number.isFinite(n)) return value;
      return t("why.factors.discount_pct_value", { pct: bare });
    }
    return t("why.factors.discount_fixed_value", { amount: trimmed });
  }
  if (key === "quadrant") {
    // L4-22: BE emits machine tokens (star/plowhorse/puzzle/dog); never show raw English.
    const q = value.trim().toLowerCase();
    const known = ["star", "plowhorse", "puzzle", "dog"] as const;
    if ((known as readonly string[]).includes(q)) {
      const label = t(`why.factors.quadrant_${q}`);
      // If translation missing, fall back to the already-localized quadrant factor label.
      if (label && label !== `why.factors.quadrant_${q}`) return label;
    }
    return value;
  }
  // money keys (margin_per_unit, bundle_price): pass through numeric string.
  return value;
}

/**
 * Build the card explainability model.
 *
 * - factors: non-empty normalized why_factors → labeled rows via i18n
 * - legacy: no factors, but non-empty why_data → prose only
 * - empty: neither (caller should hide the block)
 */
/**
 * A failed preview cannot keep claiming photo_ready. Drop that factor so WHY,
 * the failed-photo badge, Retry, and export eligibility never disagree.
 */
export function reconcileWhyFactorsForPreview(
  factors: WhyFactor[],
  previewState?: string,
): WhyFactor[] {
  if (previewState !== "failed") return factors;
  return factors.filter((factor) => factor.key !== "photo_ready");
}

export function buildWhyExplain(
  suggestion: Pick<CampaignSuggestion, "why_factors" | "why_data">,
  t: Translate,
  previewState?: string,
): WhyExplainModel {
  const factors = reconcileWhyFactorsForPreview(
    normalizeWhyFactors(suggestion.why_factors),
    previewState,
  );
  if (factors.length > 0) {
    return {
      mode: "factors",
      rows: factors.map((f) => ({
        key: f.key,
        label: t(`why.factors.${f.key}`),
        value: formatWhyFactorValue(f.key, f.value, t),
      })),
      legacyText: "",
    };
  }
  const legacy =
    typeof suggestion.why_data === "string" ? suggestion.why_data.trim() : "";
  if (legacy) {
    return { mode: "legacy", rows: [], legacyText: legacy };
  }
  return { mode: "empty", rows: [], legacyText: "" };
}

/** True when the card should paint a why-this-post surface. */
export function hasWhyExplain(model: WhyExplainModel): boolean {
  return model.mode !== "empty";
}
