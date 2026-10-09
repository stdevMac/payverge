/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import MenuBuilder from "../MenuBuilder";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: jest.fn(),
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => key,
}));

jest.mock("@/api/business", () => ({
  businessApi: {
    getBusiness: jest.fn().mockResolvedValue({
      id: 1,
      name: "Loading Bistro",
      default_currency: "USD",
    }),
    // Keep getMenu pending forever so MenuBuilder stays in its loading state.
    getMenu: jest.fn(() => new Promise(() => {})),
  },
  // MenuBuilder imports MenuItemOption as a type; runtime export is unused.
}));

jest.mock("@/api/currency", () => ({
  getSupportedLanguages: jest.fn().mockResolvedValue([]),
  getBusinessLanguages: jest.fn().mockResolvedValue([]),
  updateBusinessLanguages: jest.fn().mockResolvedValue(undefined),
}));

jest.mock("@/api/inventory", () => ({
  inventoryApi: {
    getSummary: jest.fn().mockResolvedValue({ menu_item_statuses: [] }),
  },
}));

describe("MenuBuilder loading state", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (useBusinessAccess as jest.Mock).mockReturnValue({
      hasAccess: true,
      aiConfigured: true,
      loading: false,
    });
  });

  it("renders skeleton placeholders, not AI tools, while menu loads", () => {
    render(<MenuBuilder businessId={1} />);

    expect(screen.getByTestId("menu-loading-skeleton")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /sync translations/i }),
    ).not.toBeInTheDocument();
  });
});
