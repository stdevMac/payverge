import fs from "fs";
import path from "path";

describe("root layout wires cookie consent globally", () => {
  const layout = fs.readFileSync(
    path.join(process.cwd(), "src/app/layout.tsx"),
    "utf8",
  );

  it("imports the CookieConsentProvider", () => {
    expect(layout).toMatch(
      /import\s+\{\s*CookieConsentProvider\s*\}\s+from\s+["']@\/contexts\/CookieConsentContext["']/,
    );
  });

  it("imports the CookieConsent banner", () => {
    expect(layout).toMatch(
      /import\s+\{?\s*CookieConsent\s*\}?\s+from\s+["']@\/components\/CookieConsent["']/,
    );
  });

  it("renders the provider and the banner inside the translation provider", () => {
    expect(layout).toMatch(/<CookieConsentProvider>/);
    expect(layout).toMatch(/<CookieConsent\s*\/>/);
  });
});
