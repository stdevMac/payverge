/**
 * Guest diner chrome (table QR, storefront, scan) — not marketing/operator.
 * Locale prefixes /es and /es-ar are stripped so /es-ar/t/:code matches.
 */
export function isGuestChromePath(pathname?: string | null): boolean {
  const raw =
    pathname ??
    (typeof window !== "undefined" ? window.location.pathname : "");
  const path = raw.replace(/^\/(es-ar|es)(?=\/|$)/i, "") || "/";
  return (
    // Instance root: venue page or venue directory.
    path === "/" ||
    path.startsWith("/t/") ||
    path.startsWith("/b/") ||
    path === "/scan" ||
    path.startsWith("/scan/")
  );
}
