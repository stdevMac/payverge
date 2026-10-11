/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

import InventoryManager from "@/components/business/InventoryManager";
import { getBusiness, getMenu } from "@/api/business";
import { inventoryApi } from "@/api/inventory";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";

jest.mock("@/api/business", () => ({
  getMenu: jest.fn(),
  getBusiness: jest.fn(),
}));
jest.mock("@/api/inventory", () => ({
  inventoryApi: {
    getSettings: jest.fn(),
    updateSettings: jest.fn(),
    listItems: jest.fn(),
    listRecipes: jest.fn(),
    listMovements: jest.fn(),
    getSummary: jest.fn(),
    createItem: jest.fn(),
    updateItem: jest.fn(),
    deleteItem: jest.fn(),
    replaceRecipe: jest.fn(),
    createAdjustment: jest.fn(),
  },
}));
jest.mock("@/hooks/useBusinessAccess", () => ({ useBusinessAccess: jest.fn() }));

describe("InventoryManager — delete confirmation", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (useBusinessAccess as jest.Mock).mockReturnValue({
      access: null,
      loading: false,
      error: null,
      hasAccess: true,
      isSuspended: false,
      lockState: "active",
      aiConfigured: false,
      refetch: jest.fn(),
    });
    (getMenu as jest.Mock).mockResolvedValue({
      categories: [],
      parsed_categories: [],
    });
    (getBusiness as jest.Mock).mockResolvedValue({ default_currency: "USD" });
    (inventoryApi.getSettings as jest.Mock).mockResolvedValue({
      inventory_enabled: true,
      auto_deduct_on_order_approval: true,
      low_stock_warnings_enabled: true,
      availability_sync_mode: "warn",
    });
    (inventoryApi.listItems as jest.Mock).mockResolvedValue([
      {
        id: 11,
        business_id: 42,
        name: "Tomatoes",
        unit: "kg",
        current_quantity: 5,
        reorder_threshold: 2,
        cost_per_unit: 1.5,
        is_active: true,
      },
    ]);
    (inventoryApi.listRecipes as jest.Mock).mockResolvedValue([]);
    (inventoryApi.listMovements as jest.Mock).mockResolvedValue([]);
    (inventoryApi.getSummary as jest.Mock).mockResolvedValue({
      settings: {
        inventory_enabled: true,
        auto_deduct_on_order_approval: true,
        low_stock_warnings_enabled: true,
        availability_sync_mode: "warn",
      },
      total_items: 1,
      low_stock_items: 0,
      out_of_stock_items: 0,
      total_recipes: 0,
      menu_items_tracked: 0,
      menu_items_low_stock: 0,
      menu_items_out_of_stock: 0,
      low_stock_details: [],
      out_of_stock_details: [],
      menu_item_statuses: [],
    });
    (inventoryApi.deleteItem as jest.Mock).mockResolvedValue({});
  });

  it("requires confirmation before deleting an inventory item", async () => {
    render(<InventoryManager businessId={42} />);

    // Items tab is the default; the row renders with a Delete trash button.
    const deleteBtn = await screen.findByRole("button", { name: "Delete" });

    // Pressing delete must NOT immediately fire the API — it opens a confirm.
    fireEvent.click(deleteBtn);
    expect(inventoryApi.deleteItem).not.toHaveBeenCalled();

    // The confirmation dialog appears, naming the item.
    expect(
      await screen.findByText("Delete inventory item?"),
    ).toBeInTheDocument();

    // Confirm in the dialog → API fires once with the right id.
    const dialog = screen.getByRole("dialog");
    const confirmBtn = Array.from(dialog.querySelectorAll("button")).find(
      (b) => b.textContent === "Delete",
    ) as HTMLButtonElement;
    fireEvent.click(confirmBtn);

    await waitFor(() => {
      expect(inventoryApi.deleteItem).toHaveBeenCalledTimes(1);
    });
    expect(inventoryApi.deleteItem).toHaveBeenCalledWith(42, 11);
  });
});
