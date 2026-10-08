/** Map axios/backend errors that carry code period_locked. */
export function isPeriodLockedError(err: unknown): boolean {
  if (!err || typeof err !== "object") return false;
  const e = err as {
    response?: { status?: number; data?: { code?: string; error?: string } };
    code?: string;
  };
  if (e.response?.data?.code === "period_locked") return true;
  if (e.response?.status === 409 && e.response?.data?.code === "period_locked") return true;
  const msg = e.response?.data?.error ?? "";
  return typeof msg === "string" && msg.includes("period is locked");
}

export function periodLockedMessage(
  err: unknown,
  t: (key: string, params?: Record<string, string | number>) => string,
): string {
  const e = err as { response?: { data?: { locked_through?: string; error?: string } } };
  const through = e.response?.data?.locked_through;
  if (through) {
    return t("periodLock.lockedToastThrough", { date: through });
  }
  return t("periodLock.lockedToast");
}
