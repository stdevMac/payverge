/**
 * D1 / L3-10: after an empty category exists (e.g. just created), the header
 * category stat must include it — not only categories that already have items.
 *
 * Pure `countMenuSearchResults` unit tests pass even if MenuBuilder never
 * wires the helper into DashboardTabShell stats. This mounts MenuBuilder and
 * asserts the rendered header stats.
 *
 * Revert-proof: skip empty categories in the counter (or stop using it in
 * index.tsx) → categories value becomes 1 while empty + filled exist.
 *
 * @jest-environment jsdom
 */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";

jest.mock("../hooks/useMenuData", () => ({
  useMenuData: () => ({
    menu: [
      {
        name: "Mains",
        description: "",
        items: [
          {
            id: "steak",
            name: "Steak",
            description: "",
            price: 25,
            is_available: true,
          },
          {
            id: "soup",
            name: "Soup",
            description: "",
            price: 8,
            is_available: true,
          },
        ],
      },
      // Newly created empty category — must bump header category count (L3-10).
      {
        name: "Desserts",
        description: "",
        items: [],
      },
    ],
    itemOrderability: {},
    setMenu: jest.fn(),
    menuVersion: 1,
    setMenuVersion: jest.fn(),
    isLoading: false,
    error: null,
    setError: jest.fn(),
    business: { id: 1, default_currency: "USD" },
    defaultCurrency: "USD",
    loadMenu: jest.fn(),
    tString: (key: string) => key,
    supportedLanguages: [{ code: "en", native_name: "English", name: "English" }],
    businessLanguages: [{ language_code: "en", is_default: true }],
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
    isSavingItem: false,
  }),
}));

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => ({ hasAccess: true, aiConfigured: false, loading: false }),
}));
jest.mock("@/hooks/useBusinessUrlId", () => ({ useBusinessUrlId: () => "1" }));
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => key,
}));
jest.mock("@/api/inventory", () => ({
  inventoryApi: {
    getSummary: jest.fn().mockResolvedValue({ menu_item_statuses: [] }),
  },
}));
jest.mock("next/navigation", () => ({
  useSearchParams: () => new URLSearchParams(),
  useRouter: () => ({ push: jest.fn() }),
}));
jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: Object.assign(jest.fn(), { error: jest.fn(), success: jest.fn() }),
}));

// Capture searchResultsCount wiring into the toolbar as a second surface.
let latestHeaderProps: {
  searchResultsCount?: { totalItems: number; totalCategories: number };
} = {};

jest.mock("../components/MenuPageHeader", () => ({
  MenuPageHeader: (props: {
    searchResultsCount: { totalItems: number; totalCategories: number };
  }) => {
    latestHeaderProps = props;
    return (
      <div data-testid="menu-search-stats">
        items={props.searchResultsCount.totalItems} cats=
        {props.searchResultsCount.totalCategories}
      </div>
    );
  },
}));
jest.mock("../components/MenuList", () => ({
  MenuList: () => null,
}));
jest.mock("../components/ActiveLanguagePills", () => ({
  ActiveLanguagePills: () => null,
}));
jest.mock("../components/LanguagesPopover", () => ({
  LanguagesPopover: () => null,
}));
jest.mock("../components/AIMenuOnboardingModal", () => ({
  AIMenuOnboardingModal: () => null,
}));
jest.mock("../MenuLoadingSkeleton", () => ({
  MenuLoadingSkeleton: () => null,
}));
jest.mock("../../OffersManager", () => ({ __esModule: true, default: () => null }));
jest.mock("../../BundlesManager", () => ({ __esModule: true, default: () => null }));
jest.mock("../../modals/AddCategoryModal", () => ({ __esModule: true, default: () => null }));
jest.mock("../../modals/EditCategoryModal", () => ({ __esModule: true, default: () => null }));
jest.mock("../../modals/AddItemModal", () => ({ __esModule: true, default: () => null }));
jest.mock("../../modals/EditItemModal", () => ({ __esModule: true, default: () => null }));
jest.mock("../../modals/ConfirmationModal", () => ({ __esModule: true, default: () => null }));
jest.mock("../print/PrintMenuButton", () => ({ PrintMenuButton: () => null }));
jest.mock("../print/PrintMenuStudio", () => ({ PrintMenuStudio: () => null }));
jest.mock("../MenuEngineeringMatrix", () => ({ __esModule: true, default: () => null }));
jest.mock("../../DirectorConsole/ProposalCard", () => ({
  __esModule: true,
  default: () => null,
}));
jest.mock("../components/AddSourceMenu", () => ({
  AddSourceMenu: () => null,
}));

import MenuBuilder from "../index";

describe("MenuBuilder L3-10 header/search stats include empty categories (DOM)", () => {
  beforeEach(() => {
    latestHeaderProps = {};
  });

  it("counts the empty category in searchResultsCount and shell header", async () => {
    render(<MenuBuilder businessId={1} />);

    await waitFor(() => {
      expect(screen.getByTestId("menu-search-stats")).toHaveTextContent(
        "items=2 cats=2",
      );
    });

    expect(latestHeaderProps.searchResultsCount).toEqual({
      totalItems: 2,
      totalCategories: 2,
    });

    // DashboardTabShell header stats also surface the category count.
    // PageHeader: value + label as sibling spans under one row item.
    await waitFor(() => {
      expect(screen.getByText("header.categories")).toBeInTheDocument();
    });
    const categoryLabel = screen.getByText("header.categories");
    const categoryStat = categoryLabel.parentElement;
    // Two categories (one empty) — value must be 2, not 1.
    expect(categoryStat?.textContent).toMatch(/^2\s*header\.categories$/);
  });
});
