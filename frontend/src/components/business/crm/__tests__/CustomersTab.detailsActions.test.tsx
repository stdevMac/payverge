/** @jest-environment jsdom */
import { fireEvent, render, screen } from "@testing-library/react";
import CustomersTab from "../CustomersTab";

const ES_AR: Record<string, string> = {
  "businessDashboard.crm.customerDetails": "Detalle del cliente",
  "businessDashboard.crm.details.viewBills": "Ver cuentas del cliente",
  "businessDashboard.crm.editNotes": "Editar notas del cliente",
  "businessDashboard.crm.editAllergies": "Editar alergias alimentarias",
  "businessDashboard.crm.adjustPoints": "Ajustar puntos de fidelidad",
  "businessDashboard.crm.editTags": "Editar etiquetas del cliente",
  "businessDashboard.crm.close": "Cerrar detalle",
  "businessDashboard.crm.customerList": "Clientes",
  "businessDashboard.crm.searchPlaceholder": "Buscar clientes",
  "businessDashboard.crm.addCustomer": "Agregar cliente",
  "businessDashboard.crm.name": "Nombre",
  "businessDashboard.crm.email": "Correo",
  "businessDashboard.crm.phone": "Celular",
  "businessDashboard.crm.loading": "Cargando",
  "businessDashboard.crm.summary.totalCustomers": "Total de clientes",
  "businessDashboard.crm.summary.activeThisMonth": "Activos este mes",
  "businessDashboard.crm.summary.avgLifetimeSpend": "Gasto promedio",
  "businessDashboard.crm.summary.topTierCustomers": "Clientes top",
  "businessDashboard.crm.tiers.all": "Todas",
  "businessDashboard.crm.filterByTierAria": "Filtrar por nivel",
};

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "es-ar", setLocale: jest.fn() }),
  getTranslation: (key: string) => ES_AR[key] ?? key,
}));

jest.mock("@/api/business", () => ({
  getBusiness: jest.fn(() => Promise.resolve({ default_currency: "ARS" })),
}));

jest.mock("@/api/loyalty", () => ({
  getLoyalty: jest.fn(() =>
    Promise.resolve({
      program: {
        id: 1,
        business_id: 1,
        enabled: true,
        points_per_dollar: 1,
        redemption_points_per_dollar: 100,
      },
      tiers: [],
    }),
  ),
}));

jest.mock("@/components/common/CurrencyConverter", () => ({
  CurrencyPrice: ({ amount }: { amount: number }) => <span>${amount}</span>,
}));

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { success: jest.fn(), error: jest.fn() },
}));

const GUEST = {
  id: 1,
  customer_id: 99,
  name: "Ana Perez",
  email: "ana@x.test",
  loyalty_points: 10,
  loyalty_tier: "Gold",
  total_spent: 100,
  visits: 2,
};

jest.mock("@/api/crm", () => ({
  businessCRMAPI: {
    getCustomers: jest.fn(() =>
      Promise.resolve({
        customers: [GUEST],
        total: 1,
        total_pages: 1,
        summary: {
          total_customers: 1,
          active_this_month: 1,
          avg_lifetime_spend: 100,
          top_tier_count: 1,
        },
      }),
    ),
    getCustomerDetails: jest.fn(() =>
      Promise.resolve({
        customer: { ...GUEST, customer: { name: GUEST.name, email: GUEST.email } },
      }),
    ),
    updateCustomerNotes: jest.fn(),
    updateCustomerTags: jest.fn(),
    addCustomer: jest.fn(),
    adjustCustomerLoyaltyPoints: jest.fn(),
    unlinkCustomer: jest.fn(),
    exportCustomers: jest.fn(),
    getCRMStatus: jest.fn(() => Promise.resolve({ enabled: true })),
    toggleCRM: jest.fn(),
  },
}));

describe("CustomersTab detail actions at phone widths", () => {
  it("keeps every long ES-AR action reachable and wraps the footer", async () => {
    render(
      <CustomersTab businessId={1} onNavigateToTab={jest.fn()} />,
    );
    fireEvent.click(await screen.findByRole("button", { name: "Ana Perez" }));

    const actions = await screen.findByTestId("crm-details-actions");
    expect(actions.className).toMatch(/flex-col|flex-wrap/);

    for (const label of [
      "Ver cuentas del cliente",
      "Editar notas del cliente",
      "Editar alergias alimentarias",
      "Ajustar puntos de fidelidad",
      "Editar etiquetas del cliente",
      "Cerrar detalle",
    ]) {
      expect(screen.getByRole("button", { name: label })).toBeInTheDocument();
    }

    expect(actions.className).not.toMatch(/flex-nowrap/);
    expect(actions.className).not.toMatch(/overflow-hidden/);
  });
});
