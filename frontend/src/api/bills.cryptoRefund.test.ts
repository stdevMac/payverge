/**
 * Wave 4 Task 14 — honest crypto refund API client + status helpers.
 */
import {
  isCryptoRefundCompleted,
  requestCryptoRefund,
  submitCryptoRefundTx,
  getPaymentRefundDestination,
  listCryptoRefunds,
  type CryptoRefundStatus,
} from "./bills";

jest.mock("./tools/instance", () => ({
  axiosInstance: {
    get: jest.fn(),
    post: jest.fn(),
  },
}));

import { axiosInstance } from "./tools/instance";

const mockedAxios = axiosInstance as jest.Mocked<typeof axiosInstance>;

describe("isCryptoRefundCompleted", () => {
  const cases: Array<[CryptoRefundStatus, boolean]> = [
    ["requested", false],
    ["approved", false],
    ["awaiting_signature", false],
    ["submitted", false],
    ["confirming", false],
    ["confirmed", true],
    ["failed", false],
    ["rejected", false],
    ["cancelled", false],
  ];
  it.each(cases)("status %s → completed=%s", (status, want) => {
    expect(isCryptoRefundCompleted(status)).toBe(want);
  });
});

describe("crypto refund API", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("requestCryptoRefund posts integer base units and idempotency key", async () => {
    mockedAxios.post.mockResolvedValueOnce({
      data: {
        refund: {
          id: 1,
          status: "requested",
          amount_base_units: 5_000_000,
          honest_lifecycle_label: "Refund requested — awaiting approval",
        },
      },
    });

    const refund = await requestCryptoRefund(
      9,
      {
        bill_id: 3,
        payment_id: 7,
        amount_base_units: 5_000_000,
        reason: "guest complaint",
        idempotency_key: "idem-1",
      },
      { pin: "1234", idempotencyKey: "idem-1" },
    );

    expect(mockedAxios.post).toHaveBeenCalledWith(
      "/inside/businesses/9/crypto-refunds",
      expect.objectContaining({
        amount_base_units: 5_000_000,
        idempotency_key: "idem-1",
      }),
      expect.objectContaining({
        headers: expect.objectContaining({
          "Idempotency-Key": "idem-1",
          "X-Manager-Pin": "1234",
        }),
      }),
    );
    expect(refund.status).toBe("requested");
    expect(isCryptoRefundCompleted(refund.status)).toBe(false);
  });

  it("submitCryptoRefundTx never claims confirmed status from client", async () => {
    mockedAxios.post.mockResolvedValueOnce({
      data: {
        refund: {
          id: 2,
          status: "submitted",
          submitted_tx_hash: "0xabc",
          honest_lifecycle_label: "Transaction submitted — not yet refunded",
        },
      },
    });

    const refund = await submitCryptoRefundTx(
      9,
      2,
      "0xabc",
      { pin: "1234", idempotencyKey: "sub-1" },
    );
    expect(refund.status).toBe("submitted");
    expect(isCryptoRefundCompleted(refund.status)).toBe(false);
  });

  it("getPaymentRefundDestination surfaces manual support when no evidence", async () => {
    mockedAxios.get.mockResolvedValueOnce({
      data: {
        destination: {
          payment_id: 7,
          has_evidence: false,
          manual_support_required: true,
        },
      },
    });
    const dest = await getPaymentRefundDestination(9, 7);
    expect(dest.manual_support_required).toBe(true);
    expect(dest.has_evidence).toBe(false);
  });

  it("listCryptoRefunds scopes to a payment via payment_id when given", async () => {
    mockedAxios.get.mockResolvedValueOnce({ data: { refunds: [] } });
    await listCryptoRefunds(9, 7);
    expect(mockedAxios.get).toHaveBeenCalledWith(
      "/inside/businesses/9/crypto-refunds?payment_id=7",
    );
  });

  it("listCryptoRefunds omits payment_id for the business-wide list", async () => {
    mockedAxios.get.mockResolvedValueOnce({ data: { refunds: [] } });
    await listCryptoRefunds(9);
    expect(mockedAxios.get).toHaveBeenCalledWith(
      "/inside/businesses/9/crypto-refunds",
    );
  });
});
