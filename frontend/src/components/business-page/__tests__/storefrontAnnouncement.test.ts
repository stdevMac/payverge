import {
  addDaysToDateKey,
  announcementStorageKey,
  businessDateKey,
  findUpcomingException,
} from "../storefrontAnnouncement";

describe("storefrontAnnouncement", () => {
  it("computes YYYY-MM-DD in the business timezone", () => {
    const noonUtc = new Date("2026-08-17T16:00:00Z");
    expect(businessDateKey(noonUtc, "America/Argentina/Buenos_Aires")).toBe(
      "2026-08-17",
    );
    expect(businessDateKey(noonUtc, "Pacific/Auckland")).toBe("2026-08-18");
  });

  it("adds days across month boundaries", () => {
    expect(addDaysToDateKey("2026-08-30", 3)).toBe("2026-09-02");
  });

  it("picks the nearest exception inside a 7-day window", () => {
    const now = new Date("2026-08-17T12:00:00Z");
    const picked = findUpcomingException(
      [
        { exception_date: "2026-08-25", label: "too far" },
        { exception_date: "2026-08-20", label: "soon" },
        { exception_date: "2026-08-18", label: "nearest" },
        { exception_date: "2026-08-16", label: "past" },
      ],
      "UTC",
      now,
    );
    expect(picked?.label).toBe("nearest");
  });

  it("returns null when nothing is upcoming", () => {
    expect(
      findUpcomingException(
        [{ exception_date: "2026-09-01", label: "later" }],
        "UTC",
        new Date("2026-08-17T12:00:00Z"),
      ),
    ).toBeNull();
  });

  it("scopes dismissal keys per business + exception", () => {
    expect(
      announcementStorageKey(9, { id: 4, exception_date: "2026-08-18" }),
    ).toBe("storefront-announce-dismissed:9:4");
  });
});
