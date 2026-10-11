import fs from "fs";
import path from "path";

/**
 * L3-13: there is no `modal-fade` class — Root B is the real NextUI fix.
 * Hand-rolled cmdk-fade is 120ms; verify overlays use pointer-events-aware
 * patterns or short fades that do not leave a permanent swallow layer.
 */
describe("L3-13 hand-rolled fade analogues", () => {
  it("documents that modal-fade class does not exist in the app", () => {
    // Scan a few known overlay homes — none should reference modal-fade.
    const files = [
      "src/components/business/schedule/ShiftEditorModal.tsx",
      "src/components/business/schedule/ScheduleSettingsModal.tsx",
      "src/components/business/commandPalette/CommandPalette.tsx",
    ];
    for (const rel of files) {
      const src = fs.readFileSync(path.join(__dirname, "..", "..", rel), "utf8");
      expect(src).not.toMatch(/modal-fade/);
      // cmdk-fade is the 120ms enter only — acceptable; Root B covers NextUI.
      if (rel.includes("ShiftEditor") || rel.includes("ScheduleSettings")) {
        expect(src).toMatch(/cmdk-fade_120ms/);
      }
    }
  });
});
