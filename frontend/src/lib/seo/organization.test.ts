import fs from "fs";
import path from "path";
import {
  buildOrganizationJsonLd,
  buildSiteStructuredData,
  buildWebsiteJsonLd,
} from "./organization";

const SITE = "https://pos.example.test";

describe("buildOrganizationJsonLd", () => {
  const org = buildOrganizationJsonLd(SITE, {
    name: "Payverge",
    twitterHandle: "payverge_io",
  });

  it("falls back to a public logo path that exists in the frontend bundle", () => {
    expect(org.logo).toBe(`${SITE}/images/logo.webp`);
    const rel = org.logo.replace(SITE, "");
    // public/ is served at site root
    const disk = path.join(process.cwd(), "public", rel.replace(/^\//, ""));
    expect(fs.existsSync(disk)).toBe(true);
  });

  it("carries the instance identity instead of the upstream brand (R2-1)", () => {
    const built = buildOrganizationJsonLd(SITE, {
      name: "Trattoria",
      legalName: "Trattoria SRL",
      logoUrl: "https://cdn.example.test/trattoria.png",
    });
    expect(built.name).toBe("Trattoria");
    expect(built).toMatchObject({ legalName: "Trattoria SRL" });
    expect(built.logo).toBe("https://cdn.example.test/trattoria.png");
    expect(JSON.stringify(built)).not.toContain("Payverge");
    expect(
      buildOrganizationJsonLd(SITE, { name: "T", logoUrl: "/media/logo.png" })
        .logo,
    ).toBe(`${SITE}/media/logo.png`);
    expect(buildOrganizationJsonLd(SITE, { name: "T", legalName: "T" })).not
      .toHaveProperty("legalName");
    expect(buildWebsiteJsonLd(SITE, "Trattoria").name).toBe("Trattoria");
  });

  it("derives every URL from the runtime site origin", () => {
    for (const origin of ["https://a.example.test", "https://b.example.test/"]) {
      const built = buildOrganizationJsonLd(origin, { name: "Payverge" });
      const clean = origin.replace(/\/$/, "");
      expect(built.url).toBe(clean);
      expect(buildWebsiteJsonLd(origin, "Payverge").url).toBe(clean);
      expect(JSON.stringify(built)).not.toContain("payverge.io");
    }
  });

  it("points nowhere the instance does not serve (no /contact, no /blog search)", () => {
    const serialized = JSON.stringify([org, buildWebsiteJsonLd(SITE, "Payverge")]);
    expect(serialized).not.toContain("/contact");
    expect(serialized).not.toContain("/blog");
    expect(serialized).not.toContain("SearchAction");
  });

  it("keeps availableLanguage and inLanguage on the Organization root (#866)", () => {
    expect(org.availableLanguage).toEqual(
      expect.arrayContaining(["English", "Spanish", "Spanish (Argentina)"]),
    );
    expect(org.inLanguage).toEqual(expect.arrayContaining(["en", "es", "es-AR"]));
  });

  it("does not advertise the stale parent-brand LinkedIn company URL (#36)", () => {
    expect(org.sameAs.join(" ")).not.toMatch(/linkedin\.com/i);
    expect(org.sameAs).toContain("https://twitter.com/payverge_io");
  });

  it("omits sameAs profiles when no handle is configured", () => {
    expect(buildOrganizationJsonLd(SITE, { name: "Payverge" }).sameAs).toEqual([]);
  });
});

describe("buildSiteStructuredData", () => {
  it("emits no Organization when the site is a single venue (R2-1)", () => {
    const data = buildSiteStructuredData(SITE, {
      name: "Trattoria",
      homeMode: "venue",
    });
    expect(data.organization).toBeNull();
    expect(data.website).toMatchObject({ "@type": "WebSite", name: "Trattoria" });
  });

  it.each(["directory", "empty", null, undefined])(
    "emits the instance Organization in %s mode",
    (homeMode) => {
      const data = buildSiteStructuredData(SITE, { name: "Group", homeMode });
      expect(data.organization).toMatchObject({
        "@type": "Organization",
        name: "Group",
      });
    },
  );
});
