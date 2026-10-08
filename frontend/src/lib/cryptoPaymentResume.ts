/**
 * Persisted double-spend guard for in-flight USDC direct transfers.
 *
 * PaymentProcessor holds txHash/transferConfirmed/quoteToken in React state.
 * On a page refresh the modal wrapper returns null, the component unmounts,
 * and that state is lost — dropping the guest to "ready" where a second
 * writeContract would double-spend. We mirror the guard here, keyed by
 * billToken (guest public_token), so a reload can rehydrate it.
 *
 * quoteToken is bill+amount-bound with a 30-min TTL (low blast radius) and is
 * required by finalizePayment on restore, so it is stored. No amounts or
 * wallet data are persisted.
 *
 * This helper is intentionally generic (not USDC-specific in its shape) so the
 * cross-chain plan can reuse it verbatim for direct USDC and cross-chain flows.
 */

export const CRYPTO_INFLIGHT_KEY = "payverge_crypto_inflight";

/**
 * Client-side staleness bound for a persisted in-flight record. The quoteToken
 * is bill+amount-bound with a 30-minute server TTL, so a record older than this
 * carries an expired token that finalizePayment will reject. Treat such records
 * as absent and evict them so the guest cleanly falls back to a fresh quote
 * instead of looping on a server rejection. Slightly under 30 min so we never
 * rehydrate a token the server is about to (or already did) expire.
 */
const CRYPTO_INFLIGHT_TTL_MS = 30 * 60 * 1000;

export interface CryptoInflight {
  /** Guest bill capability (public_token) this in-flight transfer belongs to. */
  billToken: string;
  /**
   * The on-chain transfer hash. Re-notifying the same hash is idempotent
   * backend-side. For the cross-chain (LI.FI) flow this is the FINAL Base
   * settlement hash — never a mid-route source-chain hash, which would fail
   * the backend's USDC-on-Base verification.
   */
  txHash: string;
  /** Signed, server-locked USD quote token. finalizePayment needs it on restore. */
  quoteToken: string;
  /** True once the transfer is confirmed on-chain (>=3 confs). */
  confirmed: boolean;
  /**
   * Optional LI.FI route id, echoed back to the backend for traceability on a
   * cross-chain resume. Absent for the direct-USDC flow.
   */
  lifiRouteId?: string;
  /**
   * The source token SYMBOL the first notify recorded (e.g. "WETH"). MUST be
   * persisted and re-sent verbatim on resume: the backend's idempotency match
   * (PaymentMatchesConfirmedInput) compares SourceToken exactly, so a remount
   * that fell back to "UNKNOWN" would 409 against the already-recorded payment.
   * Absent for the direct-USDC flow (no source token to carry).
   */
  sourceToken?: string;
  /**
   * The RESOLVED source-chain name the first notify recorded (e.g. "Base"),
   * already passed through getChainName. Re-sent verbatim on resume for the
   * same exact-match reason as sourceToken; the backend compares SourceChain
   * exactly. Absent for the direct-USDC flow.
   */
  sourceChain?: string;
  /**
   * Optional numeric source chain id (e.g. 8453) backing sourceChain, retained
   * for traceability/debugging on resume. Not directly compared by the backend.
   */
  sourceChainId?: number;
  /**
   * Epoch ms when the record was written. Stamped by writeCryptoInflight and
   * used by readCryptoInflight to drop records past CRYPTO_INFLIGHT_TTL_MS.
   * Optional on the input; a stored record without it is treated as expired.
   */
  savedAt?: number;
}

function isCryptoInflight(v: unknown): v is CryptoInflight {
  if (typeof v !== "object" || v === null) return false;
  const r = v as Record<string, unknown>;
  return (
    typeof r.billToken === "string" &&
    typeof r.txHash === "string" &&
    typeof r.quoteToken === "string" &&
    typeof r.confirmed === "boolean"
  );
}

/** Read the in-flight record, but only if it belongs to `billToken`. */
export function readCryptoInflight(billToken: string): CryptoInflight | null {
  if (typeof window === "undefined") return null;
  const raw = window.sessionStorage.getItem(CRYPTO_INFLIGHT_KEY);
  if (!raw) return null;
  try {
    const parsed: unknown = JSON.parse(raw);
    if (!isCryptoInflight(parsed)) return null;
    if (parsed.billToken !== billToken) return null;
    // Evict records whose quote token has aged out of the server's 30-min TTL;
    // a record with no savedAt cannot prove it is fresh, so it is evicted too.
    if (
      typeof parsed.savedAt !== "number" ||
      Date.now() - parsed.savedAt > CRYPTO_INFLIGHT_TTL_MS
    ) {
      clearCryptoInflight();
      return null;
    }
    return parsed;
  } catch {
    return null;
  }
}

export function writeCryptoInflight(record: CryptoInflight): void {
  if (typeof window === "undefined") return;
  // Stamp savedAt (preserving an explicit one) so reads can enforce the TTL.
  const stamped: CryptoInflight = {
    ...record,
    savedAt: record.savedAt ?? Date.now(),
  };
  window.sessionStorage.setItem(CRYPTO_INFLIGHT_KEY, JSON.stringify(stamped));
}

export function clearCryptoInflight(): void {
  if (typeof window === "undefined") return;
  window.sessionStorage.removeItem(CRYPTO_INFLIGHT_KEY);
}
