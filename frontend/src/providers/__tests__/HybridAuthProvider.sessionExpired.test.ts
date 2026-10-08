/** @jest-environment node */
import fs from "node:fs";
import path from "node:path";

const SOURCE = fs.readFileSync(
  path.resolve(__dirname, "../HybridAuthProvider.tsx"),
  "utf-8",
);

describe("HybridAuthProvider — session death keeps the page mounted (P1-10)", () => {
  const handlerWindow = (): string => {
    const match = SOURCE.match(
      /const handleSessionExpired = \(\) => \{[\s\S]{0,1200}?\n    \};/,
    );
    expect(match).not.toBeNull();
    return match![0];
  };

  test("the interceptor path no longer hard-navigates or force-logs-out", () => {
    const handler = handlerWindow();
    expect(handler).not.toContain("window.location.href");
    expect(handler).not.toContain('clearSession("refresh_failed")');
  });

  test("the interceptor path surfaces the re-auth modal instead", () => {
    expect(handlerWindow()).toContain("setIsSessionExpired(true)");
  });

  test("the interceptor path stops proactive refresh and the session hint", () => {
    const handler = handlerWindow();
    expect(handler).toContain("refreshCleanupRef.current?.()");
    expect(handler).toContain(
      'localStorage.removeItem("payverge_had_session")',
    );
  });

  test("navigation still happens only from the modal CTA", () => {
    const modal = SOURCE.match(/<SessionTimeoutWarning[\s\S]{0,800}?\/>/);
    expect(modal?.[0]).toContain("router.replace");
  });
});
