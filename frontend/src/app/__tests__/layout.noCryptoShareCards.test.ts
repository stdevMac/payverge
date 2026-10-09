import fs from "node:fs";
import path from "node:path";

describe("marketing share cards without live-crypto claims (#931)", () => {
  it("drops USDC and crypto-payments keywords from the root layout", () => {
    const src = fs.readFileSync(
      path.join(__dirname, "../layout.tsx"),
      "utf8",
    );
    expect(src).not.toMatch(/USDC where enabled/i);
    expect(src).not.toMatch(/crypto payments for restaurants/i);
  });
});
