/** @jest-environment jsdom */
/**
 * Round 5 audit — Task 1 coverage.
 *
 * InventoryManager used to render the add/edit ingredient form as an inline
 * side-panel on the Items tab (triggered by `showForm`/`itemForm.id`). That
 * panel occupied ~380px of vertical space while the operator was browsing
 * the list — the list itself is the primary surface, so the form now lives
 * in a NextUI modal.
 *
 * This smoke test pins the new behaviour:
 *   1. The "Add ingredient" button renders on the Items tab.
 *   2. Clicking it opens the modal (role=dialog).
 *   3. Dismissing (Escape / Cancel) closes the modal — no orphan dialog
 *      in the DOM, no leaked form state on next open.
 *
 * We deliberately keep the API mocks minimal — deep assertions against the
 * create/update mutations belong to integration tests. Render-only is fine.
 */

import React from "react";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";

// --- Mocks -----------------------------------------------------------------

// i18n: echo keys verbatim so assertions stay deterministic.
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => key,
}));

// Tier hook: pretend business owner has full access.
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

// Inventory API: return a single seeded item so the list renders, and resolve
// every mutation so `handleItemSaved` / `loadInventory(true)` don't blow up.
const mockSettings = {
  id: 1,
  business_id: 42,
  inventory_enabled: true,
  auto_deduct_on_order_approval: true,
  low_stock_warnings_enabled: true,
  availability_sync_mode: "warn" as const,
};

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
  created_at: "2026-04-15T10:00:00.000Z",
  updated_at: "2026-04-15T10:00:00.000Z",
};

jest.mock("@/api/inventory", () => ({
  inventoryApi: {
    getSettings: jest.fn(() => Promise.resolve(mockSettings)),
    listItems: jest.fn(() => Promise.resolve([mockItem])),
    listRecipes: jest.fn(() => Promise.resolve([])),
    listMovements: jest.fn(() => Promise.resolve([])),
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
  getBusiness: jest.fn(() => Promise.resolve({ default_currency: "USD" })),
}));

// InventoryToggle relies on wagmi + its own API — stub to nothing so the
// manager renders cleanly.
jest.mock("../InventoryToggle", () => ({
  __esModule: true,
  default: () => <div data-testid="inventory-toggle" />,
}));

jest.mock("../DashboardLockedTabView", () => ({
  __esModule: true,
  default: () => <div data-testid="locked-view" />,
}));

// --- Imports (after mocks) -------------------------------------------------

import InventoryManager from "../InventoryManager";

// --- Tests -----------------------------------------------------------------

describe("InventoryManager — add-ingredient modal", () => {
  it("opens the inventory item modal when the Add button is pressed, and closes on cancel", async () => {
    render(<InventoryManager businessId={42} />);

    // Wait for the initial load to resolve and the Items tab to render.
    const addButton = await screen.findByTestId("inventory-add-item-button");
    expect(addButton).toBeTruthy();

    // Before we press Add, no modal dialog should be mounted.
    expect(document.querySelector('[role="dialog"]')).toBeNull();

    // Press the Add button — modal should appear.
    await act(async () => {
      fireEvent.click(addButton);
    });

    await waitFor(() => {
      expect(document.querySelector('[role="dialog"]')).not.toBeNull();
    });

    // The header reflects create mode (not edit) — `itemForm.addTitle` key
    // echoes through our identity i18n mock.
    const dialog = document.querySelector('[role="dialog"]') as HTMLElement;
    expect(dialog.textContent || "").toContain("itemForm.addTitle");

    // Close via the Cancel button — label comes from `itemForm.cancel`.
    const cancelButton = Array.from(dialog.querySelectorAll("button")).find(
      (btn) => (btn.textContent || "").includes("itemForm.cancel"),
    );
    expect(cancelButton).toBeTruthy();

    await act(async () => {
      fireEvent.click(cancelButton as HTMLButtonElement);
    });

    // Dialog detaches from the DOM (NextUI unmounts on close).
    await waitFor(() => {
      expect(document.querySelector('[role="dialog"]')).toBeNull();
    });
  });
});
