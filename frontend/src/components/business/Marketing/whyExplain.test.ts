import {
  buildWhyExplain,
  hasWhyExplain,
  normalizeWhyFactors,
  reconcileWhyFactorsForPreview,
} from "./whyExplain";
import type { WhyFactor } from "@/api/marketing";

const t = (key: string) => key;

describe("normalizeWhyFactors", () => {
  it("keeps valid unique factors", () => {
    const input: WhyFactor[] = [
      { key: "qty_sold", value: "18 orders", weight: 18 },
      { key: "photo_ready", value: "Has a photo ready to post" },
    ];
    expect(normalizeWhyFactors(input)).toEqual(input);
  });

  it("drops empty keys/values, bad keys, and duplicates", () => {
    expect(
      normalizeWhyFactors([
        { key: "", value: "x" },
        { key: "qty_sold", value: "  " },
        { key: "Bad-Key", value: "nope" },
        { key: "qty_sold", value: "first" },
        { key: "qty_sold", value: "second" },
        { key: "margin_per_unit", value: " 4.50 " },
        null as unknown as WhyFactor,
      ]),
    ).toEqual([
      { key: "qty_sold", value: "first" },
      { key: "margin_per_unit", value: "4.50" },
    ]);
  });

  it("returns empty for nullish / non-array", () => {
    expect(normalizeWhyFactors(undefined)).toEqual([]);
    expect(normalizeWhyFactors(null)).toEqual([]);
    expect(normalizeWhyFactors([] as WhyFactor[])).toEqual([]);
  });
});

describe("buildWhyExplain", () => {
  it("prefers structured why_factors over why_data", () => {
    const model = buildWhyExplain(
      {
        why_data: "Legacy prose that must not be shown as primary",
        why_factors: [
          { key: "qty_sold", value: "18" },
          { key: "margin_per_unit", value: "6.5" },
        ],
      },
      t,
    );
    expect(model.mode).toBe("factors");
    expect(model.rows).toEqual([
      {
        key: "qty_sold",
        label: "why.factors.qty_sold",
        // L4-22: FE formats count units via i18n (mock returns key).
        value: "why.factors.count_value",
      },
      {
        key: "margin_per_unit",
        label: "why.factors.margin_per_unit",
        value: "6.5",
      },
    ]);
    expect(model.legacyText).toBe("");
    expect(hasWhyExplain(model)).toBe(true);
  });

  it("falls back to why_data when factors are missing (no invented metrics)", () => {
    const model = buildWhyExplain(
      { why_data: "  Top seller this month  ", why_factors: undefined },
      t,
    );
    expect(model.mode).toBe("legacy");
    expect(model.rows).toEqual([]);
    expect(model.legacyText).toBe("Top seller this month");
    expect(hasWhyExplain(model)).toBe(true);
  });

  it("falls back to why_data when factors are all invalid", () => {
    const model = buildWhyExplain(
      {
        why_data: "Only legacy left",
        why_factors: [{ key: "", value: "" }],
      },
      t,
    );
    expect(model.mode).toBe("legacy");
    expect(model.legacyText).toBe("Only legacy left");
  });

  it("returns empty when neither factors nor why_data exist", () => {
    const model = buildWhyExplain({ why_data: "   ", why_factors: [] }, t);
    expect(model.mode).toBe("empty");
    expect(hasWhyExplain(model)).toBe(false);
  });

  it("does not invent rows from rank/metrics when API omits explainability", () => {
    // Callers may pass a full suggestion; only why_factors / why_data are read.
    const model = buildWhyExplain(
      {
        why_data: "",
        why_factors: undefined,
      },
      t,
    );
    expect(model).toEqual({ mode: "empty", rows: [], legacyText: "" });
  });

  it("drops non-finite weights but keeps the factor", () => {
    expect(
      normalizeWhyFactors([
        { key: "photo_ready", value: "yes", weight: Number.NaN },
        { key: "margin", value: "ok", weight: Infinity },
        { key: "qty_sold", value: "3", weight: 3 },
      ]),
    ).toEqual([
      { key: "photo_ready", value: "yes" },
      { key: "margin", value: "ok" },
      { key: "qty_sold", value: "3", weight: 3 },
    ]);
  });

  it("rejects free-form / spaced keys (not valid factor ids)", () => {
    expect(
      normalizeWhyFactors([
        { key: "Best time to post", value: "Friday" },
        { key: "1starts_with_digit", value: "no" },
        { key: "has-dash", value: "no" },
        { key: "photo_ready", value: "yes" },
      ]),
    ).toEqual([{ key: "photo_ready", value: "yes" }]);
  });
});

describe("L4-22 FE formats boolean/discount/ending_soon wire values", () => {
  const tParams = (
    key: string,
    params?: Record<string, string | number>,
  ) => {
    if (key === "why.factors.photo_ready_value")
      return "Has a photo ready to post";
    if (key === "why.factors.ending_soon_value")
      return `Ends within ${params?.count} days`;
    if (key === "why.factors.discount_pct_value")
      return `${params?.pct}% off`;
    if (key === "why.factors.discount_fixed_value")
      return `${params?.amount} off`;
    if (key === "why.factors.weak_signal_value")
      return `Only ${params?.count} orders — treat this as a hint, not a trend`;
    return key;
  };

  it("localizes photo_ready / ending_soon / discount from numeric wire", () => {
    const model = buildWhyExplain(
      {
        why_data: "",
        why_factors: [
          { key: "photo_ready", value: "1" },
          { key: "ending_soon", value: "7" },
          { key: "discount", value: "15%" },
        ],
      },
      tParams,
    );
    expect(model.rows.map((r) => r.value)).toEqual([
      "Has a photo ready to post",
      "Ends within 7 days",
      "15% off",
    ]);
  });

  it("drops photo_ready when the live preview failed so WHY cannot disagree", () => {
    const factors = [
      { key: "bundle_price", value: "68" },
      { key: "photo_ready", value: "1" },
    ];
    expect(reconcileWhyFactorsForPreview(factors, "failed")).toEqual([
      { key: "bundle_price", value: "68" },
    ]);
    expect(reconcileWhyFactorsForPreview(factors, "ready")).toEqual(factors);
    const model = buildWhyExplain(
      { why_data: "", why_factors: factors },
      tParams,
      "failed",
    );
    expect(model.rows.map((row) => row.key)).toEqual(["bundle_price"]);
  });

  it("labels a thin move-item sample as a weak signal", () => {
    const model = buildWhyExplain(
      {
        why_data: "",
        why_factors: [{ key: "weak_signal", value: "4" }],
      },
      tParams,
    );
    expect(model.rows).toEqual([
      {
        key: "weak_signal",
        label: "why.factors.weak_signal",
        value:
          "Only 4 orders — treat this as a hint, not a trend",
      },
    ]);
  });
});

describe("pct_below_mean honesty", () => {
  const tParams = (
    key: string,
    params?: Record<string, string | number>,
  ) => {
    if (key === "why.factors.pct_below_mean") return "Sales vs average";
    if (key === "why.factors.pct_below_mean_value")
      return `${params?.pct}% below average sales`;
    return key;
  };

  it("never surfaces the audited 'Below average traffic 100%' label pair", () => {
    const model = buildWhyExplain(
      {
        why_data: "",
        why_factors: [
          { key: "weakest_window", value: "Sat 8–9am" },
          { key: "pct_below_mean", value: "100%" },
        ],
      },
      tParams,
    );
    expect(model.mode).toBe("factors");
    const pctRow = model.rows.find((r) => r.key === "pct_below_mean");
    expect(pctRow).toBeDefined();
    expect(pctRow!.label).toBe("Sales vs average");
    expect(pctRow!.value).toBe("100% below average sales");
    const painted = `${pctRow!.label} ${pctRow!.value}`;
    expect(painted).not.toMatch(/Below average traffic/i);
    expect(painted).not.toBe("Below average traffic 100%");
  });

  it("recomputes fraction values against a 0–1 base into whole percents", () => {
    const model = buildWhyExplain(
      {
        why_data: "",
        why_factors: [{ key: "pct_below_mean", value: "0.38" }],
      },
      tParams,
    );
    expect(model.rows[0].value).toBe("38% below average sales");
  });
});

describe("L4-22 quadrant factor values localize", () => {
  it("maps machine quadrant tokens to i18n labels", () => {
    const t = (key: string) => {
      if (key === "why.factors.quadrant") return "Menu role";
      if (key === "why.factors.quadrant_star") return "Estrella";
      return key;
    };
    const model = buildWhyExplain(
      {
        why_data: "",
        why_factors: [{ key: "quadrant", value: "star" }],
      },
      t,
    );
    expect(model.rows[0]?.value).toBe("Estrella");
    expect(model.rows[0]?.value).not.toBe("star");
  });
});
