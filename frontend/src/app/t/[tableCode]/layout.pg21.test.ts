/**
 * PG-21: tableCode layout must SSR-seed GuestTranslationProvider with messages
 * so menu/bill/profile nav is not English on first paint.
 */
import fs from "fs";
import path from "path";

const layout = fs.readFileSync(path.join(__dirname, "layout.tsx"), "utf8");
const menu = fs.readFileSync(path.join(__dirname, "menu/page.tsx"), "utf8");
const bill = fs.readFileSync(path.join(__dirname, "bill/page.tsx"), "utf8");
const profile = fs.readFileSync(
  path.join(__dirname, "profile/page.tsx"),
  "utf8",
);

describe("PG-21 guest SSR chrome via table layout", () => {
  it("layout loads guest messages and seeds the provider", () => {
    expect(layout).toMatch(/loadGuestMessages/);
    expect(layout).toMatch(/GuestTranslationProvider/);
    expect(layout).toMatch(/initialMessages/);
    expect(layout).toMatch(/x-payverge-locale/);
  });

  it("menu/bill/profile do not re-wrap with an empty provider", () => {
    // Empty nested providers would wipe SSR messages back to English.
    expect(menu).not.toMatch(
      /<GuestTranslationProvider\s+businessId=\{undefined\}\s*>/,
    );
    expect(bill).not.toMatch(
      /<GuestTranslationProvider\s+businessId=\{undefined\}\s*>/,
    );
    expect(profile).not.toMatch(
      /<GuestTranslationProvider[\s>]/,
    );
    expect(profile).not.toMatch(
      /from ["']@\/i18n\/GuestTranslationProvider["']/,
    );
  });
});
