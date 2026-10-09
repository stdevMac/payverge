import {
  applyDefaultLanguageDraft,
  applyLockedLanguageDraft,
  applyUnlockedLanguageDraft,
  buildBusinessLanguagesPayload,
  canSaveLanguageDraft,
} from "./businessLanguages";

describe("business language draft helpers", () => {
  it("does not wipe existing languages on a locked-tier change", () => {
    const result = applyLockedLanguageDraft(
      { selected: ["en", "es"], defaultLanguage: "en" },
      "de",
    );

    expect(result.selected).toEqual(["en", "es", "de"]);
    expect(result.defaultLanguage).toBe("de");
    expect(result.requiresRelabelConfirm).toBe(true);
    expect(result.needsDefaultChoice).toBe(false);
  });

  it("treats a single-language locked change as a relabel, not a silent rewrite", () => {
    const result = applyLockedLanguageDraft(
      { selected: ["en"], defaultLanguage: "en" },
      "de",
    );

    expect(result.selected).toEqual(["de"]);
    expect(result.defaultLanguage).toBe("de");
    expect(result.requiresRelabelConfirm).toBe(true);
  });

  it("does not silently promote set-order [0] when the default is removed", () => {
    const result = applyUnlockedLanguageDraft(
      { selected: ["en", "de", "fr"], defaultLanguage: "en" },
      ["de", "fr"],
    );

    expect(result.selected).toEqual(["de", "fr"]);
    expect(result.defaultLanguage).toBe("");
    expect(result.needsDefaultChoice).toBe(true);
    expect(canSaveLanguageDraft(result)).toBe(false);
  });

  it("keeps the current default when it remains selected", () => {
    const result = applyUnlockedLanguageDraft(
      { selected: ["en", "de"], defaultLanguage: "en" },
      ["en", "de", "fr"],
    );

    expect(result.defaultLanguage).toBe("en");
    expect(result.needsDefaultChoice).toBe(false);
    expect(canSaveLanguageDraft(result)).toBe(true);
  });

  it("requires an explicit default pick after the previous default is removed", () => {
    const afterRemoval = applyUnlockedLanguageDraft(
      { selected: ["en", "de"], defaultLanguage: "en" },
      ["de"],
    );
    const afterChoice = applyDefaultLanguageDraft(afterRemoval, "de");

    expect(afterRemoval.needsDefaultChoice).toBe(true);
    expect(afterChoice.defaultLanguage).toBe("de");
    expect(afterChoice.needsDefaultChoice).toBe(false);
    expect(buildBusinessLanguagesPayload(afterChoice)).toEqual({
      language_codes: ["de"],
      default_code: "de",
    });
  });
});
