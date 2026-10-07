/**
 * Single source of truth for public (unauthenticated) path prefixes consumed
 * by `frontend/src/components/auth/AuthGate.tsx` (client render-gate) and
 * `frontend/src/providers/HybridAuthProvider.tsx` (session bootstrap guard).
 * NOTE: `frontend/src/middleware.ts` does NOT consume these lists — it only
 * does route rewriting + CSP; there is no edge cookie-gate today.
 *
 * Two lists exist because the layers have different responsibilities:
 *
 * - `EDGE_PUBLIC_PREFIXES` — the historical base list of genuinely public
 *   guest/legal/auth routes. Kept as the base set that CLIENT_PUBLIC_PREFIXES
 *   extends; every entry must correspond to a real route under src/app.
 *
 * - `CLIENT_PUBLIC_PREFIXES` — AuthGate gates based on authenticated
 *   session-info. A path here means "render without requiring an
 *   authenticated principal." This list is a SUPERSET of the edge list and
 *   adds paths that are genuinely public in the client app (e.g.
 *   /verify-email reads a token from the URL; /reservations/:code identifies
 *   the guest by confirmation code, not session).
 *
 * - `CLIENT_AUTH_RENDER_THROUGH_PREFIXES` — routes that are not public, but
 *   own their own authenticated/unauthenticated UI. AuthGate must not blank
 *   the whole route-group chrome before those pages can render.
 *
 * Divergence between the two lists should be intentional, not accidental.
 * When adding a public path:
 *   - If the route needs to render without ANY auth (cookie or otherwise)
 *     → add to CLIENT_PUBLIC_PREFIXES only.
 *   - If the route should ALSO bypass the edge cookie check
 *     → add to both EDGE_PUBLIC_PREFIXES and CLIENT_PUBLIC_PREFIXES.
 */
export const EDGE_PUBLIC_PREFIXES: readonly string[] = [
  // "/" is the instance home: the primary venue, a venue directory, or a
  // redirect to /dashboard when nothing is published yet.
  "/",
  "/privacy-policy",
  "/refund",
  "/terms-and-conditions",
  "/scan",
  "/staff/login",
  "/staff/accept-invitation",
  "/forgot-password",
  "/reset-password",
  "/register",
  "/business/register",
  "/unsubscribe", // tokenized no-auth email opt-out (P2-10)
  "/t/",
  "/b/",
  "/delivery/",
  "/images/",
];

export const CLIENT_PUBLIC_PREFIXES: readonly string[] = [
  ...EDGE_PUBLIC_PREFIXES,
  "/verify-email",
  "/reservations/",
];

export const CLIENT_AUTH_RENDER_THROUGH_PREFIXES: readonly string[] = [
  "/dashboard",
];

export const CLIENT_PROTECTED_PREFIXES: readonly string[] = [
  "/app",
  // `/dashboard` is the default post-OAuth landing surface. It renders its own
  // signed-out value-prop (so it is ALSO in CLIENT_AUTH_RENDER_THROUGH_PREFIXES,
  // which keeps AuthGate from redirecting anonymous visitors away), but it must
  // count as "protected" here so HybridAuthProvider's init guard fires
  // /auth/session-info even when there is no local session hint yet. Without
  // this, a first-time login (fresh device / incognito / cleared storage /
  // post-logout) lands on /dashboard with a valid httpOnly session_token cookie
  // that is never checked — the guest appears logged out right after signing in.
  "/dashboard",
  "/business/",
  "/admin",
  "/account",
  // /profile/ is diner CRM (customer JWT) — public to AuthGate; do NOT list as
  // operator-protected or guests bounce to the business dashboard.
];

/**
 * Returns true if `pathname` is covered by one of `prefixes`. The root "/"
 * must match exactly (not via startsWith) to avoid making every path public.
 */
export function matchesPublicPrefix(
  pathname: string,
  prefixes: readonly string[],
): boolean {
  return prefixes.some((p) => {
    if (p === "/") {
      return pathname === "/";
    }
    if (p.endsWith("/")) {
      return pathname.startsWith(p);
    }
    if (pathname === p) {
      return true;
    }
    if (!pathname.startsWith(p)) {
      return false;
    }
    // Require a real boundary after the prefix so "/contacted" doesn't match
    // "/contact", but "/reset-password?token=..." and "/blog/post" do.
    const next = pathname.charAt(p.length);
    return next === "/" || next === "?" || next === "#";
  });
}
