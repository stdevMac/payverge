import { PaymentMethod } from "@/types/alternativePayments";
import {
  isLatamOperatorVenue,
  recordPaymentMethodI18nSuffix,
  recordPaymentTenderOptions,
} from "./tenderOptions";

describe("recordPaymentTenderOptions", () => {
  it("hides Venmo and labels card as Mercado Pago for es-AR", () => {
    expect(isLatamOperatorVenue("es-AR", "US")).toBe(true);
    const values = recordPaymentTenderOptions("es-AR", "US").map((o) => o.value);
    expect(values).toEqual([
      PaymentMethod.CASH,
      PaymentMethod.CARD,
      PaymentMethod.OTHER,
    ]);
    expect(values).not.toContain(PaymentMethod.VENMO);
    expect(values).not.toContain(PaymentMethod.CRYPTO);
    expect(recordPaymentMethodI18nSuffix(PaymentMethod.CARD, "es-AR", "US")).toBe(
      "mercadopago",
    );
  });

  it("hides Venmo for es-es and es_AR locale tags without a country", () => {
    expect(isLatamOperatorVenue("es-es", null)).toBe(true);
    expect(isLatamOperatorVenue("es_AR", undefined)).toBe(true);
    expect(
      recordPaymentTenderOptions("es_AR").map((o) => o.value),
    ).not.toContain(PaymentMethod.VENMO);
  });

  it("hides Venmo for an AR venue even when the operator chrome is English", () => {
    expect(isLatamOperatorVenue("en", "AR")).toBe(true);
    expect(
      recordPaymentTenderOptions("en", "AR").map((o) => o.value),
    ).not.toContain(PaymentMethod.VENMO);
    expect(recordPaymentMethodI18nSuffix(PaymentMethod.CARD, "en", "AR")).toBe(
      "mercadopago",
    );
  });

  it("keeps Venmo for a US English venue", () => {
    expect(isLatamOperatorVenue("en", "US")).toBe(false);
    const values = recordPaymentTenderOptions("en", "US").map((o) => o.value);
    expect(values).toContain(PaymentMethod.VENMO);
    expect(recordPaymentMethodI18nSuffix(PaymentMethod.CARD, "en", "US")).toBe(
      "card",
    );
  });
});
