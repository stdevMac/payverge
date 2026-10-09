import { serviceCallAgeTone } from "./ServiceCallsQueue";

describe("serviceCallAgeTone", () => {
  const now = Date.parse("2026-08-12T00:00:00Z");

  it("stays calm under 5 minutes", () => {
    expect(
      serviceCallAgeTone(new Date(now - 2 * 60_000).toISOString(), now),
    ).toContain("ink");
  });

  it("warns amber from 5 minutes", () => {
    expect(
      serviceCallAgeTone(new Date(now - 6 * 60_000).toISOString(), now),
    ).toContain("amber");
  });

  it("escalates rose from 15 minutes", () => {
    expect(
      serviceCallAgeTone(new Date(now - 16 * 60_000).toISOString(), now),
    ).toContain("rose");
  });
});
