import { readFileSync } from "node:fs";
import { join } from "node:path";

import { SplittingAPI } from "@/api/splitting";
import { axiosInstance } from "@/api/tools/instance";
import { asDollars } from "@/types/money";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    get: jest.fn(),
    post: jest.fn(),
  },
}));

describe("splitting api", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("loads split options by guest bill token", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: { success: true, bill: { bill_number: "B42-opaque" }, items: [], split_options: {} },
    });

    await SplittingAPI.getSplitOptions("B42-opaque");

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/guest/bill/B42-opaque/split/options",
    );
  });

  it("posts equal split calculations to the bill-token route and returns the aligned result", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: {
        result: {
          split_method: "equal",
          total_amount: 100,
          tip_amount: 0,
          grand_total: 100,
          created_at: "2026-04-09T12:00:00Z",
          people: [
            {
              person_id: "person_1",
              name: "Alice",
              base_amount: 40,
              tax_amount: 8,
              service_fee_amount: 2,
              tip_amount: 0,
              total_amount: 50,
            },
          ],
        },
      },
    });

    const result = await SplittingAPI.calculateEqualSplit({
      bill_token: "B42-opaque",
      num_people: 2,
      people: { person_1: "Alice", person_2: "Bob" },
    });

    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/guest/bill/B42-opaque/split/equal",
      {
        num_people: 2,
        people: { person_1: "Alice", person_2: "Bob" },
      },
    );
    expect(result.people[0]).toMatchObject({
      person_id: "person_1",
      name: "Alice",
      total_amount: 50,
    });
  });

  it("formats validation requests for the guest bill-token endpoint", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: {
        valid: true,
        result: {
          total_amount: 75,
          grand_total: 75,
        },
      },
    });

    await SplittingAPI.validateSplit({
      bill_token: "B42-opaque",
      split_method: "custom",
      data: {
        bill_token: "B42-opaque",
        amounts: { person_1: asDollars(50), person_2: asDollars(25) },
        people: { person_1: "Alice", person_2: "Bob" },
      },
    });

    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/guest/bill/B42-opaque/split/validate",
      {
        method: "custom",
        amounts: { person_1: asDollars(50), person_2: asDollars(25) },
        people: { person_1: "Alice", person_2: "Bob" },
      },
    );
  });

  it("creates a custom split hold by bill number", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: {
        success: true,
        share: { id: 9, amount: 12.5, amount_cents: 1250, status: "held" },
        state: { bill_number: "B42-opaque", available_amount: 37.5 },
      },
    });

    const result = await SplittingAPI.createSplitHold("B42-opaque", {
      mode: "custom",
      amount: asDollars(12.5),
      display_name: "Sara",
    });

    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/guest/bill/B42-opaque/split/holds",
      {
        mode: "custom",
        amount: "12.50",
        display_name: "Sara",
      },
      expect.objectContaining({
        headers: expect.objectContaining({ "X-Request-Id": expect.any(String) }),
      }),
    );
    expect(result.share.id).toBe(9);
  });

  it("executes a held split share with request idempotency", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: {
        success: true,
        applied: false,
        bill_status: "partial",
        remaining_amount: 25,
        share: { id: 9, status: "settled" },
      },
    });

    await SplittingAPI.executeHeldShare("B42-opaque", {
      share_id: 9,
      payment_method: "crypto",
      transaction_hash: "0xtx",
      payer_address: "0xguest",
      tip_amount: asDollars(1.25),
    });

    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/guest/bill/B42-opaque/split/execute",
      {
        share_id: 9,
        payment_method: "crypto",
        transaction_hash: "0xtx",
        payer_address: "0xguest",
        tip_amount: "1.25",
      },
      expect.objectContaining({
        headers: expect.objectContaining({ "X-Request-Id": expect.any(String) }),
      }),
    );
  });

  it("loads a guest split-share receipt by bill number and share id", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: {
        success: true,
        receipt: {
          share_id: 9,
          bill_number: "B42-opaque",
          status: "settled",
          amount: 3.45,
          amount_cents: 345,
          tip_amount: 1.25,
          tip_cents: 125,
          grand_total: 4.7,
          grand_total_cents: 470,
          items: [{ id: "shared-bottle", name: "Shared Bottle", fraction: "1/3" }],
        },
      },
    });

    const receipt = await SplittingAPI.getSplitShareReceipt("B42-opaque", 9);

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/guest/bill/B42-opaque/split/shares/9/receipt",
    );
    expect(receipt.share_id).toBe(9);
    expect(receipt.grand_total_cents).toBe(470);
  });

  it("loads guest-owned split shares by bill number", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: {
        success: true,
        shares: [
          {
            id: 9,
            mode: "custom",
            amount: 12.5,
            amount_cents: 1250,
            status: "held",
          },
        ],
      },
    });

    const shares = await SplittingAPI.getMySplitShares("B42-opaque");

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/guest/bill/B42-opaque/split/my-shares",
    );
    expect(shares[0]?.id).toBe(9);
  });

  it("releases a held split share by bill number and share id", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: {
        success: true,
        share: {
          id: 9,
          mode: "custom",
          amount: 12.5,
          amount_cents: 1250,
          status: "released",
        },
        state: {
          bill_number: "B42-opaque",
          held_amount: 0,
          held_cents: 0,
          available_amount: 50,
          available_cents: 5000,
          shares: [],
        },
      },
    });

    const response = await SplittingAPI.releaseHeldShare("B42-opaque", 9);

    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/guest/bill/B42-opaque/split/shares/9/release",
    );
    expect(response.share.status).toBe("released");
    expect(response.state.held_cents).toBe(0);
  });
});

describe("split calculator guest identifier contract (issue 893)", () => {
  // Exactly the two identifiers a scanned bill hands the guest: a
  // human-readable display number, and the unguessable capability token.
  // Only the token resolves — every /guest/bill/:bill_token/split/* handler
  // looks the bill up through `bills.public_token = ?`
  // (backend/internal/handlers/splitting.go). A calculator whose request field
  // is spelled `bill_number` invites callers to send the display number, which
  // the route can only answer with 404 Bill not found.
  const guestBill = {
    bill_number: "B141-562fe886-193",
    public_token: "3d0c6d5f6f9c4a3f8b21c0f4b7e9a512",
  };

  beforeEach(() => {
    jest.clearAllMocks();
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: {
        valid: true,
        result: {
          split_method: "equal",
          total_amount: 0,
          tip_amount: 0,
          grand_total: 0,
          created_at: "2026-04-09T12:00:00Z",
          people: [],
        },
      },
    });
  });

  it("routes every calculator through the capability token, never the display bill number", async () => {
    await SplittingAPI.calculateEqualSplit({
      bill_token: guestBill.public_token,
      num_people: 2,
      people: { person_1: "Alice", person_2: "Bob" },
    });
    await SplittingAPI.calculateCustomSplit({
      bill_token: guestBill.public_token,
      amounts: { person_1: asDollars(50) },
      people: { person_1: "Alice" },
    });
    await SplittingAPI.calculateItemSplit({
      bill_token: guestBill.public_token,
      item_selections: { person_1: ["item_1"] },
      people: { person_1: "Alice" },
    });
    await SplittingAPI.validateSplit({
      bill_token: guestBill.public_token,
      split_method: "equal",
      data: {
        bill_token: guestBill.public_token,
        num_people: 2,
        people: { person_1: "Alice", person_2: "Bob" },
      },
    });

    const urls = (axiosInstance.post as jest.Mock).mock.calls.map(
      (call) => call[0] as string,
    );
    expect(urls).toEqual([
      `/guest/bill/${guestBill.public_token}/split/equal`,
      `/guest/bill/${guestBill.public_token}/split/custom`,
      `/guest/bill/${guestBill.public_token}/split/items`,
      `/guest/bill/${guestBill.public_token}/split/validate`,
    ]);
    urls.forEach((url) => {
      expect(url).not.toContain(guestBill.bill_number);
      // A renamed field that the caller never populates would silently
      // produce "/guest/bill/undefined/..." — also a 404.
      expect(url).not.toContain("undefined");
    });
  });

  it("never interpolates a bill number into a /guest/bill/ URL", () => {
    const source = readFileSync(join(__dirname, "splitting.ts"), "utf8");
    const guestBillUrls = source.match(/`\/guest\/bill\/\$\{[^`]*`/g) ?? [];

    expect(guestBillUrls.length).toBeGreaterThan(0);
    guestBillUrls.forEach((url) => {
      expect(url).not.toMatch(/bill_number/);
    });
  });
});
