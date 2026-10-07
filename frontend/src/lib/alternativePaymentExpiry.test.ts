import {
  ALTERNATIVE_PAYMENT_REQUEST_MAX_AGE_MS,
  alternativePaymentRequestIsExpired,
  formatAlternativePaymentRequestAge,
} from "./alternativePaymentExpiry";

describe("alternativePaymentRequestIsExpired", () => {
  const now = Date.parse("2026-08-14T15:00:00.000Z");

  it("expires requests older than 24 hours from created_at", () => {
    expect(
      alternativePaymentRequestIsExpired(
        { timestamp: now - ALTERNATIVE_PAYMENT_REQUEST_MAX_AGE_MS - 1 },
        now,
      ),
    ).toBe(true);
  });

  it("keeps requests younger than 24 hours confirmable", () => {
    expect(
      alternativePaymentRequestIsExpired(
        { timestamp: now - ALTERNATIVE_PAYMENT_REQUEST_MAX_AGE_MS + 60_000 },
        now,
      ),
    ).toBe(false);
  });

  it("honors an earlier stored expiresAt", () => {
    expect(
      alternativePaymentRequestIsExpired(
        {
          timestamp: now - 60 * 60 * 1000,
          expiresAt: "2026-08-14T14:50:00.000Z",
        },
        now,
      ),
    ).toBe(true);
  });
});

describe("formatAlternativePaymentRequestAge", () => {
  const now = Date.parse("2026-08-14T15:00:00.000Z");

  it("formats multi-hour age compactly", () => {
    expect(
      formatAlternativePaymentRequestAge(now - 55 * 60 * 60 * 1000, now),
    ).toBe("55h");
  });
});
