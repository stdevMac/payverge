import fs from "fs";
import path from "path";

describe("PluginManager L6-32", () => {
  const src = fs.readFileSync(
    path.join(__dirname, "..", "PluginManager.tsx"),
    "utf8",
  );

  it("guards config modal close with useDirtyForm + ConfirmationModal", () => {
    expect(src).toMatch(/useDirtyForm\(pluginConfig\)/);
    expect(src).toMatch(/confirmDiscardConfig/);
    expect(src).toMatch(/ConfirmationModal/);
    expect(src).toMatch(/markConfigClean/);
  });
});
