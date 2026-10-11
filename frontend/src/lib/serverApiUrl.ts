/**
 * API base for code that runs on the Next.js server.
 *
 * Browser code uses getPublicConfig().apiUrl (same-origin `/api/v1` by
 * default). On the server a relative URL is useless and, in Docker, localhost
 * points at the frontend container, so server-side fetches resolve the private
 * backend: INTERNAL_API_URL → BACKEND_INTERNAL_URL/api/v1 → absolute API_URL
 * → PUBLIC_URL/api/v1. See src/config/publicConfig.ts.
 */
export { getServerApiUrl } from "@/config/publicConfig";
