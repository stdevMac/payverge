// Did the tab end up on the page the shot asked for? The app may add or drop
// query params (locale, normalized tabs), which is fine — but a pathname
// change means we were redirected away (most commonly AuthGate bouncing an
// expired session from /business/... to the signed-out /dashboard page), and
// capturing that would silently save a login screen instead of the shot.
// If the request pinned a ?tab= and the destination shows a DIFFERENT tab,
// that's a bounce too (e.g. staff permissions falling back to overview).
export function landedOnRequestedPage(requestedPath, actualUrl) {
  let requested;
  let actual;
  try {
    requested = new URL(requestedPath, "https://placeholder.invalid");
    actual = new URL(actualUrl);
  } catch {
    return false;
  }
  if (normalizePath(requested.pathname) !== normalizePath(actual.pathname)) {
    return false;
  }
  const requestedTab = requested.searchParams.get("tab");
  const actualTab = actual.searchParams.get("tab");
  if (requestedTab && actualTab && requestedTab !== actualTab) {
    return false;
  }
  return true;
}

function normalizePath(pathname) {
  return pathname.replace(/\/+$/, "") || "/";
}
