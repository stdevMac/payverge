/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import InventoryManager from "../InventoryManager";

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
  // Never-resolving so the component stays in its first-load state.
  getMenu: jest.fn(() => new Promise(() => {})),
  getBusiness: jest.fn(() => new Promise(() => {})),
}));
jest.mock("@/api/inventory", () => ({
  inventoryApi: {
    listItems: jest.fn(() => new Promise(() => {})),
    getSettings: jest.fn(() => new Promise(() => {})),
    listMovements: jest.fn(() => new Promise(() => {})),
    listRecipes: jest.fn(() => new Promise(() => {})),
    getSummary: jest.fn(() => new Promise(() => {})),
  },
}));

describe("InventoryManager — first-load skeleton", () => {
  it("renders the content skeleton (no bare boxed spinner) while loading", () => {
    const { container } = render(<InventoryManager businessId={42} />);
    expect(screen.getByRole("status")).toHaveAttribute("aria-busy", "true");
    // The old boxed-spinner used .animate-spin; the skeleton must not.
    expect(container.querySelector(".animate-spin")).toBeNull();
  });
});
