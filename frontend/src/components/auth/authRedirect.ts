import { resolvePostLoginRedirect } from "@/utils/safeRedirect";
import type { Locale } from "@/i18n/localeRegistry";

/**
 * Builds the destination AuthGate sends unauthenticated users to. Targets
 * /dashboard, which renders the full email+password AuthModal — unlike
 * /staff/login (Google + 6-digit code only), which dead-ends email/password
 * owners. The original path is preserved as `redirect` so the destination can
 * forward post-login once it consumes the param.
 */
export function buildAuthRedirectUrl(pathname: string): string {
  return `/dashboard?redirect=${encodeURIComponent(pathname)}`;
}

/** Recovery entry that keeps a validated same-origin post-login destination. */
export function buildForgotPasswordHref(redirect?: string | null): string {
  const safe = resolvePostLoginRedirect(redirect);
  if (!safe) return "/forgot-password";
  return `/forgot-password?redirect=${encodeURIComponent(safe)}`;
}

/**
 * Login return from recovery; preserves the original protected destination.
 * Non-English recovery opens the sign-in dialog and carries the language as
 * ?lang= (/dashboard has no locale-prefixed variant, and "/" is the venue).
 */
export function buildLoginReturnHref(
  redirect?: string | null,
  locale?: Locale | null,
): string {
  const params: string[] = [];
  if (locale && locale !== "en") {
    params.push("auth=signin", `lang=${encodeURIComponent(locale)}`);
  }
  const safe = resolvePostLoginRedirect(redirect);
  if (safe) params.push(`redirect=${encodeURIComponent(safe)}`);
  return params.length ? `/dashboard?${params.join("&")}` : "/dashboard";
}
