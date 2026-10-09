import { useParams } from "next/navigation";

/**
 * Returns the business identifier to use in USER-VISIBLE navigation URLs
 * (router.push/replace, window.location, Link hrefs, redirects).
 *
 * It prefers the identifier already in the address bar — the `[businessId]`
 * route segment, which is the business slug for every authenticated entry point
 * — and only falls back to the provided id if the route segment is absent.
 *
 * Why: hidden XHR/fetch API calls may freely use the numeric business id (the
 * backend resolves slug-or-id), but the numeric id must never surface in the
 * URL bar or redirects. Components that hold only a numeric `businessId` prop
 * use this to keep navigation slug-based without threading a slug prop through.
 */
export function useBusinessUrlId(fallback: string | number): string {
  const params = useParams();
  const routeId = params?.businessId;
  const segment = Array.isArray(routeId) ? routeId[0] : routeId;
  return segment || String(fallback);
}
