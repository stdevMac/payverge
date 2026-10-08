export const GUEST_TABLE_LANDING_FALLBACK_POLL_MS = 30_000;

/**
 * Fallback polling for the guest table landing. The SSE stream is the primary
 * refresh; poll only when it is down, or when the first table payload has not
 * arrived yet (the transient-failure screen still needs retries).
 */
export function shouldPollGuestTableLanding(opts: {
  tableCode: string | undefined;
  streamConnected: boolean;
  hasTableData: boolean;
}): boolean {
  if (!opts.tableCode) return false;
  return !opts.streamConnected || !opts.hasTableData;
}
