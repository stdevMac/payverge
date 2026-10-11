/**
 * The TCP peer of a request, carried from the Node HTTP server to route
 * handlers.
 *
 * Route handlers only see a web `Request`, never the socket. Next fills
 * `X-Forwarded-For` from the socket only when the client sent none (`??=`),
 * so in a route handler a forged header and a real peer look the same.
 * `installPeerStamp()` (run from src/instrumentation.ts) records the socket
 * address on every incoming request under PEER_STAMP_HEADER, prefixed with a
 * per-process nonce. A client can send that header too, but it cannot know
 * the nonce, and the stamp overwrites it anyway once installed.
 *
 * This module has no Node imports so the proxy can use it anywhere.
 */

export const PEER_STAMP_HEADER = "x-payverge-peer";

const NONCE_KEY = Symbol.for("payverge.peerStamp.nonce");

type StampGlobal = typeof globalThis & { [NONCE_KEY]?: string };

export interface StampedPeer {
  /** Socket remote address; IPv4-mapped IPv6 is reduced to plain IPv4. */
  readonly address: string;
  /** Whether the socket itself was TLS (Next normally serves plain http). */
  readonly protocol: "http" | "https";
}

/**
 * The nonce shared by the stamp and its readers. Lives on globalThis because
 * instrumentation and each route handler are separate webpack bundles in the
 * same process.
 */
export function setPeerStampNonce(nonce: string | null): void {
  const g = globalThis as StampGlobal;
  if (nonce) g[NONCE_KEY] = nonce;
  else delete g[NONCE_KEY];
}

export function getPeerStampNonce(): string | undefined {
  return (globalThis as StampGlobal)[NONCE_KEY];
}

const IP_CHARS_RE = /^[0-9A-Fa-f:.]{2,45}$/;
const MAPPED_V4_RE = /^::ffff:(\d{1,3}(?:\.\d{1,3}){3})$/i;

/** Drop an IPv6 zone and reduce `::ffff:a.b.c.d` to `a.b.c.d`. */
export function normalizePeerAddress(raw: string): string | null {
  const address = raw.trim().replace(/%.*$/, "");
  if (!IP_CHARS_RE.test(address)) return null;
  const mapped = MAPPED_V4_RE.exec(address);
  return mapped ? mapped[1] : address;
}

export function formatPeerStamp(
  nonce: string,
  protocol: "http" | "https",
  address: string,
): string {
  return `${nonce};${protocol};${address}`;
}

/**
 * The peer recorded for this request, or null when the stamp is missing,
 * forged or not installed in this process.
 */
export function readPeerStamp(headers: Headers): StampedPeer | null {
  const nonce = getPeerStampNonce();
  if (!nonce) return null;
  const parts = (headers.get(PEER_STAMP_HEADER) ?? "").split(";");
  if (parts.length !== 3 || parts[0] !== nonce) return null;
  const protocol = parts[1];
  if (protocol !== "http" && protocol !== "https") return null;
  const address = normalizePeerAddress(parts[2]);
  return address ? { address, protocol } : null;
}
