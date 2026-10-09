import {
  btnGhostIcon,
  btnGhostIconActive,
  btnPrimary,
  btnPrimaryCompact,
  btnSecondary,
  btnTonalDanger,
  btnTonalSuccess,
  touchIconBtn,
} from "@/components/ui/buttonStyles";

describe("dashboard mobile touch-target contract", () => {
  it("keeps icon controls at least 44px until a fine hover pointer is present", () => {
    for (const recipe of [touchIconBtn, btnGhostIcon, btnGhostIconActive]) {
      expect(recipe).toContain("h-11");
      expect(recipe).toContain("w-11");
      expect(recipe).toContain("min-w-11");
      expect(recipe).toContain("[@media(hover:hover)_and_(pointer:fine)]:h-");
    }
  });

  it("gives text actions a 44px coarse-pointer height while preserving desktop density", () => {
    for (const recipe of [
      btnPrimary,
      btnPrimaryCompact,
      btnSecondary,
      btnTonalSuccess,
      btnTonalDanger,
    ]) {
      expect(recipe).toContain("min-h-11");
      expect(recipe).toContain(
        "[@media(hover:hover)_and_(pointer:fine)]:min-h-0",
      );
    }
  });
});
