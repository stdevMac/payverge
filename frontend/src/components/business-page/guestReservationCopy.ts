export type ReservationConfirmationStatus =
  | "pending"
  | "confirmed"
  | "waitlist"
  | null;

export function reservationConfirmationHeading(
  status: ReservationConfirmationStatus,
  copy: { requested: string; confirmed: string; waitlist: string },
): string {
  if (status === "waitlist") return copy.waitlist;
  if (status === "confirmed") return copy.confirmed;
  return copy.requested;
}
