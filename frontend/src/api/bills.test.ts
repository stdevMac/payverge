import {
  createServiceCall,
  getActiveBillConflictID,
  getServiceCallStatus,
  guestBillRef,
  tryGuestBillRef,
  ServiceCallCooldownError,
  RateLimitCooldownError,
  getAllActiveBills,
  getAllBillsForStatus,
  getBillByNumber,
  getBusinessBills,
  parseBillsResponse,
  getCryptoQuote,
  getOpenBillByTableCode,
  getTableByCode,
  isActiveBillStatus,
  processCrossChainPayment,
  updateBillPayment,
} from "@/api/bills";
import { axiosInstance } from "@/api/tools/instance";
import { asDollars } from "@/types/money";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    get: jest.fn(),
    post: jest.fn(),
    put: jest.fn(),
    patch: jest.fn(),
  },
}));

describe("guestBillRef", () => {
  it("prefers public_token when present", () => {
    expect(
      guestBillRef({
        bill_number: "B7-1720512345",
        public_token: "a1b2c3d4e5f60718293a4b5c6d7e8f90",
      }),
    ).toBe("a1b2c3d4e5f60718293a4b5c6d7e8f90");
  });

  it("fails closed when the guest capability is missing", () => {
    expect(() =>
      guestBillRef({ bill_number: "B7-1720512345", public_token: undefined }),
    ).toThrow("missing public access token");
    expect(() =>
      guestBillRef({ bill_number: "B7-1720512345", public_token: "" }),
    ).toThrow("missing public access token");
  });
});

describe("tryGuestBillRef", () => {
  it("returns the token when present", () => {
    expect(
      tryGuestBillRef({
        bill_number: "B7-1720512345",
        public_token: "a1b2c3d4e5f60718293a4b5c6d7e8f90",
      }),
    ).toBe("a1b2c3d4e5f60718293a4b5c6d7e8f90");
  });

  // PV-LIVE-20260720-001: render paths must not throw when capability is blank.
  it("returns null instead of throwing when the guest capability is missing", () => {
    expect(
      tryGuestBillRef({ bill_number: "B7-1720512345", public_token: undefined }),
    ).toBeNull();
    expect(
      tryGuestBillRef({ bill_number: "B7-1720512345", public_token: "" }),
    ).toBeNull();
    expect(
      tryGuestBillRef({ bill_number: "B7-1720512345", public_token: "   " }),
    ).toBeNull();
  });
});

describe("bills api", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("posts guest crypto payments to the bill-token route", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: { success: true },
    });

    const token = "a1b2c3d4e5f60718293a4b5c6d7e8f90";
    await updateBillPayment(token, {
      transaction_hash: "0xabc",
      amount_paid: asDollars(25),
      tip_amount: asDollars(3),
      payment_method: "crypto",
      blockchain_network: "base",
      quote_token: "tok.existing",
    });

    expect(axiosInstance.post).toHaveBeenCalledWith(
      `/guest/bill/${token}/crypto-payment`,
      {
        transaction_hash: "0xabc",
        amount_paid: 25,
        tip_amount: 3,
        payment_method: "crypto",
        blockchain_network: "base",
        quote_token: "tok.existing",
      },
    );
  });

  it("puts public_token in guest bill path when guestBillRef is used", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: { bill: { bill_number: "B7", public_token: "tok-abc" }, items: [] },
    });
    const ref = guestBillRef({
      bill_number: "B7-1720512345",
      public_token: "tok-abc-public",
    });
    await getBillByNumber(ref);
    expect(axiosInstance.get).toHaveBeenCalledWith("/guest/bill/tok-abc-public");
    expect(ref).toBe("tok-abc-public");
  });

  it("rethrows guest payment errors so response.data.code is preserved", async () => {
    const original = {
      response: {
        data: {
          code: "payment_failed",
          error: "Unable to verify payment transaction",
        },
      },
    };
    (axiosInstance.post as jest.Mock).mockRejectedValue(original);

    await expect(
      processCrossChainPayment("B42-opaque", {
        transaction_hash: "0xabc",
        amount_paid: asDollars(25),
        tip_amount: asDollars(0),
        source_chain: "Ethereum",
        source_token: "USDC",
        quote_token: "tok.existing",
      }),
    ).rejects.toBe(original);
  });

  it("loads all active bill pages until exhausted", async () => {
    (axiosInstance.get as jest.Mock)
      .mockResolvedValueOnce({
        data: {
          bills: [{ id: 1, status: "open" }],
          total: 2,
          page: 1,
          page_size: 1,
          total_pages: 2,
        },
      })
      .mockResolvedValueOnce({
        data: {
          bills: [{ id: 2, status: "open" }],
          total: 2,
          page: 2,
          page_size: 1,
          total_pages: 2,
        },
      });

    const result = await getAllActiveBills(42, { pageSize: 1 });

    expect(result.items.map((bill) => bill.id)).toEqual([1, 2]);
    expect(result.capped).toBe(false);
  });

  it("preserves partial bills when loading all active bill pages", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValueOnce({
      data: {
        bills: [
          { id: 1, status: "partial", paid_amount: 10, total_amount: 40 },
        ],
        total: 1,
        page: 1,
        page_size: 100,
        total_pages: 1,
      },
    });

    const result = await getAllActiveBills(42);

    expect(result.items).toEqual([
      expect.objectContaining({ id: 1, status: "partial" }),
    ]);
  });

  it("treats partial bill status as active", () => {
    expect(isActiveBillStatus("open")).toBe(true);
    expect(isActiveBillStatus("partial")).toBe(true);
    expect(isActiveBillStatus("paid")).toBe(false);
    expect(isActiveBillStatus("closed")).toBe(false);
  });

  it("extracts the active bill id only from a 409 occupancy response", () => {
    expect(
      getActiveBillConflictID({
        response: {
          status: 409,
          data: { code: "counter_occupied", active_bill_id: 73 },
        },
      }),
    ).toBe(73);
    expect(
      getActiveBillConflictID({
        response: { status: 500, data: { active_bill_id: 73 } },
      }),
    ).toBeNull();
    expect(
      getActiveBillConflictID({
        response: { status: 409, data: { active_bill_id: "73" } },
      }),
    ).toBeNull();
  });

  it("loads all bill pages for a status filter with exact page urls", async () => {
    (axiosInstance.get as jest.Mock)
      .mockResolvedValueOnce({
        data: {
          bills: [{ id: 1, status: "paid" }],
          total: 2,
          page: 1,
          page_size: 1,
          total_pages: 2,
        },
      })
      .mockResolvedValueOnce({
        data: {
          bills: [{ id: 2, status: "closed" }],
          total: 2,
          page: 2,
          page_size: 1,
          total_pages: 2,
        },
      });

    const result = await getAllBillsForStatus(42, "paid,closed", {
      pageSize: 1,
    });

    expect(result.items.map((bill) => bill.id)).toEqual([1, 2]);
    expect(result.capped).toBe(false);
    expect(axiosInstance.get).toHaveBeenNthCalledWith(
      1,
      "/inside/businesses/42/bills?page=1&page_size=1&status=paid%2Cclosed",
    );
    expect(axiosInstance.get).toHaveBeenNthCalledWith(
      2,
      "/inside/businesses/42/bills?page=2&page_size=1&status=paid%2Cclosed",
    );
    expect(axiosInstance.get).toHaveBeenCalledTimes(2);
  });

  it("passes server-side bill search and date filters", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValueOnce({
      data: {
        bills: [],
        total: 0,
        page: 1,
        page_size: 25,
        total_pages: 1,
      },
    });

    await getBusinessBills(42, {
      page: 2,
      pageSize: 25,
      status: "paid,closed",
      search: "table 9",
      dateFrom: "2026-05-11",
      dateTo: "2026-05-12",
    });

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/bills?page=2&page_size=25&status=paid%2Cclosed&search=table+9&date_from=2026-05-11&date_to=2026-05-12",
    );
  });

  it("rejects a 200 bills payload that omits the bills array (#647)", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValueOnce({
      data: { total: 0 },
    });

    await expect(getBusinessBills(42)).rejects.toThrow(
      "invalid bills list payload",
    );
    expect(() => parseBillsResponse({ total: 0 })).toThrow(
      "invalid bills list payload",
    );
    expect(() => parseBillsResponse(null)).toThrow("invalid bills list payload");
    expect(parseBillsResponse({ bills: [], total: 0 })).toEqual({
      bills: [],
      total: 0,
    });
  });

  it("uses default pagination and caps all status bill fetches", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: {
        bills: [{ id: 1, status: "paid" }],
        total: 11,
        page: 1,
        page_size: 100,
        total_pages: 11,
      },
    });

    const result = await getAllBillsForStatus(42, "paid", { maxPages: 1 });

    expect(result.items.map((bill) => bill.id)).toEqual([1]);
    expect(result.capped).toBe(true);
    expect(result.warning).toBe("live_data_cap_reached");
    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/bills?page=1&page_size=100&status=paid",
    );
  });
});

describe("getOpenBillByTableCode", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("returns null bill on 200-with-null AND on legacy 404", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValueOnce({
      data: { bill: null, items: [] },
    });
    await expect(getOpenBillByTableCode("t1")).resolves.toEqual({
      bill: null,
      items: [],
    });

    (axiosInstance.get as jest.Mock).mockRejectedValueOnce({
      response: { status: 404 },
    });
    await expect(getOpenBillByTableCode("t1")).resolves.toEqual({
      bill: null,
      items: [],
    });
  });

  it("rethrows non-404 errors", async () => {
    const boom = { response: { status: 500 } };
    (axiosInstance.get as jest.Mock).mockRejectedValueOnce(boom);
    await expect(getOpenBillByTableCode("t1")).rejects.toBe(boom);
  });

  it("throws RateLimitCooldownError on 429 with retry_after (NEW-4)", async () => {
    (axiosInstance.get as jest.Mock).mockRejectedValue({
      response: {
        status: 429,
        data: { error: "Rate limit exceeded", retry_after: 12 },
      },
    });
    const err = await getOpenBillByTableCode("t1").catch((e) => e);
    expect(err).toBeInstanceOf(RateLimitCooldownError);
    expect(err).toMatchObject({
      name: "RateLimitCooldownError",
      retryAfterSeconds: 12,
    });
  });

  it("falls back to Retry-After header when body has no retry field (REV-4)", async () => {
    (axiosInstance.get as jest.Mock).mockRejectedValue({
      response: {
        status: 429,
        data: { error: "Too Many Requests" },
        headers: { "retry-after": "18" },
      },
    });
    const err = await getOpenBillByTableCode("t1").catch((e) => e);
    expect(err).toBeInstanceOf(RateLimitCooldownError);
    expect(err).toMatchObject({
      name: "RateLimitCooldownError",
      retryAfterSeconds: 18,
    });
  });
});

describe("getCryptoQuote", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("posts the local amount and returns the locked quote", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: {
        quote_id: 77,
        // Exact amount: 108 USD plus the server-chosen sub-cent binding offset.
        usd_microunits: 108_004_213,
        usd_amount: 108.004213,
        rate: 1.08,
        settlement_address: "0x3333333333333333333333333333333333333333",
        chain_id: 8453,
        token: "USDC",
        expires_at: 1_700_000_000,
        quote_token: "tok.abc",
      },
    });

    const quote = await getCryptoQuote("B42", {
      amount_paid: 100,
      tip_amount: 0,
      payment_method: "usdc_payment",
    });

    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/guest/bill/B42/crypto-quote",
      {
        amount_paid: 100,
        tip_amount: 0,
        payment_method: "usdc_payment",
      },
    );
    expect(quote.quote_id).toBe(77);
    // The exact amount is passed through untouched (no rounding to cents).
    expect(quote.usd_microunits).toBe(108_004_213);
    expect(quote.quote_token).toBe("tok.abc");
  });

  it("rethrows the original error so response.data.code is preserved", async () => {
    const original = {
      response: {
        data: { code: "crypto_quote_expired", error: "Bill not found" },
      },
    };
    (axiosInstance.post as jest.Mock).mockRejectedValue(original);

    await expect(
      getCryptoQuote("B42", {
        amount_paid: 100,
        tip_amount: 0,
        payment_method: "usdc_payment",
      }),
    ).rejects.toBe(original);
  });
});

describe("updateBillPayment quote_token", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("forwards quote_token to the backend", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: { success: true },
    });

    await updateBillPayment("B42", {
      transaction_hash: "0xabc",
      amount_paid: asDollars(100),
      tip_amount: asDollars(0),
      payment_method: "crypto",
      quote_token: "tok.abc",
    });

    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/guest/bill/B42/crypto-payment",
      expect.objectContaining({ quote_token: "tok.abc" }),
    );
  });
});

describe("guest service call api", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("posts the reason and returns the call status", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: { status: "open" },
    });

    await expect(createServiceCall("T42", "water")).resolves.toBe("open");
    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/guest/table/T42/service-call",
      { reason: "water" },
    );
  });

  it("throws a typed cooldown error on 429 with the retry window", async () => {
    (axiosInstance.post as jest.Mock).mockRejectedValue({
      response: {
        status: 429,
        data: { error: "cooling down", retry_after_seconds: 45 },
      },
    });

    await expect(createServiceCall("T42", "check")).rejects.toMatchObject({
      name: "ServiceCallCooldownError",
      retryAfterSeconds: 45,
    });
    await expect(
      createServiceCall("T42", "check").catch(
        (e) => e instanceof ServiceCallCooldownError,
      ),
    ).resolves.toBe(true);
  });

  it("rethrows non-429 errors untouched", async () => {
    const boom = {
      response: { status: 404, data: { error: "Table not found" } },
    };
    (axiosInstance.post as jest.Mock).mockRejectedValue(boom);

    await expect(createServiceCall("nope", "order")).rejects.toBe(boom);
  });

  it("polls status with cache disabled", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: { status: "acknowledged", reason: "check" },
    });

    await expect(getServiceCallStatus("T42")).resolves.toEqual({
      status: "acknowledged",
      reason: "check",
    });
    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/guest/table/T42/service-call",
      expect.objectContaining({
        _useCache: false,
        params: expect.objectContaining({ _t: expect.any(Number) }),
      }),
    );
  });

  it("drops a non-enum reason from the status poll", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: { status: "open", reason: "<script>" },
    });

    await expect(getServiceCallStatus("T42")).resolves.toEqual({
      status: "open",
      reason: null,
    });
  });
});

describe("getTableByCode language", () => {
  beforeEach(() => {
    (axiosInstance.get as jest.Mock).mockReset();
  });

  it("forwards the guest language so catalog names are translated", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: { table: { table_code: "T1" }, categories: [] },
    });
    await getTableByCode("T1", "es");
    expect(axiosInstance.get).toHaveBeenCalledWith("/guest/table/T1", {
      params: { language: "es" },
    });
  });
});
