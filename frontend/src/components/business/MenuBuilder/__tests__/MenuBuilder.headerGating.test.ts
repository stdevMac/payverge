import { readFileSync } from "fs";
import { join } from "path";

// F14: the menu-item search bar + availability filter + grid/list ViewToggle
// (MenuPageHeader) only apply to the Menu tab. They must not render on the
// Offers / Bundles / Engineering sub-tabs where those controls do nothing.
// #186: language chrome (LanguagesPopover / ActiveLanguagePills) is also
// Menu-tab-only — Engineering is read-only analytics.
describe("MenuBuilder header gating (F14 / #186)", () => {
  const source = readFileSync(
    join(__dirname, "..", "index.tsx"),
    "utf8",
  );

  it("renders MenuPageHeader only when the Menu tab is active", () => {
    expect(source).toMatch(
      /\{selectedTab === "menu" && \(\s*<MenuPageHeader/,
    );
  });

  it("gates LanguagesPopover to the Menu tab", () => {
    expect(source).toMatch(
      /\{selectedTab === "menu" \? \(\s*<LanguagesPopover/,
    );
  });

  it("gates ActiveLanguagePills to the Menu tab", () => {
    expect(source).toMatch(
      /\{selectedTab === "menu" \? \(\s*<ActiveLanguagePills/,
    );
  });

  it("renders exclusive tab panels with stable test ids (#133)", () => {
    expect(source).toMatch(/data-testid="menu-tab-panel-menu"/);
    expect(source).toMatch(/data-testid="menu-tab-panel-offers"/);
    expect(source).toMatch(/data-testid="menu-tab-panel-engineering"/);
    // Engineering must not mount OffersManager.
    const engIdx = source.indexOf('selectedTab === "engineering"');
    const engSlice = source.slice(engIdx, engIdx + 600);
    expect(engSlice).not.toMatch(/OffersManager/);
  });
});
