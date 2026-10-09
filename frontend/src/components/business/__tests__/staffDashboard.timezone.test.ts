import fs from "fs";
import path from "path";

/**
 * #662: staff dashboards used to synthesize a business row with no IANA
 * timezone. Kitchen then resolved the missing zone to UTC, so EN/es printed
 * 22:12 while es-AR still looked venue-local.
 */
const dashboardPage = path.join(
  __dirname,
  "../../../app/(shop)/business/[businessId]/dashboard/page.tsx",
);

describe("staff dashboard business copies the venue timezone (#662)", () => {
  const src = fs.readFileSync(dashboardPage, "utf8");

  it("copies timezone from the loaded business row onto staffBusiness", () => {
    expect(src).toMatch(/timezone:\s*business\?\.timezone/);
  });

  it("passes the loaded business timezone into Kitchen as a fallback", () => {
    expect(src).toMatch(
      /businessTimezone=\{\s*finalBusiness\.timezone \?\? business\?\.timezone \?\? null/,
    );
  });
});

describe("Kitchen getBusiness settle is the cold-load source of truth", () => {
  const kitchenSrc = fs.readFileSync(
    path.join(__dirname, "../Kitchen.tsx"),
    "utf8",
  );

  it("applies getBusiness().timezone when no timezone prop was passed", () => {
    expect(kitchenSrc).toMatch(/if \(!businessTimezoneProp\)/);
    expect(kitchenSrc).toMatch(/setBusinessTimezone\(biz\?\.timezone/);
  });
});
