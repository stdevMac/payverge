import fs from "fs";
import path from "path";

describe("guest table menu view-only banner", () => {
  const source = fs.readFileSync(path.resolve(__dirname, "page.tsx"), "utf8");

  it("renders a persistent banner using the localized ordering-disabled copy", () => {
    // The banner must use the existing localized key, not a hardcoded string.
    expect(source).toContain('t("menu.orderingDisabled")');
    // Kitchen-off (not hours-closed) owns this copy; hours use GuestClosedBanner.
    // No dismiss flag — the notice stays while the guest browses.
    expect(source).toMatch(/!kitchenOrdersOn \?/);
    expect(source).toContain("GuestClosedBanner");
  });

  it("is NOT dismissible — the view-only menu has no add-to-cart affordance", () => {
    // A dismissible banner left the guest with an unexplained view-only menu.
    expect(source).not.toContain("viewOnlyDismissed");
    expect(source).not.toContain("setViewOnlyDismissed");
  });

  it("exposes the notice as a persistent status region", () => {
    // Banner copy is rendered inside JSX (role="status"), not only inside the
    // toast callback. Lock the accessible role so the persistent surface stays.
    expect(source).toMatch(/role="status"/);
  });
});
