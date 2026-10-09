import fs from "fs";
import path from "path";

describe("GuestTableView landing task priority", () => {
  const source = fs.readFileSync(
    path.resolve(__dirname, "GuestTableView.tsx"),
    "utf8",
  );

  it("filters landing promotions through sellable-target helpers (issue 345)", () => {
    expect(source).toContain("filterGuestSellableOffers");
    expect(source).toContain("filterGuestSellableBundles");
  });

  it("places service actions (menu/bill) before CRM rewards in the source DOM order", () => {
    const serviceIdx = source.indexOf('data-testid="landing-service-actions"');
    const menuIdx = source.indexOf('data-testid="landing-browse-menu"');
    const crmIdx = source.indexOf('data-testid="landing-crm-rewards"');
    const promoIdx = source.indexOf('data-testid="landing-promotions-teaser"');

    expect(serviceIdx).toBeGreaterThan(-1);
    expect(menuIdx).toBeGreaterThan(-1);
    expect(crmIdx).toBeGreaterThan(-1);

    // Service stack comes first; rewards after.
    expect(serviceIdx).toBeLessThan(crmIdx);
    expect(menuIdx).toBeLessThan(crmIdx);
    // Promo teaser sits above the tall service stack so it never rests under
    // the sticky dock on short phones (issue 83).
    expect(promoIdx).toBeGreaterThan(-1);
    expect(promoIdx).toBeLessThan(serviceIdx);
    expect(serviceIdx).toBeLessThan(crmIdx);
  });

  it("uses logical start alignment for the sm+ landing header (RTL wrap)", () => {
    expect(source).toContain("sm:items-start sm:text-start");
    expect(source).not.toMatch(/\bsm:text-left\b/);
  });

  it("keeps closed/kitchen banner before service actions", () => {
    const bannerKitchen = source.indexOf("kitchenOrdersOn");
    const serviceIdx = source.indexOf('data-testid="landing-service-actions"');
    expect(bannerKitchen).toBeGreaterThan(-1);
    expect(bannerKitchen).toBeLessThan(serviceIdx);
  });

  it("uses closed browse description when business is closed", () => {
    expect(source).toContain("table.browseMenuDescriptionClosed");
    expect(source).toMatch(
      /isBusinessClosed\s*\?\s*t\("table\.browseMenuDescriptionClosed"\)/,
    );
    expect(source).toContain('t("table.browseMenuDescription")');
  });

  it("uses compact CRM rewards when closed or a bill is open", () => {
    expect(source).toMatch(
      /variant=\{\s*isBusinessClosed\s*\|\|\s*!!currentBill\s*\?\s*"compact"\s*:\s*"full"\s*\}/,
    );
  });

  it("passes businessClosed into CallWaiterButton for closed-mode hint", () => {
    expect(source).toMatch(
      /<CallWaiterButton[\s\S]*?businessClosed=\{isBusinessClosed\}/,
    );
  });

  it("wires business timezone into OpenClosedPill (device TZ must not lie)", () => {
    expect(source).toMatch(
      /<OpenClosedPill[\s\S]*?timezone=\{business\?\.timezone\}/,
    );
  });
});

describe("closed browse CTA i18n", () => {
  const messagesDir = path.resolve(__dirname, "../../i18n/guest-messages");
  const locales = fs
    .readdirSync(messagesDir)
    .filter((f) => f.endsWith(".json") && !f.startsWith("."));

  it("ships browseMenuDescriptionClosed in all guest locales", () => {
    expect(locales.length).toBe(21);
    for (const file of locales) {
      const data = JSON.parse(
        fs.readFileSync(path.join(messagesDir, file), "utf8"),
      );
      const closed = data.table?.browseMenuDescriptionClosed;
      expect(typeof closed).toBe("string");
      expect(closed.length).toBeGreaterThan(0);
      // Closed copy must not promise add-to-order.
      expect(closed.toLowerCase()).not.toMatch(/add items/);
    }
  });

  it("en closed copy is browse-only", () => {
    const en = JSON.parse(
      fs.readFileSync(path.join(messagesDir, "en.json"), "utf8"),
    );
    expect(en.table.browseMenuDescriptionClosed).toMatch(/paused|browse/i);
    expect(en.table.browseMenuDescription).toMatch(/add items/i);
  });
});
