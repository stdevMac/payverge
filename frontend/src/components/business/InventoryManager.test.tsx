/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";

import InventoryManager from "@/components/business/InventoryManager";
import { getBusiness, getMenu } from "@/api/business";
import { inventoryApi } from "@/api/inventory";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";

// Operator-tier locale is driven per-test via this mutable holder. jest.mock is
// hoisted, so the variable must be `mock`-prefixed to be referenced inside the
// factory. Defaults to "en" so the English-label assertions below stay green;
// individual tests flip it with mockSimpleLocale(...) to exercise es-AR money
// formatting. getTranslation is kept real so labels still resolve.
let mockOperatorLocale = "en";
const mockSimpleLocale = (code: string) => {
  mockOperatorLocale = code;
};
jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/getTranslation");
  return {
    useSimpleLocale: () => ({
      locale: mockOperatorLocale,
      setLocale: jest.fn(),
    }),
    getTranslation: actual.getTranslation,
  };
});

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
    listMovementsPage: jest.fn(),
    getSummary: jest.fn(),
    createItem: jest.fn(),
    updateItem: jest.fn(),
    deleteItem: jest.fn(),
    replaceRecipe: jest.fn(),
    createAdjustment: jest.fn(),
  },
}));

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: jest.fn(),
}));

describe("InventoryManager", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockOperatorLocale = "en"; // reset module-level locale (clearMocks won't)
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
    (inventoryApi.listItems as jest.Mock).mockResolvedValue([]);
    (inventoryApi.listRecipes as jest.Mock).mockResolvedValue([]);
    (inventoryApi.listMovements as jest.Mock).mockResolvedValue([]);
    (inventoryApi.listMovementsPage as jest.Mock).mockResolvedValue({
      movements: [],
      total: 0,
      offset: 0,
      limit: 25,
    });
    (inventoryApi.getSummary as jest.Mock).mockResolvedValue({
      settings: {
        inventory_enabled: false,
        auto_deduct_on_order_approval: true,
        low_stock_warnings_enabled: true,
        availability_sync_mode: "warn",
      },
      total_items: 0,
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
  });

  it("shows the activation flow when inventory is disabled", async () => {
    (inventoryApi.getSettings as jest.Mock).mockResolvedValue({
      inventory_enabled: false,
      auto_deduct_on_order_approval: true,
      low_stock_warnings_enabled: true,
      availability_sync_mode: "warn",
    });

    render(<InventoryManager businessId={42} />);

    expect(
      await screen.findByText("Activate Inventory Management"),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Activate Inventory" }),
    ).toBeTruthy();
    expect(screen.queryByText("Add Inventory Item")).toBeNull();
  });

  it("shows the organized inventory workspace when inventory is enabled", async () => {
    (inventoryApi.getSettings as jest.Mock).mockResolvedValue({
      inventory_enabled: true,
      auto_deduct_on_order_approval: true,
      low_stock_warnings_enabled: true,
      availability_sync_mode: "warn",
    });

    render(<InventoryManager businessId={42} />);

    expect(await screen.findByText("Tracked Items")).toBeTruthy();
    expect(screen.getByText("Add Inventory Item")).toBeTruthy();

    // The disable action now lives behind the Settings tab (progressive
    // disclosure), not in the always-visible workspace header.
    fireEvent.click(screen.getByRole("tab", { name: "Settings" }));
    expect(
      await screen.findByRole("button", { name: "Disable Inventory" }),
    ).toBeTruthy();
  });

  it("keeps the inventory workspace available when menu loading fails", async () => {
    (inventoryApi.getSettings as jest.Mock).mockResolvedValue({
      inventory_enabled: true,
      auto_deduct_on_order_approval: true,
      low_stock_warnings_enabled: true,
      availability_sync_mode: "warn",
    });
    (getMenu as jest.Mock).mockRejectedValue(new Error("menu unavailable"));

    render(<InventoryManager businessId={42} />);

    expect(await screen.findByText("Tracked Items")).toBeTruthy();
    expect(screen.getByText("Add Inventory Item")).toBeTruthy();
  });

  it("warns that values are shown in USD when the business currency can't be loaded (INV-L6)", async () => {
    (inventoryApi.getSettings as jest.Mock).mockResolvedValue({
      inventory_enabled: true,
      auto_deduct_on_order_approval: true,
      low_stock_warnings_enabled: true,
      availability_sync_mode: "warn",
    });
    (getBusiness as jest.Mock).mockResolvedValue(null); // currency load failed

    render(<InventoryManager businessId={42} />);

    expect(await screen.findByText("Tracked Items")).toBeTruthy();
    expect(screen.getByText(/shown in USD/i)).toBeTruthy();
  });

  it("does not warn about currency when the business currency loads (INV-L6)", async () => {
    (inventoryApi.getSettings as jest.Mock).mockResolvedValue({
      inventory_enabled: true,
      auto_deduct_on_order_approval: true,
      low_stock_warnings_enabled: true,
      availability_sync_mode: "warn",
    });
    (getBusiness as jest.Mock).mockResolvedValue({ default_currency: "EUR" });

    render(<InventoryManager businessId={42} />);

    expect(await screen.findByText("Tracked Items")).toBeTruthy();
    expect(screen.queryByText(/shown in USD/i)).toBeNull();
  });

  const enableInventory = () => {
    (inventoryApi.getSettings as jest.Mock).mockResolvedValue({
      inventory_enabled: true,
      auto_deduct_on_order_approval: true,
      low_stock_warnings_enabled: true,
      availability_sync_mode: "warn",
    });
  };

  it("tags a negative on-hand quantity as Oversold while keeping the true value visible (F12)", async () => {
    enableInventory();
    (inventoryApi.listItems as jest.Mock).mockResolvedValue([
      {
        id: 1,
        business_id: 42,
        name: "Flour",
        category: "Baking",
        current_quantity: -0.35,
        unit: "kg",
        reorder_threshold: 0,
        sku: "",
        cost_per_unit: 0,
        is_active: true,
        created_at: "",
        updated_at: "",
      },
    ]);

    render(<InventoryManager businessId={42} />);

    // The true negative value is still shown (not floored to 0 / hidden).
    expect(await screen.findByText("-0.35 kg")).toBeTruthy();
    // ...and it carries an explicit Oversold tag so it doesn't read as a glitch.
    expect(screen.getByText("Oversold")).toBeTruthy();
  });

  it("does not tag a positive on-hand quantity as Oversold (F12)", async () => {
    enableInventory();
    (inventoryApi.listItems as jest.Mock).mockResolvedValue([
      {
        id: 2,
        business_id: 42,
        name: "Sugar",
        category: "Baking",
        current_quantity: 4,
        unit: "kg",
        reorder_threshold: 0,
        sku: "",
        cost_per_unit: 0,
        is_active: true,
        created_at: "",
        updated_at: "",
      },
    ]);

    render(<InventoryManager businessId={42} />);

    expect(await screen.findByText("4 kg")).toBeTruthy();
    expect(screen.queryByText("Oversold")).toBeNull();
  });

  it("renders movement timestamps in the business timezone, not the device timezone (R17)", async () => {
    enableInventory();
    // Business is in Buenos Aires (UTC-3). The util must render wall-clock in
    // that zone regardless of the test runner's local timezone.
    (getBusiness as jest.Mock).mockResolvedValue({
      default_currency: "USD",
      timezone: "America/Argentina/Buenos_Aires",
    });
    // 2026-03-15T02:30:00Z → 23:30 the previous day (Mar 14) in Buenos Aires.
    (inventoryApi.listItems as jest.Mock).mockResolvedValue([
      {
        id: 7,
        business_id: 42,
        name: "Flour",
        category: "Baking",
        current_quantity: 5,
        unit: "kg",
        reorder_threshold: 0,
        sku: "",
        cost_per_unit: 0,
        is_active: true,
        created_at: "",
        updated_at: "",
      },
    ]);
    (inventoryApi.listMovementsPage as jest.Mock).mockResolvedValue({
      movements: [
        {
          id: 101,
          inventory_item_id: 7,
          movement_type: "purchase",
          quantity_delta: 5,
          quantity_before: 0,
          quantity_after: 5,
          reason: "",
          created_at: "2026-03-15T02:30:00Z",
        },
      ],
      total: 1,
      offset: 0,
      limit: 25,
    });

    render(<InventoryManager businessId={42} />);

    // Open the Activity tab where movements render.
    fireEvent.click(await screen.findByRole("tab", { name: "Activity" }));

    // Business-time wall-clock: 11:30 PM on Mar 14 in Buenos Aires (UTC-3).
    expect(await screen.findByText(/23:30/)).toBeTruthy();
    // The UTC/device wall-clock (02:30 AM, Mar 15) must NOT appear — proving the
    // timestamp was zoned to the business, not left in UTC or the device zone.
    expect(screen.queryByText(/02:30\s?AM/)).toBeNull();
    expect(screen.queryByText(/2:30\s?AM/)).toBeNull();
  });

  it("prefixes currency for es-AR operators (no es-ES suffix format) (audit H5)", async () => {
    mockSimpleLocale("es-AR");
    enableInventory();
    // Item value = current_quantity * cost_per_unit = 1 * 212.1 = 212.10.
    (inventoryApi.listItems as jest.Mock).mockResolvedValue([
      {
        id: 3,
        business_id: 9,
        name: "Aceite",
        category: "Pantry",
        current_quantity: 1,
        unit: "L",
        reorder_threshold: 0,
        sku: "",
        cost_per_unit: 212.1,
        is_active: true,
        created_at: "",
        updated_at: "",
      },
    ]);

    render(<InventoryManager businessId={9} />);

    // es-AR prefixes the currency symbol: "US$ 212,10" (comma decimal), NOT the
    // es-ES suffix convention "212,10 US$". Intl may emit NBSP/narrow-NBSP, so
    // \s? matches loosely. The value renders in both the item cell and the total
    // stock tile, so assert on all matches (findAllByText).
    const prefixed = await screen.findAllByText(/US\$\s?212,10/);
    expect(prefixed.length).toBeGreaterThan(0);
    expect(screen.queryByText(/212,10\s?US\$/)).toBeNull();
  });

  it("names inventory setting switches (#446)", async () => {
    (inventoryApi.getSettings as jest.Mock).mockResolvedValue({
      inventory_enabled: true,
      auto_deduct_on_order_approval: true,
      low_stock_warnings_enabled: true,
      availability_sync_mode: "warn",
    });
    render(<InventoryManager businessId={42} />);
    fireEvent.click(await screen.findByRole("tab", { name: "Settings" }));
    expect(
      await screen.findByRole("switch", { name: /auto deduct on approval/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("switch", { name: /low stock warnings/i }),
    ).toBeInTheDocument();
  });
});
