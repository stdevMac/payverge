import { interpolateTranslatedViewNotice } from "./translatedViewNotice";

describe("L3-6 interpolateTranslatedViewNotice", () => {
  it("fills both {viewLanguage} and {editLanguage} (no residual braces)", () => {
    const out = interpolateTranslatedViewNotice(
      "You're viewing the {viewLanguage} translation — read-only. Switch to {editLanguage} to edit.",
      "Español",
      "English",
    );
    expect(out).toBe(
      "You're viewing the Español translation — read-only. Switch to English to edit.",
    );
    expect(out).not.toMatch(/\{/);
  });

  it("legacy dual {language}: first is viewed, second is edit language", () => {
    const out = interpolateTranslatedViewNotice(
      "You're viewing the {language} translation — read-only. Switch to {language} to edit.",
      "Español",
      "English",
    );
    expect(out).toBe(
      "You're viewing the Español translation — read-only. Switch to English to edit.",
    );
    expect(out).not.toContain("{language}");
  });

  it("single-arg String.replace would leave the second placeholder literal", () => {
    // Documents the pre-fix defect the helper replaces.
    const template =
      "You're viewing the {language} translation — read-only. Switch to {language} to edit.";
    const broken = template.replace("{language}", "English");
    expect(broken).toContain("{language}");
    const fixed = interpolateTranslatedViewNotice(template, "Español", "English");
    expect(fixed).not.toContain("{language}");
  });
});
