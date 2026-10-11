/**
 * RFC 4122 version 4 UUID from the platform CSPRNG.
 *
 * crypto.randomUUID only exists in secure contexts (HTTPS or localhost), so a
 * restaurant LAN install served over plain http does not have it.
 * crypto.getRandomValues is available in every browser context and in Node,
 * so it backs the fallback. Request ids, idempotency keys and session ids must
 * stay unguessable; do not fall back to Math.random.
 */
export function randomUUID(): string {
  const webCrypto = globalThis.crypto;
  if (typeof webCrypto.randomUUID === "function") {
    return webCrypto.randomUUID();
  }
  const bytes = webCrypto.getRandomValues(new Uint8Array(16));
  bytes[6] = (bytes[6] & 0x0f) | 0x40; // version 4
  bytes[8] = (bytes[8] & 0x3f) | 0x80; // RFC 4122 variant
  const hex = Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}
