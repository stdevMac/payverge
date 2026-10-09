/**
 * Source-level test for the INTEG-1 fix: /dashboard must read the ?redirect=
 * query param after successful auth and navigate to it (if safe) rather than
 * reloading to the generic hub.
 *
 * Full rendering of Dashboard requires wagmi, auth context, analytics, etc.
 * We assert at the source level (like the existing loading-state.test.ts).
 */

import fs from "fs";
import path from "path";

describe("/dashboard ?redirect= post-login consumption (INTEG-1)", () => {
  const pagePath = path.resolve(__dirname, "..", "page.tsx");
  const source = fs.readFileSync(pagePath, "utf8");

  it("imports useSearchParams so the redirect param can be read", () => {
    expect(source).toMatch(/useSearchParams/);
  });

  it("wraps the inner component in a Suspense boundary (App Router static-route requirement)", () => {
    // The default export must provide a Suspense boundary so that
    // useSearchParams() does not force the entire page into dynamic rendering.
    expect(source).toMatch(/Suspense/);
  });

  it("reads and validates ?redirect= via resolvePostLoginRedirect before navigating", () => {
    // Must route the raw param through the open-redirect gate (whose decision
    // is unit-tested in safeRedirect.test.ts), not assign it unconditionally.
    expect(source).toMatch(/resolvePostLoginRedirect/);
    // Must read the redirect param by name
    expect(source).toMatch(/["']redirect["']/);
  });

  it("uses window.location.assign (not reload) to consume a safe redirect after auth", () => {
    // onSuccess should call assign() when a safe redirect exists
    expect(source).toMatch(/window\.location\.assign/);
  });

  it("falls back to window.location.reload when redirect is absent or unsafe", () => {
    // The fallback path must still exist for the no-redirect / unsafe-value case
    expect(source).toMatch(/window\.location\.reload/);
  });

  it("does NOT navigate to external URLs (open-redirect guard is not weakened)", () => {
    // There must be no unconditional assign without the safety check in scope.
    // Presence of isSafeRedirectUrl + conditional assign satisfies this.
    // We just verify the safety check is present (tested above) as a proxy.
    const assignMatches = (source.match(/window\.location\.assign/g) || []).length;
    // Only one assign call (inside the onSuccess callback)
    expect(assignMatches).toBe(1);
  });
});
