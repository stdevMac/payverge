// Returns true when the current page (or the explicitly-provided pathname)
// is a public diner route — the instance root, the storefront (`/b/<customUrl>`), a table
// (`/t/<tableCode>`), a delivery-tracking deep link (`/delivery/<n>/track`),
// or a reservation confirm/cancel deep link (`/reservations/<code>`). These
// routes are rendered for unauthenticated diners — any /auth/refresh call
// against them deterministically 401s, wipes the client cache, and fires a
// spurious auth:session-expired event. Keep this in sync with the diner
// entries of CLIENT_PUBLIC_PREFIXES in publicPaths.ts.
//
// Pass `pathname` explicitly when running outside the browser (SSR) or
// when guarding logic that runs before the URL is available.
export function isGuestRoute(pathname?: string): boolean {
  const path =
    pathname ??
    (typeof window !== "undefined" ? window.location.pathname : "");
  // The instance root ("/", "/es", "/es-ar") serves a venue page or the
  // venue directory — a diner surface too.
  return (
    /^\/(?:b|t|delivery|reservations)\//.test(path) ||
    (path !== "" && /^(?:\/(?:es|es-ar))?\/?$/i.test(path))
  );
}
