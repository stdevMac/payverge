/**
 * Regenerate deploy/cloudflare-cidrs.caddy from Cloudflare's published IP lists.
 *
 * Usage (from repo root, network required):
 *   /usr/bin/node frontend/node_modules/tsx/dist/cli.mjs scripts/update-cloudflare-cidrs.ts
 *   /usr/bin/node frontend/node_modules/tsx/dist/cli.mjs scripts/update-cloudflare-cidrs.ts --check --max-age-days 45
 *
 * Pure helpers (parse + format) are exportable for offline unit tests —
 * importing this module does NOT fetch. Only main() performs network I/O.
 *
 * The scheduled check compares this snapshot with Cloudflare's published lists
 * and rejects an old snapshot. It never fetches trust data at request time.
 */

import * as crypto from "node:crypto";
import * as fs from "node:fs";
import * as path from "node:path";
import { fileURLToPath } from "node:url";

export const CLOUDFLARE_IPS_V4_URL = "https://www.cloudflare.com/ips-v4";
export const CLOUDFLARE_IPS_V6_URL = "https://www.cloudflare.com/ips-v6";

const DEFAULT_OUTPUT_REL = path.join("deploy", "cloudflare-cidrs.caddy");

/** Parse a Cloudflare ips-v4 / ips-v6 body into a unique CIDR list (order preserved). */
export function parseCloudflareIpList(raw: string): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const line of raw.split(/\r?\n/)) {
    const cidr = line.trim();
    if (!cidr || cidr.startsWith("#")) continue;
    // Basic CIDR shape: IPv4 or IPv6 prefix + /length
    if (!/^[0-9a-fA-F:.]+\/\d{1,3}$/.test(cidr)) {
      throw new Error(`invalid Cloudflare CIDR line: ${JSON.stringify(cidr)}`);
    }
    if (seen.has(cidr)) continue;
    seen.add(cidr);
    out.push(cidr);
  }
  return out;
}

/** Stable content hash over the canonical CIDR payload (newline-joined + trailing NL). */
export function contentHash(cidrs: readonly string[]): string {
  const payload = cidrs.join("\n") + (cidrs.length > 0 ? "\n" : "");
  return crypto.createHash("sha256").update(payload, "utf8").digest("hex");
}

export type FormatCloudflareCidrsInput = {
  ipv4: readonly string[];
  ipv6: readonly string[];
  /** ISO-8601 UTC timestamp, e.g. 2026-07-10T22:47:48Z */
  generatedAt: string;
};

/**
 * Pure: raw IPv4 + IPv6 CIDR lists → Caddy snippet body for import inside
 * `servers { ... }` as `trusted_proxies static ...` + client_ip_headers.
 */
export function formatCloudflareCidrsCaddy(
  input: FormatCloudflareCidrsInput,
): string {
  const cidrs = [...input.ipv4, ...input.ipv6];
  if (cidrs.length === 0) {
    throw new Error("formatCloudflareCidrsCaddy: empty CIDR list");
  }
  const hash = contentHash(cidrs);
  const indented = cidrs.map((c) => `\t${c}`).join(" \\\n");

  return [
    "# Cloudflare published IP ranges — checked-in trusted_proxies source for Caddy.",
    "#",
    "# Provenance:",
    "#   IPv4: https://www.cloudflare.com/ips-v4",
    "#   IPv6: https://www.cloudflare.com/ips-v6",
    `# Generated-at: ${input.generatedAt}`,
    `# Content-hash: sha256:${hash}`,
    "#",
    "# This file is the ONLY trusted Cloudflare CIDR source. Never fetch trust data",
    "# at request time. Regenerate offline via:",
    "#   /usr/bin/node frontend/node_modules/tsx/dist/cli.mjs scripts/update-cloudflare-cidrs.ts",
    "#",
    "# A scheduled CI check compares this file with the published lists and",
    "# rejects snapshots older than the configured review window.",
    "#",
    "# Import inside the global servers block:",
    "#   {",
    "#     servers {",
    "#       import /etc/caddy/cloudflare-cidrs.caddy",
    "#     }",
    "#   }",
    "",
    "trusted_proxies static \\",
    `${indented}`,
    "",
    "# Prefer Cloudflare's authenticated client-IP header, then XFF.",
    "client_ip_headers CF-Connecting-IP X-Forwarded-For",
    "",
  ].join("\n");
}

function repoRootFromThisFile(): string {
  // Prefer import.meta when available (tsx / ESM); fall back to cwd for CJS.
  try {
    const here = path.dirname(fileURLToPath(import.meta.url));
    return path.resolve(here, "..");
  } catch {
    return process.cwd();
  }
}

async function fetchText(url: string): Promise<string> {
  const res = await fetch(url, {
    headers: { Accept: "text/plain" },
    redirect: "follow",
  });
  if (!res.ok) {
    throw new Error(`fetch ${url} failed: HTTP ${res.status}`);
  }
  return res.text();
}

/** Network + filesystem side effects. Not imported by unit tests. */
export async function main(
  options: {
    outPath?: string;
    fetchImpl?: (url: string) => Promise<string>;
    now?: () => Date;
  } = {},
): Promise<string> {
  const fetchImpl = options.fetchImpl ?? fetchText;
  const now = options.now ?? (() => new Date());
  const root = repoRootFromThisFile();
  const outPath = options.outPath ?? path.join(root, DEFAULT_OUTPUT_REL);

  const [v4raw, v6raw] = await Promise.all([
    fetchImpl(CLOUDFLARE_IPS_V4_URL),
    fetchImpl(CLOUDFLARE_IPS_V6_URL),
  ]);

  const ipv4 = parseCloudflareIpList(v4raw);
  const ipv6 = parseCloudflareIpList(v6raw);
  const generatedAt = now().toISOString().replace(/\.\d{3}Z$/, "Z");
  const body = formatCloudflareCidrsCaddy({ ipv4, ipv6, generatedAt });

  fs.mkdirSync(path.dirname(outPath), { recursive: true });
  fs.writeFileSync(outPath, body, "utf8");
  return outPath;
}

export function validateCloudflareSnapshot(
  snapshot: string,
  ipv4: readonly string[],
  ipv6: readonly string[],
  now: Date,
  maxAgeDays: number,
): void {
  if (!Number.isFinite(maxAgeDays) || maxAgeDays <= 0) {
    throw new Error("--max-age-days must be a positive number");
  }
  const hash = snapshot.match(/^# Content-hash: sha256:([a-f0-9]{64})$/m)?.[1];
  const generatedAt = snapshot.match(/^# Generated-at: (\S+)$/m)?.[1];
  if (!hash || !generatedAt) {
    throw new Error("snapshot is missing generated-at or content-hash metadata");
  }
  const lines = snapshot.split(/\r?\n/);
  const blockStart = lines.findIndex((line) => line.trim() === "trusted_proxies static \\");
  if (blockStart === -1) {
    throw new Error("snapshot is missing the trusted_proxies block");
  }
  const snapshotCidrs: string[] = [];
  for (const line of lines.slice(blockStart + 1)) {
    if (!line.trim()) break;
    snapshotCidrs.push(line.replace(/\s*\\\s*$/, "").trim());
  }
  const parsedSnapshotCidrs = parseCloudflareIpList(snapshotCidrs.join("\n"));
  if (contentHash(parsedSnapshotCidrs) !== hash) {
    throw new Error("snapshot body does not match its content hash");
  }
  const expectedHash = contentHash([...ipv4, ...ipv6]);
  if (hash !== expectedHash) {
    throw new Error("snapshot hash does not match Cloudflare's current published CIDRs");
  }
  const generatedMS = Date.parse(generatedAt);
  if (!Number.isFinite(generatedMS)) {
    throw new Error("snapshot generated-at metadata is invalid");
  }
  const ageMS = now.getTime() - generatedMS;
  if (ageMS < 0) {
    throw new Error("snapshot generated-at metadata is in the future");
  }
  if (ageMS > maxAgeDays * 24 * 60 * 60 * 1000) {
    throw new Error(`snapshot is older than ${maxAgeDays} days`);
  }
}

export async function checkSnapshot(
  options: {
    outPath?: string;
    fetchImpl?: (url: string) => Promise<string>;
    now?: () => Date;
    maxAgeDays?: number;
  } = {},
): Promise<string> {
  const fetchImpl = options.fetchImpl ?? fetchText;
  const now = options.now ?? (() => new Date());
  const root = repoRootFromThisFile();
  const outPath = options.outPath ?? path.join(root, DEFAULT_OUTPUT_REL);
  const [v4raw, v6raw] = await Promise.all([
    fetchImpl(CLOUDFLARE_IPS_V4_URL),
    fetchImpl(CLOUDFLARE_IPS_V6_URL),
  ]);
  validateCloudflareSnapshot(
    fs.readFileSync(outPath, "utf8"),
    parseCloudflareIpList(v4raw),
    parseCloudflareIpList(v6raw),
    now(),
    options.maxAgeDays ?? 45,
  );
  return outPath;
}

function readMaxAgeDays(args: readonly string[]): number {
  const index = args.indexOf("--max-age-days");
  if (index === -1) return 45;
  const value = Number(args[index + 1]);
  if (!Number.isFinite(value) || value <= 0) {
    throw new Error("--max-age-days must be followed by a positive number");
  }
  return value;
}

// Run only when this file is the process entrypoint (tsx / node), not on import.
const entry = process.argv[1] ? path.resolve(process.argv[1]) : "";
const isEntrypoint = /(?:^|\/)update-cloudflare-cidrs\.(?:ts|js|mjs|cjs)$/.test(entry);
if (isEntrypoint) {
  const check = process.argv.includes("--check");
  const action = check
    ? checkSnapshot({ maxAgeDays: readMaxAgeDays(process.argv.slice(2)) })
    : main();
  action
    .then((out) => {
      process.stdout.write(`${check ? "checked" : "wrote"} ${out}\n`);
    })
    .catch((err: unknown) => {
      const msg =
        err instanceof Error ? (err.stack ?? err.message) : String(err);
      process.stderr.write(`${msg}\n`);
      process.exitCode = 1;
    });
}
