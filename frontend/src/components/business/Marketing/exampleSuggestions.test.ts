import {
  buildExampleSuggestions,
  EXAMPLE_IMAGE_DISH,
  EXAMPLE_IMAGE_OFFER,
} from "./exampleSuggestions";
import { buildWhyExplain } from "./whyExplain";

const t = (k: string) => k;

it("builds a featured-dish example and an offer example", () => {
  const ex = buildExampleSuggestions(t);
  expect(ex).toHaveLength(2);
  expect(ex.find((s) => s.play === "featured_dish")).toBeDefined();
  expect(ex.find((s) => s.play === "offer")).toBeDefined();
});

it("each example carries a bundled same-origin photo", () => {
  const urls = buildExampleSuggestions(t).map((s) => s.image_url);
  expect(urls).toContain(EXAMPLE_IMAGE_DISH);
  expect(urls).toContain(EXAMPLE_IMAGE_OFFER);
  urls.forEach((u) => expect(u).toMatch(/^\/marketing-examples\//));
});

it("gives the offer example a discount so its badge renders", () => {
  const offer = buildExampleSuggestions(t).find((s) => s.play === "offer")!;
  expect(offer.discount_type).toBe("percentage");
  expect(offer.discount_value ?? 0).toBeGreaterThan(0);
});

it("localizes all copy through the provided t()", () => {
  const seen: string[] = [];
  const spyT = (k: string) => {
    seen.push(k);
    return k;
  };
  const ex = buildExampleSuggestions(spyT);
  expect(seen).toContain("example.cards.featured.title");
  expect(seen).toContain("example.cards.featured.name");
  expect(seen).toContain("example.cards.offer.why");
  expect(ex[0].title).toBe("example.cards.featured.title");
});

// #826: the example offer card's discount why-factor must be wire-shaped
// ("20%"), because formatWhyFactorValue owns the "off" wording. Passing the
// localized display string ("20% off") re-wrapped it → "20% off off".
it("renders the example discount row as '20% off' exactly once", () => {
  const messages: Record<string, string> = {
    "why.factors.discount": "Discount",
    "why.factors.discount_pct_value": "{pct}% off",
    "why.factors.discount_fixed_value": "{amount} off",
  };
  const tr = (key: string, params?: Record<string, string | number>) => {
    let out = messages[key] ?? key;
    for (const [name, val] of Object.entries(params ?? {})) {
      out = out.replace(`{${name}}`, String(val));
    }
    return out;
  };
  const offer = buildExampleSuggestions(tr).find((s) => s.play === "offer")!;
  const discountFactor = offer.why_factors?.find((f) => f.key === "discount");
  expect(discountFactor?.value).toBe("20%");
  const row = buildWhyExplain(offer, tr).rows.find((r) => r.key === "discount")!;
  expect(row.value).toBe("20% off");
  expect(row.value.match(/off/g)).toHaveLength(1);
});

it("includes S2 why_factors so empty businesses demo explainability", () => {
  const ex = buildExampleSuggestions(t);
  for (const s of ex) {
    expect(s.ranking_version).toBe("s2");
    expect(s.why_factors?.length).toBeGreaterThan(0);
  }
});
