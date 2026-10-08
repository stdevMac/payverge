import fs from "fs";
import path from "path";

describe("guest table menu ordering guards", () => {
  const source = fs.readFileSync(path.resolve(__dirname, "page.tsx"), "utf8");

  it("guards every frontend ordering submission entrypoint", () => {
    expect(source).toMatch(
      /const addToCart = useCallback\([\s\S]*?if \(!isOrderingEnabled\)[\s\S]*?showOrderingDisabledToast\(\)/,
    );
    expect(source).toMatch(
      /const handleCreateBill = useCallback\(async \(\) => \{[\s\S]*?if \(!isOrderingEnabled\)[\s\S]*?showOrderingDisabledToast\(\)[\s\S]*?submitCartAsOrder\(null, "create-bill"\)/,
    );
    expect(source).toMatch(
      /const handleAddItemsToBill = useCallback\(async \(\) => \{[\s\S]*?if \(!isOrderingEnabled\)[\s\S]*?showOrderingDisabledToast\(\)[\s\S]*?submitCartAsOrder/,
    );
  });

  it("passes the ordering flag into the AI waiter", () => {
    expect(source).toContain("isOrderingEnabled={isOrderingEnabled}");
  });

  it("uses only server quotes for promotion totals", () => {
    expect(source).not.toContain("localPromotionPreview");
    expect(source).not.toContain("eligibleSubtotalFor");
    expect(source).not.toContain("clampDiscount");
    expect(source).toContain("neutralCartSubtotal");
  });

  it("consumes the menu item_orderability projection", () => {
    expect(source).toContain("item_orderability");
    expect(source).toContain("setItemOrderability");
    expect(source).toContain("itemOrderability={itemOrderability}");
  });

  it("labels orderable inventory warnings as low stock", () => {
    expect(source).toContain('t("menu.lowStock")');
    expect(source).not.toMatch(
      /bundleWarning[\s\S]{0,800}t\("menu\.filters\.available"\)/,
    );
  });

  it("keeps the mobile filter button accessible when its text is hidden", () => {
    expect(source).toMatch(
      /aria-label=\{t\("menu\.filters\.title"\) \|\| "Filters"\}[\s\S]{0,900}<span className="hidden sm:inline">/,
    );
    expect(source).toContain("aria-expanded={showFilters}");
  });

  it("names the header cart opener as a view-cart action", () => {
    expect(source).toContain('t("accessibility.cartButton", { count: cartItemCount })');
    expect(source).toContain('t("menu.viewCart")');
    expect(source).not.toContain('aria-label={t("menu.yourOrder")}');
  });
});
