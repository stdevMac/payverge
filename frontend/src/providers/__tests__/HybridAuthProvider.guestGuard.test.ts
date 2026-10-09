/** @jest-environment node */
import fs from "node:fs";
import path from "node:path";

const SOURCE = fs.readFileSync(
  path.resolve(__dirname, "../HybridAuthProvider.tsx"),
  "utf-8",
);

describe("HybridAuthProvider — guest-route guard on /auth/refresh", () => {
  test("imports the shared isGuestRoute helper", () => {
    expect(SOURCE).toMatch(/from\s+["']@\/utils\/guestRoute["']/);
  });

  test("imports the shared public path helper", () => {
    expect(SOURCE).toMatch(/from\s+["']@\/utils\/publicPaths["']/);
    expect(SOURCE).toContain("CLIENT_PUBLIC_PREFIXES");
    expect(SOURCE).toContain("CLIENT_PROTECTED_PREFIXES");
    expect(SOURCE).toContain("matchesPublicPrefix");
  });

  test("short-circuits performRefresh when on a guest route", () => {
    // performRefresh body must reference isGuestRoute as an early return.
    // Match a window of ~500 chars after `performRefresh = useCallback`.
    const match = SOURCE.match(/performRefresh\s*=\s*useCallback[\s\S]{0,500}/);
    expect(match?.[0]).toMatch(/isGuestRoute\(\)/);
  });

  test("uses the shared refreshAuthSession helper for proactive refresh", () => {
    expect(SOURCE).toMatch(/from\s+["']@\/utils\/refreshAuth["']/);
    expect(SOURCE).toContain("await refreshAuthSession(apiUrl)");
    expect(SOURCE).toMatch(
      /onSessionExpired:[\s\S]{0,200}localStorage\.removeItem\(SESSION_HINT_KEY\)/,
    );
  });

  test("keeps operator chrome mounted on a hinted session retry (#621)", () => {
    const match = SOURCE.match(
      /retrySessionBootstrap\s*=\s*useCallback[\s\S]{0,900}/,
    );
    expect(match?.[0]).toMatch(/keepChrome/);
    expect(match?.[0]).toMatch(/SESSION_HINT_KEY/);
    expect(SOURCE).toMatch(
      /if \(isInitialized && user && initAttemptedRef\.current\)/,
    );
  });

  test("treats unrecovered rotation as a transient proactive-refresh failure", () => {
    expect(SOURCE).toContain("return result;");
    expect(SOURCE).toContain("if (refreshed.ok)");
    expect(SOURCE).not.toContain(
      "return result.ok || (!result.ok && result.alreadyRotated)",
    );
  });

  test("short-circuits anonymous non-protected bootstraps before session-info", () => {
    expect(SOURCE).toContain("function hasSessionHint()");
    expect(SOURCE).toMatch(
      /!\s*hasSessionHint\(\)\s*&&\s*\(isPublicSurface\s*\|\|\s*!\s*isProtectedSurface\)/,
    );
    // Guard must precede the *bootstrap* session-info call. refreshSession()
    // (P0-1 funnel hydration) also awaits getSessionInfo and may appear earlier
    // in the file — compare relative to the guard site, not file-wide first hit.
    const guardIdx = SOURCE.indexOf(
      "!hasSessionHint() && (isPublicSurface || !isProtectedSurface)",
    );
    expect(guardIdx).toBeGreaterThan(-1);
    const bootstrapSessionInfo = SOURCE.indexOf(
      "await getSessionInfo()",
      guardIdx,
    );
    expect(bootstrapSessionInfo).toBeGreaterThan(guardIdx);
  });
});

