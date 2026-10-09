/**
 * Static image hosts for CSP `img-src`, derived from the same manifest that
 * feeds next/image `remotePatterns` (src/config/imageOrigins.json), so the
 * two can never drift.
 *
 * The manifest is deliberately generic: Google account avatars and the
 * Unsplash photos used by demo data (issue 346). Uploads are served
 * same-origin from `/media/*` ('self'). Deployment-specific hosts (an S3 or
 * CDN bucket) are added at RUNTIME through MEDIA_ORIGINS, which the CSP
 * builder (src/lib/security/csp.ts) appends per request; add them to the
 * manifest only when next/image should also optimize them (build-time).
 */
import manifest from "./imageOrigins.json";

export function imageCspOrigins(): string[] {
  const out = new Set<string>();
  for (const entry of manifest as Array<{ protocol: string; hostname: string }>) {
    out.add(`${entry.protocol}://${entry.hostname.toLowerCase()}`);
  }
  return [...out];
}
