import fs from "fs";
import path from "path";

const SOURCE = fs.readFileSync(
  path.join(__dirname, "../CounterManager.tsx"),
  "utf8",
);

describe("Counter settings mobile header (#466)", () => {
  it("stacks the Disable action instead of clipping it in one row", () => {
    expect(SOURCE).toMatch(/flex-col gap-3 sm:flex-row sm:items-center sm:justify-between/);
    expect(SOURCE).not.toMatch(
      /flex items-center justify-between gap-4[\s\S]{0,80}settings\.title/,
    );
  });
});
