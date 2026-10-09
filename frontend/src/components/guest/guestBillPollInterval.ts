export const GUEST_BILL_POLL_DEFAULT_MS = 10_000;
export const GUEST_BILL_POLL_FAST_MS = 3_000;
/** Slow backstop while the live table stream is already pushing bill changes. */
export const GUEST_BILL_POLL_SSE_BACKSTOP_MS = 30_000;

export function guestBillPollInterval({
  isActive = true,
  splitPanelOpen = false,
  paymentInFlight = false,
  sseConnected = false,
}: {
  isActive?: boolean;
  splitPanelOpen?: boolean;
  paymentInFlight?: boolean;
  sseConnected?: boolean;
} = {}): number {
  if (isActive && (splitPanelOpen || paymentInFlight)) {
    return GUEST_BILL_POLL_FAST_MS;
  }
  if (sseConnected) {
    return GUEST_BILL_POLL_SSE_BACKSTOP_MS;
  }
  return GUEST_BILL_POLL_DEFAULT_MS;
}
