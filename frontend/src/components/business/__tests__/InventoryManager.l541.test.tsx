/** @jest-environment jsdom */
/**
 * L5-41: page-level error banner must clear when a subsequent save succeeds.
 * InventoryItemModal is mocked so we drive onError → onSaved on the real
 * InventoryManager handlers (handleItemSaved).
 */
import React from "react";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => key,
}));

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => ({
    hasAccess: true,
    loading: false,
    isSuspended: false,
    lockState: "active",
  }),
}));

const mockItem = {
  id: 10,
  business_id: 42,
  name: "Mozzarella",
  sku: "MOZ-01",
  category: "Dairy",
  unit: "kg",
  current_quantity: 5,
  reorder_threshold: 2,
  cost_per_unit: 8.5,
  is_active: true,
  created_at: "",
  updated_at: "",
};

const mockSettings = {
  id: 1,
  business_id: 42,
  inventory_enabled: true,
  auto_deduct_on_order_approval: true,
  low_stock_warnings_enabled: true,
  availability_sync_mode: "warn" as const,
};

jest.mock("@/api/inventory", () => ({
  inventoryApi: {
    getSettings: jest.fn(() => Promise.resolve(mockSettings)),
    listItems: jest.fn(() => Promise.resolve([mockItem])),
    listRecipes: jest.fn(() => Promise.resolve([])),
    listMovements: jest.fn(() => Promise.resolve([])),
    listMovementsPage: jest.fn(() =>
      Promise.resolve({ movements: [], next_cursor: null }),
    ),
    getSummary: jest.fn(() =>
      Promise.resolve({ menu_item_statuses: [], item_health: [] }),
    ),
    createItem: jest.fn(() => Promise.resolve(mockItem)),
    updateItem: jest.fn(() => Promise.resolve(mockItem)),
    deleteItem: jest.fn(() => Promise.resolve(undefined)),
    createAdjustment: jest.fn(() => Promise.resolve(undefined)),
    replaceRecipe: jest.fn(() => Promise.resolve(undefined)),
    updateSettings: jest.fn(() => Promise.resolve(mockSettings)),
  },
}));

jest.mock("@/api/business", () => ({
  getMenu: jest.fn(() =>
    Promise.resolve({ categories: [], parsed_categories: [] }),
  ),
  getBusiness: jest.fn(() =>
    Promise.resolve({ id: 42, default_currency: "USD" }),
  ),
}));

jest.mock("../InventoryToggle", () => ({
  __esModule: true,
  default: () => <div data-testid="inventory-toggle" />,
}));

jest.mock("../DashboardLockedTabView", () => ({
  __esModule: true,
  default: () => <div data-testid="locked-view" />,
}));

// Drive error lifecycle through the real parent handlers.
jest.mock("../inventory/InventoryItemModal", () => ({
  __esModule: true,
  default: function MockItemModal({
    isOpen,
    onError,
    onSaved,
  }: {
    isOpen: boolean;
    onError?: (m: string) => void;
    onSaved: (r: { item: typeof mockItem; mode: "create" | "edit" }) => void;
  }) {
    if (!isOpen) return null;
    return (
      <div role="dialog" data-testid="mock-item-modal">
        <button
          type="button"
          data-testid="force-item-error"
          onClick={() => onError?.("save boom")}
        >
          fail
        </button>
        <button
          type="button"
          data-testid="force-item-success"
          onClick={() => onSaved({ item: mockItem, mode: "edit" })}
        >
          ok
        </button>
      </div>
    );
  },
}));

jest.mock("../inventory/QuickAdjustDrawer", () => ({
  __esModule: true,
  default: () => null,
}));

import InventoryManager from "../InventoryManager";

describe("InventoryManager L5-41 error banner lifecycle", () => {
  it("clears the page error banner after a successful item save that followed a failure", async () => {
    render(<InventoryManager businessId={42} />);

    const addButton = await screen.findByTestId("inventory-add-item-button");
    await act(async () => {
      fireEvent.click(addButton);
    });
    expect(await screen.findByTestId("mock-item-modal")).toBeInTheDocument();

    fireEvent.click(screen.getByTestId("force-item-error"));
    expect(await screen.findByText("save boom")).toBeInTheDocument();

    fireEvent.click(screen.getByTestId("force-item-success"));
    await waitFor(() => {
      expect(screen.queryByText("save boom")).not.toBeInTheDocument();
    });
  });
});
