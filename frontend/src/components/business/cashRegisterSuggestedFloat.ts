import type { CashRegisterSession } from "@/api/cashRegister";
import { asDollars, type Dollars } from "@/types/money";

/**
 * #652: next shift inherits the last *declared* opening bank.
 * Counted cash / expected / variance already include that close's over/short
 * and must never become the suggested float.
 */
export function suggestedOpeningFloatFromDeclaredBank(
  declared: unknown,
): Dollars | null {
  // Number(null) === 0; a missing /current field must not become a $0 bank.
  if (declared == null || declared === "") return null;
  const opening = Number(declared);
  if (!Number.isFinite(opening) || opening < 0) return null;
  return asDollars(opening);
}

export function suggestedOpeningFloatFromClosedSession(
  session:
    | Pick<CashRegisterSession, "opening_float" | "status">
    | null
    | undefined,
): Dollars | null {
  if (!session || session.status !== "closed") return null;
  return suggestedOpeningFloatFromDeclaredBank(session.opening_float);
}
