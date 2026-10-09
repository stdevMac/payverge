/** @jest-environment jsdom */
/**
 * Related to issue 184: opening Edit Item must seed Price and Food cost
 * with the same two-decimal money string (12 → "12.00", 3.5 → "3.50").
 */
import React from "react";
import { act, fireEvent, render, screen } from "@testing-library/react";

const EDITING_ITEM = {
  id: 1,
  name: "Chocolate Tart",
  description: "",
  price: 12,
  cogs: 3.5,
  is_available: true,
  options: [],
  allergens: [],
  dietary_tags: [],
  sort_order: 0,
};

jest.mock("../hooks/useMenuData", () => ({
  useMenuData: () => ({
    menu: [{ name: "Dessert", description: "", items: [EDITING_ITEM] }],
    setMenu: jest.fn(),
    menuVersion: 1,
    setMenuVersion: jest.fn(),
    isLoading: false,
    error: null,
    setError: jest.fn(),
    defaultCurrency: "USD",
    loadMenu: jest.fn(),
    tString: (k: string) => k,
    supportedLanguages: [],
    businessLanguages: [],
    selectedLanguages: [],
    setSelectedLanguages: jest.fn(),
    defaultLanguage: "en",
    setDefaultLanguage: jest.fn(),
    isLanguageLoading: false,
    currentViewLanguage: "en",
    setCurrentViewLanguage: jest.fn(),
    languagesLoaded: true,
    handleTranslateMenu: jest.fn(),
    handleSyncAllTranslations: jest.fn(),
    handleLanguageUpdate: jest.fn(),
    isTranslating: false,
    syncProgress: null,
  }),
}));

jest.mock("../hooks/useMenuMutations", () => ({
  useMenuMutations: () => ({
    selectedCategoryIndex: 0,
    setSelectedCategoryIndex: jest.fn(),
    selectedItemIndex: 0,
    setSelectedItemIndex: jest.fn(),
    editingCategory: null,
    setEditingCategory: jest.fn(),
    editingItem: EDITING_ITEM,
    setEditingItem: jest.fn(),
    handleAddCategory: jest.fn(),
    handleUpdateCategory: jest.fn(),
    handleDeleteCategory: jest.fn(),
    handleAddItem: jest.fn(),
    handleUpdateItem: jest.fn(),
    handleDeleteItem: jest.fn(),
    handleGenerateBreakdown: jest.fn(),
    handleGeneratePhoto: jest.fn(),
    handleEnhancePhoto: jest.fn(),
    isGeneratingBreakdown: false,
    isGeneratingPhoto: false,
    isEnhancingPhoto: false,
  }),
}));

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => ({ hasAccess: true, aiConfigured: false, loading: false }),
}));

jest.mock("@/api/inventory", () => ({
  inventoryApi: {
    getSummary: jest.fn().mockResolvedValue({ menu_item_statuses: [] }),
  },
}));

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { error: jest.fn() },
}));

jest.mock("../components/MenuPageHeader", () => ({ MenuPageHeader: () => null }));
jest.mock("../components/ActiveLanguagePills", () => ({
  ActiveLanguagePills: () => null,
}));
jest.mock("../components/AIMenuOnboardingModal", () => ({
  AIMenuOnboardingModal: () => null,
}));
jest.mock("../MenuLoadingSkeleton", () => ({ MenuLoadingSkeleton: () => null }));
jest.mock("../../OffersManager", () => ({ __esModule: true, default: () => null }));
jest.mock("../../BundlesManager", () => ({ __esModule: true, default: () => null }));
jest.mock("../../modals/AddCategoryModal", () => ({
  __esModule: true,
  default: () => null,
}));
jest.mock("../../modals/EditCategoryModal", () => ({
  __esModule: true,
  default: () => null,
}));
jest.mock("../../modals/ConfirmationModal", () => ({
  __esModule: true,
  default: () => null,
}));
jest.mock("../../modals/AddItemModal", () => ({
  __esModule: true,
  default: () => null,
}));

jest.mock("../components/MenuList", () => ({
  MenuList: ({
    handleEditItem,
  }: {
    handleEditItem: (categoryIndex: number, itemIndex: number) => void;
  }) => (
    <button type="button" onClick={() => handleEditItem(0, 0)}>
      open-edit
    </button>
  ),
}));

jest.mock("../../modals/EditItemModal", () => ({
  __esModule: true,
  default: (props: Record<string, unknown>) =>
    require("./_moneySeedHarness").editHarness(props),
}));

import MenuBuilder from "../index";

it("seeds Price and Food cost with matching 2-decimal strings (#184)", async () => {
  render(<MenuBuilder businessId={1} />);
  await act(async () => {
    await Promise.resolve();
  });
  fireEvent.click(screen.getByText("open-edit"));
  expect(screen.getByTestId("seeded-price")).toHaveTextContent("12.00");
  expect(screen.getByTestId("seeded-cogs")).toHaveTextContent("3.50");
});
