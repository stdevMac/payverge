/**
 * Own-media URLs: public uploads this deployment serves itself at
 * `/media/<key>` (backend local driver, or the s3 driver without
 * S3_PUBLIC_BASE_URL). The backend persists them relative (`/media/<key>`)
 * unless PUBLIC_URL makes them absolute (`${PUBLIC_URL}/media/<key>`, the
 * site's own origin). Allowlists that only accept absolute https URLs on a
 * bucket/CDN host must also accept these, or every upload on a default
 * install is treated as foreign.
 *
 * Mirrors the backend's s3.IsOwnMediaURL: the key must satisfy the strict
 * object-key grammar, and query strings, fragments, credentials, dot
 * segments and protocol-relative `//host/...` forms never qualify.
 */

const MEDIA_PREFIX = "/media/";
const MAX_KEY_LENGTH = 1024;
const KEY_CHARS = /^[A-Za-z0-9\-_.~!()+,=@/]+$/;
const RELATIVE_BASE = "https://own-media.invalid";

function currentOrigin(): string | undefined {
  return typeof window !== "undefined" && window.location
    ? window.location.origin
    : undefined;
}

function isValidMediaKey(key: string): boolean {
  if (!key || key.length > MAX_KEY_LENGTH || !KEY_CHARS.test(key)) {
    return false;
  }
  return key
    .split("/")
    .every((segment) => segment.length > 0 && !segment.startsWith("."));
}

/**
 * True when `raw` is a same-origin `/media/<key>` URL: relative, or absolute
 * on `siteOrigin` (defaults to the browser's origin; on the server only the
 * relative form qualifies).
 */
export function isOwnMediaUrl(
  raw: string,
  siteOrigin: string | undefined = currentOrigin(),
): boolean {
  if (typeof raw !== "string" || !raw || raw !== raw.trim()) return false;
  if (/[\\\u0000-\u001f\u007f]/u.test(raw)) return false;

  let path: string;
  if (raw.startsWith("/")) {
    if (raw.startsWith("//")) return false;
    let url: URL;
    try {
      url = new URL(raw, RELATIVE_BASE);
    } catch {
      return false;
    }
    // Any normalization (dot segments, query, fragment, escapes) means the
    // string is not the plain path the backend persists.
    if (url.origin !== RELATIVE_BASE || url.pathname !== raw) return false;
    path = url.pathname;
  } else {
    if (!siteOrigin) return false;
    let url: URL;
    let site: URL;
    try {
      url = new URL(raw);
      site = new URL(siteOrigin);
    } catch {
      return false;
    }
    if (url.protocol !== "http:" && url.protocol !== "https:") return false;
    if (url.username || url.password) return false;
    if (url.origin !== site.origin) return false;
    if (raw !== `${url.origin}${url.pathname}`) return false;
    path = url.pathname;
  }

  if (!path.startsWith(MEDIA_PREFIX)) return false;
  return isValidMediaKey(path.slice(MEDIA_PREFIX.length));
}

/**
 * The relative `/media/<key>` form only. Use it where the decision must not
 * depend on the runtime origin (code that also renders on the server), next
 * to a check that already accepts absolute http(s) URLs.
 */
export function isRelativeOwnMediaUrl(raw: string): boolean {
  return isOwnMediaUrl(raw, "");
}
