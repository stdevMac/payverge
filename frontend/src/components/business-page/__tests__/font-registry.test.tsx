import fs from "fs";
import path from "path";

const CBLP = path.resolve(
  __dirname,
  "../ConvertingBusinessLandingPage.tsx",
);
const PMD = path.resolve(__dirname, "../PublicMenuDisplay.tsx");
const DESIGN_CUSTOMIZATION = path.resolve(
  __dirname,
  "../../business/DesignCustomization.tsx",
);

const BANNED_TOKENS = [
  "font-roboto",
  "font-poppins",
  "font-playfair",
  "font-montserrat",
  "font-source",
  "Source Sans Pro",
  "Playfair Display",
];

describe("business-page font registry", () => {
  it.each([CBLP, PMD, DESIGN_CUSTOMIZATION])(
    "%s does not reference unregistered font tokens",
    (filePath) => {
      const src = fs.readFileSync(filePath, "utf8");
      for (const token of BANNED_TOKENS) {
        expect(src).not.toContain(token);
      }
    },
  );
});
