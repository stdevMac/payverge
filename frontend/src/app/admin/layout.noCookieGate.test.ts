/**
 * Source gate: admin layout must not reintroduce an SSR cookies()/session-info
 * bounce that assumes parent-domain COOKIE_DOMAIN=.payverge.io (rejected by
 * SEC-5 / #302 / #314). Soft UX redirect lives in AdminLayoutClient; API
 * AuthenticationAdminMiddleware is the security boundary.
 */
import { readFileSync } from "fs";
import path from "path";

describe("admin layout — no frontend-host cookie gate (#288 redesign)", () => {
  const source = readFileSync(
    path.join(__dirname, "layout.tsx"),
    "utf8",
  );

  // Strip block + line comments so documentation mentioning cookies()/session-info
  // does not falsely trip the gate.
  const codeOnly = source
    .replace(/\/\*[\s\S]*?\*\//g, "")
    .replace(/^\s*\/\/.*$/gm, "");

  it("does not import next/headers or call cookies() for an SSR bounce", () => {
    expect(codeOnly).not.toMatch(/from\s+["']next\/headers["']/);
    expect(codeOnly).not.toMatch(/\bcookies\s*\(/);
    expect(codeOnly).not.toMatch(/from\s+["']next\/navigation["']/);
    expect(codeOnly).not.toMatch(/\bredirect\s*\(/);
  });

  it("does not call /auth/session-info or requireAdminSession", () => {
    expect(codeOnly).not.toMatch(/session-info/);
    expect(codeOnly).not.toMatch(/requireAdminSession/);
    expect(codeOnly).not.toMatch(/getServerApiUrl/);
  });

  it("documents host-only cookie scope on a separate API host (no parent-domain assumption)", () => {
    // Deployment-neutral example host: any split-origin API (api.example.com).
    expect(source).toMatch(/api\.example\.com/);
    expect(source).toMatch(/host-only/);
    // Must not instruct operators to set a parent-domain COOKIE_DOMAIN.
    expect(source).not.toMatch(/COOKIE_DOMAIN=\./);
  });

  it("delegates chrome gating to AdminLayoutClient", () => {
    expect(codeOnly).toMatch(/AdminLayoutClient/);
  });
});

describe("AdminLayoutClient — defer AdminShell chunk (#288)", () => {
  const source = readFileSync(
    path.join(__dirname, "AdminLayoutClient.tsx"),
    "utf8",
  );
  const codeOnly = source
    .replace(/\/\*[\s\S]*?\*\//g, "")
    .replace(/^\s*\/\/.*$/gm, "");

  it("lazy-loads AdminShell via next/dynamic (not a static import)", () => {
    expect(codeOnly).toMatch(/from\s+["']next\/dynamic["']/);
    expect(codeOnly).toMatch(/dynamic\s*\(/);
    expect(codeOnly).toMatch(/AdminShell/);
    expect(codeOnly).not.toMatch(
      /import\s*\{\s*AdminShell\s*\}\s*from\s+["']@\/components\/admin\/AdminShell["']/,
    );
  });
});
