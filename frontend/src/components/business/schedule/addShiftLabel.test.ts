import { addShiftAccessibleName } from "./addShiftLabel";

describe("addShiftAccessibleName (#448)", () => {
  it("interpolates target and localized date", () => {
    const name = addShiftAccessibleName(
      "Add shift for {target} on {date}",
      "Dana Ruiz",
      "2026-07-08",
      "en",
    );
    expect(name).toMatch(/Dana Ruiz/);
    expect(name).toMatch(/Jul/);
    expect(name).toMatch(/8/);
  });

  it("falls back to unique suffix when the template has no placeholders", () => {
    const a = addShiftAccessibleName("addShiftFor", "Servers", "2026-07-06", "en");
    const b = addShiftAccessibleName("addShiftFor", "Cooks", "2026-07-07", "en");
    expect(a).not.toBe(b);
    expect(a).toContain("Servers");
    expect(b).toContain("Cooks");
  });
});
