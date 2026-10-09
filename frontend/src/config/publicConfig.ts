/**
 * Runtime public configuration (Phase B, open-source self-hosting).
 *
 * One frontend image must serve any origin without a rebuild. Next.js inlines
 * literal `process.env.NEXT_PUBLIC_*` reads at build time, so browser-visible
 * settings that vary per deployment cannot come from there. Instead:
 *
 *   - On the server, `getPublicConfig()` reads the process environment at
 *     request time (dynamic lookups are never inlined). Canonical names are
 *     unprefixed (`PUBLIC_URL`, `API_URL`, ...); each key's `NEXT_PUBLIC_*`
 *     twin is read as a runtime fallback.
 *   - The root layout serializes the resolved values for the whitelisted keys
 *     below into `window.__PAYVERGE_ENV__` with a nonce'd inline script, the
 *     first body script. It runs before any page client module evaluates
 *     (their chunks are required from flight data streamed after it). Only
 *     `instrumentation-client.ts`, evaluated by Next's async main chunk, can
 *     run earlier, so it defers config reads via `onPublicEnvReady`.
 *   - In the browser, `getPublicConfig()` reads that object.
 *
 * Literal `process.env.NEXT_PUBLIC_*` reads exist ONLY in this file, as the
 * build-time fallback for images built the old way (and for unit tests). The
 * `no-restricted-syntax` rule in eslint.config.mjs forbids new direct reads of
 * runtime-class keys anywhere else.
 *
 * Build-time-only keys (NEXT_PUBLIC_RELEASE_SHA, NEXT_PUBLIC_VERSION,
 * NEXT_PUBLIC_BUILD_TIMESTAMP, NEXT_PUBLIC_SENTRY_RELEASE) describe the image
 * itself and stay inlined. See docs/self-hosting/frontend-config.md.
 */

type EnvLike = Readonly<Record<string, string | undefined>>;

/**
 * Whitelist of browser-visible keys. Only these ever reach
 * `window.__PAYVERGE_ENV__`; adding a key here publishes it to every visitor,
 * so never add anything secret.
 */
export const PUBLIC_ENV_KEYS = [
  "PUBLIC_URL",
  "API_URL",
  "SUPPORT_EMAIL",
  "NETWORK",
  "RPC_URL",
  "MAINTENANCE_MODE",
  "VAPID_PUBLIC_KEY",
  "MEDIA_ORIGINS",
  "SENTRY_DSN",
  "SENTRY_ENVIRONMENT",
  "SENTRY_ENABLED",
  "SENTRY_TRACES_SAMPLE_RATE",
  "SENTRY_REPLAYS_SESSION_SAMPLE_RATE",
  "SENTRY_REPLAYS_ON_ERROR_SAMPLE_RATE",
  "POSTHOG_KEY",
  "POSTHOG_HOST",
  "LIFI_INTEGRATOR",
] as const;

type PublicEnvKey = (typeof PUBLIC_ENV_KEYS)[number];
export type PublicEnv = Readonly<Record<PublicEnvKey, string>>;

/**
 * Runtime environment names per key, first non-empty wins.
 *
 * RPC_URL and SENTRY_* deliberately have NO unprefixed alias: the backend
 * reads `RPC_URL` (which may embed a paid provider key) and `SENTRY_DSN`
 * (a different Sentry project), and a shared env file must never publish
 * those to browsers by accident.
 */
const RUNTIME_ENV_NAMES: Readonly<Record<PublicEnvKey, readonly string[]>> = {
  // APP_BASE_URL is deliberately NOT read: the backend uses it for its own
  // API origin, so a shared env file would point canonicals there.
  PUBLIC_URL: ["PUBLIC_URL", "NEXT_PUBLIC_PUBLIC_URL"],
  API_URL: ["API_URL", "NEXT_PUBLIC_API_URL"],
  SUPPORT_EMAIL: ["SUPPORT_EMAIL", "NEXT_PUBLIC_SUPPORT_EMAIL"],
  NETWORK: ["NETWORK", "NEXT_PUBLIC_NETWORK"],
  RPC_URL: ["PUBLIC_RPC_URL", "NEXT_PUBLIC_RPC_URL"],
  MAINTENANCE_MODE: ["MAINTENANCE_MODE", "NEXT_PUBLIC_MAINTENANCE_MODE"],
  VAPID_PUBLIC_KEY: ["VAPID_PUBLIC_KEY", "NEXT_PUBLIC_VAPID_PUBLIC_KEY"],
  MEDIA_ORIGINS: ["MEDIA_ORIGINS", "NEXT_PUBLIC_MEDIA_ORIGINS"],
  SENTRY_DSN: ["FRONTEND_SENTRY_DSN", "NEXT_PUBLIC_SENTRY_DSN"],
  SENTRY_ENVIRONMENT: [
    "FRONTEND_SENTRY_ENVIRONMENT",
    "NEXT_PUBLIC_SENTRY_ENVIRONMENT",
  ],
  SENTRY_ENABLED: ["FRONTEND_SENTRY_ENABLED", "NEXT_PUBLIC_SENTRY_ENABLED"],
  SENTRY_TRACES_SAMPLE_RATE: [
    "FRONTEND_SENTRY_TRACES_SAMPLE_RATE",
    "NEXT_PUBLIC_SENTRY_TRACES_SAMPLE_RATE",
  ],
  SENTRY_REPLAYS_SESSION_SAMPLE_RATE: [
    "FRONTEND_SENTRY_REPLAYS_SESSION_SAMPLE_RATE",
    "NEXT_PUBLIC_SENTRY_REPLAYS_SESSION_SAMPLE_RATE",
  ],
  SENTRY_REPLAYS_ON_ERROR_SAMPLE_RATE: [
    "FRONTEND_SENTRY_REPLAYS_ON_ERROR_SAMPLE_RATE",
    "NEXT_PUBLIC_SENTRY_REPLAYS_ON_ERROR_SAMPLE_RATE",
  ],
  POSTHOG_KEY: ["POSTHOG_KEY", "NEXT_PUBLIC_POSTHOG_KEY"],
  POSTHOG_HOST: ["POSTHOG_HOST", "NEXT_PUBLIC_POSTHOG_HOST"],
  LIFI_INTEGRATOR: ["LIFI_INTEGRATOR", "NEXT_PUBLIC_LIFI_INTEGRATOR"],
};

/**
 * Build-time inlined values (literal reads, replaced by Next at build time).
 * Only consulted when the runtime environment and `window.__PAYVERGE_ENV__`
 * are both silent (for example `next dev` reading NEXT_PUBLIC_* from a local
 * env file). Evaluated lazily so tests can mutate process.env.
 */
function inlinedBuildEnv(): Partial<Record<PublicEnvKey, string | undefined>> {
  return {
    PUBLIC_URL: process.env.NEXT_PUBLIC_PUBLIC_URL,
    API_URL: process.env.NEXT_PUBLIC_API_URL,
    SUPPORT_EMAIL: process.env.NEXT_PUBLIC_SUPPORT_EMAIL,
    NETWORK: process.env.NEXT_PUBLIC_NETWORK,
    RPC_URL: process.env.NEXT_PUBLIC_RPC_URL,
    MAINTENANCE_MODE: process.env.NEXT_PUBLIC_MAINTENANCE_MODE,
    VAPID_PUBLIC_KEY: process.env.NEXT_PUBLIC_VAPID_PUBLIC_KEY,
    MEDIA_ORIGINS: process.env.NEXT_PUBLIC_MEDIA_ORIGINS,
    SENTRY_DSN: process.env.NEXT_PUBLIC_SENTRY_DSN,
    SENTRY_ENVIRONMENT: process.env.NEXT_PUBLIC_SENTRY_ENVIRONMENT,
    SENTRY_ENABLED: process.env.NEXT_PUBLIC_SENTRY_ENABLED,
    SENTRY_TRACES_SAMPLE_RATE:
      process.env.NEXT_PUBLIC_SENTRY_TRACES_SAMPLE_RATE,
    SENTRY_REPLAYS_SESSION_SAMPLE_RATE:
      process.env.NEXT_PUBLIC_SENTRY_REPLAYS_SESSION_SAMPLE_RATE,
    SENTRY_REPLAYS_ON_ERROR_SAMPLE_RATE:
      process.env.NEXT_PUBLIC_SENTRY_REPLAYS_ON_ERROR_SAMPLE_RATE,
    POSTHOG_KEY: process.env.NEXT_PUBLIC_POSTHOG_KEY,
    POSTHOG_HOST: process.env.NEXT_PUBLIC_POSTHOG_HOST,
    LIFI_INTEGRATOR: process.env.NEXT_PUBLIC_LIFI_INTEGRATOR,
  };
}

export type ConfigMode = "production" | "development";

const DEV_PUBLIC_URL = "http://localhost:3000";
const DEV_API_URL = "http://localhost:8080/api/v1";
const SAME_ORIGIN_API_URL = "/api/v1";
const DEFAULT_RPC_URL: Record<string, string> = {
  base: "https://mainnet.base.org",
  baseSepolia: "https://sepolia.base.org",
};
// No vendor fallback: an unset SUPPORT_EMAIL stays empty and callers hide
// the contact affordance instead of pointing users at another deployment.
const DEFAULT_SUPPORT_EMAIL = "";
const DEFAULT_LIFI_INTEGRATOR = "Payverge";
const KNOWN_NETWORKS = new Set(["base", "baseSepolia"]);

function currentMode(): ConfigMode {
  return process.env.NODE_ENV === "production" ? "production" : "development";
}

function clean(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

function firstConfigured(env: EnvLike, names: readonly string[]): string {
  for (const name of names) {
    const value = clean(env[name]);
    if (value) return value;
  }
  return "";
}

/** Hosts where plain http is accepted even in production (local stacks). */
export function isLoopbackHost(hostname: string): boolean {
  const host = hostname.toLowerCase().replace(/^\[|\]$/g, "");
  return (
    host === "localhost" ||
    host.endsWith(".localhost") ||
    host === "127.0.0.1" ||
    host.startsWith("127.") ||
    host === "::1" ||
    host === "0.0.0.0"
  );
}

/**
 * Private-network hosts (RFC 1918 IPv4, IPv6 ULA, mDNS/home/internal names).
 * A restaurant LAN install without TLS (`PUBLIC_URL=http://192.168.1.20:3000`)
 * must still print QR codes and canonical links that point at itself.
 */
function isPrivateNetworkHost(hostname: string): boolean {
  const host = hostname.toLowerCase().replace(/^\[|\]$/g, "");
  const v4 = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/.exec(host);
  if (v4) {
    const [a, b] = [Number(v4[1]), Number(v4[2])];
    return (
      a === 10 || (a === 172 && b >= 16 && b <= 31) || (a === 192 && b === 168)
    );
  }
  if (host.includes(":")) return /^f[cd][0-9a-f]{2}:/.test(host);
  return /\.(?:local|lan|internal|home\.arpa)$/.test(host);
}

/** Hosts where an explicitly configured plain-http URL is accepted in production. */
export function allowsPlainHttp(hostname: string): boolean {
  return isLoopbackHost(hostname) || isPrivateNetworkHost(hostname);
}

type UrlCheck = { ok: true; url: URL } | { ok: false; rule: string };

function checkAbsoluteUrl(value: string, mode: ConfigMode): UrlCheck {
  let url: URL;
  try {
    url = new URL(value);
  } catch {
    return { ok: false, rule: "malformed URL" };
  }
  if (url.protocol !== "https:" && url.protocol !== "http:") {
    return { ok: false, rule: "must use http or https" };
  }
  if (url.username || url.password) {
    return { ok: false, rule: "URL credentials are forbidden" };
  }
  if (
    mode === "production" &&
    url.protocol !== "https:" &&
    !allowsPlainHttp(url.hostname)
  ) {
    return {
      ok: false,
      rule: "HTTPS required in production (except localhost and private-network hosts)",
    };
  }
  return { ok: true, url };
}

function stripTrailingSlashes(value: string): string {
  return value.replace(/\/+$/, "");
}

/**
 * Resolve the raw environment into the full set of public values, applying
 * defaults and dropping anything unsafe (invalid URLs fall back to defaults so
 * a typo can never put `javascript:` or credentials into an href).
 */
function resolveFrom(
  lookup: (key: PublicEnvKey) => string,
  mode: ConfigMode,
  sameOriginApiDefault: boolean,
): PublicEnv {
  const raw = (key: PublicEnvKey) => lookup(key);

  let publicUrl = DEV_PUBLIC_URL;
  const rawPublicUrl = raw("PUBLIC_URL");
  if (rawPublicUrl) {
    const checked = checkAbsoluteUrl(rawPublicUrl, mode);
    if (checked.ok) publicUrl = checked.url.origin;
  }

  let apiUrl =
    mode === "production" || sameOriginApiDefault
      ? SAME_ORIGIN_API_URL
      : DEV_API_URL;
  const rawApiUrl = raw("API_URL");
  if (rawApiUrl) {
    if (rawApiUrl.startsWith("/") && !rawApiUrl.startsWith("//")) {
      apiUrl = stripTrailingSlashes(rawApiUrl) || SAME_ORIGIN_API_URL;
    } else {
      const checked = checkAbsoluteUrl(rawApiUrl, mode);
      if (checked.ok) apiUrl = stripTrailingSlashes(checked.url.toString());
    }
  }

  const rawNetwork = raw("NETWORK");
  const network = KNOWN_NETWORKS.has(rawNetwork) ? rawNetwork : "baseSepolia";

  let rpcUrl = DEFAULT_RPC_URL[network];
  const rawRpcUrl = raw("RPC_URL");
  if (rawRpcUrl) {
    const checked = checkAbsoluteUrl(rawRpcUrl, "development");
    if (checked.ok) rpcUrl = rawRpcUrl;
  }

  const rawSupport = raw("SUPPORT_EMAIL");
  const supportEmail = /^[^\s@<>"']+@[^\s@<>"']+\.[^\s@<>"']+$/.test(rawSupport)
    ? rawSupport
    : DEFAULT_SUPPORT_EMAIL;

  let posthogHost = "";
  const rawPosthogHost = raw("POSTHOG_HOST");
  if (rawPosthogHost) {
    const checked = checkAbsoluteUrl(rawPosthogHost, mode);
    if (checked.ok) posthogHost = checked.url.origin;
  }

  let sentryDsn = "";
  const rawSentryDsn = raw("SENTRY_DSN");
  if (rawSentryDsn) {
    try {
      const dsn = new URL(rawSentryDsn);
      // A DSN legitimately carries its public key in the username slot.
      if (dsn.protocol === "https:" || dsn.protocol === "http:") {
        sentryDsn = rawSentryDsn;
      }
    } catch {
      // malformed DSN: Sentry stays off
    }
  }

  return Object.freeze({
    PUBLIC_URL: publicUrl,
    API_URL: apiUrl,
    SUPPORT_EMAIL: supportEmail,
    NETWORK: network,
    RPC_URL: rpcUrl,
    MAINTENANCE_MODE: raw("MAINTENANCE_MODE") === "true" ? "true" : "false",
    VAPID_PUBLIC_KEY: raw("VAPID_PUBLIC_KEY"),
    MEDIA_ORIGINS: raw("MEDIA_ORIGINS"),
    SENTRY_DSN: sentryDsn,
    SENTRY_ENVIRONMENT: raw("SENTRY_ENVIRONMENT"),
    SENTRY_ENABLED: raw("SENTRY_ENABLED"),
    SENTRY_TRACES_SAMPLE_RATE: raw("SENTRY_TRACES_SAMPLE_RATE"),
    SENTRY_REPLAYS_SESSION_SAMPLE_RATE: raw(
      "SENTRY_REPLAYS_SESSION_SAMPLE_RATE",
    ),
    SENTRY_REPLAYS_ON_ERROR_SAMPLE_RATE: raw(
      "SENTRY_REPLAYS_ON_ERROR_SAMPLE_RATE",
    ),
    POSTHOG_KEY: raw("POSTHOG_KEY"),
    POSTHOG_HOST: posthogHost,
    LIFI_INTEGRATOR: raw("LIFI_INTEGRATOR") || DEFAULT_LIFI_INTEGRATOR,
  });
}

/**
 * Server-side resolution: runtime env (canonical name, then its NEXT_PUBLIC_* twin),
 * then the build-time inlined value, then the default.
 */
export function resolvePublicEnv(
  env: EnvLike,
  mode: ConfigMode = currentMode(),
  options: { includeBuildFallback?: boolean } = {},
): PublicEnv {
  const includeBuildFallback = options.includeBuildFallback ?? true;
  const inlined = includeBuildFallback ? inlinedBuildEnv() : {};
  return resolveFrom(
    (key) =>
      firstConfigured(env, RUNTIME_ENV_NAMES[key]) || clean(inlined[key]),
    mode,
    Boolean(clean(env.BACKEND_INTERNAL_URL)),
  );
}

/**
 * Reads exactly the whitelisted keys from a browser-supplied object. Anything
 * else on `window.__PAYVERGE_ENV__` is ignored.
 */
function resolveFromBrowserObject(
  source: Readonly<Record<string, unknown>>,
  mode: ConfigMode,
): PublicEnv {
  return resolveFrom(
    // An unconfigured PUBLIC_URL is published blank (see
    // serializePublicEnvScript): the page's own origin beats the dev default
    // for links and QR codes built in the browser.
    (key) =>
      clean(source[key]) || (key === "PUBLIC_URL" ? browserOrigin() : ""),
    mode,
    // The server already resolved API_URL; this only matters if the key is
    // missing entirely, where same-origin is the safe production answer.
    false,
  );
}

function browserOrigin(): string {
  try {
    const origin = window.location.origin;
    return /^https?:\/\//.test(origin) ? origin : "";
  } catch {
    return "";
  }
}

export interface PublicConfig {
  /** Canonical public origin, no trailing slash (e.g. https://pos.example). */
  readonly publicUrl: string;
  /** API base: `/api/v1` (same-origin) or an absolute URL, no trailing slash. */
  readonly apiUrl: string;
  readonly supportEmail: string;
  readonly network: "base" | "baseSepolia";
  readonly rpcUrl: string;
  readonly maintenanceMode: boolean;
  readonly vapidPublicKey: string;
  /** Raw comma-separated extra media origins (see aiImageOrigins.ts). */
  readonly mediaOrigins: string;
  readonly sentryDsn: string;
  readonly sentryEnvironment: string;
  readonly sentryEnabled: string;
  readonly sentryTracesSampleRate: string;
  readonly sentryReplaysSessionSampleRate: string;
  readonly sentryReplaysOnErrorSampleRate: string;
  readonly posthogKey: string;
  readonly posthogHost: string;
  readonly lifiIntegrator: string;
}

export function toPublicConfig(env: PublicEnv): PublicConfig {
  return Object.freeze({
    publicUrl: env.PUBLIC_URL,
    apiUrl: env.API_URL,
    supportEmail: env.SUPPORT_EMAIL,
    network: env.NETWORK === "base" ? "base" : "baseSepolia",
    rpcUrl: env.RPC_URL,
    maintenanceMode: env.MAINTENANCE_MODE === "true",
    vapidPublicKey: env.VAPID_PUBLIC_KEY,
    mediaOrigins: env.MEDIA_ORIGINS,
    sentryDsn: env.SENTRY_DSN,
    sentryEnvironment: env.SENTRY_ENVIRONMENT,
    sentryEnabled: env.SENTRY_ENABLED,
    sentryTracesSampleRate: env.SENTRY_TRACES_SAMPLE_RATE,
    sentryReplaysSessionSampleRate: env.SENTRY_REPLAYS_SESSION_SAMPLE_RATE,
    sentryReplaysOnErrorSampleRate: env.SENTRY_REPLAYS_ON_ERROR_SAMPLE_RATE,
    posthogKey: env.POSTHOG_KEY,
    posthogHost: env.POSTHOG_HOST,
    lifiIntegrator: env.LIFI_INTEGRATOR,
  } satisfies PublicConfig);
}

declare global {
  interface Window {
    __PAYVERGE_ENV__?: Readonly<Record<string, unknown>>;
  }
}

let browserCache: { source: object; env: PublicEnv } | null = null;

/**
 * The resolved whitelisted values as an env-keyed object. Server: from the
 * process environment at call time. Browser: from `window.__PAYVERGE_ENV__`,
 * or the build-time fallback when the page was not rendered by the root
 * layout (e.g. unit tests).
 */
export function getPublicEnv(): PublicEnv {
  if (typeof window === "undefined") {
    return resolvePublicEnv(process.env);
  }
  const injected = window.__PAYVERGE_ENV__;
  if (injected && typeof injected === "object") {
    if (browserCache && browserCache.source === injected) {
      return browserCache.env;
    }
    const env = resolveFromBrowserObject(injected, currentMode());
    browserCache = { source: injected, env };
    return env;
  }
  // Nothing injected (unit tests under jsdom, or a page rendered outside the
  // root layout): fall back to whatever `process.env` the bundle exposes —
  // Next's browser polyfill is an empty object, so in a real browser this is
  // just the build-time inlined values plus defaults.
  return resolvePublicEnv(browserProcessEnv(), currentMode());
}

function browserProcessEnv(): EnvLike {
  try {
    return typeof process !== "undefined" && process.env ? process.env : {};
  } catch {
    return {};
  }
}

/** Event the injected config script dispatches on `window` once it has run. */
export const PUBLIC_ENV_READY_EVENT = "payverge:public-env";

/**
 * Run `callback` once the runtime public config can be read.
 *
 * Next loads its main client chunk as an async script and evaluates
 * `instrumentation-client` inside it, so in the browser that module can run
 * before the parser reaches the root layout's inline config script. Anything
 * that reads config at that stage (Sentry init) must wait: on the config
 * script's ready event, or at the latest on DOMContentLoaded, after which a
 * still-missing object means the build-time fallback is all there is.
 * Server side and whenever the config is already present, it runs now.
 */
export function onPublicEnvReady(callback: () => void): void {
  if (
    typeof window === "undefined" ||
    window.__PAYVERGE_ENV__ ||
    typeof document === "undefined" ||
    document.readyState !== "loading"
  ) {
    callback();
    return;
  }
  let done = false;
  const run = () => {
    if (done) return;
    done = true;
    window.removeEventListener(PUBLIC_ENV_READY_EVENT, run);
    document.removeEventListener("DOMContentLoaded", run);
    callback();
  };
  window.addEventListener(PUBLIC_ENV_READY_EVENT, run);
  document.addEventListener("DOMContentLoaded", run);
}

/** Public, browser-safe configuration for the current runtime. */
export function getPublicConfig(): PublicConfig {
  return toPublicConfig(getPublicEnv());
}

/**
 * RPC endpoint for one chain: the configured RPC_URL serves the configured
 * NETWORK; the other chain keeps its public default.
 */
export function getRpcUrlFor(network: "base" | "baseSepolia"): string {
  const config = getPublicConfig();
  return config.network === network ? config.rpcUrl : DEFAULT_RPC_URL[network];
}

/** Canonical origin (no trailing slash). */
export function getSiteUrl(): string {
  return getPublicConfig().publicUrl;
}

/**
 * The site URL the server put in the HTML. Equals getSiteUrl() except in a
 * browser whose server published no PUBLIC_URL: the server rendered the dev
 * default there, while getSiteUrl() returns the page's own origin. A client
 * component rendering the URL must hydrate from this value, then switch
 * (see useSiteUrl), or React keeps the server's text and hrefs.
 */
export function getRenderedSiteUrl(): string {
  if (typeof window !== "undefined") {
    const injected = window.__PAYVERGE_ENV__;
    if (injected && typeof injected === "object" && !clean(injected.PUBLIC_URL)) {
      return DEV_PUBLIC_URL;
    }
  }
  return getSiteUrl();
}

/** Absolute canonical URL for a root-relative path. */
export function absoluteSiteUrl(path = "/"): string {
  const normalized = path.startsWith("/") ? path : `/${path}`;
  return normalized === "/" ? getSiteUrl() : `${getSiteUrl()}${normalized}`;
}

/**
 * Server-only: the explicitly configured runtime value for a public key (no
 * defaults). Used where "unset" must stay distinguishable from a default,
 * e.g. getServerApiUrl() keeping unit tests fetch-free.
 */
function readExplicitPublicEnv(
  key: PublicEnvKey,
  env: EnvLike = process.env,
): string {
  return (
    firstConfigured(env, RUNTIME_ENV_NAMES[key]) ||
    clean(inlinedBuildEnv()[key])
  );
}

/**
 * API base for code running on the Next.js server (route handlers, server
 * components, middleware). Browsers use getPublicConfig().apiUrl instead.
 *
 * Order: INTERNAL_API_URL (full private base) → BACKEND_INTERNAL_URL + /api/v1
 * → absolute API_URL → (production only) PUBLIC_URL + /api/v1, a hairpin
 * through the public edge → "" (nothing configured: callers skip server-side
 * fetches, which keeps unit tests network-free).
 */
export function getServerApiUrl(
  env: EnvLike = process.env,
  mode: ConfigMode = currentMode(),
): string {
  const internal = clean(env.INTERNAL_API_URL);
  if (internal) return stripTrailingSlashes(internal);
  const backend = clean(env.BACKEND_INTERNAL_URL);
  if (backend) return `${stripTrailingSlashes(backend)}/api/v1`;
  const api = readExplicitPublicEnv("API_URL", env);
  if (/^https?:\/\//i.test(api)) return stripTrailingSlashes(api);
  const site =
    mode === "production" ? readExplicitPublicEnv("PUBLIC_URL", env) : "";
  if (site) {
    const checked = checkAbsoluteUrl(site, "development");
    if (checked.ok) {
      const path = api.startsWith("/") ? stripTrailingSlashes(api) : "/api/v1";
      return `${checked.url.origin}${path}`;
    }
  }
  return "";
}

/**
 * Configuration problems, by key and rule only. Values are never included
 * (URLs can carry secrets by mistake). Logged at server start; resolution
 * itself never throws so a typo degrades to defaults instead of a crash loop.
 */
export function validatePublicEnv(
  env: EnvLike,
  mode: ConfigMode = currentMode(),
): string[] {
  const issues: string[] = [];
  const value = (key: PublicEnvKey) =>
    firstConfigured(env, RUNTIME_ENV_NAMES[key]);

  const publicUrl = value("PUBLIC_URL");
  if (!publicUrl) {
    if (mode === "production") issues.push("PUBLIC_URL: missing");
  } else {
    const checked = checkAbsoluteUrl(publicUrl, mode);
    if (!checked.ok) issues.push(`PUBLIC_URL: ${checked.rule}`);
    else if (checked.url.pathname !== "/" || checked.url.search) {
      issues.push("PUBLIC_URL: must be an origin (path and query are ignored)");
    }
  }

  const apiUrl = value("API_URL");
  if (apiUrl && !(apiUrl.startsWith("/") && !apiUrl.startsWith("//"))) {
    const checked = checkAbsoluteUrl(apiUrl, mode);
    if (!checked.ok) issues.push(`API_URL: ${checked.rule}`);
  }

  const rpcUrl = value("RPC_URL");
  if (rpcUrl) {
    const checked = checkAbsoluteUrl(rpcUrl, "development");
    if (!checked.ok) issues.push(`RPC_URL: ${checked.rule}`);
  }

  const network = value("NETWORK");
  if (network && !KNOWN_NETWORKS.has(network)) {
    issues.push("NETWORK: must be base or baseSepolia");
  }

  const support = value("SUPPORT_EMAIL");
  if (support && !/^[^\s@<>"']+@[^\s@<>"']+\.[^\s@<>"']+$/.test(support)) {
    issues.push("SUPPORT_EMAIL: malformed");
  }

  const posthogHost = value("POSTHOG_HOST");
  if (posthogHost) {
    const checked = checkAbsoluteUrl(posthogHost, mode);
    if (!checked.ok) issues.push(`POSTHOG_HOST: ${checked.rule}`);
  }

  const backend = clean(env.BACKEND_INTERNAL_URL);
  if (backend) {
    const checked = checkAbsoluteUrl(backend, "development");
    if (!checked.ok) issues.push(`BACKEND_INTERNAL_URL: ${checked.rule}`);
  } else if (mode === "production" && !/^https?:\/\//i.test(apiUrl ?? "")) {
    // Same-origin API without the proxy: the edge must route /api/v1 and
    // /media, and optimized /media images cannot load (the optimizer fetches
    // them inside this server, where the /media route is off).
    issues.push(
      "BACKEND_INTERNAL_URL: missing (same-origin proxy off; optimized /media images will fail)",
    );
  }

  return issues;
}

/**
 * JSON for an inline <script>. Escapes `<`, `>`, `&` and the JS line
 * separators so a value can never close the script element or break parsing.
 */
export function escapeJsonForScript(json: string): string {
  return json
    .replace(/</g, "\\u003c")
    .replace(/>/g, "\\u003e")
    .replace(/&/g, "\\u0026")
    .replace(/\u2028/g, "\\u2028")
    .replace(/\u2029/g, "\\u2029");
}

/**
 * Inline script body that publishes the whitelisted values. Built by
 * iterating the whitelist, so a non-whitelisted key in `env` cannot leak.
 * It then dispatches PUBLIC_ENV_READY_EVENT for code that started before the
 * parser reached it (see onPublicEnvReady).
 */
export function serializePublicEnvScript(env?: PublicEnv): string {
  const resolved = env ?? getPublicEnv();
  const payload: Record<string, string> = {};
  for (const key of PUBLIC_ENV_KEYS) {
    payload[key] = typeof resolved[key] === "string" ? resolved[key] : "";
  }
  if (!env) {
    // Unset or invalid PUBLIC_URL resolves to the dev default on the server.
    // Publish it blank so the browser uses its own origin instead (startup
    // already logs the problem in production).
    const explicit = readExplicitPublicEnv("PUBLIC_URL");
    if (!explicit || !checkAbsoluteUrl(explicit, currentMode()).ok) {
      payload.PUBLIC_URL = "";
    }
  }
  return (
    `window.__PAYVERGE_ENV__=Object.freeze(${escapeJsonForScript(
      JSON.stringify(payload),
    )});` +
    `if(window.dispatchEvent)window.dispatchEvent(new Event(${JSON.stringify(
      PUBLIC_ENV_READY_EVENT,
    )}));`
  );
}
