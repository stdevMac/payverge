import fs from "fs";
import path from "path";

describe("MenuBuilder L3-3 overlay seam", () => {
  const src = fs.readFileSync(
    path.join(__dirname, "..", "index.tsx"),
    "utf8",
  );

  it("suspends AddItemModal while sanitization review is open", () => {
    expect(src).toMatch(
      /isOpen=\{isAddItemOpen && !sanitizationReview\}/,
    );
  });
});
