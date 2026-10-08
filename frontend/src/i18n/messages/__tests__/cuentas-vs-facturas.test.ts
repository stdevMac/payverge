/** @jest-environment node */
import esDashboard from "../es/businessDashboard.json";
import esArDashboard from "../es-ar/businessDashboard.json";
import esBillManager from "../es/billManager.json";
import esArBillManager from "../es-ar/billManager.json";

/**
 * NEW-5: restaurant bills are "Cuentas"; fiscal invoices stay "Facturas".
 * Sidebar bills tab, live-bills analytics card, and bill manager page heading
 * must agree on Cuentas (es + es-ar).
 */
describe("NEW-5 Cuentas vs Facturas (operator bills terminology)", () => {
  it("es live bills strings use Cuentas, not Facturas", () => {
    expect(esDashboard.dashboard.tabs.liveBills).toBe("Cuentas en Vivo");
    expect(esDashboard.dashboard.liveBills.title).toBe("Cuentas en Vivo");
    expect(esDashboard.tabs.bills).toBe("Cuentas");
  });

  it("es-ar mirrors Cuentas for live bills", () => {
    // es-ar is a sparse overlay — assert the override when present, else es.
    const live =
      esArDashboard.dashboard?.tabs?.liveBills ??
      esDashboard.dashboard.tabs.liveBills;
    const title =
      esArDashboard.dashboard?.liveBills?.title ??
      esDashboard.dashboard.liveBills.title;
    expect(live).toBe("Cuentas en Vivo");
    expect(title).toBe("Cuentas en Vivo");
  });

  it("bill manager page title is Cuentas in es and es-ar", () => {
    expect(esBillManager.title).toBe("Cuentas");
    expect(esArBillManager.title).toBe("Cuentas");
  });

  it("fiscal/sidebar invoice labels remain Facturas (not relabeled to Cuentas)", () => {
    expect(esDashboard.tabs.fiscal).toBe("Facturas");
    expect(esDashboard.accountingDashboard.tabs.invoices).toBe("Facturas");
  });
});
