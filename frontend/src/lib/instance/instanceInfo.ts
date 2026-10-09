/**
 * GET /api/v1/instance: what this install is called and which optional
 * integrations it has configured (backend/internal/server/instance_handler.go,
 * docs/api/instance.md). The frontend reads it to brand the UI and hide
 * surfaces whose integration is missing, so a default self-host shows no dead
 * UI and nothing that points at the upstream project.
 *
 * This module has no React and no Node imports: the client hook
 * (src/hooks/useInstance.ts), the server fetcher (./serverInstance.ts) and
 * middleware all share it.
 */

type InstanceRegistrationMode = "invite" | "open" | "closed";

export interface InstanceFeatures {
  readonly ai: boolean;
  readonly whatsapp: boolean;
  readonly telegram: boolean;
  readonly email: boolean;
  readonly google_oauth: boolean;
  readonly crypto: boolean;
  /** Capability, not activation: ARCA surfaces still require country AR. */
  readonly fiscal_ar: boolean;
}

export interface InstanceInfo {
  readonly product_name: string;
  readonly company_name: string;
  readonly legal_entity: string;
  readonly logo_url: string;
  readonly brand_color: string;
  readonly public_url: string;
  readonly support_email: string;
  readonly security_email: string;
  /** Operator's own terms page (LEGAL_TERMS_URL); absolute http(s) or "". */
  readonly legal_terms_url: string;
  /** Operator's own privacy policy (LEGAL_PRIVACY_URL); absolute http(s) or "". */
  readonly legal_privacy_url: string;
  readonly registration_mode: InstanceRegistrationMode;
  readonly email_verification: string;
  readonly features: InstanceFeatures;
  readonly demo: InstanceDemo;
}

/**
 * Demo state. `enabled` = the seeded showroom exists (DEMO_DATA). `mode` =
 * this install IS the public demo (DEMO_MODE): show the demo banner and the
 * one-click demo sign-in. `reset_utc` = nightly reset time ("HH:MM", UTC).
 */
interface InstanceDemo {
  readonly enabled: boolean;
  readonly mode?: boolean;
  readonly reset_utc?: string;
}

const RESET_UTC_RE = /^(?:[01]\d|2[0-3]):[0-5]\d$/;

/** True only when the server confirmed this install is the public demo. */
export function isPublicDemo(info: InstanceInfo | null | undefined): boolean {
  return info?.demo.mode === true;
}

/** Upstream brand defaults; every one is overridable from the backend env. */
const DEFAULT_PRODUCT_NAME = "Payverge";
// eslint-disable-next-line no-restricted-syntax -- the brand default itself (tailwind brand.DEFAULT)
const DEFAULT_BRAND_COLOR = "#1a6b6a";

/** Name of the window global the root layout seeds the client cache with. */
const INSTANCE_WINDOW_KEY = "__PAYVERGE_INSTANCE__";

const HEX_COLOR_RE = /^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6})$/;

function str(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

function bool(value: unknown): boolean {
  return value === true;
}

/** Only an http(s) URL or a same-origin path is usable as a logo/link. */
function safeUrl(value: unknown): string {
  const raw = str(value);
  if (!raw) return "";
  if (raw.startsWith("/") && !raw.startsWith("//")) return raw;
  try {
    const parsed = new URL(raw);
    return parsed.protocol === "https:" || parsed.protocol === "http:"
      ? raw
      : "";
  } catch {
    return "";
  }
}

/** Only an absolute http(s) URL (a relative one could loop back to itself). */
function absoluteUrl(value: unknown): string {
  const raw = safeUrl(value);
  return raw.startsWith("/") ? "" : raw;
}

function email(value: unknown): string {
  const raw = str(value);
  return /^[^\s@?&#]+@[^\s@?&#]+\.[^\s@?&#]+$/.test(raw) ? raw : "";
}

/**
 * Narrow an untrusted /instance body. Returns null when it is not an object
 * or the registration mode is unknown, so callers can tell "unknown" from a
 * confirmed configuration.
 */
export function parseInstanceInfo(raw: unknown): InstanceInfo | null {
  if (!raw || typeof raw !== "object") return null;
  const data = raw as Record<string, unknown>;
  const registration = str(data.registration_mode).toLowerCase();
  if (
    registration !== "invite" &&
    registration !== "open" &&
    registration !== "closed"
  ) {
    return null;
  }
  const f =
    data.features && typeof data.features === "object"
      ? (data.features as Record<string, unknown>)
      : {};
  const demo =
    data.demo && typeof data.demo === "object"
      ? (data.demo as Record<string, unknown>)
      : {};
  const color = str(data.brand_color);
  return {
    product_name: str(data.product_name) || DEFAULT_PRODUCT_NAME,
    company_name: str(data.company_name),
    legal_entity: str(data.legal_entity),
    logo_url: safeUrl(data.logo_url),
    brand_color: HEX_COLOR_RE.test(color) ? color : DEFAULT_BRAND_COLOR,
    public_url: safeUrl(data.public_url).replace(/\/+$/, ""),
    support_email: email(data.support_email),
    security_email: email(data.security_email),
    legal_terms_url: absoluteUrl(data.legal_terms_url),
    legal_privacy_url: absoluteUrl(data.legal_privacy_url),
    registration_mode: registration,
    email_verification: str(data.email_verification),
    features: {
      ai: bool(f.ai),
      whatsapp: bool(f.whatsapp),
      telegram: bool(f.telegram),
      email: bool(f.email),
      google_oauth: bool(f.google_oauth),
      crypto: bool(f.crypto),
      fiscal_ar: bool(f.fiscal_ar),
    },
    demo: {
      enabled: bool(demo.enabled),
      mode: bool(demo.mode),
      reset_utc: RESET_UTC_RE.test(str(demo.reset_utc))
        ? str(demo.reset_utc)
        : bool(demo.mode)
          ? "03:00"
          : "",
    },
  };
}

/** The name to show for the product; never empty. */
export function productNameOf(info: InstanceInfo | null | undefined): string {
  return info?.product_name || DEFAULT_PRODUCT_NAME;
}

/** The public name of this site: company → product. Never empty. */
export function siteNameOf(info: InstanceInfo | null | undefined): string {
  return info?.company_name || productNameOf(info);
}

/** Who the legal template speaks for: legal entity → company → product. */
export function legalNameOf(info: InstanceInfo | null | undefined): string {
  return (
    info?.legal_entity || info?.company_name || productNameOf(info)
  );
}

/**
 * JS that seeds `window.__PAYVERGE_INSTANCE__` (JSON-escaped against
 * `</script>`). Null serializes as null so the client hook fetches instead.
 */
export function serializeInstanceScript(info: InstanceInfo | null): string {
  const json = JSON.stringify(info)
    .replace(/</g, "\\u003c")
    .replace(/>/g, "\\u003e")
    .replace(/&/g, "\\u0026")
    .replace(/\u2028/g, "\\u2028")
    .replace(/\u2029/g, "\\u2029");
  return `window.${INSTANCE_WINDOW_KEY}=${json};`;
}

/** The seed the root layout wrote, parsed again (never trusted blindly). */
export function readInstanceSeed(): InstanceInfo | null {
  if (typeof window === "undefined") return null;
  try {
    const seed = (window as unknown as Record<string, unknown>)[
      INSTANCE_WINDOW_KEY
    ];
    return parseInstanceInfo(seed);
  } catch {
    return null;
  }
}

/**
 * `r g b` channels for the --pv-brand-rgb CSS variable, or null when the
 * install keeps the default colour (the Tailwind fallback already is it).
 */
export function brandRgbChannels(
  info: InstanceInfo | null | undefined,
): string | null {
  const color = info?.brand_color;
  if (!color || !HEX_COLOR_RE.test(color)) return null;
  let hex = color.slice(1).toLowerCase();
  if (hex.length === 3) hex = hex.replace(/./g, (c) => c + c);
  if (`#${hex}` === DEFAULT_BRAND_COLOR) return null;
  const n = parseInt(hex, 16);
  return `${(n >> 16) & 255} ${(n >> 8) & 255} ${n & 255}`;
}
