/**
 * Same-origin API proxy: `${PUBLIC_URL}/api/v1/*` → `${BACKEND_INTERNAL_URL}/api/v1/*`.
 * Off (404) unless BACKEND_INTERNAL_URL is set. Contract and limits live in
 * src/lib/proxy/backendProxy.ts and docs/self-hosting/frontend-config.md.
 */
import { createProxyHandlers } from "@/lib/proxy/backendProxy";

export const dynamic = "force-dynamic";
export const runtime = "nodejs";

const handlers = createProxyHandlers({ prefix: "/api/v1/" });

export const GET = handlers.GET;
export const HEAD = handlers.HEAD;
export const POST = handlers.POST;
export const PUT = handlers.PUT;
export const PATCH = handlers.PATCH;
export const DELETE = handlers.DELETE;
export const OPTIONS = handlers.OPTIONS;
