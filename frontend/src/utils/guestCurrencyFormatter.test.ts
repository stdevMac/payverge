import {
  BIDI_FSI,
  BIDI_PDI,
  convertGuestAmount,
  formatConvertedGuestCurrency,
  formatGuestCurrency,
  formatGuestRate,
  normalizeGuestLocale,
} from "@/utils/guestCurrencyFormatter";

/** Strip FSI/PDI isolates so assertions can check the numeric payload. */
function unisolate(s: string): string {
  if (s.startsWith(BIDI_FSI) && s.endsWith(BIDI_PDI)) {
    return s.slice(BIDI_FSI.length, s.length - BIDI_PDI.length);
  }
  return s;
}

describe("formatGuestCurrency", () => {
  it("defaults to en-US formatting when no guest locale is given", () => {
    expect(unisolate(formatGuestCurrency(1234.56, "USD"))).toBe("$1,234.56");
  });

  it("treats undefined / empty locale as en-US", () => {
    expect(unisolate(formatGuestCurrency(1234.56, "USD", undefined))).toBe(
      "$1,234.56",
    );
    expect(unisolate(formatGuestCurrency(1234.56, "USD", ""))).toBe(
      "$1,234.56",
    );
  });

  it("applies the diner's locale to grouping/decimal separators", () => {
    // German swaps the comma/period roles: 1.234,56 €
    const de = unisolate(formatGuestCurrency(1234.56, "EUR", "de"));
    expect(de).toContain("1.234,56");
    expect(de).toContain("€");
  });

  it("honors zero-decimal currencies (JPY) under a non-en locale", () => {
    // 1234.5 rounds to whole yen (1,235) — proving 0 fraction digits.
    expect(unisolate(formatGuestCurrency(1234.5, "JPY", "ja"))).toContain(
      "1,235",
    );
  });

  it("defensively normalizes underscore locale forms (es_AR -> es-AR)", () => {
    // es-AR groups with '.' and uses ',' for decimals, like German.
    expect(unisolate(formatGuestCurrency(1234.56, "USD", "es_AR"))).toContain(
      "1.234,56",
    );
  });

  it("formats USDC with the diner locale decimal/grouping convention", () => {
    // Same separators as the EUR/USD bill totals — never toFixed "1234.50".
    const de = unisolate(formatGuestCurrency(1234.5, "USDC", "de"));
    expect(de).toContain("USDC");
    expect(de).toContain("1.234,50");
    expect(de).not.toMatch(/1234\.50/);
  });

  it("returns a locale-aware bare amount when the currency code is empty", () => {
    expect(unisolate(formatGuestCurrency(1234.56, "", "de"))).toContain(
      "1.234,56",
    );
  });

  it("wraps every amount in FSI/PDI so RTL negatives stay one isolated run (NEW-14)", () => {
    const positive = formatGuestCurrency(68, "USD", "ar");
    const negative = formatGuestCurrency(-10.2, "USD", "ar");

    expect(positive.startsWith(BIDI_FSI)).toBe(true);
    expect(positive.endsWith(BIDI_PDI)).toBe(true);
    expect(negative.startsWith(BIDI_FSI)).toBe(true);
    expect(negative.endsWith(BIDI_PDI)).toBe(true);

    // Inner payload still contains the minus + currency token as one string;
    // the isolate markers prevent surrounding RTL from splitting the run.
    const inner = unisolate(negative);
    expect(inner).toMatch(/-/);
    expect(inner).toMatch(/US\$|\$|USD/);
    // No nested FSI — a single isolate pair wraps the whole amount.
    expect(inner.includes(BIDI_FSI)).toBe(false);
    expect(inner.includes(BIDI_PDI)).toBe(false);

    // LTR locales also get the wrap so call sites can treat output uniformly.
    const enNeg = formatGuestCurrency(-10.2, "USD", "en");
    expect(enNeg).toBe(`${BIDI_FSI}-$${Number(10.2).toFixed(2)}${BIDI_PDI}`);
  });
});

describe("convert then format", () => {
  it("converts default→display then formats in the diner locale", () => {
    const converted = convertGuestAmount(1000, "ARS", "USD", 0.001);
    expect(converted).toEqual({ amount: 1, currency: "USD" });
    expect(
      unisolate(formatConvertedGuestCurrency(1000, "ARS", "USD", "en", 0.001)),
    ).toBe("$1.00");
  });

  it("keeps the source currency when no rate is available", () => {
    expect(convertGuestAmount(1000, "ARS", "USD", null)).toEqual({
      amount: 1000,
      currency: "ARS",
    });
    expect(
      unisolate(formatConvertedGuestCurrency(1000, "ARS", "USD", "en", null)),
    ).toMatch(/ARS|\$/);
  });

  it("does not convert when currencies match", () => {
    expect(convertGuestAmount(12.5, "USD", "USD", 2)).toEqual({
      amount: 12.5,
      currency: "USD",
    });
  });
});

describe("formatGuestRate", () => {
  it("uses comma decimals in de / de-DE / fr, period in en", () => {
    expect(formatGuestRate(8.875, "de")).toBe("8,875");
    expect(formatGuestRate(8.875, "de-DE")).toBe("8,875");
    expect(formatGuestRate(10.5, "fr")).toBe("10,5");
    expect(formatGuestRate(8.875, "en")).toBe("8.875");
    expect(formatGuestRate(8.875, "en")).not.toBe("8.88");
  });

  it("strips trailing zeros and handles non-finite input", () => {
    expect(formatGuestRate(4, "de")).toBe("4");
    expect(formatGuestRate(4.5, "de")).toBe("4,5");
    expect(formatGuestRate(Number.NaN, "de")).toBe("0");
    expect(formatGuestRate(Number.POSITIVE_INFINITY, "es")).toBe("0");
  });
});

describe("normalizeGuestLocale", () => {
  // Exported for guest surfaces that build a raw Intl.NumberFormat (display-rate
  // conversion, currency-less totals) and need the same locale normalization as
  // formatGuestCurrency without going through the currency formatter.
  it("falls back to en-US for empty / whitespace / undefined input", () => {
    expect(normalizeGuestLocale(undefined)).toBe("en-US");
    expect(normalizeGuestLocale("")).toBe("en-US");
    expect(normalizeGuestLocale("   ")).toBe("en-US");
  });

  it("passes through a canonical BCP-47 tag unchanged", () => {
    expect(normalizeGuestLocale("de")).toBe("de");
  });

  it("rewrites underscore locale forms to hyphenated BCP-47", () => {
    expect(normalizeGuestLocale("es_AR")).toBe("es-AR");
    expect(normalizeGuestLocale("zh_Hant_TW")).toBe("zh-Hant-TW");
  });
});
