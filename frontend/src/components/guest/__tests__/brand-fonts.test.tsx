import fs from "fs";
import path from "path";

const GUEST_FILES = [
  "../GuestTableView.tsx",
  "../GuestMenuViews.tsx",
  "../CRMSignupCard.tsx",
  "../../navigation/PersistentGuestNav.tsx",
  "../../business-page/ConvertingBusinessLandingPage.tsx",
];

describe("brand fonts", () => {
  it.each(GUEST_FILES)("no phantom font variables in %s", (rel) => {
    const src = fs.readFileSync(path.resolve(__dirname, rel), "utf8");
    expect(src).not.toMatch(/--font-playfair-sc/);
    expect(src).not.toMatch(/--font-karla/);
  });
});
