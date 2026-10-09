import {
  CLIENT_AUTH_RENDER_THROUGH_PREFIXES,
  CLIENT_PROTECTED_PREFIXES,
  CLIENT_PUBLIC_PREFIXES,
  EDGE_PUBLIC_PREFIXES,
  matchesPublicPrefix,
} from "../publicPaths";

describe("public path matching", () => {
  it("does not make every route below a locale prefix public", () => {
    expect(matchesPublicPrefix("/es-ar/dashboard", CLIENT_PUBLIC_PREFIXES)).toBe(
      false,
    );
  });

  it("keeps the instance home and legal pages public", () => {
    for (const route of [
      "/",
      "/privacy-policy",
      "/refund",
      "/terms-and-conditions",
      "/b/aurora",
    ]) {
      expect(matchesPublicPrefix(route, EDGE_PUBLIC_PREFIXES)).toBe(true);
      expect(matchesPublicPrefix(route, CLIENT_PUBLIC_PREFIXES)).toBe(true);
    }
  });

  it("keeps dashboard self-gated without classifying it as public", () => {
    expect(matchesPublicPrefix("/dashboard", CLIENT_PUBLIC_PREFIXES)).toBe(false);
    expect(matchesPublicPrefix("/dashboard", EDGE_PUBLIC_PREFIXES)).toBe(false);
    expect(matchesPublicPrefix("/dashboard", CLIENT_AUTH_RENDER_THROUGH_PREFIXES)).toBe(true);
  });

  it("classifies dashboard as protected so first-login session-info fires", () => {
    // Regression: /dashboard is the default post-OAuth landing surface. It must
    // count as protected so HybridAuthProvider's init guard calls
    // /auth/session-info even with no local session hint — otherwise a
    // first-time login (fresh device / incognito / cleared storage) lands with
    // a valid httpOnly cookie that is never checked and looks logged out.
    expect(matchesPublicPrefix("/dashboard", CLIENT_PROTECTED_PREFIXES)).toBe(true);
    expect(matchesPublicPrefix("/dashboard?tab=overview", CLIENT_PROTECTED_PREFIXES)).toBe(true);
    // It stays self-gated so AuthGate still renders the signed-out value-prop
    // for anonymous visitors instead of hard-redirecting them.
    expect(matchesPublicPrefix("/dashboard", CLIENT_AUTH_RENDER_THROUGH_PREFIXES)).toBe(true);
  });

  it("protects the installed-app resolver from every public render path", () => {
    expect(matchesPublicPrefix("/app", EDGE_PUBLIC_PREFIXES)).toBe(false);
    expect(matchesPublicPrefix("/app", CLIENT_PUBLIC_PREFIXES)).toBe(false);
    expect(
      matchesPublicPrefix("/app", CLIENT_AUTH_RENDER_THROUGH_PREFIXES),
    ).toBe(false);
    expect(matchesPublicPrefix("/app", CLIENT_PROTECTED_PREFIXES)).toBe(true);
  });

  it("does not let the root prefix make every route public", () => {
    expect(matchesPublicPrefix("/inside", CLIENT_PUBLIC_PREFIXES)).toBe(false);
  });

  it("matches non-slashed prefixes on segment boundaries", () => {
    expect(matchesPublicPrefix("/refund", CLIENT_PUBLIC_PREFIXES)).toBe(true);
    expect(matchesPublicPrefix("/refunds", CLIENT_PUBLIC_PREFIXES)).toBe(
      false,
    );
  });

  it("keeps the password-reset flow public so AuthGate does not redirect unauthenticated users", () => {
    const resetRoutes = [
      "/forgot-password",
      "/reset-password",
      "/reset-password?token=abc123",
    ];

    for (const route of resetRoutes) {
      expect(matchesPublicPrefix(route, EDGE_PUBLIC_PREFIXES)).toBe(true);
      expect(matchesPublicPrefix(route, CLIENT_PUBLIC_PREFIXES)).toBe(true);
    }
  });

  it("keeps scan public because it is a guest entry point", () => {
    expect(matchesPublicPrefix("/scan", EDGE_PUBLIC_PREFIXES)).toBe(true);
    expect(matchesPublicPrefix("/scan", CLIENT_PUBLIC_PREFIXES)).toBe(true);
  });

  it("does not carry dead prefixes for routes that no longer exist", () => {
    for (const dead of [
      "/auth",
      "/menu",
      "/qr",
      "/pricing",
      "/blog",
      "/contact",
      "/tools",
      "/custom-intake",
      "/es-ar/pricing",
    ]) {
      expect(matchesPublicPrefix(dead, EDGE_PUBLIC_PREFIXES)).toBe(false);
      expect(matchesPublicPrefix(dead, CLIENT_PUBLIC_PREFIXES)).toBe(false);
    }
  });
});
