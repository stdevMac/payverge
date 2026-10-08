/** @jest-environment jsdom */
/**
 * L-5 (2026-06-14 audit): reservation slot timestamps were rendered in the
 * VIEWER's browser timezone instead of the BUSINESS timezone, so a Dubai venue
 * viewed from another timezone showed every slot shifted (e.g. evening hours as
 * pre-dawn). The backend emits correct absolute UTC instants; only the client
 * display conversion was wrong. These tests pin tz-correct grouping/formatting.
 *
 * The assertions are CI-timezone-independent: they feed the SAME absolute
 * instant with TWO different business timezones and require different results.
 * The old viewer-tz code ignores the timezone arg, so both calls returned the
 * same value — failing these tests regardless of the machine's TZ.
 */
import { getServicePeriod, formatTimeSlot } from "../GuestReservationForm";

describe("reservation slot rendering uses the business timezone (L-5)", () => {
  // 2026-06-15T14:00:00Z == 18:00 in Asia/Dubai (+04, Dinner)
  //                      == 07:00 in America/Los_Angeles (-07 PDT, Early)
  const instant = "2026-06-15T14:00:00.000Z";

  it("groups a slot by the business-local hour, not the viewer's", () => {
    expect(getServicePeriod(instant, "Asia/Dubai")).toBe("Dinner");
    expect(getServicePeriod(instant, "America/Los_Angeles")).toBe("Early");
  });

  it("formats the slot time in the business timezone", () => {
    expect(formatTimeSlot(instant, "Asia/Dubai")).not.toBe(
      formatTimeSlot(instant, "America/Los_Angeles"),
    );
    // Dubai is 18:00 -> contains "6" (12h "6:00 PM") or "18" (24h) but never "7".
    expect(formatTimeSlot(instant, "Asia/Dubai")).toMatch(/18:00|6:00/);
  });

  it("falls back to a valid period/label when no business timezone is given", () => {
    expect(["Early", "Lunch", "Dinner", "Late"]).toContain(
      getServicePeriod(instant),
    );
    expect(formatTimeSlot(instant)).toMatch(/\d/);
  });
});
