/**
 * Brand, community and support links — the single home for every outbound
 * contact/support/social link the frontend renders.
 *
 * Defaults are deliberately conservative for a self-hosted install:
 *   - support points at the public GitHub repository;
 *   - no contact mailbox, no social profile, no phone number, no WhatsApp,
 *     no chat handle, no booking calendar;
 *   - every optional entry that is unset is HIDDEN by the UI — never rendered
 *     as a dead link and never silently pointed at somebody else's inbox
 *     (including the upstream project's own accounts).
 *
 * Hook-up: the contact mailbox follows the deployment's runtime SUPPORT_EMAIL
 * (`getPublicConfig().supportEmail`: process env on the server,
 * window.__PAYVERGE_ENV__ in the browser). Unset or malformed, it stays empty
 * and every mailto: entry is hidden. Further operator-facing sources (e.g.
 * GET /api/v1/instance) feed `resolveBrandLinks(overrides)` the same way,
 * without touching any call site.
 */

import { getPublicConfig } from "./publicConfig";

/** Public source repository for the open-source project. */
export const PROJECT_REPO_URL = "https://github.com/stdevMac/payverge";

export interface BrandLinks {
  /** Source repository. Always set. */
  readonly repoUrl: string;
  /** Where operators and visitors ask for help. Always set. */
  readonly supportUrl: string;
  /** Telegram chat/channel. Optional — hidden when unset. */
  readonly telegramUrl?: string;
  /** WhatsApp click-to-chat link. Optional — hidden when unset. */
  readonly whatsappUrl?: string;
  /** Booking page for a support/discovery call. Optional — hidden when unset. */
  readonly bookCallUrl?: string;
  /** Public contact mailbox. Optional — mailto entries hidden when unset. */
  readonly contactEmail?: string;
}

export type BrandLinkOverrides = {
  readonly [K in keyof BrandLinks]?: string | null;
};

// Only the two always-present links have defaults. Everything else — the
// contact mailbox (SUPPORT_EMAIL) included — stays unset
// until the operator configures it, so a zero-config install renders no
// mailto: and no social link that would route its guests and staff to
// someone else.
export const DEFAULT_BRAND_LINKS: BrandLinks = Object.freeze({
  repoUrl: PROJECT_REPO_URL,
  supportUrl: `${PROJECT_REPO_URL}/issues`,
});

const URL_KEYS = [
  "repoUrl",
  "supportUrl",
  "telegramUrl",
  "whatsappUrl",
  "bookCallUrl",
] as const;

/** Returns the trimmed URL when it is an absolute http(s) URL, else undefined. */
export function sanitizeBrandUrl(raw: unknown): string | undefined {
  if (typeof raw !== "string") return undefined;
  const value = raw.trim();
  if (!value) return undefined;
  try {
    const parsed = new URL(value);
    if (parsed.protocol !== "https:" && parsed.protocol !== "http:") {
      return undefined;
    }
    if (parsed.username || parsed.password) return undefined;
    return value;
  } catch {
    return undefined;
  }
}

/** Returns the trimmed address when it looks like one mailbox, else undefined. */
export function sanitizeBrandEmail(raw: unknown): string | undefined {
  if (typeof raw !== "string") return undefined;
  const value = raw.trim();
  if (!/^[^\s@?&#]+@[^\s@?&#]+\.[^\s@?&#]+$/.test(value)) return undefined;
  return value;
}

/**
 * Merge overrides onto the defaults. An override of `""` or `null` explicitly
 * clears an optional entry (hiding it); a malformed value is ignored and falls
 * back to the default. `repoUrl` and `supportUrl` can be replaced but never
 * cleared — the UI always has somewhere to send people for help.
 */
export function resolveBrandLinks(
  overrides: BrandLinkOverrides = {},
): BrandLinks {
  const resolved: Record<string, string | undefined> = {
    ...DEFAULT_BRAND_LINKS,
  };

  for (const key of URL_KEYS) {
    if (!(key in overrides)) continue;
    const raw = overrides[key];
    const cleared = raw === null || (typeof raw === "string" && !raw.trim());
    if (cleared) {
      if (key !== "repoUrl" && key !== "supportUrl") resolved[key] = undefined;
      continue;
    }
    const value = sanitizeBrandUrl(raw);
    if (value) resolved[key] = value;
  }

  if ("contactEmail" in overrides) {
    const raw = overrides.contactEmail;
    if (raw === null || (typeof raw === "string" && !raw.trim())) {
      resolved.contactEmail = undefined;
    } else {
      const value = sanitizeBrandEmail(raw);
      if (value) resolved.contactEmail = value;
    }
  }

  return Object.freeze(resolved) as unknown as BrandLinks;
}

/**
 * The links every component renders. Read once at module load, which in the
 * browser is after the root layout's config script has run (it precedes
 * every page client module; see docs/self-hosting/frontend-config.md).
 */
export const brandLinks: BrandLinks = resolveBrandLinks({
  contactEmail: getPublicConfig().supportEmail,
});
