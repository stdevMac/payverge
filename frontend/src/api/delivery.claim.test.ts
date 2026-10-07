import { getDeliveryClaimConflict } from "./delivery";

describe("getDeliveryClaimConflict", () => {
  it("returns null for non-409 errors", () => {
    expect(getDeliveryClaimConflict(new Error("nope"))).toBeNull();
    expect(
      getDeliveryClaimConflict({ response: { status: 500, data: {} } }),
    ).toBeNull();
  });

  it("extracts claim_held payload naming the holder", () => {
    const conflict = getDeliveryClaimConflict({
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
    const conflict = getDeliveryClaimConflict({
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
});
