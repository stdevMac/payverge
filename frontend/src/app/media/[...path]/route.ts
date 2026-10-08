/**
 * Same-origin media: `${PUBLIC_URL}/media/*` → `${BACKEND_INTERNAL_URL}/media/*`
 * (uploads served by the backend's local storage driver). Read-only; off
 * (404) unless BACKEND_INTERNAL_URL is set. See src/lib/proxy/backendProxy.ts.
 */
import { proxyToBackend } from "@/lib/proxy/backendProxy";

export const dynamic = "force-dynamic";
export const runtime = "nodejs";

const options = { prefix: "/media/", methods: ["GET", "HEAD"] } as const;

export function GET(request: Request): Promise<Response> {
  return proxyToBackend(request, options);
}

export function HEAD(request: Request): Promise<Response> {
  return proxyToBackend(request, options);
}
