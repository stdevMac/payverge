/**
 * Shared proposal apply/undo error mapping + expiry copy (L4-18 / L4-19).
 */

export type UndoErrorInput = {
  status?: number;
  code?: string;
};

/** Undo window advertised to operators (matches director_action_service). */
export const UNDO_WINDOW_HOURS = 24;

/**
 * Map undo failure status/code → i18n key under proposal.errors.* or errors.*.
 * Accurate 24h window language lives on the keys themselves.
 */
export function mapUndoErrorKey(err: UndoErrorInput): string {
  if (err.code === "already_undone") return "proposal.errors.alreadyUndone";
  if (err.status === 409) return "proposal.errors.undoStale";
  if (err.status === 410) return "proposal.errors.undoExpired";
  return "proposal.errors.undoFailed";
}

/** Format a remaining-time line for proposal.expires_at (ISO). */
export function formatProposalExpiry(
  expiresAt: string | undefined | null,
  nowMs: number,
  t: (key: string, params?: Record<string, string | number>) => string,
): string | null {
  if (!expiresAt) return null;
  const end = Date.parse(expiresAt);
  if (Number.isNaN(end)) return null;
  const remainingMs = end - nowMs;
  if (remainingMs <= 0) {
    return t("proposal.expired");
  }
  const minutes = Math.max(1, Math.round(remainingMs / 60000));
  if (minutes < 60) {
    return t("proposal.expiresInMinutes", { count: minutes });
  }
  const hours = Math.round(minutes / 60);
  return t("proposal.expiresInHours", { count: hours });
}

/**
 * Applied cards may only follow their origin thread (L4-19 / R3-AI-11 fix).
 */
export function filterProposalsForThread<
  T extends { id: string },
>(
  proposals: T[],
  opts: {
    threadId: number | null;
    appliedThreadById: Record<string, number>;
    appliedIds: Set<string>;
  },
): T[] {
  return proposals.filter((p) => {
    if (!opts.appliedIds.has(p.id)) return true; // staged for current answer
    const origin = opts.appliedThreadById[p.id];
    if (origin == null) return false;
    return opts.threadId != null && origin === opts.threadId;
  });
}
