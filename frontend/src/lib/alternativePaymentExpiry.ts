export const ALTERNATIVE_PAYMENT_REQUEST_MAX_AGE_MS = 24 * 60 * 60 * 1000;

export function alternativePaymentRequestIsExpired(
  request: { timestamp: number; expiresAt?: string | null },
  nowMs = Date.now(),
): boolean {
  if (request.expiresAt) {
    const explicit = Date.parse(request.expiresAt);
    if (Number.isFinite(explicit) && explicit <= nowMs) {
      return true;
    }
  }
  if (!Number.isFinite(request.timestamp) || request.timestamp <= 0) {
    return true;
  }
  return nowMs - request.timestamp >= ALTERNATIVE_PAYMENT_REQUEST_MAX_AGE_MS;
}

export function formatAlternativePaymentRequestAge(
  timestamp: number,
  nowMs = Date.now(),
): string {
  const elapsedMs = Math.max(0, nowMs - timestamp);
  const minutes = Math.floor(elapsedMs / 60_000);
  if (minutes < 60) {
    return `${Math.max(1, minutes)}m`;
  }
  const hours = Math.floor(minutes / 60);
  if (hours < 24 * 7) {
    return `${hours}h`;
  }
  return `${Math.floor(hours / 24)}d`;
}

export function formatAlternativePaymentRequestedAt(
  timestamp: number,
  locale: string,
): string {
  if (!Number.isFinite(timestamp) || timestamp <= 0) {
    return "";
  }
  return new Intl.DateTimeFormat(locale, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(timestamp));
}
