import fs from "node:fs";
import path from "node:path";

describe("core hook dependency policy", () => {
  it("does not suppress dependency analysis in shared lifecycle hooks", () => {
    const files = ["useDialogBehavior.ts", "useHashTabs.ts"];
    const offenders = files.filter((file) =>
      /eslint-disable[^\n]*react-hooks\/exhaustive-deps/.test(
        fs.readFileSync(path.resolve(__dirname, file), "utf8"),
      ),
    );

    expect(offenders).toEqual([]);
  });
});
