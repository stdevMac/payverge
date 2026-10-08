/**
 * Returns the preferred URL identifier for a business.
 * Prefers string business_id, falls back to numeric id.
 */
function getBusinessUrl(business: {
  business_id?: string;
  id: number;
}): string {
  return business.business_id || String(business.id);
}

/**
 * The cross-venue overview (venue picker, totals, "register a new venue").
 * Plain `/dashboard` sends an operator with exactly one venue straight into
 * that venue, so every "back to all venues" control must use this path or it
 * bounces the operator back where they came from.
 */
export const VENUES_OVERVIEW_PATH = "/dashboard?venues=all";

/** Full-page navigation to the cross-venue overview. */
export function navigateToVenuesOverview(): void {
  // eslint-disable-next-line @next/next/no-location-assign-relative-destination -- deliberate full-page navigation (see doc comment)
  window.location.href = VENUES_OVERVIEW_PATH;
}

/**
 * Builds the dashboard path for a business.
 */
export function getBusinessDashboardPath(business: {
  business_id?: string;
  id: number;
}): string {
  return `/business/${getBusinessUrl(business)}/dashboard`;
}

/**
 * Operator dashboard path from a `/business/[businessId]` route param
 * (numeric id or slug). Never returns the public custom-URL protocol path
 * `/business/:id` — that segment is the storefront shim in `route.ts`.
 */
export function getOperatorDashboardPath(
  businessId: string,
  tab?: string,
): string {
  const base = `/business/${businessId}/dashboard`;
  return tab ? `${base}?tab=${encodeURIComponent(tab)}` : base;
}

/**
 * True when `pathname` is the public custom-URL protocol shim:
 * `/business/:id` with no further operator segments.
 *
 * `/business/register` is the signup flow, not a tenant slug.
 */
export function isPublicBusinessProtocolPath(pathname: string): boolean {
  if (!pathname) return false;
  const path = pathname.split(/[?#]/, 1)[0].replace(/\/+$/, "") || "/";
  if (path === "/business" || path === "/business/register") return false;
  return /^\/business\/[^/]+$/.test(path);
}

/** Opens the focused first task after workspace creation. */
export function getBusinessFirstValuePath(business: {
  business_id?: string;
  id: number;
}): string {
  return `${getBusinessDashboardPath(business)}?tab=menu&onboarding=first-value`;
}

/**
 * The sub-tab keys inside BusinessSettings. The dashboard mounts
 * <BusinessSettings/> at `?tab=settings`; this `section` query param then
 * selects the initial sub-tab so deep links (e.g. the email "manage
 * notifications" link) can land directly on Notifications. Keep in sync with
 * the `tabs` array in BusinessSettings.tsx.
 */
export const SETTINGS_SECTIONS = [
  "profile",
  "payments",
  "localization",
  "notifications",
] as const;

export type SettingsSection = (typeof SETTINGS_SECTIONS)[number];

/**
 * Resolves the BusinessSettings sub-tab from a `?section=` query value,
 * defaulting to "profile" for missing/unknown values so a bad deep link
 * degrades gracefully instead of rendering a blank panel.
 */
export function resolveInitialSettingsTab(
  section: string | null | undefined,
): SettingsSection {
  return (SETTINGS_SECTIONS as readonly string[]).includes(section ?? "")
    ? (section as SettingsSection)
    : "profile";
}

/**
 * Deep-links to a business's Settings → Notifications sub-tab. Used by the
 * operator account page to route each business to its email/notification
 * preferences.
 */
export function getBusinessNotificationSettingsPath(business: {
  business_id?: string;
  id: number;
}): string {
  return `${getBusinessDashboardPath(business)}?tab=settings&section=notifications`;
}

/** Sub-tabs inside BusinessPageEditor (`?tab=business-page&section=`). */
export const BUSINESS_PAGE_SECTIONS = [
  "essentials",
  "design",
  "operations",
  "reviews",
  "contact",
] as const;

export type BusinessPageSection = (typeof BUSINESS_PAGE_SECTIONS)[number];

/**
 * Builds the dashboard deep-link that mounts <BusinessPageEditor/> (the
 * "Look & feel" / Business Page editor). The dashboard reads `?tab=` via
 * useSearchParams and renders BusinessPageEditor when tab === "business-page"
 * (dashboard/page.tsx case "business-page"). There is no standalone
 * `/business/<id>/design` route — that path 404s.
 *
 * Optional `section` deep-links into a Business Page sub-tab (e.g. Contact
 * from Settings → Profile) so operators don't land on Essentials and have to
 * hunt for the Contact editor (#225).
 */
export function getBusinessPageEditorPath(
  business: {
    business_id?: string;
    id: number;
  },
  section?: BusinessPageSection,
): string {
  const base = `${getBusinessDashboardPath(business)}?tab=business-page`;
  if (
    section &&
    section !== "essentials" &&
    (BUSINESS_PAGE_SECTIONS as readonly string[]).includes(section)
  ) {
    return `${base}&section=${section}`;
  }
  return base;
}
