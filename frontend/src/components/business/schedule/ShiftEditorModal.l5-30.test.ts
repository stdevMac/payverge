import fs from "fs";
import path from "path";

describe("ShiftEditorModal + ScheduleSettingsModal L5-30", () => {
  const shift = fs.readFileSync(
    path.join(__dirname, "ShiftEditorModal.tsx"),
    "utf8",
  );
  const settings = fs.readFileSync(
    path.join(__dirname, "ScheduleSettingsModal.tsx"),
    "utf8",
  );

  it("ShiftEditor routes Esc through dirty discard confirm", () => {
    expect(shift).toMatch(/confirmingDiscard/);
    expect(shift).toMatch(/requestClose/);
    expect(shift).toMatch(
      /useDialogKeyboard\(dialogRef,\s*requestClose,\s*!confirmingDiscard\)/,
    );
    expect(shift).toMatch(/ConfirmationModal/);
  });

  it("ScheduleSettingsModal has Esc handler + dirty discard", () => {
    expect(settings).toMatch(
      /useDialogKeyboard\(dialogRef,\s*requestClose,\s*!confirmingDiscard\)/,
    );
    expect(settings).toMatch(/confirmingDiscard/);
    expect(settings).toMatch(/ConfirmationModal/);
  });
});
