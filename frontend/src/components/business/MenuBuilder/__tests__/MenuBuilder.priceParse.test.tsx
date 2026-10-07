/** @jest-environment jsdom */
/**
 * MONEY regression: the Add/Edit item submit handlers in MenuBuilder/index.tsx
 * must parse the typed dollar price with parseLocaleDecimal (locale-tolerant),
 * NOT parseFloat. In comma-decimal operator locales (es / es-AR) an operator
 * types "5,50"; parseFloat("5,50") === 5 would silently save $5.00 instead of
 * $5.50. The modal already enables submit for "5,50" (it validates with
 * parseLocaleDecimal), so the old parseFloat created a validate/submit mismatch
 * that truncated cents.
 *
 * This test drives the REAL onAddItemSubmit / onUpdateItemSubmit handlers (it
 * does not stub them) and asserts the dollar value handed to the mutation hook
 * is 5.5, not 5.
 */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";

// Capture the payloads the real submit handlers build.
const mockHandleAddItem = jest.fn().mockResolvedValue(true);
const mockHandleUpdateItem = jest.fn().mockResolvedValue(true);

const EDITING_ITEM = {
  id: 1,
  name: "Existing",
  description: "",
  price: 9.99,
  is_available: true,
  options: [],
  allergens: [],
  dietary_tags: [],
  sort_order: 0,
};

jest.mock("../hooks/useMenuData", () => ({
  useMenuData: () => ({
    menu: [{ name: "Cat", description: "", items: [EDITING_ITEM] }],
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
    handleAddItem: mockHandleAddItem,
    handleUpdateItem: mockHandleUpdateItem,
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
  inventoryApi: { getSummary: jest.fn().mockResolvedValue({ menu_item_statuses: [] }) },
}));

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { error: jest.fn() },
}));

// Stub heavy children that are irrelevant here. MenuList exposes the
// add/edit triggers so we can open the real modals through index.tsx.
jest.mock("../components/MenuPageHeader", () => ({ MenuPageHeader: () => null }));
jest.mock("../components/ActiveLanguagePills", () => ({ ActiveLanguagePills: () => null }));
jest.mock("../components/AIMenuOnboardingModal", () => ({ AIMenuOnboardingModal: () => null }));
jest.mock("../MenuLoadingSkeleton", () => ({ MenuLoadingSkeleton: () => null }));
jest.mock("../../OffersManager", () => ({ __esModule: true, default: () => null }));
jest.mock("../../BundlesManager", () => ({ __esModule: true, default: () => null }));
jest.mock("../../modals/AddCategoryModal", () => ({ __esModule: true, default: () => null }));
jest.mock("../../modals/EditCategoryModal", () => ({ __esModule: true, default: () => null }));
jest.mock("../../modals/ConfirmationModal", () => ({ __esModule: true, default: () => null }));

jest.mock("../components/MenuList", () => ({
  MenuList: ({
    onAddItemOpen,
    handleEditItem,
  }: {
    onAddItemOpen: (categoryIndex: number) => void;
    handleEditItem: (categoryIndex: number, itemIndex: number) => void;
  }) => (
    <div>
      <button onClick={() => onAddItemOpen(0)}>open-add</button>
      <button onClick={() => handleEditItem(0, 0)}>open-edit</button>
    </div>
  ),
}));

// The modal stand-ins (see _priceParseHarness) receive the REAL itemPrice /
// setItemPrice / onAddItem (or onUpdateItem) wired by index.tsx. Clicking
// "set-price" pushes "5,50" into the real itemPrice state (re-rendering index,
// so the submit closure is fresh) and clicking "submit" invokes the real
// submit handler — exercising parseLocaleDecimal -> asDollars end to end.
jest.mock("../../modals/AddItemModal", () => ({
  __esModule: true,
  default: (props: Record<string, unknown>) =>
    require("./_priceParseHarness").addHarness(props),
}));
jest.mock("../../modals/EditItemModal", () => ({
  __esModule: true,
  default: (props: Record<string, unknown>) =>
    require("./_priceParseHarness").editHarness(props),
}));

import MenuBuilder from "../index";

beforeEach(() => jest.clearAllMocks());

it("create: a comma-decimal price '5,50' is submitted as 5.5, not 5 (MONEY)", async () => {
  render(<MenuBuilder businessId={1} />);

  fireEvent.click(screen.getByText("open-add"));
  fireEvent.click(screen.getByText("set-price-onAddItem"));
  fireEvent.click(screen.getByText("submit-onAddItem"));

  await waitFor(() => expect(mockHandleAddItem).toHaveBeenCalledTimes(1));
  const payload = mockHandleAddItem.mock.calls[0][0];
  expect(payload.price).toBe(5.5);
  expect(payload.price).not.toBe(5);
});

it("update: a comma-decimal price '5,50' is submitted as 5.5, not 5 (MONEY)", async () => {
  render(<MenuBuilder businessId={1} />);

  fireEvent.click(screen.getByText("open-edit"));
  fireEvent.click(screen.getByText("set-price-onUpdateItem"));
  fireEvent.click(screen.getByText("submit-onUpdateItem"));

  await waitFor(() => expect(mockHandleUpdateItem).toHaveBeenCalledTimes(1));
  const payload = mockHandleUpdateItem.mock.calls[0][0];
  expect(payload.price).toBe(5.5);
  expect(payload.price).not.toBe(5);
});
