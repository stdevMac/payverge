/** @jest-environment jsdom */
import React from "react";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { success: jest.fn(), error: jest.fn() },
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
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
  getMenu: jest.fn().mockResolvedValue({
    version: 4,
    categories: [],
    parsed_categories: [
      {
        id: "cat-mains",
        name: "Mains",
        description: "Mains",
        items: [
          {
            id: "demo-steak",
            name: "Steak Plate",
            description: "Grilled steak",
            price: 24,
            is_available: true,
          },
          {
            id: "demo-bowl",
            name: "Harvest Bowl",
            description: "Seasonal greens",
            price: 16,
            is_available: true,
          },
        ],
      },
    ],
  }),
  getBusiness: jest.fn().mockResolvedValue({ default_currency: "USD" }),
  updateMenuItem: jest.fn().mockResolvedValue({ version: 5 }),
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
        name: "Premium Beef",
        category: "Protein",
        unit: "kg",
        current_quantity: 0,
        reorder_threshold: 2,
        cost_per_unit: 21.5,
        is_active: true,
        created_at: "",
        updated_at: "",
      },
      {
        id: 2,
        business_id: 42,
        name: "Mixed Greens",
        category: "Produce",
        unit: "kg",
        current_quantity: 10,
        reorder_threshold: 2,
        cost_per_unit: 4,
        is_active: true,
        created_at: "",
        updated_at: "",
      },
    ]),
    listRecipes: jest.fn().mockResolvedValue([
      {
        id: 10,
        business_id: 42,
        menu_item_id: "demo-steak",
        menu_item_name: "Steak Plate",
        inventory_item_id: 1,
        quantity_required: 0.35,
      },
    ]),
    listMovements: jest.fn().mockResolvedValue([]),
    getSummary: jest.fn().mockResolvedValue({
      low_stock_items: 0,
      out_of_stock_items: 1,
      menu_items_out_of_stock: 1,
      low_stock_details: [],
      out_of_stock_details: [
        {
          inventory_item_id: 1,
          name: "Premium Beef",
          current_quantity: 0,
          reorder_threshold: 2,
          unit: "kg",
          status: "out_of_stock",
        },
      ],
      menu_item_statuses: [
        {
          menu_item_id: "demo-steak",
          menu_item_name: "Steak Plate",
          category_name: "Mains",
          manual_available: true,
          has_recipe: true,
          status: "out_of_stock",
          max_possible_servings: 0,
          recommended_available: false,
          shows_warning: true,
          blocks_sale: true,
          affected_inventory: ["Premium Beef"],
          warning_inventory: [],
        },
        {
          menu_item_id: "demo-bowl",
          menu_item_name: "Harvest Bowl",
          category_name: "Mains",
          manual_available: true,
          has_recipe: false,
          status: "untracked",
          max_possible_servings: -1,
          recommended_available: true,
          shows_warning: false,
          blocks_sale: false,
          affected_inventory: [],
          warning_inventory: [],
        },
      ],
    }),
  },
}));

import { updateMenuItem } from "@/api/business";
import InventoryManager from "../InventoryManager";

async function renderManager() {
  await act(async () => {
    render(<InventoryManager businessId={42} />);
  });
  await waitFor(() =>
    expect(screen.getByText("Premium Beef")).toBeInTheDocument(),
  );
}

describe("InventoryManager dinner-service QA leftovers", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("exposes a single status filter via chips, not KPI tiles or a reorder toggle", async () => {
    await renderManager();
    expect(screen.queryByText("toolbar.reorderOnly")).not.toBeInTheDocument();
    expect(screen.getAllByText("reorder.button")).toHaveLength(1);
    expect(screen.getByTestId("inventory-status-filter")).toBeInTheDocument();
    expect(screen.getByTestId("inventory-kpi-out").tagName).not.toBe("BUTTON");
    expect(screen.getByTestId("inventory-kpi-low").tagName).not.toBe("BUTTON");
    expect(screen.getByTestId("inventory-kpi-tracked").tagName).not.toBe(
      "BUTTON",
    );

    fireEvent.click(screen.getByTestId("inventory-kpi-out"));
    expect(screen.getByText("Mixed Greens")).toBeInTheDocument();

    fireEvent.click(screen.getByTestId("inventory-status-chip-out"));
    await waitFor(() => {
      expect(screen.getByText("Premium Beef")).toBeInTheDocument();
      expect(screen.queryByText("Mixed Greens")).not.toBeInTheDocument();
    });
  });

  it("keeps Low Stock KPI neutral when count is zero and emphasizes Out of Stock", async () => {
    await renderManager();
    const low = screen.getByTestId("inventory-kpi-low");
    const out = screen.getByTestId("inventory-kpi-out");
    expect(low.className).not.toMatch(/amber/);
    expect(low.textContent).toMatch(/0/);
    expect(out.className).toMatch(/rose/);
    expect(out.textContent).toMatch(/1/);
  });

  it("surfaces Receive stock on out-of-stock rows and blocked dishes", async () => {
    await renderManager();
    expect(screen.getByTestId("inventory-receive-1")).toBeInTheDocument();
    expect(screen.getByTestId("inventory-blocks-1")).toBeInTheDocument();
  });

  it("shows an honest recipe list and incomplete-coverage banner", async () => {
    await renderManager();
    fireEvent.click(screen.getByText("tabs.recipes"));
    await waitFor(() =>
      expect(screen.getByTestId("recipes-dish-list")).toBeInTheDocument(),
    );
    expect(screen.getByTestId("recipes-empty-guide")).toBeInTheDocument();
    expect(screen.getByTestId("recipes-coverage-warning").textContent).toBe(
      "recipes.incompleteCoverage",
    );
    expect(screen.getByTestId("recipe-pick-demo-steak")).toBeInTheDocument();
    expect(screen.getByTestId("recipe-pick-demo-bowl")).toBeInTheDocument();
    expect(screen.getByText("recipes.statusMapped")).toBeInTheDocument();
    expect(screen.getByText("recipes.statusUnmapped")).toBeInTheDocument();

    fireEvent.click(screen.getByTestId("recipe-pick-demo-steak"));
    expect(screen.getByTestId("recipes-dish-list")).toBeInTheDocument();
    expect(screen.queryByTestId("recipes-empty-guide")).not.toBeInTheDocument();
  });

  it("offers a confirmed 86 CTA when a mapped ingredient is at 0", async () => {
    await renderManager();
    expect(screen.getByTestId("inventory-86-1")).toBeInTheDocument();
    expect(screen.queryByTestId("inventory-86-2")).not.toBeInTheDocument();

    fireEvent.click(screen.getByTestId("inventory-86-1"));
    expect(
      await screen.findByText("items.eightySixConfirmTitle"),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByText("items.eightySixConfirm"));

    await waitFor(() => {
      expect(updateMenuItem).toHaveBeenCalledTimes(1);
    });
    expect(updateMenuItem).toHaveBeenCalledWith(
      42,
      0,
      0,
      expect.objectContaining({
        id: "demo-steak",
        is_available: false,
        description: "Grilled steak",
      }),
      4,
      "cat-mains",
      "demo-steak",
    );
  });
});
