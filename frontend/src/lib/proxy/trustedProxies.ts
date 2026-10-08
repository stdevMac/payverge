/**
 * FRONTEND_TRUSTED_PROXIES: comma-separated IPs/CIDRs of the edge in front of
 * the Next server (Caddy, a PaaS router). Only a request whose TCP peer is in
 * this list may pass its client-identity headers (X-Forwarded-For and
 * friends) through the same-origin proxy. Default: empty, so no client can
 * pick the IP the backend rate-limits on. Server-only; never published.
 */
import { BlockList, isIP } from "node:net";

import { normalizePeerAddress } from "./peerStamp";

type EnvLike = Readonly<Record<string, string | undefined>>;

const KEY = "FRONTEND_TRUSTED_PROXIES";

interface ParsedTrust {
  readonly list: BlockList | null;
  /** Problems by entry position, never the value. */
  readonly issues: readonly string[];
}

let cache: { raw: string; parsed: ParsedTrust } | null = null;

function parse(raw: string): ParsedTrust {
  const list = new BlockList();
  const issues: string[] = [];
  let entries = 0;
  raw.split(",").forEach((part, index) => {
    const entry = part.trim();
    if (!entry) return;
    const [rawAddress, rawPrefix, ...extra] = entry.split("/");
    const address = normalizePeerAddress(rawAddress);
    const family = address ? isIP(address) : 0;
    const max = family === 4 ? 32 : 128;
    const prefix = rawPrefix === undefined ? max : Number(rawPrefix);
    if (
      !address ||
      !family ||
      extra.length > 0 ||
      (rawPrefix !== undefined && !/^\d{1,3}$/.test(rawPrefix)) ||
      prefix > max
    ) {
      issues.push(`${KEY}: entry ${index + 1} is not an IP or CIDR (ignored)`);
      return;
    }
    if (prefix === 0) {
      issues.push(`${KEY}: entry ${index + 1} trusts every address (ignored)`);
      return;
    }
    list.addSubnet(address, prefix, family === 4 ? "ipv4" : "ipv6");
    entries += 1;
  });
  return { list: entries > 0 ? list : null, issues };
}

function parsed(env: EnvLike): ParsedTrust {
  const raw = (env[KEY] ?? "").trim();
  if (!cache || cache.raw !== raw) cache = { raw, parsed: parse(raw) };
  return cache.parsed;
}

/** Whether `address` (a stamped socket peer) is a configured trusted edge. */
export function isTrustedProxy(
  address: string,
  env: EnvLike = process.env,
): boolean {
  const { list } = parsed(env);
  if (!list) return false;
  const normalized = normalizePeerAddress(address);
  const family = normalized ? isIP(normalized) : 0;
  if (!normalized || !family) return false;
  return list.check(normalized, family === 4 ? "ipv4" : "ipv6");
}

/** Boot-time warnings for FRONTEND_TRUSTED_PROXIES (logged by instrumentation). */
export function trustedProxyIssues(env: EnvLike = process.env): string[] {
  return [...parsed(env).issues];
}
