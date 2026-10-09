import { guestPaymentToastMessage } from "./guestPaymentToast";

const t = (key: string) => `L:${key}`;

describe("guestPaymentToastMessage", () => {
  it("maps a coded API error to payment.errors.*", () => {
    const err: any = new Error("English backend");
    err.response = {
      status: 409,
      data: { code: "split_share_conflict", error: "English backend" },
    };
    expect(guestPaymentToastMessage(err, t)).toBe(
      "L:payment.errors.splitShareConflict",
    );
  });

  it("maps network errors to generic", () => {
    const err: any = new Error("Network Error");
    err.code = "ERR_NETWORK";
    // No response → network-like via isApiNetworkError when status/code present
    expect(guestPaymentToastMessage(err, t)).toBe("L:payment.errors.generic");
  });

  it("maps failure results with code without an axios response", () => {
    expect(
      guestPaymentToastMessage(
        { success: false, code: "auth_failed", message: "Bad credentials" },
        t,
      ),
    ).toBe("L:payment.errors.authFailed");
  });

  it("never returns raw English from a codeless failure result", () => {
    const msg = guestPaymentToastMessage(
      { success: false, message: "Table split failed in English" },
      t,
    );
    expect(msg).toBe("L:payment.errors.generic");
    expect(msg).not.toContain("English");
  });
});
