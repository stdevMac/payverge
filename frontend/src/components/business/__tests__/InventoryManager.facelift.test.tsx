/** @jest-environment jsdom */
import React from "react";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  // t() calls getTranslation(`businessDashboard.inventoryManager.${key}`); strip
  // that prefix so the component renders the bare key the test queries by.
  getTranslation: (key: string) =>
    key.replace(/^businessDashboard\.inventoryManager\./, ""),
}));
jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => ({
    access: null,
    loading: false,
    error: null,
    hasAccess: true,
    isSuspended: false,
    lockState: "active",
    aiConfigured: false,
    refetch: jest.fn(),
  }),
}));
jest.mock("@/api/business", () => ({
  getMenu: jest
    .fn()
    .mockResolvedValue({ categories: [], parsed_categories: [] }),
  getBusiness: jest.fn().mockResolvedValue({ default_currency: "USD" }),
}));

jest.mock("@/api/inventory", () => ({
  inventoryApi: {
    getSettings: jest.fn().mockResolvedValue({
      inventory_enabled: true,
      auto_deduct_on_order_approval: true,
      low_stock_warnings_enabled: true,
      availability_sync_mode: "warn",
    }),
    listItems: jest.fn().mockResolvedValue([
      {
        id: 1,
        business_id: 42,
        name: "Olive Oil",
        category: "Pantry",
        unit: "liter",
        current_quantity: 0,
        reorder_threshold: 2,
        cost_per_unit: 10,
        is_active: true,
        created_at: "",
        updated_at: "",
      },
      {
        id: 2,
        business_id: 42,
        name: "Tomatoes",
        category: "Produce",
        unit: "kg",
        current_quantity: 2,
        reorder_threshold: 5,
        cost_per_unit: 1,
        is_active: true,
        created_at: "",
        updated_at: "",
      },
      {
        id: 3,
        business_id: 42,
        name: "Basil",
        category: "Produce",
        unit: "unit",
        current_quantity: 20,
        reorder_threshold: 5,
        cost_per_unit: 0.5,
        is_active: true,
        created_at: "",
        updated_at: "",
      },
    ]),
    listRecipes: jest.fn().mockResolvedValue([]),
    listMovements: jest.fn().mockResolvedValue([]),
    getSummary: jest.fn().mockResolvedValue({ menu_item_statuses: [] }),
  },
}));

import InventoryManager from "../InventoryManager";

async function renderManager() {
  await act(async () => {
    render(<InventoryManager businessId={42} />);
  });
  await waitFor(() =>
    expect(screen.getByText("Olive Oil")).toBeInTheDocument(),
  );
}

describe("InventoryManager facelift", () => {
  it("renders all items by default sorted by attention (out first)", async () => {
    await renderManager();
    const rows = screen.getAllByRole("row").map((r) => r.textContent || "");
    const oilIdx = rows.findIndex((r) => r.includes("Olive Oil"));
    const basilIdx = rows.findIndex((r) => r.includes("Basil"));
    expect(oilIdx).toBeLessThan(basilIdx);
  });

  it("filters to out-of-stock when the Out status chip is clicked", async () => {
    await renderManager();
    fireEvent.click(screen.getByTestId("inventory-status-chip-out"));
    await waitFor(() => {
      expect(screen.getByText("Olive Oil")).toBeInTheDocument();
      expect(screen.queryByText("Basil")).not.toBeInTheDocument();
    });
  });

  it("searches by name", async () => {
    await renderManager();
    fireEvent.change(screen.getByPlaceholderText("toolbar.searchPlaceholder"), {
      target: { value: "basil" },
    });
    await waitFor(() => {
      expect(screen.getByText("Basil")).toBeInTheDocument();
      expect(screen.queryByText("Olive Oil")).not.toBeInTheDocument();
    });
  });

  it("does not render the removed Adjustments tab", async () => {
    await renderManager();
    expect(screen.queryByText("tabs.adjustments")).not.toBeInTheDocument();
  });
});
