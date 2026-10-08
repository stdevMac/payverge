import fs from "fs";
import path from "path";

/**
 * L5-31 residual halves after 75256d70f: covered by Root A (stacking) +
 * Root C (document Escape). Regression: kiosk still uses useDialogKeyboard
 * and a high fixed z above the top menu once Root A untraps stacking.
 */
describe("KioskClockIn L5-31 regression", () => {
  const kioskSrc = fs.readFileSync(
    path.join(__dirname, "KioskClockIn.tsx"),
    "utf8",
  );
  const hookSrc = fs.readFileSync(
    path.join(__dirname, "useDialogKeyboard.ts"),
    "utf8",
  );
  const layoutSrc = fs.readFileSync(
    path.join(__dirname, "..", "DashboardLayout.tsx"),
    "utf8",
  );

  it("uses useDialogKeyboard for Escape (Root C document listener)", () => {
    expect(kioskSrc).toMatch(/useDialogKeyboard\(dialogRef,\s*onClose\)/);
    expect(hookSrc).toMatch(/document\.addEventListener\("keydown"/);
  });

  it("kiosk fixed layer is z-[60]+ and dashboard no longer traps at z-[1]", () => {
    expect(kioskSrc).toMatch(/z-\[60\]/);
    expect(layoutSrc).not.toMatch(
      /data-testid="dashboard-shell"[\s\S]{0,1500}className="relative z-\[1\] flex h-full flex-col"/,
    );
  });
});
