import { ensureReservationClaim } from "./ensureReservationClaim";

function claimConflict(
  code: string,
  name = "Ana",
): { response: { status: number; data: Record<string, string> } } {
  return {
    response: {
      status: 409,
      data: { code, claimed_by_name: name },
    },
  };
}

describe("ensureReservationClaim (L4-8)", () => {
  it("succeeds when claim is free", async () => {
    const claim = jest.fn().mockResolvedValue({});
    const result = await ensureReservationClaim({
      claim,
      canSteal: false,
      confirmSteal: () => false,
    });
    expect(result).toEqual({ ok: true });
    expect(claim).toHaveBeenCalledWith();
  });

  it("steals after confirm when manager receives claim_steal_required", async () => {
    const claim = jest
      .fn()
      .mockRejectedValueOnce(claimConflict("claim_steal_required", "Bob"))
      .mockResolvedValueOnce({});
    const confirmSteal = jest.fn().mockResolvedValue(true);

    const result = await ensureReservationClaim({
      claim,
      canSteal: true,
      confirmSteal,
    });

    expect(result).toEqual({ ok: true });
    expect(confirmSteal).toHaveBeenCalledWith("Bob");
    expect(claim).toHaveBeenLastCalledWith({ steal: true });
  });

  it("awaits an async confirmSteal before stealing", async () => {
    let resolveConfirm!: (ok: boolean) => void;
    const claim = jest
      .fn()
      .mockRejectedValueOnce(claimConflict("claim_steal_required", "Bob"))
      .mockResolvedValueOnce({});
    const confirmSteal = jest.fn(
      () =>
        new Promise<boolean>((resolve) => {
          resolveConfirm = resolve;
        }),
    );

    const pending = ensureReservationClaim({
      claim,
      canSteal: true,
      confirmSteal,
    });

    await Promise.resolve();
    expect(claim).toHaveBeenCalledTimes(1);

    resolveConfirm(true);
    await expect(pending).resolves.toEqual({ ok: true });
    expect(claim).toHaveBeenLastCalledWith({ steal: true });
  });

  it("aborts when operator declines steal", async () => {
    const claim = jest
      .fn()
      .mockRejectedValue(claimConflict("claim_steal_required", "Bob"));
    const result = await ensureReservationClaim({
      claim,
      canSteal: true,
      confirmSteal: () => false,
    });
    expect(result.ok).toBe(false);
    if (!result.ok) {
      expect(result.reason).toBe("user_cancelled");
    }
    expect(claim).toHaveBeenCalledTimes(1);
  });

  it("returns held when another operator holds and steal is not allowed", async () => {
    const claim = jest
      .fn()
      .mockRejectedValue(claimConflict("claim_held", "Ana"));
    const result = await ensureReservationClaim({
      claim,
      canSteal: false,
      confirmSteal: () => true,
    });
    expect(result.ok).toBe(false);
    if (!result.ok) {
      expect(result.reason).toBe("held");
      expect(result.conflict?.claimed_by_name).toBe("Ana");
    }
  });

  it("fails closed when claim helper is a no-op that never throws on held claims", async () => {
    // Rule 3: if ensureReservationClaim is bypassed (claim always resolves),
    // production mutates would hit claim_held on the next API call — this test
    // locks the success path to a real claim invocation.
    const claim = jest.fn().mockResolvedValue({});
    await ensureReservationClaim({
      claim,
      canSteal: false,
      confirmSteal: () => false,
    });
    expect(claim).toHaveBeenCalledTimes(1);
  });
});
