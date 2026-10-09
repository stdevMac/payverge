import * as buttonStyles from "./buttonStyles";

describe("buttonStyles focus rings", () => {
  it("uses full-opacity brand rings instead of 30/40 alpha", () => {
    const recipes = Object.values(buttonStyles).filter(
      (value) => typeof value === "string",
    );

    for (const recipe of recipes) {
      expect(recipe).not.toMatch(/focus-visible:ring-brand\/(30|40)\b/);
    }

    expect(buttonStyles.btnPrimary).toContain("focus-visible:ring-brand");
    expect(buttonStyles.btnPrimary).not.toContain("focus-visible:ring-brand/");
  });
});
