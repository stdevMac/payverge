import fs from "fs";
import path from "path";

const read = (rel: string) =>
  fs.readFileSync(path.resolve(__dirname, rel), "utf8");

const FILES = {
  aiWizard: "../AIWizard.tsx",
  pdf: "../PDFDigitizer.tsx",
  emptyStates: "../../components/EmptyStates.tsx",
  topMenu: "../../../../ui/top-menu/TopMenu.tsx",
};

describe("blue→teal rebrand: no find-replace fingerprints", () => {
  it("AIWizard has no no-op from-brand→to-brand gradient and no invalid border-brand/10/50", () => {
    const src = read(FILES.aiWizard);
    expect(src).not.toMatch(/from-brand\/5 to-brand\/5/);
    expect(src).not.toMatch(/border-brand\/10\/50/);
  });
  it("PDFDigitizer has no no-op gradients, no double-opacity border, no blue shadow rgba", () => {
    const src = read(FILES.pdf);
    expect(src).not.toMatch(/from-brand\/5 to-brand\/5/);
    expect(src).not.toMatch(/from-brand to-brand(?!-dark)/);
    expect(src).not.toMatch(/border-brand\/10\/50/);
    expect(src).not.toMatch(/rgba\(59,\s*130,\s*246/);
  });
  it("EmptyStates has no no-op from-brand→to-brand gradient", () => {
    const src = read(FILES.emptyStates);
    expect(src).not.toMatch(/from-brand\/5 to-brand\/5/);
  });
  it("TopMenu has no no-op gradient and no invalid /5/50 opacity stop", () => {
    const src = read(FILES.topMenu);
    expect(src).not.toMatch(/from-brand to-brand\b/);
    expect(src).not.toMatch(/from-brand\/5\/50/);
  });
});
