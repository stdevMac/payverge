/**
 * Guest table routes must not statically pull LI.FI / AiWaiter into the
 * first-load chunk. These are source gates: the heavy modules stay behind
 * next/dynamic import() so the bundler can split them.
 */
import fs from "fs";
import path from "path";

function read(relative: string): string {
  return fs.readFileSync(path.join(__dirname, "..", relative), "utf8");
}

const AI_WAITER_VALUE_IMPORT =
  /^import\s+\{[^}]*\bAiWaiter\b[^}]*\}\s+from\s+["'][^"']*\/AiWaiter["']/m;

describe("guest route lazy imports", () => {
  it("loads PaymentProcessor with next/dynamic, not a static value import", () => {
    const src = read("components/guest/PaymentSection.tsx");
    expect(src).not.toMatch(/^import\s+PaymentProcessor\s+from\s+/m);
    expect(src).not.toMatch(
      /from\s+["']\.\.\/payment\/PaymentProcessor["']/,
    );
    expect(src).toContain('import("../payment/PaymentProcessor")');
  });

  it("reads USDC addresses from @/constants/usdc, not lib/lifi/config", () => {
    const src = read("components/payment/PaymentProcessor.tsx");
    expect(src).not.toMatch(/from\s+["'][^"']*lib\/lifi\/config["']/);
    expect(src).toMatch(/from\s+["']@\/constants\/usdc["']/);
  });

  it("loads AiWaiter dynamically on the guest menu", () => {
    const src = read("app/t/[tableCode]/menu/page.tsx");
    expect(src).not.toMatch(AI_WAITER_VALUE_IMPORT);
    expect(src).toMatch(/import\(\s*["'][^"']*\/AiWaiter["']\s*\)/);
  });

  it("loads AiWaiter dynamically on the business landing page", () => {
    const src = read("components/business-page/ConvertingBusinessLandingPage.tsx");
    expect(src).not.toMatch(AI_WAITER_VALUE_IMPORT);
    expect(src).toMatch(/import\(\s*["'][^"']*\/AiWaiter["']\s*\)/);
  });
});
