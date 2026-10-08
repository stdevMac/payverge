import fs from "fs";
import path from "path";

describe("guest menu closed-hours honesty", () => {
  const source = fs.readFileSync(path.resolve(__dirname, "page.tsx"), "utf8");
  const landingSource = fs.readFileSync(
    path.resolve(
      __dirname,
      "../../../../components/guest/GuestTableView.tsx",
    ),
    "utf8",
  );
  const enMessages = JSON.parse(
    fs.readFileSync(
      path.resolve(
        __dirname,
        "../../../../i18n/guest-messages/en.json",
      ),
      "utf8",
    ),
  );

  it("composes isOrderingEnabled with business closed from orderability", () => {
    expect(source).toContain("isBusinessClosedFromOrderability");
    expect(source).toContain("isGuestOrderingEnabled");
    expect(source).toMatch(/businessClosed:\s*isBusinessClosed/);
    expect(landingSource).toContain("isBusinessClosedFromOrderability");
    expect(landingSource).toContain("isGuestOrderingEnabled");
  });

  it("mounts GuestClosedBanner on menu and landing when hours-closed", () => {
    expect(source).toContain("GuestClosedBanner");
    expect(source).toContain("isBusinessClosed");
    expect(landingSource).toContain("GuestClosedBanner");
    expect(landingSource).toMatch(/isBusinessClosed \?[\s\S]*?<GuestClosedBanner/);
    const bannerSource = fs.readFileSync(
      path.resolve(
        __dirname,
        "../../../../components/guest/GuestClosedBanner.tsx",
      ),
      "utf8",
    );
    expect(bannerSource).toContain('data-testid="guest-business-closed-banner"');
  });

  it("mutes offers/bundles strip when closed", () => {
    expect(source).toMatch(
      /!isBusinessClosed\s*&&\s*\([\s\S]*filteredOffers\.length/,
    );
    expect(landingSource).toMatch(
      /promotionCount > 0 && !isBusinessClosed/,
    );
  });

  it("keeps open bill reachable on landing even when ordering is off", () => {
    // Bill card must not be gated on isOrderingEnabled (pay while closed).
    expect(landingSource).toMatch(
      /\{currentBill \? \([\s\S]*?landing-current-bill/,
    );
    expect(landingSource).not.toMatch(
      /isOrderingEnabled && currentBill/,
    );
  });

  it("ships menu.businessClosed keys used by guestOrderErrors", () => {
    expect(enMessages.menu.businessClosed).toBeTruthy();
    expect(enMessages.menu.businessClosedDescription).toBeTruthy();
  });

  it("prefers kitchen-off banner over closed-hours when both would apply", () => {
    // kitchenOrdersOn false branch first, then isBusinessClosed.
    expect(source).toMatch(
      /!kitchenOrdersOn \?[\s\S]*?menu\.orderingDisabled[\s\S]*?: isBusinessClosed \?[\s\S]*?GuestClosedBanner/,
    );
    expect(landingSource).toMatch(
      /!kitchenOrdersOn \?[\s\S]*?menu\.orderingDisabled[\s\S]*?: isBusinessClosed \?[\s\S]*?GuestClosedBanner/,
    );
  });
});
