/**
 * Validates that a redirect URL is a safe internal path.
 * Prevents open redirect attacks from localStorage-stored URLs and query params (#296).
 */
export function isSafeRedirectUrl(url: string): boolean {
  if (!url) return false;
  // Reject whitespace / control chars that can disguise scheme-relative URLs.
  if (/[\s\u0000-\u001f\u007f]/.test(url)) return false;
  let decoded = url;
  try {
    decoded = decodeURIComponent(url);
  } catch {
    return false;
  }
  if (/[\s\u0000-\u001f\u007f]/.test(decoded)) return false;
  const pathPart = decoded.split("?")[0] ?? "";
  return (
    decoded.startsWith("/") &&
    !decoded.startsWith("//") &&
    !decoded.startsWith("/\\") &&
    !pathPart.includes(":")
  );
}

/**
 * Resolve a raw `?redirect=` query value into the path we may safely navigate
 * to after login, or `null` when there is nothing safe to honor (caller should
 * fall back to its default destination). Centralizes the open-redirect gate so
 * the post-login decision is unit-testable, not just inline in a JSX callback.
 */
export function resolvePostLoginRedirect(
  raw: string | null | undefined,
): string | null {
  return raw && isSafeRedirectUrl(raw) ? raw : null;
}
