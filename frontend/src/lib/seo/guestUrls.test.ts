import {
  guestOgLocale,
  guestPublicPath,
  guestPublicUrl,
} from "./guestUrls";

describe("guestPublicPath (#864 / #922)", () => {
  it("prefixes es and es-AR diner URLs and leaves English unprefixed", () => {
    expect(guestPublicPath("es", "/b/parrilla-quebracho-azul")).toBe(
      "/es/b/parrilla-quebracho-azul",
    );
    expect(guestPublicPath("es-AR", "/t/FV214XU12D")).toBe(
      "/es-ar/t/FV214XU12D",
    );
    expect(guestPublicPath("es-ar", "/t/FV214XU12D/menu")).toBe(
      "/es-ar/t/FV214XU12D/menu",
    );
    expect(guestPublicPath("en", "/b/parrilla-quebracho-azul")).toBe(
      "/b/parrilla-quebracho-azul",
    );
    expect(guestPublicPath("fr", "/t/FV214XU12D")).toBe("/t/FV214XU12D");
  });
});

describe("guestOgLocale", () => {
  it("maps diner locales onto Open Graph locale tags", () => {
    expect(guestOgLocale("en")).toBe("en_US");
    expect(guestOgLocale("es")).toBe("es_ES");
    expect(guestOgLocale("es-AR")).toBe("es_AR");
    expect(guestOgLocale("es-ar")).toBe("es_AR");
  });
});

describe("guestPublicUrl", () => {
  it("builds an absolute payverge.io URL", () => {
    expect(guestPublicUrl("es", "/b/aurora")).toBe(
      "https://payverge.io/es/b/aurora",
    );
  });
});
