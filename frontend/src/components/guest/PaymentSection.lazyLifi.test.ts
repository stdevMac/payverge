/** @jest-environment node */
import fs from "node:fs";
import path from "node:path";

const SOURCE = fs.readFileSync(
  path.resolve(__dirname, "PaymentSection.tsx"),
  "utf-8",
);

describe("PaymentSection — defers @lifi/sdk via lazy CrossChainPayment (P-1)", () => {
  it("does not statically import CrossChainPayment", () => {
    // A static `import CrossChainPayment from "../payment/CrossChainPayment"`
    // pulls @lifi/sdk into the guest bill chunk for everyone.
    expect(SOURCE).not.toMatch(
      /^import\s+CrossChainPayment\s+from\s+["']\.\.\/payment\/CrossChainPayment["'];?\s*$/m,
    );
  });

  it("loads CrossChainPayment through next/dynamic with ssr:false", () => {
    expect(SOURCE).toMatch(
      /const\s+CrossChainPayment\s*=\s*dynamic\(\s*\(\)\s*=>\s*import\(\s*["']\.\.\/payment\/CrossChainPayment["']\s*\)[\s\S]*?ssr:\s*false/,
    );
  });

  it("only mounts CrossChainPayment when the cross-chain modal is open", () => {
    // Conditional mounting is what actually keeps the @lifi chunk off first
    // paint — a permanently-mounted dynamic() would still load on render.
    expect(SOURCE).toMatch(
      /\{isCrossChainPaymentOpen\s*&&\s*\(\s*<CrossChainPayment/,
    );
  });
});
