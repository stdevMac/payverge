import { reservationConfirmationHeading } from "./guestReservationCopy";

const copy = {
  requested: "Reservation Requested!",
  confirmed: "Reservation Confirmed",
  waitlist: "Waitlist request received",
};

describe("reservationConfirmationHeading", () => {
  it("follows reservation.status instead of a hardcoded requested title", () => {
    expect(reservationConfirmationHeading("pending", copy)).toBe(
      copy.requested,
    );
    expect(reservationConfirmationHeading("confirmed", copy)).toBe(
      copy.confirmed,
    );
    expect(reservationConfirmationHeading("waitlist", copy)).toBe(
      copy.waitlist,
    );
  });
});
