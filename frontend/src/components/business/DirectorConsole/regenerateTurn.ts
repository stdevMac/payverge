/**
 * L4-15 — Regenerate must not append a duplicate user turn.
 * Pure helpers for which transcript rows survive a regenerate start.
 */

type TranscriptRole = "user" | "assistant" | "system";

export interface TranscriptLike {
  id: number;
  role: TranscriptRole;
}

/** Drop the assistant message being regenerated from the local list. */
export function messagesAfterRegenerateStart<T extends TranscriptLike>(
  messages: T[],
  assistantMessageId: number,
): T[] {
  return messages.filter((m) => m.id !== assistantMessageId);
}

/**
 * Only the trailing assistant turn may be regenerated: the server replaces the
 * LAST assistant row(s), so regenerating any earlier turn would hard-delete an
 * unrelated answer and append a misplaced reply. Used both to gate the
 * affordance (render) and to refuse stray calls (handler) — defense in depth.
 */
export function isTrailingAssistant<T extends TranscriptLike>(
  messages: T[],
  assistantMessageId: number,
): boolean {
  if (messages.length === 0) return false;
  const last = messages[messages.length - 1];
  return last.role === "assistant" && last.id === assistantMessageId;
}

/**
 * Whether the optimistic pending user bubble should render.
 * Regenerate reuses the prior user turn — hide the duplicate.
 */
export function shouldShowPendingUserBubble(opts: {
  regenerate?: boolean;
}): boolean {
  return !opts.regenerate;
}
