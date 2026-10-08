/**
 * Task 19 — user-facing marketing labels must be honest.
 * Guards the audited strings "OUTPUT" and "Below average traffic 100%".
 */
import en from "@/i18n/messages/en/marketingDashboard.json";
import es from "@/i18n/messages/es/marketingDashboard.json";

describe("marketing label honesty (Task 19)", () => {
  it("does not label the ready-count KPI as OUTPUT (it is a ready-card counter)", () => {
    expect(en.hero.ready).not.toMatch(/^output$/i);
    expect(en.hero.ready.toLowerCase()).not.toBe("output");
    // Spanish must also avoid the literal "Output" / "Salida" industrial wording.
    expect(es.hero.ready.toLowerCase()).not.toBe("output");
    expect(es.hero.ready.toLowerCase()).not.toBe("salida");
  });

  it("never paints the audited 'Below average traffic' factor label", () => {
    expect(en.why.factors.pct_below_mean).not.toMatch(/below average traffic/i);
    expect(en.why.factors.pct_below_mean).not.toMatch(/^Below average traffic$/);
    expect(es.why.factors.pct_below_mean.toLowerCase()).not.toMatch(
      /por debajo del tráfico medio/,
    );
  });

  // #831: the demo gallery must not feature specific dishes/offers this venue
  // does not sell ("Truffle Tagliatelle", "Weekend Brunch"). Example cards use
  // unmistakably generic placeholder names addressed to the operator.
  it("example cards name generic placeholders, not fake specific dishes", () => {
    const fakes = [
      /truffle tagliatelle/i,
      /tagliatelle con trufa/i,
      /weekend brunch/i,
      /brunch de fin de semana/i,
    ];
    for (const name of [
      en.example.cards.featured.name,
      en.example.cards.offer.name,
      es.example.cards.featured.name,
      es.example.cards.offer.name,
    ]) {
      for (const fake of fakes) {
        expect(name).not.toMatch(fake);
      }
    }
    // Placeholders speak to the operator, so they cannot read as a real menu item.
    expect(en.example.cards.featured.name).toMatch(/^your /i);
    expect(en.example.cards.offer.name).toMatch(/^your /i);
    expect(es.example.cards.featured.name).toMatch(/^tu /i);
    expect(es.example.cards.offer.name).toMatch(/^tu /i);
  });

  it("feed ready group heading exists and is distinct from the overall queue heading", () => {
    // Overall queue is concepts; "Ready to post" is only the ready subset.
    expect(en.feed.heading.toLowerCase()).not.toBe("ready to post");
    expect(en.feed.readyGroup.toLowerCase()).toBe("ready to post");
    expect(es.feed.readyGroup.toLowerCase()).toMatch(/list/);
  });
});
