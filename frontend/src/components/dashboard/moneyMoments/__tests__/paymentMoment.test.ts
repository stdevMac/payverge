import { parsePaymentEvent, formatMethodLabel } from "../paymentMoment";

describe("parsePaymentEvent", () => {
  it("parses a valid payment payload (amount in dollars)", () => {
    expect(
      parsePaymentEvent({ bill_id: 7, amount: 12.5, method: "crypto" }),
    ).toEqual({ amount: 12.5, method: "crypto", billId: 7 });
  });

  it("treats a missing bill id as null but still celebrates", () => {
    expect(parsePaymentEvent({ amount: 4, method: "card" })).toEqual({
      amount: 4,
      method: "card",
      billId: null,
    });
  });

  it("rejects non-positive or non-numeric amounts (never celebrate a false positive)", () => {
    expect(parsePaymentEvent({ amount: 0, method: "card" })).toBeNull();
    expect(parsePaymentEvent({ amount: -3, method: "card" })).toBeNull();
    expect(parsePaymentEvent({ method: "card" })).toBeNull();
    expect(parsePaymentEvent({ amount: "12.50", method: "card" })).toBeNull();
    expect(parsePaymentEvent({ amount: Number.NaN, method: "card" })).toBeNull();
    expect(parsePaymentEvent({ amount: Infinity, method: "card" })).toBeNull();
  });

  it("defaults a missing method to an empty string", () => {
    expect(parsePaymentEvent({ amount: 9 })).toEqual({
      amount: 9,
      method: "",
      billId: null,
    });
  });
});

describe("formatMethodLabel", () => {
  const t = (key: string) => key.split(".").pop() as string;

  it("maps known methods to friendly labels", () => {
    expect(formatMethodLabel("crypto", t)).toBe("methodCrypto");
    expect(formatMethodLabel("cross-chain", t)).toBe("methodCrossChain");
    expect(formatMethodLabel("card", t)).toBe("methodCard");
    expect(formatMethodLabel("cash", t)).toBe("methodCash");
  });

  it("title-cases an unknown method as a graceful fallback", () => {
    expect(formatMethodLabel("venmo", t)).toBe("Venmo");
    expect(formatMethodLabel("apple pay", t)).toBe("Apple Pay");
  });

  it("returns an empty string for a blank method", () => {
    expect(formatMethodLabel("", t)).toBe("");
  });
});
