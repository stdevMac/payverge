import {
  GUEST_BILL_POLL_DEFAULT_MS,
  GUEST_BILL_POLL_FAST_MS,
  GUEST_BILL_POLL_SSE_BACKSTOP_MS,
  guestBillPollInterval,
} from "./guestBillPollInterval";

describe("guestBillPollInterval", () => {
  it("defaults to 10s", () => {
    expect(GUEST_BILL_POLL_DEFAULT_MS).toBe(10_000);
    expect(guestBillPollInterval({})).toBe(GUEST_BILL_POLL_DEFAULT_MS);
    expect(guestBillPollInterval({ isActive: true })).toBe(
      GUEST_BILL_POLL_DEFAULT_MS,
    );
  });

  it("keeps an inactive bill at 10s when split is open", () => {
    expect(
      guestBillPollInterval({ isActive: false, splitPanelOpen: true }),
    ).toBe(GUEST_BILL_POLL_DEFAULT_MS);
  });

  it("uses 3s when split is open on an active bill", () => {
    expect(GUEST_BILL_POLL_FAST_MS).toBe(3_000);
    expect(guestBillPollInterval({ splitPanelOpen: true })).toBe(
      GUEST_BILL_POLL_FAST_MS,
    );
    expect(
      guestBillPollInterval({ isActive: true, splitPanelOpen: true }),
    ).toBe(GUEST_BILL_POLL_FAST_MS);
  });

  it("uses 3s when payment is in flight", () => {
    expect(guestBillPollInterval({ paymentInFlight: true })).toBe(
      GUEST_BILL_POLL_FAST_MS,
    );
  });

  it("backs off to 30s when the live stream is connected and the bill is quiet", () => {
    expect(GUEST_BILL_POLL_SSE_BACKSTOP_MS).toBe(30_000);
    expect(guestBillPollInterval({ sseConnected: true })).toBe(
      GUEST_BILL_POLL_SSE_BACKSTOP_MS,
    );
    expect(
      guestBillPollInterval({ isActive: true, sseConnected: true }),
    ).toBe(GUEST_BILL_POLL_SSE_BACKSTOP_MS);
  });

  it("stays at 3s when the stream is connected but payment is in flight", () => {
    expect(
      guestBillPollInterval({
        sseConnected: true,
        paymentInFlight: true,
      }),
    ).toBe(GUEST_BILL_POLL_FAST_MS);
  });
});
