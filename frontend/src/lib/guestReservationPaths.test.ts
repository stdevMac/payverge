import {
  guestReservationCancelPath,
  guestReservationDetailsPath,
} from "./guestReservationPaths";

describe("guestReservationPaths", () => {
  it("builds details path with lang", () => {
    expect(guestReservationDetailsPath("ABC123", "es")).toBe(
      "/reservations/ABC123?lang=es",
    );
    expect(guestReservationDetailsPath("ABC123", "ja")).toBe(
      "/reservations/ABC123?lang=ja",
    );
  });

  it("builds cancel path with lang", () => {
    expect(guestReservationCancelPath("ABC123", "fr")).toBe(
      "/reservations/ABC123/cancel?lang=fr",
    );
  });

  it("defaults language to en", () => {
    expect(guestReservationDetailsPath("X")).toBe("/reservations/X?lang=en");
  });
});
