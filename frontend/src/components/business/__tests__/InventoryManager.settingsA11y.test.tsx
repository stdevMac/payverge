/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { computeAccessibleName } from "dom-accessibility-api";
import {
  announcedSwitchName,
  labelledByTargetsHaveText,
} from "@/components/ui/namedControl";

jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/getTranslation");
  return {
    useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
    getTranslation: actual.getTranslation,
  };
});

jest.mock("@/api/business", () => ({
  getMenu: jest.fn().mockResolvedValue({ categories: [], parsed_categories: [] }),
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
    updateSettings: jest.fn(),
    listItems: jest.fn().mockResolvedValue([]),
    listRecipes: jest.fn().mockResolvedValue([]),
    listMovements: jest.fn().mockResolvedValue([]),
    listMovementsPage: jest.fn().mockResolvedValue({
      movements: [],
      total: 0,
      offset: 0,
      limit: 25,
    }),
    getSummary: jest.fn().mockResolvedValue({
      settings: { inventory_enabled: true },
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
    }),
    createItem: jest.fn(),
    updateItem: jest.fn(),
    deleteItem: jest.fn(),
    replaceRecipe: jest.fn(),
    createAdjustment: jest.fn(),
  },
}));

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => ({
    hasAccess: true,
    isSuspended: false,
    loading: false,
  }),
}));

import InventoryManager from "@/components/business/InventoryManager";

describe("InventoryManager settings accessible names (#446)", () => {
  it("names inventory switches by purpose and state", async () => {
    render(<InventoryManager businessId={42} />);
    fireEvent.click(await screen.findByRole("tab", { name: "Settings" }));

    const autoDeduct = await screen.findByRole("switch", {
      name: "Auto deduct on approval",
    });
    expect(announcedSwitchName(autoDeduct)).toBe("Auto deduct on approval, on");
    expect(labelledByTargetsHaveText(autoDeduct)).toBe(true);
    expect(autoDeduct.getAttribute("aria-describedby")).toBeTruthy();

    const warnings = screen.getByRole("switch", {
      name: "Low stock warnings",
    });
    expect(announcedSwitchName(warnings)).toBe("Low stock warnings, on");
    expect(labelledByTargetsHaveText(warnings)).toBe(true);

    const sync = screen.getByRole("button", {
      name: "Availability sync mode, Warn on low stock, 86 when empty",
    });
    expect(computeAccessibleName(sync)).toMatch(/Availability sync mode/i);
    expect(computeAccessibleName(sync)).not.toBe(
      "Warn on low stock, 86 when empty",
    );
  });
});
