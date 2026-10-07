/**
 * Server-only runtime settings that never reach the browser bundle or
 * window.__PAYVERGE_ENV__. Read at request time so one image serves any
 * deployment. See docs/self-hosting/frontend-config.md.
 */
import { resolvePublicEnv } from "@/config/publicConfig";

type EnvLike = Readonly<Record<string, string | undefined>>;

function flag(value: string | undefined): boolean | undefined {
  const normalized = (value ?? "").trim().toLowerCase();
  if (["1", "true", "yes", "on"].includes(normalized)) return true;
  if (["0", "false", "no", "off"].includes(normalized)) return false;
  return undefined;
}

/**
 * Search-engine indexing is opt-in: a fresh self-hosted instance must not get
 * crawled. `SEO_INDEXING=true` lifts the robots.txt `Disallow: /` and the
 * root `noindex` meta.
 */
export function isSeoIndexingEnabled(env: EnvLike = process.env): boolean {
  return flag(env.SEO_INDEXING) === true;
}

/**
 * Cloudflare Web Analytics (beacon from static.cloudflareinsights.com) is
 * allowed by the CSP only when explicitly enabled, or when `EDGE=cloudflare`
 * and `CLOUDFLARE_INSIGHTS` is unset.
 */
export function isCloudflareInsightsEnabled(env: EnvLike = process.env): boolean {
  const explicit = flag(env.CLOUDFLARE_INSIGHTS);
  if (explicit !== undefined) return explicit;
  return (env.EDGE ?? "").trim().toLowerCase() === "cloudflare";
}

/**
 * LOGO_URL, the operator logo the backend publishes as /api/v1/instance
 * logo_url. Read here only so the runtime CSP can allow its host in img-src;
 * cspOrigin() drops anything that is not a clean absolute http(s) URL.
 */
export function getLogoUrl(env: EnvLike = process.env): string {
  return (env.LOGO_URL ?? "").trim();
}

/** Optional brand Twitter/X handle for metadata + Organization sameAs. */
export function getTwitterHandle(env: EnvLike = process.env): string {
  const raw = (env.SEO_TWITTER_HANDLE ?? "").trim().replace(/^@/, "");
  return /^[A-Za-z0-9_]{1,15}$/.test(raw) ? raw : "";
}

const EMAIL_RE = /^[^\s@<>"']+@[^\s@<>"']+\.[^\s@<>"']+$/;

/**
 * Contact for /.well-known/security.txt: SECURITY_EMAIL, else SUPPORT_EMAIL,
 * else "" (the route then 404s rather than naming someone else's inbox).
 */
export function getSecurityEmail(env: EnvLike = process.env): string {
  const raw = (env.SECURITY_EMAIL ?? "").trim();
  if (EMAIL_RE.test(raw)) return raw;
  // Build-time inlined values only count for the real process environment;
  // an explicit env object (tests, callers) is taken as the whole truth.
  return resolvePublicEnv(env, undefined, {
    includeBuildFallback: env === process.env,
  }).SUPPORT_EMAIL;
}
