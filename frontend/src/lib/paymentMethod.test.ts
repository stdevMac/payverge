import {
  PAYMENT_METHODS,
  canonicalizePaymentMethod,
  isPaymentMethod,
  methodFilterOptions,
  parseMethodFilter,
  type PaymentMethod,
} from "./paymentMethod";

describe("PAYMENT_METHODS", () => {
  it("is the six-value canonical set in stable order", () => {
    expect(PAYMENT_METHODS).toEqual([
      "crypto",
      "cross_chain",
      "card",
      "cash",
      "wallet",
      "other",
    ]);
  });
});

describe("canonicalizePaymentMethod", () => {
  it.each<[string, string | undefined, PaymentMethod]>([
    ["crypto", "payments", "crypto"],
    ["", "payments", "crypto"],
    ["usdc_payment", "payments", "crypto"],
    ["cross-chain", "payments", "cross_chain"],
    ["cross_chain", "payments", "cross_chain"],
    ["stripe", "payments", "card"],
    ["mercadopago", "alternative_payments", "card"],
    ["cash", "alternative_payments", "cash"],
    ["card", "alternative_payments", "card"],
    ["venmo", "alternative_payments", "wallet"],
    ["paypal", "payments", "wallet"],
    ["plugin", "payments", "other"],
    ["other", "alternative_payments", "other"],
    ["usd", "payments", "other"],
    ["", "alternative_payments", "other"],
    ["  CRYPTO  ", "payments", "crypto"],
  ])("%s (%s) → %s", (raw, source, want) => {
    expect(canonicalizePaymentMethod(raw, source)).toBe(want);
  });

  it("never leaves the canonical set", () => {
    for (const raw of [
      "stripe",
      "plugin",
      "manual",
      "Online",
      "foo_plugin",
      null,
      undefined,
    ]) {
      expect(isPaymentMethod(canonicalizePaymentMethod(raw as string))).toBe(
        true,
      );
    }
  });
});

describe("methodFilterOptions", () => {
  const labelFor = (m: PaymentMethod | "all") =>
    m === "all" ? "All Methods" : m.toUpperCase();

  it("always includes All Methods and nothing else when available is empty", () => {
    expect(methodFilterOptions([], labelFor)).toEqual([
      { key: "all", label: "All Methods" },
    ]);
    expect(methodFilterOptions(null, labelFor)).toEqual([
      { key: "all", label: "All Methods" },
    ]);
  });

  it("generates options only from methods present for the business+window", () => {
    // Server says only crypto + card exist — cash/wallet/etc must not appear.
    const opts = methodFilterOptions(["card", "crypto"], labelFor);
    expect(opts.map((o) => o.key)).toEqual(["all", "crypto", "card"]);
    expect(opts.map((o) => o.key)).not.toContain("cash");
    expect(opts.map((o) => o.key)).not.toContain("stripe");
    expect(opts.map((o) => o.key)).not.toContain("manual");
    expect(opts.map((o) => o.key)).not.toContain("plugin");
  });

  it("preserves canonical order regardless of server array order", () => {
    const opts = methodFilterOptions(
      ["wallet", "crypto", "other", "cash"],
      labelFor,
    );
    expect(opts.map((o) => o.key)).toEqual([
      "all",
      "crypto",
      "cash",
      "wallet",
      "other",
    ]);
  });

  it("drops unknown server values so a stale key cannot produce 0-of-0", () => {
    const opts = methodFilterOptions(
      ["crypto", "stripe", "Online", "manual"],
      labelFor,
    );
    expect(opts.map((o) => o.key)).toEqual(["all", "crypto"]);
  });
});

describe("parseMethodFilter", () => {
  it("returns null for all / empty / non-canonical keys", () => {
    expect(parseMethodFilter("all")).toBeNull();
    expect(parseMethodFilter("")).toBeNull();
    expect(parseMethodFilter("stripe")).toBeNull();
    expect(parseMethodFilter("manual")).toBeNull();
  });

  it("returns the method for canonical keys", () => {
    expect(parseMethodFilter("crypto")).toBe("crypto");
    expect(parseMethodFilter("wallet")).toBe("wallet");
  });
});
