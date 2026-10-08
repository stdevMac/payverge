import en from "@/i18n/messages/en/deliverySettings.json";
import es from "@/i18n/messages/es/deliverySettings.json";

describe("delivery payment mode is method copy, not settlement outcome", () => {
  it("does not call an online method Paid", () => {
    expect(en.dispatch.detail.paymentMode.online).not.toMatch(/paid/i);
    expect(es.dispatch.detail.paymentMode.online).not.toMatch(/pagado/i);
  });
});
