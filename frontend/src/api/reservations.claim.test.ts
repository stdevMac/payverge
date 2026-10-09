import { getReservationClaimConflict } from "./reservations";

describe("getReservationClaimConflict (L4-8)", () => {
  it("returns null for non-409 errors", () => {
    expect(getReservationClaimConflict(new Error("nope"))).toBeNull();
    expect(
      getReservationClaimConflict({ response: { status: 500, data: {} } }),
    ).toBeNull();
  });

  it("extracts claim_held payload naming the holder", () => {
    const conflict = getReservationClaimConflict({
      response: {
        status: 409,
        data: {
          code: "claim_held",
          claimed_by_name: "Ana",
          claimed_by_role: "server",
        },
      },
    });
    expect(conflict).toEqual(
      expect.objectContaining({
        code: "claim_held",
        claimed_by_name: "Ana",
      }),
    );
  });

  it("extracts claim_steal_required for managers", () => {
    const conflict = getReservationClaimConflict({
      response: {
        status: 409,
        data: {
          code: "claim_steal_required",
          claimed_by_name: "Bob",
        },
      },
    });
    expect(conflict?.code).toBe("claim_steal_required");
    expect(conflict?.claimed_by_name).toBe("Bob");
  });

  it("fails when production only surfaces generic 400s without claim codes", () => {
    // Rule 3 shape: if claim conflict helper is removed/broken, 409 claim
    // payloads must not be silently treated as non-conflicts.
    const conflict = getReservationClaimConflict({
      response: {
        status: 409,
        data: { code: "claim_held", claimed_by_name: "Sam" },
      },
    });
    expect(conflict).not.toBeNull();
    expect(conflict?.code).toBe("claim_held");
  });
});
