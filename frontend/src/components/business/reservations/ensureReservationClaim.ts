import {
  getReservationClaimConflict,
  type ReservationClaimConflict,
} from "@/api/reservations";

export type EnsureReservationClaimResult =
  | { ok: true }
  | {
      ok: false;
      reason: "user_cancelled" | "held" | "forbidden" | "failed";
      conflict?: ReservationClaimConflict | null;
    };

type ClaimFn = (options?: { steal?: boolean }) => Promise<unknown>;

/**
 * L4-8: take (or refresh) exclusive operator claim before a mutate.
 * Managers/owners may steal after an explicit confirm when another hold is fresh.
 */
export async function ensureReservationClaim(args: {
  claim: ClaimFn;
  canSteal: boolean;
  confirmSteal: (holderName: string) => boolean | Promise<boolean>;
}): Promise<EnsureReservationClaimResult> {
  try {
    await args.claim();
    return { ok: true };
  } catch (err) {
    const conflict = getReservationClaimConflict(err);
    if (conflict?.code === "claim_steal_required" && args.canSteal) {
      const who = conflict.claimed_by_name || "?";
      if (!(await args.confirmSteal(who))) {
        return { ok: false, reason: "user_cancelled", conflict };
      }
      try {
        await args.claim({ steal: true });
        return { ok: true };
      } catch (retryErr) {
        const retryConflict = getReservationClaimConflict(retryErr);
        return {
          ok: false,
          reason:
            retryConflict?.code === "claim_forbidden" ? "forbidden" : "failed",
          conflict: retryConflict ?? conflict,
        };
      }
    }
    if (conflict?.code === "claim_forbidden") {
      return { ok: false, reason: "forbidden", conflict };
    }
    if (conflict) {
      return { ok: false, reason: "held", conflict };
    }
    return { ok: false, reason: "failed", conflict: null };
  }
}
