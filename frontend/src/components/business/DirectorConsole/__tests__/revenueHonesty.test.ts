import {
  hasGroundedRevenueRange,
  isRevenueAdvice,
  playHasGroundedFigures,
  shouldHideUngroundedRevenueAdvice,
  structuredAdviceText,
} from "../revenueHonesty";
import type { BriefingPlay } from "@/api/directorConsole";

describe("revenueHonesty (#241)", () => {
  it("treats the demo director briefing as ungrounded — period without figures", () => {
    const demo =
      "Dinner revenue is tracking above the 30-day median. Cocktail attach is up and labor is on target.";
    expect(isRevenueAdvice(demo)).toBe(true);
    expect(hasGroundedRevenueRange(demo)).toBe(false);
    expect(shouldHideUngroundedRevenueAdvice(demo)).toBe(true);
  });

  it("treats the Spanish demo briefing the same way", () => {
    const demo =
      "Los ingresos de la cena superan la mediana de 30 días. El attach de cócteles sube.";
    expect(shouldHideUngroundedRevenueAdvice(demo)).toBe(true);
  });

  it("keeps advice that cites money or a counted figure", () => {
    const grounded =
      "Today so far: $4,200.00 across 84 orders. Revenue is 12% ahead of a typical Tuesday.";
    expect(hasGroundedRevenueRange(grounded)).toBe(true);
    expect(shouldHideUngroundedRevenueAdvice(grounded)).toBe(false);
    expect(
      shouldHideUngroundedRevenueAdvice("Recover ~10% of lost Tuesday revenue"),
    ).toBe(false);
  });

  it("does not suppress non-revenue operational copy", () => {
    expect(
      shouldHideUngroundedRevenueAdvice(
        "Inventory for Premium Beef is at zero. Review its usage and restock.",
      ),
    ).toBe(false);
  });

  it("does not hide live-floor, dispatch, cash, or best-seller copy", () => {
    expect(
      shouldHideUngroundedRevenueAdvice(
        "Table 4 is free. Table 1 has been waiting 22 minutes for water, claimed by Sam Server.",
      ),
    ).toBe(false);
    expect(
      shouldHideUngroundedRevenueAdvice(
        "Mesa 12 está ocupada. Mandá un mozo desde Mesas; no cierro cajas ni despacho yo.",
      ),
    ).toBe(false);
    expect(
      shouldHideUngroundedRevenueAdvice(
        "La caja está abierta con $125.00 esperados. Cerrala desde Caja cuando termines el turno.",
      ),
    ).toBe(false);
    expect(
      shouldHideUngroundedRevenueAdvice(
        "Harvest Bowl is the best seller this week: 18 orders.",
      ),
    ).toBe(false);
    expect(
      shouldHideUngroundedRevenueAdvice(
        "This week's sales mix for best sellers isn't in yet. Harvest Bowl is live on the card.",
      ),
    ).toBe(false);
    expect(
      shouldHideUngroundedRevenueAdvice(
        "The thinnest margin on the card is House Burger at $3.00 this week.",
      ),
    ).toBe(false);
    expect(
      shouldHideUngroundedRevenueAdvice(
        "Este copiloto está diseñado para ventas, operaciones, clientes y rentabilidad.",
      ),
    ).toBe(false);
  });

  it("joins structured fields before judging the claim", () => {
    const text = structuredAdviceText({
      summary: "Dinner revenue is tracking above the 30-day median.",
      diagnosis: "Cocktail attach is up and labor is on target.",
      evidence: [],
      expected_impact: "Featuring cocktails before 8pm should extend the trend.",
    });
    expect(shouldHideUngroundedRevenueAdvice(text)).toBe(true);
  });

  it("playHasGroundedFigures requires prices on a reprice and never invents them", () => {
    const incomplete: BriefingPlay = {
      kind: "reprice_up",
      tab: "menu",
      item_name: "Branzino",
      current_price: null,
      suggested_price: null,
      monthly_impact: 180,
    };
    expect(playHasGroundedFigures(incomplete)).toBe(false);
    expect(
      playHasGroundedFigures({
        ...incomplete,
        current_price: 24,
        suggested_price: 27,
      }),
    ).toBe(true);
  });
});
