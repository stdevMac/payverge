/**
 * Complete App Router page inventory for Payverge frontend.
 * Generated from `find src/app -name page.tsx` (74 routes) and annotated for crawl.
 *
 * Tokens: {businessId}, {customUrl}, {tableCode}, {billId},
 * {deliveryNumber}, {confirmationCode}, {token}
 */

type PageAuth = "none" | "owner" | "admin" | "staff" | "skip";

export interface AppPage {
  id: string;
  /** URL path template (App Router path without route groups) */
  path: string;
  label: string;
  auth: PageAuth;
  /** Notes for the report when skip or partial */
  note?: string;
  /** Group for reporting */
  group:
    | "public"
    | "auth"
    | "owner"
    | "guest"
    | "admin"
    | "staff"
    | "dynamic-skip";
}

/** Static / resolvable pages we always crawl when tokens are available. */
export const APP_PAGES: AppPage[] = [
  // ── Public ──────────────────────────────────────────────────────
  { id: "page.home", path: "/", label: "Venue home", auth: "none", group: "public" },
  {
    id: "page.privacy",
    path: "/privacy-policy",
    label: "Privacy",
    auth: "none",
    group: "public",
  },
  {
    id: "page.terms",
    path: "/terms-and-conditions",
    label: "Terms",
    auth: "none",
    group: "public",
  },
  { id: "page.refund", path: "/refund", label: "Refund policy", auth: "none", group: "public" },
  {
    id: "page.unsubscribe",
    path: "/unsubscribe",
    label: "Unsubscribe",
    auth: "none",
    group: "public",
  },
  { id: "page.scan", path: "/scan", label: "Scan", auth: "none", group: "public" },

  // ── Auth / account ────────────────────────────────────────────────
  {
    id: "page.forgot-password",
    path: "/forgot-password",
    label: "Forgot password",
    auth: "none",
    group: "auth",
  },
  {
    id: "page.reset-password",
    path: "/reset-password",
    label: "Reset password",
    auth: "none",
    group: "auth",
  },
  {
    id: "page.verify-email",
    path: "/verify-email",
    label: "Verify email",
    auth: "none",
    group: "auth",
  },
  {
    id: "page.business.register",
    path: "/business/register",
    label: "Business register",
    auth: "none",
    group: "auth",
  },
  {
    id: "page.staff.login",
    path: "/staff/login",
    label: "Staff login",
    auth: "none",
    group: "auth",
  },
  {
    id: "page.staff.accept",
    path: "/staff/accept-invitation",
    label: "Staff accept invitation",
    auth: "none",
    group: "auth",
  },
  {
    id: "page.account",
    path: "/account",
    label: "Account",
    auth: "owner",
    group: "owner",
  },
  {
    id: "page.dashboard",
    path: "/dashboard",
    label: "Owner dashboard hub",
    auth: "owner",
    group: "owner",
  },

  // ── Owner business surfaces (beyond tab= crawl) ───────────────────
  {
    id: "page.business.public",
    path: "/business/{businessId}",
    label: "Business ID public redirect",
    auth: "none",
    group: "guest",
  },
  {
    id: "page.business.printers",
    path: "/business/{businessId}/settings/printers",
    label: "Printers settings page",
    auth: "owner",
    group: "owner",
  },
  {
    id: "page.b.custom",
    path: "/b/{customUrl}",
    label: "Public business page",
    auth: "none",
    group: "guest",
  },

  // ── Guest table ───────────────────────────────────────────────────
  {
    id: "page.t.home",
    path: "/t/{tableCode}",
    label: "Guest table home",
    auth: "none",
    group: "guest",
  },
  {
    id: "page.t.menu",
    path: "/t/{tableCode}/menu",
    label: "Guest table menu",
    auth: "none",
    group: "guest",
  },
  {
    id: "page.t.bill",
    path: "/t/{tableCode}/bill",
    label: "Guest table bill",
    auth: "none",
    group: "guest",
  },
  {
    id: "page.t.profile",
    path: "/t/{tableCode}/profile",
    label: "Guest table profile",
    auth: "none",
    group: "guest",
  },

  // ── Platform admin (local admin@local.test is role=admin) ─────────
  { id: "page.admin", path: "/admin", label: "Admin home", auth: "admin", group: "admin" },
  {
    id: "page.admin.analytics",
    path: "/admin/analytics",
    label: "Admin analytics",
    auth: "admin",
    group: "admin",
  },
  {
    id: "page.admin.businesses",
    path: "/admin/businesses",
    label: "Admin businesses",
    auth: "admin",
    group: "admin",
  },
  {
    id: "page.admin.users",
    path: "/admin/users",
    label: "Admin users",
    auth: "admin",
    group: "admin",
  },
  {
    id: "page.admin.emails",
    path: "/admin/emails",
    label: "Admin emails",
    auth: "admin",
    group: "admin",
  },
  {
    id: "page.admin.errors",
    path: "/admin/errors",
    label: "Admin errors",
    auth: "admin",
    group: "admin",
  },
  {
    id: "page.admin.escalations",
    path: "/admin/escalations",
    label: "Admin escalations",
    auth: "admin",
    group: "admin",
  },
  {
    id: "page.admin.fiscal",
    path: "/admin/fiscal",
    label: "Admin fiscal",
    auth: "admin",
    group: "admin",
  },
  {
    id: "page.admin.plugins",
    path: "/admin/plugins",
    label: "Admin plugins",
    auth: "admin",
    group: "admin",
  },
  {
    id: "page.admin.stripe",
    path: "/admin/stripe",
    label: "Admin stripe",
    auth: "admin",
    group: "admin",
  },
  {
    id: "page.admin.translations",
    path: "/admin/translations",
    label: "Admin translations",
    auth: "admin",
    group: "admin",
  },
  {
    id: "page.admin.demo",
    path: "/admin/demo",
    label: "Admin demo",
    auth: "admin",
    group: "admin",
  },

  // ── Staff (OTP seed required — crawl login only unless staff session) ─
  {
    id: "page.staff.home",
    path: "/staff/home",
    label: "Staff home",
    auth: "staff",
    group: "staff",
    note: "Requires staff OTP session; expected redirect to login without it",
  },

  // ── Dynamic (resolved at crawl time when tokens exist) ────────────
  {
    id: "page.delivery.track",
    path: "/delivery/{deliveryNumber}/track",
    label: "Delivery track",
    auth: "none",
    group: "guest",
  },
  {
    id: "page.delivery.pay",
    path: "/delivery/{deliveryNumber}/pay",
    label: "Delivery pay",
    auth: "none",
    group: "guest",
  },
  {
    id: "page.alt-payments",
    path: "/business/{businessId}/bills/{billId}/alternative-payments",
    label: "Bill alternative payments",
    auth: "owner",
    group: "owner",
  },
  {
    id: "page.reservation",
    path: "/reservations/{confirmationCode}",
    label: "Reservation confirm",
    auth: "skip",
    group: "dynamic-skip",
    note: "Needs confirmation code from live booking",
  },
  {
    id: "page.join",
    path: "/join/{token}",
    label: "Join invite",
    auth: "skip",
    group: "dynamic-skip",
    note: "Needs invite token",
  },
];

export function expandPagePath(
  template: string,
  tokens: Record<string, string | number>,
): string {
  return template.replace(/\{(\w+)\}/g, (_, key: string) => {
    const v = tokens[key];
    if (v === undefined || v === null || v === "") {
      throw new Error(`Missing token {${key}} for ${template}`);
    }
    return String(v);
  });
}

export function pathNeedsTokens(template: string): string[] {
  return [...template.matchAll(/\{(\w+)\}/g)].map((m) => m[1]);
}
