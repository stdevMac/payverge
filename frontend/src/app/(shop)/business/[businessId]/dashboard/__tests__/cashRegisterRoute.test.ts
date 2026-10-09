/** @jest-environment node */
import fs from "node:fs";
import path from "node:path";

const SOURCE = fs.readFileSync(
  path.resolve(__dirname, "../page.tsx"),
  "utf-8",
);

// Rail definitions (loader + membership) moved to the tab registry in the
// dashboard-tabs consolidation; page.tsx consumes TAB_KEYS/TAB_REGISTRY.
const REGISTRY = fs.readFileSync(
  path.resolve(
    __dirname,
    "../../../../../../components/business/tabs/tabRegistry.tsx",
  ),
  "utf-8",
);

describe("dashboard cash-register route integration", () => {
  it("allows cash-register as a valid dashboard tab", () => {
    // validTabs derives from the registry keys (single source of truth).
    expect(SOURCE).toMatch(/const\s+validTabs[\s\S]*?TAB_KEYS/);
    expect(REGISTRY).toMatch(/"cash-register":\s*\{/);
  });

  it("renders CashRegisterDashboard for cash-register instead of falling through to tab-not-found", () => {
    // The code-split binding lives in the registry; the page switch consumes it.
    expect(REGISTRY).toMatch(
      /const\s+cashRegisterLoader[^=]*=\s*\(\)\s*=>\s*import\("\.\.\/CashRegisterDashboard"\)/,
    );
    expect(REGISTRY).toMatch(/const\s+CashRegisterDashboard\s*=\s*dynamic\(/);
    expect(SOURCE).toMatch(/case\s+["']cash-register["']\s*:/);
    expect(SOURCE).toMatch(/<CashRegisterDashboard\s+businessId=\{numericBusinessId\.toString\(\)\}/);
  });
});
