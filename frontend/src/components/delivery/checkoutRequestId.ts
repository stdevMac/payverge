/**
 * Wave 4 delivery checkout idempotency: one request id per in-flight checkout,
 * persisted in sessionStorage so a retry after a network failure (or a reload
 * mid-submit) reuses the SAME X-Request-Id and the backend replays the
 * original order instead of creating a duplicate. Cleared only on success.
 *
 * The id is BOUND to a fingerprint of the checkout content: if the guest edits
 * the cart/address/tip after a lost response and resubmits, the fingerprint
 * changes and a FRESH id is minted — otherwise the backend would replay the
 * old order A while the guest believes they ordered B.
 *
 * Mirrors newSplitRequestId in api/splitting.ts.
 */
const KEY_PREFIX = "payverge_delivery_checkout_rid:";

interface StoredRequestId {
  id: string;
  fingerprint: string;
}

function newRequestId(): string {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return crypto.randomUUID();
  }
  return `delivery-${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

/** JSON.stringify with object keys sorted so key order can't change the hash. */
function stableStringify(value: unknown): string {
  if (value === null || typeof value !== "object") {
    return JSON.stringify(value) ?? "undefined";
  }
  if (Array.isArray(value)) {
    return `[${value.map(stableStringify).join(",")}]`;
  }
  const entries = Object.entries(value as Record<string, unknown>)
    .filter(([, v]) => v !== undefined)
    .sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0))
    .map(([k, v]) => `${JSON.stringify(k)}:${stableStringify(v)}`);
  return `{${entries.join(",")}}`;
}

/**
 * Tiny non-crypto hash (djb2) over a deterministic serialization of the
 * checkout-relevant content. A collision only degrades to the
 * pre-fingerprint behavior (id reuse across an edited cart), never breaks
 * checkout itself.
 */
export function computeCheckoutFingerprint(content: unknown): string {
  const serialized = stableStringify(content);
  let hash = 5381;
  for (let i = 0; i < serialized.length; i++) {
    hash = ((hash << 5) + hash + serialized.charCodeAt(i)) | 0; // hash*33 + c
  }
  return `v1:${(hash >>> 0).toString(36)}:${serialized.length}`;
}

export function getOrCreateCheckoutRequestId(
  businessId: number,
  fingerprint: string
): string {
  const key = `${KEY_PREFIX}${businessId}`;
  try {
    const raw = sessionStorage.getItem(key);
    if (raw) {
      try {
        const stored = JSON.parse(raw) as Partial<StoredRequestId>;
        if (
          stored &&
          typeof stored === "object" &&
          typeof stored.id === "string" &&
          stored.fingerprint === fingerprint
        ) {
          return stored.id;
        }
      } catch {
        // Legacy plain-string or corrupt value: fall through and re-mint.
      }
    }
    const pair: StoredRequestId = { id: newRequestId(), fingerprint };
    sessionStorage.setItem(key, JSON.stringify(pair));
    return pair.id;
  } catch {
    // Storage unavailable (private mode quota, SSR guard): fall back to a
    // per-call id — no dedupe across retries, but checkout still works.
    return newRequestId();
  }
}

export function clearCheckoutRequestId(businessId: number): void {
  try {
    sessionStorage.removeItem(`${KEY_PREFIX}${businessId}`);
  } catch {
    // best-effort
  }
}
