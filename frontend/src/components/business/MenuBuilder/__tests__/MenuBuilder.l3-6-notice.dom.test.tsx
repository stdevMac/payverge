/**
 * D1 / L3-6: translated-view banner must name the viewed language and the
 * edit language with no residual `{language}` / `{viewLanguage}` placeholders.
 *
 * Pure `interpolateTranslatedViewNotice` unit tests pass even if MenuBuilder
 * never calls the helper or swaps the arg order. This mounts MenuBuilder and
 * asserts the amber banner DOM when viewing a non-default language.
 *
 * Revert-proof: feed a dual-{language} template via tString and drop the helper
 * (single String.replace) → second placeholder stays literal and this fails.
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
        ],
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
    // Dual-{language} legacy shape — production en uses {viewLanguage}/
    // {editLanguage}; both must interpolate fully.
    tString: (key: string) => {
      if (key === "translatedViewNotice") {
        return "You're viewing the {language} translation — read-only. Switch to {language} to edit.";
      }
      return key;
    },
    supportedLanguages: [
      { code: "en", native_name: "English", name: "English" },
      { code: "es", native_name: "Español", name: "Spanish" },
    ],
    businessLanguages: [
      { language_code: "en", is_default: true },
      { language_code: "es", is_default: false },
    ],
    selectedLanguages: ["en", "es"],
    setSelectedLanguages: jest.fn(),
    defaultLanguage: "en",
    setDefaultLanguage: jest.fn(),
    isLanguageLoading: false,
    // Non-default view → banner must render.
    currentViewLanguage: "es",
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

jest.mock("../components/MenuPageHeader", () => ({
  MenuPageHeader: () => null,
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

describe("MenuBuilder L3-6 translated-view notice (DOM)", () => {
  it("renders viewed + edit language names with no residual placeholders", async () => {
    render(<MenuBuilder businessId={1} />);

    await waitFor(() => {
      expect(
        screen.getByText(/You're viewing the Español translation/i),
      ).toBeInTheDocument();
    });

    const banner = screen.getByText(/You're viewing the Español translation/i);
    expect(banner.textContent).toMatch(/Switch to English to edit/i);
    expect(banner.textContent).not.toMatch(/\{language\}/);
    expect(banner.textContent).not.toMatch(/\{viewLanguage\}/);
    expect(banner.textContent).not.toMatch(/\{editLanguage\}/);
    // Wrong-order swap (default first) would name English as the viewed locale.
    expect(banner.textContent).not.toMatch(
      /You're viewing the English translation/i,
    );
  });
});
