/**
 * #131 product lock: Menu Engineering is suggestion-only.
 * There is no one-click "Propose new price" / createPriceChangeProposal path
 * from the matrix — so the L3-20 402 toast path is retired from this surface.
 *
 * @jest-environment jsdom
 */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";

const mockGetMenuEngineering = jest.fn();

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: Object.assign(jest.fn(), {
    error: jest.fn(),
    success: jest.fn(),
  }),
}));

jest.mock("@/api/menuEngineering", () => ({
  menuEngineeringApi: {
    getMenuEngineering: (...a: unknown[]) => mockGetMenuEngineering(...a),
  },
}));

jest.mock("../hooks/useMenuData", () => ({
  useMenuData: () => ({
    menu: [
      {
        name: "Mains",
        description: "",
        items: [
          {
            id: "dish-1",
            name: "Burger",
            description: "",
            price: 10,
            currency: "USD",
            is_available: true,
            allergens: [],
            dietary_tags: [],
          },
        ],
      },
    ],
    setMenu: jest.fn(),
    itemOrderability: {},
    menuVersion: 1,
    setMenuVersion: jest.fn(),
    isLoading: false,
    error: null,
    setError: jest.fn(),
    business: { id: 1, name: "Demo", currency: "USD" },
    defaultCurrency: "USD",
    loadMenu: jest.fn(),
    tString: (k: string) => k,
    supportedLanguages: [{ code: "en", native_name: "English" }],
    businessLanguages: ["en"],
    selectedLanguages: ["en"],
    setSelectedLanguages: jest.fn(),
    defaultLanguage: "en",
    setDefaultLanguage: jest.fn(),
    isLanguageLoading: false,
    currentViewLanguage: "en",
    setCurrentViewLanguage: jest.fn(),
    languagesLoaded: true,
    handleTranslateMenu: jest.fn(),
    handleSyncAllTranslations: jest.fn(),
    cancelSyncAllTranslations: jest.fn(),
    handleLanguageUpdate: jest.fn(),
    isTranslating: false,
    syncProgress: null,
    syncError: null,
    hasLanguageChanges: false,
  }),
}));

jest.mock("../hooks/useMenuMutations", () => ({
  useMenuMutations: () => ({
    selectedCategoryIndex: null,
    setSelectedCategoryIndex: jest.fn(),
    selectedItemIndex: null,
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
    handleToggleEightySix: jest.fn(),
    handleDeleteItem: jest.fn(),
    handleGenerateBreakdown: jest.fn(),
    handleGeneratePhoto: jest.fn(),
    handleEnhancePhoto: jest.fn(),
    isGeneratingBreakdown: false,
    isGeneratingPhoto: false,
    isEnhancingPhoto: false,
    isSavingItem: false,
    dailyLimitReached: null,
    sanitizationReview: null,
    isConfirmingSanitization: false,
    confirmPendingSanitization: jest.fn(),
    cancelPendingSanitization: jest.fn(),
    // Form state stubs used by modals
    categoryName: "",
    setCategoryName: jest.fn(),
    categoryDescription: "",
    setCategoryDescription: jest.fn(),
    itemName: "",
    setItemName: jest.fn(),
    itemDescription: "",
    setItemDescription: jest.fn(),
    itemPrice: "",
    setItemPrice: jest.fn(),
    itemCogs: "",
    setItemCogs: jest.fn(),
    itemImages: [],
    setItemImages: jest.fn(),
    itemAvailable: true,
    setItemAvailable: jest.fn(),
    itemSortOrder: 0,
    setItemSortOrder: jest.fn(),
    itemOptions: [],
    setItemOptions: jest.fn(),
    itemAllergens: [],
    setItemAllergens: jest.fn(),
    itemDietaryTags: [],
    setItemDietaryTags: jest.fn(),
    newOptionName: "",
    setNewOptionName: jest.fn(),
    newOptionPrice: "",
    setNewOptionPrice: jest.fn(),
    newAllergen: "",
    setNewAllergen: jest.fn(),
    newDietaryTag: "",
    setNewDietaryTag: jest.fn(),
  }),
}));

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => ({
    hasAccess: true,
    aiConfigured: false,
    loading: false,
  }),
}));

jest.mock("@/hooks/useBusinessUrlId", () => ({
  useBusinessUrlId: () => "demo",
}));

jest.mock("next/navigation", () => ({
  useSearchParams: () => new URLSearchParams(),
}));

jest.mock("../components/MenuPageHeader", () => ({
  MenuPageHeader: () => null,
}));
jest.mock("../components/ActiveLanguagePills", () => ({
  ActiveLanguagePills: () => null,
}));
jest.mock("../components/LanguagesPopover", () => ({
  LanguagesPopover: () => null,
}));
jest.mock("../components/AddSourceMenu", () => ({
  AddSourceMenu: () => null,
}));
jest.mock("../components/MenuList", () => ({
  MenuList: () => null,
}));
jest.mock("../MenuLoadingSkeleton", () => ({
  MenuLoadingSkeleton: () => null,
}));
jest.mock("../print/PrintMenuButton", () => ({
  PrintMenuButton: () => null,
}));
jest.mock("../print/PrintMenuStudio", () => ({
  PrintMenuStudio: () => null,
}));
jest.mock("../../OffersManager", () => ({ __esModule: true, default: () => <div data-testid="offers-manager" /> }));
jest.mock("../../BundlesManager", () => ({ __esModule: true, default: () => null }));
jest.mock("../../DirectorConsole/ProposalCard", () => ({
  __esModule: true,
  default: () => null,
}));
jest.mock("../../modals/AddCategoryModal", () => ({ __esModule: true, default: () => null }));
jest.mock("../../modals/EditCategoryModal", () => ({ __esModule: true, default: () => null }));
jest.mock("../../modals/AddItemModal", () => ({ __esModule: true, default: () => null }));
jest.mock("../../modals/EditItemModal", () => ({ __esModule: true, default: () => null }));
jest.mock("../../modals/ConfirmationModal", () => ({ __esModule: true, default: () => null }));
jest.mock("../components/AIMenuOnboardingModal", () => ({
  AIMenuOnboardingModal: () => null,
}));
jest.mock("../AIMenuOnboarding/MenuSanitizationReview", () => ({
  __esModule: true,
  default: () => null,
}));

import { asDollars } from "@/types/money";
import MenuBuilder from "../index";

beforeEach(() => {
  mockGetMenuEngineering.mockResolvedValue({
    period: "week",
    median_food_cost_pct: 0.3,
    median_qty_sold: 50,
    dishes: [
      {
        menu_item_id: "dish-1",
        menu_item_name: "Burger",
        food_cost_pct: 0.4,
        qty_sold: 200,
        avg_price: asDollars(10),
        unit_cost: asDollars(4),
        margin_per_unit: asDollars(6),
        quadrant: "plowhorse",
        action: "reprice_up",
        suggested_price: asDollars(12),
      },
    ],
    rollups: [{ quadrant: "plowhorse", count: 1, revenue_share: 1 }],
    items_needing_cost: 0,
    sparse: false,
    has_sales: true,
  });
});

describe("MenuBuilder engineering suggestion-only (#131)", () => {
  it("shows a read-only suggestion and never a propose button / ProposalCard", async () => {
    render(<MenuBuilder businessId={1} />);

    // Switch to engineering via the tab rail.
    const engTab = await screen.findByRole("tab", {
      name: /tabs\.engineering/i,
    });
    engTab.click();

    await waitFor(() => {
      expect(screen.getByTestId("menu-tab-panel-engineering")).toBeInTheDocument();
    });
    expect(screen.queryByTestId("menu-tab-panel-offers")).not.toBeInTheDocument();
    expect(screen.queryByTestId("offers-manager")).not.toBeInTheDocument();

    expect(await screen.findByTestId("suggest-dish-1")).toBeInTheDocument();
    expect(screen.queryByTestId("propose-dish-1")).toBeNull();
    expect(
      screen.queryByRole("button", { name: /propose|proponer/i }),
    ).toBeNull();
  });
});
