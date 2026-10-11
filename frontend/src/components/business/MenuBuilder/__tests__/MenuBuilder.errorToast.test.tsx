/** @jest-environment jsdom */
/**
 * F23: useMenuMutations sets an `error` string on every failed add/update/
 * delete and on version conflicts, but index.tsx never rendered it — a silent
 * failure on a core operator path. The error must now surface (toast) and clear.
 */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";

const mockToastError = jest.fn();
jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { error: (...a: unknown[]) => mockToastError(...a) },
}));

const mockSetError = jest.fn();

// useMenuData returns a populated `error` (as if a mutation just failed).
jest.mock("../hooks/useMenuData", () => ({
  useMenuData: () => ({
    menu: [],
    setMenu: jest.fn(),
    menuVersion: 1,
    setMenuVersion: jest.fn(),
    isLoading: false,
    error: "No se pudo agregar el elemento",
    setError: mockSetError,
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
    editingItem: null,
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
  inventoryApi: { getSummary: jest.fn().mockResolvedValue(null) },
}));

// Stub the heavy child components — irrelevant to the error-surfacing behavior.
jest.mock("../components/MenuPageHeader", () => ({ MenuPageHeader: () => null }));
jest.mock("../components/ActiveLanguagePills", () => ({ ActiveLanguagePills: () => null }));
jest.mock("../components/AIMenuOnboardingModal", () => ({ AIMenuOnboardingModal: () => null }));
jest.mock("../components/MenuList", () => ({ MenuList: () => null }));
jest.mock("../MenuLoadingSkeleton", () => ({ MenuLoadingSkeleton: () => null }));
jest.mock("../../OffersManager", () => ({ __esModule: true, default: () => null }));
jest.mock("../../BundlesManager", () => ({ __esModule: true, default: () => null }));
jest.mock("../../modals/AddCategoryModal", () => ({ __esModule: true, default: () => null }));
jest.mock("../../modals/EditCategoryModal", () => ({ __esModule: true, default: () => null }));
jest.mock("../../modals/AddItemModal", () => ({ __esModule: true, default: () => null }));
jest.mock("../../modals/EditItemModal", () => ({ __esModule: true, default: () => null }));
jest.mock("../../modals/ConfirmationModal", () => ({ __esModule: true, default: () => null }));

import MenuBuilder from "../index";

beforeEach(() => jest.clearAllMocks());

it("surfaces a menu mutation error as a toast and clears it (F23)", async () => {
  render(<MenuBuilder businessId={1} />);

  await waitFor(() =>
    expect(mockToastError).toHaveBeenCalledWith(
      "No se pudo agregar el elemento",
    ),
  );
  // It clears the error so the next failure re-fires.
  await waitFor(() => expect(mockSetError).toHaveBeenCalledWith(null));
});

it("renders the Menu Engineering tab", async () => {
  render(<MenuBuilder businessId={1} />);

  // tString is mocked to echo the key, so the tab title surfaces as the key.
  await waitFor(() =>
    expect(screen.getByText("tabs.engineering")).toBeInTheDocument(),
  );
});
