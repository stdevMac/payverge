import {
  GUEST_TABLE_LANDING_FALLBACK_POLL_MS,
  shouldPollGuestTableLanding,
} from "./guestTableLandingPoll";

describe("guest table landing fallback poll", () => {
  it("falls back on a 30s cadence", () => {
    expect(GUEST_TABLE_LANDING_FALLBACK_POLL_MS).toBe(30_000);
  });

  it("polls only when a table code is set and the stream or the payload is missing", () => {
    expect(
      shouldPollGuestTableLanding({
        tableCode: undefined,
        streamConnected: false,
        hasTableData: false,
      }),
    ).toBe(false);
    expect(
      shouldPollGuestTableLanding({
        tableCode: "",
        streamConnected: false,
        hasTableData: false,
      }),
    ).toBe(false);
    expect(
      shouldPollGuestTableLanding({
        tableCode: "M1",
        streamConnected: true,
        hasTableData: true,
      }),
    ).toBe(false);
    expect(
      shouldPollGuestTableLanding({
        tableCode: "M1",
        streamConnected: false,
        hasTableData: true,
      }),
    ).toBe(true);
    expect(
      shouldPollGuestTableLanding({
        tableCode: "M1",
        streamConnected: true,
        hasTableData: false,
      }),
    ).toBe(true);
    expect(
      shouldPollGuestTableLanding({
        tableCode: "M1",
        streamConnected: false,
        hasTableData: false,
      }),
    ).toBe(true);
  });
});
