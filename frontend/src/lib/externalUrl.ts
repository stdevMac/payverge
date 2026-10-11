// Operator-configured external URLs (partner tracking links, courier deep
// links) are untrusted data on guest surfaces: they are rendered as anchor
// hrefs on unauthenticated pages, so a stored `javascript:` or `data:` value
// would be stored XSS. Only real web URLs are ever linked; anything else is
// rendered as plain text or dropped.
export function isSafeExternalTrackingUrl(value: string | undefined | null): boolean {
  if (!value) return false;
  try {
    const url = new URL(value);
    return url.protocol === "https:" || url.protocol === "http:";
  } catch {
    return false;
  }
}
