/**
 * F3 · Entrega tab i18n consistency (audit §9.3).
 *
 * Live defects under es:
 * - Drivers tab "Conductores" vs performance "Rendimiento de repartidores"
 * - Calificación vs Valoración for the same rating field
 * - History subtitle promised "completadas" while chips say "Entregado"
 *
 * Assert message JSON (the rendered surface for operator copy). Reverting
 * terminology to Conductores / Valoración / completadas must go red.
 */
import fs from "fs";
import path from "path";

const esPath = path.join(
  process.cwd(),
  "src/i18n/messages/es/deliverySettings.json",
);

describe("F3 Entrega Spanish terminology consistency", () => {
  const es = JSON.parse(fs.readFileSync(esPath, "utf8")) as {
    drivers: { title: string; addDriver: string };
    dispatch: {
      tabs: { drivers: string };
      history: { subtitle: string };
      filters: { delivered: string; all: string; cancelled: string; failed: string };
    };
    performance: {
      title: string;
      empty: { subtitle: string };
      columns: { driver: string; rating: string };
      kpi: { rating: string };
    };
  };

  it("uses Repartidor terminology on drivers tab and performance together", () => {
    expect(es.drivers.title).toBe("Repartidores");
    expect(es.drivers.addDriver.toLowerCase()).toContain("repartidor");
    expect(es.dispatch.tabs.drivers).toBe("Repartidores");
    expect(es.performance.title.toLowerCase()).toContain("repartidor");
    expect(es.performance.columns.driver).toBe("Repartidor");
    expect(es.performance.empty.subtitle.toLowerCase()).toContain("repartidor");
    // No mixed Conductor* for the same role.
    const blob = JSON.stringify(es);
    expect(blob).not.toMatch(/[Cc]onductor/);
  });

  it("uses Calificación consistently for rating (not Valoración)", () => {
    expect(es.performance.columns.rating).toMatch(/Calificación/i);
    expect(es.performance.kpi.rating).toMatch(/Calificación/i);
    expect(es.performance.kpi.rating).not.toMatch(/Valoración/i);
  });

  it("history subtitle matches Entregado chip (not orphan completadas)", () => {
    expect(es.dispatch.history.subtitle.toLowerCase()).toContain("entregadas");
    expect(es.dispatch.history.subtitle.toLowerCase()).not.toContain(
      "completadas",
    );
    expect(es.dispatch.filters.delivered).toBe("Entregado");
    // Chips still cover all / delivered / cancelled / failed.
    expect(es.dispatch.filters.all).toBe("Todos");
    expect(es.dispatch.filters.cancelled).toBe("Cancelado");
    expect(es.dispatch.filters.failed).toBe("Fallido");
  });
});
