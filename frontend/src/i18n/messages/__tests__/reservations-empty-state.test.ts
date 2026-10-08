import enMessages from "../en";
import esMessages from "../es";

// E2 (reservations-empty-state): the empty-state CTA label said "Create a
// reservation" but actually navigated to Settings (onAction=setActiveTab
// ("settings")) — a mismatched label. Relabel to match the real destination
// and mention the public booking page that appears once reservations are on.
describe("reservations.empty copy (R-E2 guard)", () => {
  const trees = {
    en: (enMessages as Record<string, any>).businessDashboard?.reservations
      ?.empty,
    es: (esMessages as Record<string, any>).businessDashboard?.reservations
      ?.empty,
  };

  it("en: action matches the settings destination and body mentions the public booking page", () => {
    expect(trees.en.action).toBe("Set up reservations");
    expect(trees.en.subtitle).toMatch(/public booking page/i);
  });

  it("es: action matches the settings destination and body mentions the public booking page", () => {
    expect(trees.es.action).toBe("Configurar reservas");
    expect(trees.es.subtitle).toMatch(/página.*reservas/i);
  });

  it("es is genuinely translated, not copied English", () => {
    expect(trees.es.subtitle).not.toBe(trees.en.subtitle);
    expect(trees.es.action).not.toBe(trees.en.action);
  });
});
