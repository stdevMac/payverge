import fs from "fs";
import path from "path";

const source = fs.readFileSync(path.resolve(__dirname, "page.tsx"), "utf8");
const layout = fs.readFileSync(path.resolve(__dirname, "layout.tsx"), "utf8");
const lookup = fs.readFileSync(
  path.resolve(__dirname, "../../../../lib/guest/lookupGuestTable.ts"),
  "utf8",
);

describe("guest menu page a11y + load wiring", () => {
  it("#420 uses the independent-table load gate and metadata timeout", () => {
    expect(source).toContain("shouldShowMenuLoadingGate");
    expect(source).toContain("loadGuestMenuDependencies");
    expect(source).toContain("MENU_DEPENDENCY_TIMEOUT_MS");
    expect(layout).toContain("lookupGuestTable");
    expect(lookup).toContain("AbortSignal.timeout");
    expect(lookup).toContain("GUEST_TABLE_LOOKUP_TIMEOUT_MS");
  });

  it("#422 wires the extracted menu-view radiogroup", () => {
    expect(source).toContain("MenuViewRadiogroup");
    expect(source).not.toMatch(
      /role="radio"[\s\S]{0,80}onPress=\{\(\) => setViewMode\(mode\)\}/,
    );
  });

  it("#423 exposes filter disclosure state", () => {
    expect(source).toContain("aria-expanded={showFilters}");
    expect(source).toContain("GUEST_MENU_FILTERS_PANEL_ID");
    expect(source).toContain("aria-controls={GUEST_MENU_FILTERS_PANEL_ID}");
  });

  it("#427 confirms a successful bundle add", () => {
    expect(source).toContain('t("menu.bundles.added"');
    expect(source).toContain("toast.success(confirmation)");
  });

  it("#428 does not reuse Loading menu… for quote work", () => {
    expect(source).toContain('t("menu.updatingTotal")');
    expect(source).toContain("GuestQuoteStatus");
    expect(source).not.toMatch(
      /quotePending &&[\s\S]{0,220}t\("menu\.loadingMenu"\)/,
    );
  });

  it("#509 labels the cart FAB as Subtotal until the quote-inclusive total is known", () => {
    expect(source).toMatch(/quotePending[\s\S]{0,500}common\.subtotal/);
    expect(source).not.toMatch(
      /quotePending[\s\S]{0,80}amount=\{promotionPreview\.finalTotal\}/,
    );
  });

  it("names the menu search field from the placeholder copy", () => {
    expect(source).toContain("GuestMenuSearchField");
    expect(source).toContain('t("menu.search.placeholder")');
  });

  it("#499 converts diner fmtCurrency amounts before display-currency labels", () => {
    expect(source).toContain("formatConvertedGuestCurrency");
    expect(source).not.toMatch(
      /formatGuestCurrency\(amount, code, currentLanguage\)/,
    );
    expect(source).toMatch(
      /formatConvertedGuestCurrency\([\s\S]*businessCurrencies\.default_currency[\s\S]*businessCurrencies\.display_currency/,
    );
  });
});

