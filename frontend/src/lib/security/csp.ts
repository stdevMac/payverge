/**
 * Runtime Content-Security-Policy builder (open-source self-hosting).
 *
 * The policy is assembled per request from the runtime public config, so one
 * image serves any origin: nothing deployment-specific is baked in. Inputs:
 *
 *   - PUBLIC_URL / 'self'       — the page, same-origin /api/v1 and /media
 *   - API_URL (when absolute)   — a split-origin backend (connect + img)
 *   - PUBLIC_RPC_URL / NETWORK  — wallet RPC for both supported chains
 *   - FRONTEND_SENTRY_DSN       — Sentry ingest host when browser Sentry is on
 *   - POSTHOG_HOST              — analytics host when configured
 *   - MEDIA_ORIGINS             — extra image hosts (e.g. an S3/CDN bucket)
 *   - LOGO_URL                  — the operator logo host (same env the backend
 *                                 serves as /api/v1/instance logo_url)
 *   - CLOUDFLARE_INSIGHTS / EDGE=cloudflare — the Cloudflare beacon, opt-in
 *
 * See docs/self-hosting/frontend-config.md.
 */
import { imageCspOrigins } from "@/config/imageCspOrigins";
import {
  allowsPlainHttp,
  getPublicConfig,
  getRpcUrlFor,
  isLoopbackHost,
  type PublicConfig,
} from "@/config/publicConfig";
import {
  getLogoUrl,
  isCloudflareInsightsEnabled,
} from "@/config/serverConfig";

const CLOUDFLARE_INSIGHTS_SCRIPT = "https://static.cloudflareinsights.com";
const CLOUDFLARE_INSIGHTS_BEACON = "https://cloudflareinsights.com";

// DNS names, IPv4 and bracketed IPv6 only: anything else (quotes, `*`) could
// turn into a different CSP source expression.
const SAFE_HOST_RE = /^(?:[a-z0-9-]+(?:\.[a-z0-9-]+)*|\[[0-9a-f:.]+\])$/;

/** CSP source-expression safe origin of an absolute URL, or null. */
export function cspOrigin(raw: string | undefined): string | null {
  const value = (raw ?? "").trim();
  if (!value || value.startsWith("/")) return null;
  let url: URL;
  try {
    url = new URL(value);
  } catch {
    return null;
  }
  if (url.protocol !== "https:" && url.protocol !== "http:") return null;
  if (url.protocol === "http:" && !allowsPlainHttp(url.hostname)) return null;
  if (!SAFE_HOST_RE.test(url.hostname)) return null;
  return url.origin;
}

/**
 * MEDIA_ORIGINS: comma-separated `https://host[:port]` (or bare `host`)
 * entries. Anything that is not a clean https origin is dropped.
 */
export function parseMediaOrigins(raw: string | undefined): string[] {
  const out = new Set<string>();
  for (const entry of (raw ?? "").split(",")) {
    const candidate = entry.trim();
    if (!candidate) continue;
    let url: URL;
    try {
      url = new URL(candidate.includes("://") ? candidate : `https://${candidate}`);
    } catch {
      continue;
    }
    if (url.protocol !== "https:" || url.username || url.password) continue;
    if (!SAFE_HOST_RE.test(url.hostname)) continue;
    out.add(url.origin);
  }
  return [...out];
}

export interface CspInput {
  readonly nonce: string;
  /** Development adds 'unsafe-eval' for React Refresh. */
  readonly dev: boolean;
  readonly config: PublicConfig;
  readonly cloudflareInsights: boolean;
  /** RPC endpoints the wallet stack may call (both supported chains). */
  readonly rpcUrls?: readonly string[];
  /** LOGO_URL: the instance logo, rendered as a plain <img> from its host. */
  readonly logoUrl?: string;
}

function joinUnique(values: Iterable<string | null | undefined>): string {
  const out = new Set<string>();
  for (const value of values) if (value) out.add(value);
  return [...out].join(" ");
}

export function buildContentSecurityPolicy(input: CspInput): string {
  const { nonce, dev, config, cloudflareInsights } = input;
  if (!/^[A-Za-z0-9+/=_-]+$/.test(nonce)) {
    throw new Error("CSP nonce must be base64/base64url");
  }

  const apiOrigin = cspOrigin(config.apiUrl);
  let apiIsLoopback = false;
  if (apiOrigin) {
    try {
      apiIsLoopback = isLoopbackHost(new URL(apiOrigin).hostname);
    } catch {
      apiIsLoopback = false;
    }
  }

  const scriptSrc = joinUnique([
    "'self'",
    `'nonce-${nonce}'`,
    "'strict-dynamic'",
    // Next's local React Refresh runtime evaluates generated code.
    dev ? "'unsafe-eval'" : null,
    cloudflareInsights ? CLOUDFLARE_INSIGHTS_SCRIPT : null,
  ]);

  // NextUI / CSS-in-JS injects <style> tags that cannot be nonce-gated
  // without a major UI refactor; adding a style nonce would make browsers
  // ignore 'unsafe-inline' and break the app (#289).
  const styleSrc = "'self' 'unsafe-inline'";

  const imgSrc = joinUnique([
    "'self'",
    "data:",
    "blob:",
    ...imageCspOrigins(),
    ...parseMediaOrigins(config.mediaOrigins),
    cspOrigin(input.logoUrl),
    // A split-origin backend serves /media from its own host.
    apiOrigin,
  ]);

  const connectSrc = joinUnique([
    "'self'",
    apiOrigin,
    // localhost and 127.0.0.1 are distinct origins; allow both spellings.
    ...(apiIsLoopback ? ["http://localhost:8080", "http://127.0.0.1:8080"] : []),
    ...(input.rpcUrls ?? [config.rpcUrl]).map(cspOrigin),
    config.sentryDsn ? cspOrigin(config.sentryDsn) : null,
    config.posthogHost ? cspOrigin(config.posthogHost) : null,
    cloudflareInsights ? CLOUDFLARE_INSIGHTS_BEACON : null,
    cloudflareInsights ? CLOUDFLARE_INSIGHTS_SCRIPT : null,
  ]);

  return [
    "default-src 'self'",
    `script-src ${scriptSrc}`,
    `style-src ${styleSrc}`,
    `img-src ${imgSrc}`,
    "font-src 'self' data:",
    `connect-src ${connectSrc}`,
    "frame-ancestors 'none'",
    "base-uri 'self'",
    "form-action 'self'",
    "object-src 'none'",
    "worker-src 'self' blob:",
  ].join("; ");
}

/** The policy for the current request, from the live runtime config. */
export function contentSecurityPolicyFor(nonce: string): string {
  return buildContentSecurityPolicy({
    nonce,
    dev: process.env.NODE_ENV !== "production",
    config: getPublicConfig(),
    cloudflareInsights: isCloudflareInsightsEnabled(),
    rpcUrls: [getRpcUrlFor("base"), getRpcUrlFor("baseSepolia")],
    logoUrl: getLogoUrl(),
  });
}
