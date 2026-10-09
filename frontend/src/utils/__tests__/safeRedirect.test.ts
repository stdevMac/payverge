import { isSafeRedirectUrl, resolvePostLoginRedirect } from "../safeRedirect";

describe("isSafeRedirectUrl", () => {
  it("allows simple internal paths", () => {
    expect(isSafeRedirectUrl("/dashboard")).toBe(true);
    expect(isSafeRedirectUrl("/business/123/dashboard")).toBe(true);
    expect(isSafeRedirectUrl("/staff/login")).toBe(true);
  });

  it("allows paths with query params including colons", () => {
    expect(isSafeRedirectUrl("/search?time=10:30")).toBe(true);
  });

  it("rejects protocol-relative URLs", () => {
    expect(isSafeRedirectUrl("//evil.com")).toBe(false);
    expect(isSafeRedirectUrl("/%2f%2fevil.com")).toBe(false);
    expect(isSafeRedirectUrl("/\\evil.com")).toBe(false);
    expect(isSafeRedirectUrl(" /dashboard")).toBe(false);
  });

  it("rejects absolute URLs with protocols", () => {
    expect(isSafeRedirectUrl("https://evil.com")).toBe(false);
    expect(isSafeRedirectUrl("http://evil.com")).toBe(false);
  });

  it("rejects javascript: URIs", () => {
    expect(isSafeRedirectUrl("javascript:alert(1)")).toBe(false);
  });

  it("rejects empty strings", () => {
    expect(isSafeRedirectUrl("")).toBe(false);
  });

  it("rejects paths with colons in the path segment", () => {
    expect(isSafeRedirectUrl("/foo:bar")).toBe(false);
  });
});

describe("resolvePostLoginRedirect", () => {
  it("returns a safe internal path verbatim (including its query string)", () => {
    expect(resolvePostLoginRedirect("/business/123/dashboard?tab=bills")).toBe(
      "/business/123/dashboard?tab=bills",
    );
    expect(resolvePostLoginRedirect("/dashboard")).toBe("/dashboard");
  });

  it("returns null for external/open-redirect values (caller falls back to default)", () => {
    expect(resolvePostLoginRedirect("https://evil.com")).toBeNull();
    expect(resolvePostLoginRedirect("//evil.com")).toBeNull();
    expect(resolvePostLoginRedirect("javascript:alert(1)")).toBeNull();
  });

  it("returns null for absent/empty input", () => {
    expect(resolvePostLoginRedirect(null)).toBeNull();
    expect(resolvePostLoginRedirect(undefined)).toBeNull();
    expect(resolvePostLoginRedirect("")).toBeNull();
  });
});
