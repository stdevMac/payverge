import { getTranslation } from "@/i18n/getTranslation";

describe("briefing strip i18n", () => {
  it("resolves greeting-less count badge + view-all in en (singular/plural)", () => {
    expect(getTranslation("directorConsole.strip.countOne", "en", { count: 1 })).toBe(
      "1 insight",
    );
    expect(getTranslation("directorConsole.strip.countOther", "en", { count: 20 })).toBe(
      "20 insights",
    );
    expect(getTranslation("directorConsole.strip.viewAll", "en", { count: 20 })).toBe(
      "View all (20)",
    );
  });

  it("resolves the empty (no-insights) line and drawer heading in en", () => {
    expect(getTranslation("directorConsole.strip.allClear", "en")).toContain("clear");
    expect(getTranslation("directorConsole.strip.drawerTitle", "en")).toBe(
      "Your briefing",
    );
    expect(getTranslation("directorConsole.strip.close", "en")).toBe("Close");
  });

  it("resolves es (neutral) strings, not the raw key", () => {
    expect(getTranslation("directorConsole.strip.countOther", "es", { count: 20 })).toBe(
      "20 avisos",
    );
    expect(getTranslation("directorConsole.strip.viewAll", "es", { count: 20 })).toBe(
      "Ver todos (20)",
    );
    expect(getTranslation("directorConsole.strip.drawerTitle", "es")).toBe(
      "Tu resumen",
    );
  });

  it("applies es-AR voseo override for the ask prompt and inherits the rest from es", () => {
    // askPrompt: es tuteo 'Pregúntale' -> es-AR voseo 'Preguntale'
    expect(getTranslation("directorConsole.strip.askPrompt", "es")).toContain(
      "Pregúntale",
    );
    expect(getTranslation("directorConsole.strip.askPrompt", "es-AR")).toContain(
      "Preguntale",
    );
    // drawerTitle has no voseo divergence, so es-AR inherits es.
    expect(getTranslation("directorConsole.strip.drawerTitle", "es-AR")).toBe(
      getTranslation("directorConsole.strip.drawerTitle", "es"),
    );
  });

  it("resolves honesty + as-of copy in en and es, with es-AR voseo on the CTA", () => {
    expect(getTranslation("directorConsole.honesty.notEnoughData", "en")).toBe(
      "Not enough data",
    );
    expect(getTranslation("directorConsole.honesty.notEnoughData", "es")).toBe(
      "No hay datos suficientes",
    );
    expect(getTranslation("directorConsole.strip.asOf", "en", { time: "18:27" })).toBe(
      "as of 18:27",
    );
    expect(getTranslation("directorConsole.strip.openAnalytics", "es")).toBe(
      "Abrir analítica",
    );
    expect(getTranslation("directorConsole.strip.openAnalytics", "es-AR")).toBe(
      "Abrí Analítica",
    );
  });
});
