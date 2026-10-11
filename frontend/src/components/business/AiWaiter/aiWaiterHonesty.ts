/** Empty KPI treatment: never mix a proud "0" with "No data yet" in the same row. */
export function metricsHaveConversationData(
  totalConversations: number,
): boolean {
  return Number.isFinite(totalConversations) && totalConversations > 0;
}

/** Service is on but there is no guest activity to report yet (#173). */
export function shouldShowIdleServiceCopy(opts: {
  aiEnabled: boolean;
  insightsReady: boolean;
  insightsError: boolean;
  totalConversations: number;
}): boolean {
  return (
    opts.aiEnabled &&
    opts.insightsReady &&
    !opts.insightsError &&
    !metricsHaveConversationData(opts.totalConversations)
  );
}

/**
 * Header live-chat count. Omit zeros and unknown values so the chip does not
 * look like a live service with nothing happening (#173 / #195).
 */
export function liveChatsHeaderValue(
  activeCount: number | null | undefined,
): number | null {
  if (activeCount == null || !Number.isFinite(activeCount) || activeCount <= 0) {
    return null;
  }
  return activeCount;
}

export type AiPriorityPlan = "apply" | "confirm-upselling" | "ignore";

/** Switching into hard upselling requires an explicit confirm (#240). */
export function planAiPriorityChange(
  current: string,
  next: string,
): AiPriorityPlan {
  const target = next.trim();
  if (!target || target === current) return "ignore";
  if (target === "upselling") return "confirm-upselling";
  return "apply";
}
