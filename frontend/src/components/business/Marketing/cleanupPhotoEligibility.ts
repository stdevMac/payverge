import { isOwnMediaUrl } from "@/lib/media/ownMedia";

/**
 * L4-20 — "Mejorar esta foto" only works for assets the backend can fetch
 * through the SSRF allowlist (own public bucket host) or read straight from
 * its own store (same-origin /media/<key> uploads). Demo Unsplash URLs
 * always 400; gate the button client-side with the same host class.
 */

const UNSAFE_HOST_SUFFIXES = [
  "images.unsplash.com",
  "plus.unsplash.com",
  "source.unsplash.com",
];

/**
 * True when the URL looks like a re-hostable gallery asset (https + not a
 * known external stock host). When NEXT_PUBLIC_S3 public host is unknown we
 * still block Unsplash deterministically; other https hosts remain allowed so
 * operator-uploaded CDN variants are not wrongly disabled.
 */
export function isCleanupEligiblePhotoUrl(rawUrl: string): boolean {
  const trimmed = rawUrl.trim();
  if (!trimmed) return false;
  // Uploads on the default (local) storage driver: relative /media/<key>, or
  // ${PUBLIC_URL}/media/<key> on this origin (may be http on a LAN install).
  if (isOwnMediaUrl(trimmed)) return true;
  let host = "";
  try {
    const u = new URL(trimmed);
    if (u.protocol !== "https:") return false;
    host = u.hostname.toLowerCase();
  } catch {
    return false;
  }
  if (!host) return false;
  return !UNSAFE_HOST_SUFFIXES.some(
    (suffix) => host === suffix || host.endsWith(`.${suffix}`),
  );
}
