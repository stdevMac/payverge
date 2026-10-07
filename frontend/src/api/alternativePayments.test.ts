import {
  asMoneyString,
  getBillPaymentBreakdownInside,
  getPendingAlternativePayments,
  markAlternativePayment,
  rejectPendingAlternativePayment,
  requestAlternativePayment,
} from "@/api/alternativePayments";
import { axiosInstance } from "@/api/tools/instance";
import {
  PaymentMethod,
  amountMicroToDollars,
  dollarsToAmountMicro,
} from "@/types/alternativePayments";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    get: jest.fn(),
    post: jest.fn(),
  },
}));

jest.mock("@/utils/errorLogger", () => ({
  logError: jest.fn(),
}));

describe("alternativePaymentsAPI", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("maps pending payments from backend contract into frontend payment types", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: {
        pending_payments: [
          {
            id: 5,
            bill_id: 9,
            participant_name: "Jane",
            participant_address: "guest",
            amount: 12.5,
            payment_method: "cash",
            status: "pending",
            created_at: "2026-04-03T12:00:00Z",
          },
        ],
      },
    });

    const payments = await getPendingAlternativePayments("9");

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/bills/9/pending-alternative-payments",
    );
    expect(payments).toEqual([
      {
        id: "5",
        billId: "9",
        participantName: "Jane",
        participantAddress: "guest",
        amount: "12500000",
        paymentMethod: PaymentMethod.CASH,
        timestamp: Date.parse("2026-04-03T12:00:00Z"),
        status: "pending",
      },
    ]);
  });

  it("includes request_id on confirmation and normalizes breakdown units", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: {
        message: "Alternative payment marked successfully",
        payment_breakdown: {
          total_amount: 100,
          crypto_paid: 20,
          alternative_paid: 30,
          remaining: 50,
          is_complete: false,
        },
      },
    });

    const response = await markAlternativePayment({
      billId: "9",
      participantAddress: "guest",
      amount: asMoneyString("12.50"),
      paymentMethod: PaymentMethod.CASH,
      businessConfirmation: true,
      requestId: "11",
    });

    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/inside/bills/9/alternative-payment",
      {
        participant_address: "guest",
        amount: "12.50",
        payment_method: "cash",
        business_confirmation: true,
        request_id: 11,
      },
      {
        headers: {
          "Idempotency-Key": expect.stringMatching(/^mark-alt-/),
        },
      },
    );
    expect(response.paymentBreakdown).toEqual({
      totalAmount: "100000000",
      cryptoPaid: "20000000",
      alternativePaid: "30000000",
      remaining: "50000000",
      isComplete: false,
    });
  });

  it("rejects a pending request through the operator resolution route", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: { success: true, status: "rejected" },
    });

    const response = await rejectPendingAlternativePayment(
      "9",
      "11",
      "cash was not received",
    );

    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/inside/bills/9/pending-alternative-payments/11/reject",
      { reason: "cash was not received" },
    );
    expect(response).toEqual({ success: true, status: "rejected" });
  });

  it("uses guest bill-token route for alternative payment requests", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: { request_id: 22 },
    });

    const response = await requestAlternativePayment(
      "B-opaque-42",
      asMoneyString("12.50"),
      PaymentMethod.CASH,
      "Jane",
    );

    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/guest/bill/B-opaque-42/request-alternative-payment",
      {
        amount: "12.50",
        payment_method: "cash",
        participant_name: "Jane",
      },
      {
        headers: {
          "Idempotency-Key": expect.stringMatching(/^guest-alt-/),
        },
      },
    );
    expect(response).toEqual({
      success: true,
      message: "Alternative payment request sent to business owner",
      requestId: "22",
    });
  });

  it("reuses the same idempotency key when a lost response is retried", async () => {
    const lostResponse = new TypeError("network response lost");
    (axiosInstance.post as jest.Mock)
      .mockRejectedValueOnce(lostResponse)
      .mockResolvedValueOnce({ data: { request_id: 22 } });

    await expect(
      requestAlternativePayment(
        "B-retry-safe",
        asMoneyString("12.50"),
        PaymentMethod.CASH,
        "Jane",
      ),
    ).rejects.toBe(lostResponse);
    await requestAlternativePayment(
      "B-retry-safe",
      asMoneyString("12.50"),
      PaymentMethod.CASH,
      "Jane",
    );

    const firstConfig = (axiosInstance.post as jest.Mock).mock.calls[0][2];
    const retryConfig = (axiosInstance.post as jest.Mock).mock.calls[1][2];
    expect(firstConfig.headers["Idempotency-Key"]).toBeTruthy();
    expect(retryConfig.headers["Idempotency-Key"]).toBe(
      firstConfig.headers["Idempotency-Key"],
    );
  });

  it("includes split share id for guest split alternative payment requests", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: { request_id: 23 },
    });

    await requestAlternativePayment(
      "B-opaque-42",
      asMoneyString("12.50"),
      PaymentMethod.CASH,
      "Jane",
      44,
      asMoneyString("1.25"),
    );

    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/guest/bill/B-opaque-42/request-alternative-payment",
      {
        amount: "12.50",
        tip_amount: "1.25",
        payment_method: "cash",
        participant_name: "Jane",
        split_share_id: 44,
      },
      {
        headers: {
          "Idempotency-Key": expect.stringMatching(/^guest-alt-/),
        },
      },
    );
  });

  it("rethrows the original error so response.data.code is preserved", async () => {
    const original = {
      response: {
        status: 409,
        data: { code: "split_share_conflict", error: "Share already claimed" },
      },
    };
    (axiosInstance.post as jest.Mock).mockRejectedValue(original);

    await expect(
      requestAlternativePayment(
        "B-opaque-42",
        asMoneyString("12.50"),
        PaymentMethod.CASH,
        "Jane",
      ),
    ).rejects.toBe(original);
  });

  it("getPendingAlternativePayments throws on read errors", async () => {
    (axiosInstance.get as jest.Mock).mockRejectedValue(new Error("boom"));

    await expect(getPendingAlternativePayments("9")).rejects.toThrow("boom");
  });

  it("getBillPaymentBreakdownInside throws on read errors", async () => {
    (axiosInstance.get as jest.Mock).mockRejectedValue(new Error("boom"));

    await expect(getBillPaymentBreakdownInside("9")).rejects.toThrow("boom");
  });

  it("markAlternativePayment keeps success false envelope behavior on errors", async () => {
    (axiosInstance.post as jest.Mock).mockRejectedValue(new Error("boom"));

    const result = await markAlternativePayment({
      billId: "9",
      participantAddress: "guest",
      amount: asMoneyString("12.50"),
      paymentMethod: PaymentMethod.CASH,
      businessConfirmation: true,
    });

    expect(result.success).toBe(false);
  });

  // D1 / L2-1: production markAlternativePayment (not a pure helper) must mint
  // an attempt-scoped Idempotency-Key and REUSE it on retry so backend
  // key-dedupe engages. A fresh key per click double-credits when the first
  // request landed but its response was lost.
  // Revert-proof: strip Map.get reuse → first/retry keys diverge and this fails.
  // Backend double-POST guards live in alternative_payment_idempotency_test.go.
  it("mints an attempt-scoped operator idempotency key and reuses it on retry", async () => {
    (axiosInstance.post as jest.Mock)
      .mockRejectedValueOnce(new TypeError("network response lost"))
      .mockResolvedValueOnce({ data: { payment_breakdown: {} } });

    const request = {
      billId: "9101",
      amount: asMoneyString("20.00"),
      paymentMethod: PaymentMethod.CASH,
      businessConfirmation: true,
    };
    const first = await markAlternativePayment(request);
    expect(first.success).toBe(false);
    const retry = await markAlternativePayment(request);
    expect(retry.success).toBe(true);

    const firstConfig = (axiosInstance.post as jest.Mock).mock.calls[0][2];
    const retryConfig = (axiosInstance.post as jest.Mock).mock.calls[1][2];
    expect(firstConfig.headers["Idempotency-Key"]).toBeTruthy();
    expect(retryConfig.headers["Idempotency-Key"]).toBe(
      firstConfig.headers["Idempotency-Key"],
    );
  });

  it("retires the operator idempotency key once a submit succeeds", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: { payment_breakdown: {} },
    });

    const request = {
      billId: "9102",
      amount: asMoneyString("20.00"),
      paymentMethod: PaymentMethod.CASH,
      businessConfirmation: true,
    };
    await markAlternativePayment(request);
    await markAlternativePayment(request);

    const firstKey = (axiosInstance.post as jest.Mock).mock.calls[0][2]
      .headers["Idempotency-Key"];
    const secondKey = (axiosInstance.post as jest.Mock).mock.calls[1][2]
      .headers["Idempotency-Key"];
    expect(firstKey).toBeTruthy();
    expect(secondKey).toBeTruthy();
    expect(secondKey).not.toBe(firstKey);
  });

  it("honors a caller-supplied operator idempotency key verbatim", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: { payment_breakdown: {} },
    });

    await markAlternativePayment({
      billId: "9103",
      amount: asMoneyString("20.00"),
      paymentMethod: PaymentMethod.CASH,
      businessConfirmation: true,
      idempotencyKey: "caller-key-1",
    });

    const config = (axiosInstance.post as jest.Mock).mock.calls[0][2];
    expect(config.headers["Idempotency-Key"]).toBe("caller-key-1");
  });

  it("normalizes dollar amounts to a two-decimal wire value and rejects fractional cents", () => {
    expect(asMoneyString(12.5)).toBe("12.50");
    expect(asMoneyString("0.01")).toBe("0.01");
    expect(() => asMoneyString("12.345")).toThrow(
      "Dollar amounts must use cents precision",
    );
  });

  it("round-trips backend dollar amounts through micro storage", () => {
    expect(dollarsToAmountMicro(12.5)).toBe("12500000");
    expect(amountMicroToDollars("12500000")).toBe(12.5);
    expect(dollarsToAmountMicro(100)).toBe("100000000");
    expect(amountMicroToDollars("100000000")).toBe(100);
  });

  it("covers large values and invalid numeric boundaries", () => {
    expect(asMoneyString("12345678901234567890.99")).toBe(
      "12345678901234567890.99",
    );
    expect(() => asMoneyString("12.3x")).toThrow(
      "Dollar amounts must use cents precision",
    );
    expect(() => asMoneyString("NaN")).toThrow(
      "Dollar amounts must use cents precision",
    );
    expect(() => asMoneyString(Number.MAX_SAFE_INTEGER)).toThrow(
      "Dollar amount is too large",
    );
  });
});
