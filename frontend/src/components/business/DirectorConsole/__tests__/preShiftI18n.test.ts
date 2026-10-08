import { getTranslation } from "@/i18n/getTranslation";

describe("preShift i18n", () => {
  it("no longer ships the dead empty-state keys in any locale", () => {
    // The empty 'all set' block was removed when the briefing became the
    // always-present front door — these keys must resolve to a leaf fallback,
    // not real copy. (sentenceCaseLeaf returns the humanized leaf, e.g. "Empty".)
    for (const locale of ["en", "es", "es-AR"] as const) {
      expect(getTranslation("directorConsole.preShift.empty", locale)).toBe("Empty");
      expect(getTranslation("directorConsole.preShift.emptySubline", locale)).toBe(
        "Empty subline",
      );
    }
  });

  it("resolves the learning read in en and es (not the raw key)", () => {
    const en = getTranslation("directorConsole.preShift.briefing.learning", "en");
    const es = getTranslation("directorConsole.preShift.briefing.learning", "es");
    expect(en).toContain("still learning");
    expect(es).not.toContain("briefing");
    expect(es).toContain("aprendiendo");
  });

  it("leaves figure tokens intact when no params are passed (component weaves them in)", () => {
    const tmpl = getTranslation("directorConsole.preShift.briefing.read.withPace", "en");
    expect(tmpl).toContain("{projected}");
    expect(tmpl).toContain("{pace}");
    expect(tmpl).toContain("{orders}");
    expect(tmpl).toContain("{avgTicket}");
  });

  it("keeps base es neutral tuteo and overrides only voseo-divergent briefing keys in es-AR", () => {
    // play CTA imperatives: es 'Ajusta'/'Destaca' -> es-AR voseo 'Ajustá'/'Destacá'
    expect(
      getTranslation("directorConsole.preShift.briefing.play.cta.reprice_up", "es"),
    ).toContain("Ajusta el precio");
    expect(
      getTranslation("directorConsole.preShift.briefing.play.cta.reprice_up", "es-AR"),
    ).toContain("Ajustá el precio");
    expect(
      getTranslation("directorConsole.preShift.briefing.play.cta.promote", "es-AR"),
    ).toContain("Destacá");
    // win: es 'Sigue así' -> es-AR voseo 'Seguí así'
    expect(
      getTranslation("directorConsole.preShift.briefing.win.revenue_up_wow", "es", { pct: "9%" }),
    ).toContain("Sigue así");
    expect(
      getTranslation("directorConsole.preShift.briefing.win.revenue_up_wow", "es-AR", { pct: "9%" }),
    ).toContain("Seguí así");
  });

  it("inherits non-divergent briefing copy from es into es-AR (deltas-only, no full tree)", () => {
    // moveHeading has no voseo divergence, so es-AR must inherit the es value.
    const es = getTranslation("directorConsole.preShift.briefing.moveHeading", "es");
    const esAr = getTranslation("directorConsole.preShift.briefing.moveHeading", "es-AR");
    expect(esAr).toBe(es);
    expect(esAr).toContain("jugada");
  });

  it("interpolates {count} and {names} in a card line", () => {
    const out = getTranslation("directorConsole.preShift.cards.outOfStock.other", "en", {
      count: 3,
      names: "Branzino, Risotto",
    });
    expect(out).toContain("3");
    expect(out).toContain("Branzino, Risotto");
    expect(out).not.toContain("{count}");
    expect(out).not.toContain("{names}");
  });

  it("renders singular-correct grammar at count===1 with actual duration", () => {
    const one = getTranslation("directorConsole.preShift.cards.staleBills.one", "en", {
      count: 1,
      duration: "3 days",
    });
    expect(one).toContain("1 bill open longer than 3 days");
    // the whole point of the .one/.other split: no "(s)" and no plural verb
    expect(one).not.toContain("(s)");
    expect(one).not.toContain("bills open");
    // Must not hardcode the 2h surfacing threshold when a real duration is provided.
    expect(one).not.toContain("2 hours");

    const other = getTranslation("directorConsole.preShift.cards.staleBills.other", "en", {
      count: 4,
      duration: "14 days",
    });
    expect(other).toContain("bills open longer than 14 days");
    expect(other).toContain("4");
    expect(other).not.toContain("(s)");
  });

  it("interpolates the waste amount and ingredient", () => {
    const out = getTranslation("directorConsole.preShift.cards.wasteHigh", "en", {
      amount: 180,
      ingredient: "salmon trim",
    });
    expect(out).toContain("180");
    expect(out).toContain("salmon trim");
  });

  it("flows a pre-formatted currency value (with $) through unmangled and owns no literal $", () => {
    // The JSON no longer hardcodes "$"; the formatter owns the symbol. A value
    // like "$1,234.00" must interpolate verbatim — getTranslation inserts via
    // String.prototype.replace with a capture-group-free regex, so the "$" is
    // not treated as a replacement token.
    const out = getTranslation("directorConsole.preShift.cards.wasteHigh", "en", {
      amount: "$1,234.00",
      ingredient: "salmon trim",
    });
    expect(out).toContain("$1,234.00");
    expect(out).toContain("salmon trim");
    // no double "$" — proves the literal "$" was removed from the JSON string
    expect(out).not.toContain("$$");
    expect(out).not.toContain("${amount}");
  });

  it("interpolates the same placeholders in es and applies es-AR voseo", () => {
    const es = getTranslation("directorConsole.preShift.cards.lowStock", "es", {
      count: 2,
      names: "Pan, Leche",
    });
    expect(es).toContain("2");
    expect(es).toContain("Pan, Leche");
    expect(es).not.toContain("{count}");
    expect(es).not.toContain("{names}");
    // base es is neutral tuteo
    expect(es).toContain("Repón");
    // es-AR overrides with voseo
    const esAr = getTranslation("directorConsole.preShift.cards.lowStock", "es-AR", {
      count: 2,
      names: "Pan, Leche",
    });
    expect(esAr).toContain("Reponé");
  });
});
