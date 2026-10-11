import { routeAliasRedirects } from "./routeAliasRedirects.cjs";

describe("routeAliasRedirects", () => {
  const redirects = routeAliasRedirects();

  function find(source: string) {
    return redirects.find((r) => r.source === source);
  }

  it("aliases /manifest.json to the live PWA manifest (#614)", () => {
    expect(find("/manifest.json")?.destination).toBe("/site.webmanifest");
    expect(find("/manifest.webmanifest")?.destination).toBe(
      "/site.webmanifest",
    );
  });

  it("sends /login to the dashboard sign-in dialog and /en to unprefixed English (#615)", () => {
    expect(find("/login")?.destination).toBe("/dashboard?auth=signin");
    expect(find("/es/login")?.destination).toBe(
      "/dashboard?auth=signin&lang=es",
    );
    expect(find("/es-ar/login")?.destination).toBe(
      "/dashboard?auth=signin&lang=es-AR",
    );
    expect(find("/en")?.destination).toBe("/");
    expect(find("/en/:path*")?.destination).toBe("/:path*");
  });

  it("sends the common sign-in and sign-up spellings to the real flows instead of a 404", () => {
    for (const source of ["/signin", "/sign-in"]) {
      expect(find(source)).toEqual(
        expect.objectContaining({
          destination: "/dashboard?auth=signin",
          permanent: false,
        }),
      );
      expect(find(`/es${source}`)?.destination).toBe(
        "/dashboard?auth=signin&lang=es",
      );
      expect(find(`/es-ar${source}`)?.destination).toBe(
        "/dashboard?auth=signin&lang=es-AR",
      );
    }
    for (const source of ["/signup", "/sign-up"]) {
      for (const prefix of ["", "/es", "/es-ar"]) {
        expect(find(`${prefix}${source}`)).toEqual(
          expect.objectContaining({ destination: "/business/register", permanent: false }),
        );
      }
    }
  });

  it("never sends sign-in to the instance root, which now serves the venue", () => {
    for (const r of redirects) {
      expect(r.destination).not.toMatch(/^\/(es|es-ar)?\?auth=/);
    }
  });

  it("redirects /terms to /terms-and-conditions for every locale (#646)", () => {
    expect(find("/terms")?.destination).toBe("/terms-and-conditions");
    expect(find("/es/terms")?.destination).toBe("/es/terms-and-conditions");
    expect(find("/es-ar/terms")?.destination).toBe(
      "/es-ar/terms-and-conditions",
    );
  });

  it("redirects /privacy to /privacy-policy for every locale", () => {
    for (const prefix of ["", "/es", "/es-ar"]) {
      expect(find(`${prefix}/privacy`)).toEqual(
        expect.objectContaining({
          destination: `${prefix}/privacy-policy`,
          permanent: true,
        }),
      );
    }
  });

  it("sends /docs to the repository docs and keeps it temporary (#916)", () => {
    for (const prefix of ["", "/es", "/es-ar"]) {
      expect(find(`${prefix}/docs`)).toEqual(
        expect.objectContaining({
          destination: "https://github.com/stdevMac/payverge/tree/main/docs",
          permanent: false,
        }),
      );
    }
  });

  it("drops the aliases into the removed marketing pages", () => {
    for (const source of ["/ai", "/help", "/support", "/app/help", "/demo"]) {
      expect(find(source)).toBeUndefined();
    }
  });
});
