import { isValidPhone } from "@/lib/fieldValidation";

/**
 * Shared create/edit gate for operator reservation phones (L1-16).
 * Both handleCreateReservation and handleUpdateReservation must use this
 * so garbage like "abc" cannot save on either path.
 */
export type ReservationPhoneGate = "missing" | "invalid" | "ok";

export function reservationPhoneGate(
  phone: string | undefined | null,
): ReservationPhoneGate {
  if (!phone?.trim()) return "missing";
  if (!isValidPhone(phone)) return "invalid";
  return "ok";
}
