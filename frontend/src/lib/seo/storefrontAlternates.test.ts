/** @jest-environment node */

import { buildStorefrontLanguageAlternates } from "./storefrontAlternates";

describe("buildStorefrontLanguageAlternates", () => {
  it("emits ?lang= entries per language plus a lang-less x-default", () => {
    const languages = buildStorefrontLanguageAlternates(
      "https://payverge.io",
      "aurora",
      ["en", "es", "es-AR"],
    );
    expect(languages).toEqual({
      en: "https://payverge.io/b/aurora?lang=en",
      es: "https://payverge.io/es/b/aurora",
      "es-AR": "https://payverge.io/es-ar/b/aurora",
      "x-default": "https://payverge.io/b/aurora",
    });
  });

  it("returns undefined when there are no languages (or no base URL)", () => {
    expect(
      buildStorefrontLanguageAlternates("https://payverge.io", "aurora", []),
    ).toBeUndefined();
    expect(
      buildStorefrontLanguageAlternates("", "aurora", ["en"]),
    ).toBeUndefined();
  });

  it("filters empty language codes", () => {
    const languages = buildStorefrontLanguageAlternates(
      "https://payverge.io",
      "aurora",
      ["en", ""],
    );
    expect(Object.keys(languages!)).toEqual(["en", "x-default"]);
  });
});
