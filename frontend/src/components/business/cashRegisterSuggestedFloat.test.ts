import { asDollars } from "@/types/money";
import type { CashRegisterSession } from "@/api/cashRegister";
import {
  suggestedOpeningFloatFromClosedSession,
  suggestedOpeningFloatFromDeclaredBank,
} from "./cashRegisterSuggestedFloat";

function closedShortageSession(
  overrides: Partial<CashRegisterSession> = {},
): CashRegisterSession {
  return {
    id: 254,
    business_id: 1,
    status: "closed",
    opening_float: asDollars(200),
    opening_note: "declared till float",
    opened_by_user_id: 1,
    opened_by_staff_id: null,
    opened_by_label: "manager",
    opened_at: "2026-08-18T10:00:00Z",
    cash_sales: asDollars(1274.9),
    cash_refunds: asDollars(0),
    cash_in: asDollars(0),
    cash_out: asDollars(0),
    expected_cash: asDollars(1474.9),
    counted_cash: asDollars(1473.07),
    variance: asDollars(-1.83),
    closing_note: "Falta 1,83",
    closed_by_user_id: 1,
    closed_by_staff_id: null,
    closed_by_label: "manager",
    closed_at: "2026-08-18T22:00:00Z",
    created_at: "2026-08-18T10:00:00Z",
    updated_at: "2026-08-18T22:00:00Z",
    ...overrides,
  };
}

describe("suggestedOpeningFloatFromDeclaredBank", () => {
  it("returns the declared starting bank", () => {
    expect(suggestedOpeningFloatFromDeclaredBank(200)).toBe(200);
    expect(suggestedOpeningFloatFromDeclaredBank(0)).toBe(0);
  });

  it("rejects counted-cash shaped garbage", () => {
    expect(suggestedOpeningFloatFromDeclaredBank(undefined)).toBeNull();
    expect(suggestedOpeningFloatFromDeclaredBank(null)).toBeNull();
    expect(suggestedOpeningFloatFromDeclaredBank("")).toBeNull();
    expect(suggestedOpeningFloatFromDeclaredBank(Number.NaN)).toBeNull();
    expect(suggestedOpeningFloatFromDeclaredBank(-1)).toBeNull();
  });
});

describe("suggestedOpeningFloatFromClosedSession", () => {
  it("uses opening_float, never counted cash that includes a shortage", () => {
    const session = closedShortageSession();
    expect(suggestedOpeningFloatFromClosedSession(session)).toBe(200);
    expect(suggestedOpeningFloatFromClosedSession(session)).not.toBe(1473.07);
    expect(suggestedOpeningFloatFromClosedSession(session)).not.toBe(1474.9);
  });

  it("ignores open sessions and missing rows", () => {
    expect(
      suggestedOpeningFloatFromClosedSession(
        closedShortageSession({ status: "open", closed_at: null }),
      ),
    ).toBeNull();
    expect(suggestedOpeningFloatFromClosedSession(null)).toBeNull();
    expect(suggestedOpeningFloatFromClosedSession(undefined)).toBeNull();
  });
});
