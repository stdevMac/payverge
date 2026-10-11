import { readFileSync } from "fs";
import { join } from "path";

describe("MenuBuilder engineering is suggestion-only (#131)", () => {
  const src = readFileSync(join(__dirname, "..", "index.tsx"), "utf8");

  it("does not wire one-click handleProposeReprice / ProposalCard / onPropose", () => {
    expect(src).not.toMatch(/handleProposeReprice/);
    expect(src).not.toMatch(/createPriceChangeProposal/);
    expect(src).not.toMatch(/onPropose=/);
    expect(src).not.toMatch(/<ProposalCard/);
  });
});
