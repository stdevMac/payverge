/**
 * L5-14: header pending-invite count must exclude expired rows even when
 * status is still "pending" past expires_at. The list can still show them
 * for resend; the shell header must not.
 */
import { countPendingInvitations } from "../StaffManagement";

const now = new Date("2026-06-01T12:00:00Z");

function invite(
  partial: Partial<{
    status: "pending" | "accepted" | "expired" | "revoked";
    expires_at: string;
  }> = {},
) {
  return {
    status: "pending" as const,
    expires_at: "2026-06-02T12:00:00Z",
    ...partial,
  };
}

describe("countPendingInvitations (L5-14)", () => {
  it("counts status=pending with future expires_at", () => {
    expect(
      countPendingInvitations(
        [invite(), invite({ expires_at: "2026-07-01T00:00:00Z" })],
        now,
      ),
    ).toBe(2);
  });

  it("excludes invites past expires_at even when status is still pending", () => {
    expect(
      countPendingInvitations(
        [
          invite({ expires_at: "2026-05-01T00:00:00Z" }), // expired wall-clock
          invite({ expires_at: "2026-06-15T00:00:00Z" }), // still open
        ],
        now,
      ),
    ).toBe(1);
  });

  it("excludes accepted, expired, and revoked statuses", () => {
    expect(
      countPendingInvitations(
        [
          invite({ status: "accepted" }),
          invite({ status: "expired" }),
          invite({ status: "revoked" }),
          invite({ status: "pending" }),
        ],
        now,
      ),
    ).toBe(1);
  });

  it("treats omitted status as pending when not expired", () => {
    expect(
      countPendingInvitations(
        [{ expires_at: "2026-06-10T00:00:00Z" }],
        now,
      ),
    ).toBe(1);
  });

  it("returns 0 for an empty list", () => {
    expect(countPendingInvitations([], now)).toBe(0);
  });
});
