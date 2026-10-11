/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

let latestMenuListProps: any;

jest.mock("../hooks/useMenuData", () => ({
  useMenuData: () => ({
    menu: [
      {
        name: "Mains",
        description: "",
        items: [
          {
            id: "steak",
            name: "Steak Plate",
            description: "",
            price: 25,
            is_available: true,
          },
          {
            id: "tacos",
            name: "Taco Plate",
            description: "",
            price: 15,
            is_available: true,
          },
        ],
      },
      {
        name: "Mains",
        description: "Legacy duplicate name",
        items: [
          {
            id: "salmon",
            name: "Salmon Plate",
            description: "",
            price: 20,
            is_available: true,
          },
        ],
      },
    ],
    itemOrderability: {
      steak: { state: "inventory_out", orderable: false },
      // Warn-mode zero stock now blocks sale like hard_block.
      tacos: { state: "inventory_out", orderable: false },
      salmon: { state: "inventory_warning", orderable: true },
    },
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

jest.mock("../components/MenuPageHeader", () => ({
  MenuPageHeader: ({ setSearchFilter }: any) => (
    <>
      <button onClick={() => setSearchFilter("available")}>available-filter</button>
      <button onClick={() => setSearchFilter("unavailable")}>unavailable-filter</button>
    </>
  ),
}));
jest.mock("../components/MenuList", () => ({
  MenuList: (props: any) => {
    latestMenuListProps = props;
    return null;
  },
}));
jest.mock("../components/ActiveLanguagePills", () => ({ ActiveLanguagePills: () => null }));
jest.mock("../components/LanguagesPopover", () => ({ LanguagesPopover: () => null }));
jest.mock("../components/AIMenuOnboardingModal", () => ({ AIMenuOnboardingModal: () => null }));
jest.mock("../MenuLoadingSkeleton", () => ({ MenuLoadingSkeleton: () => null }));
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
jest.mock("../../DirectorConsole/ProposalCard", () => ({ __esModule: true, default: () => null }));

import MenuBuilder from "../index";

describe("MenuBuilder authoritative orderability", () => {
  beforeEach(() => {
    latestMenuListProps = undefined;
  });

  it("passes the backend orderability decision to item badges", async () => {
    render(<MenuBuilder businessId={1} />);

    await waitFor(() => expect(latestMenuListProps).toBeDefined());
    expect(latestMenuListProps.filteredMenu[0].items[0].is_available).toBe(true);
    expect(latestMenuListProps.filteredMenu[0].items[0].orderability_state).toBe(
      "inventory_out",
    );
  });

  it("uses backend orderability for the available and unavailable filters", async () => {
    render(<MenuBuilder businessId={1} />);

    fireEvent.click(screen.getByText("available-filter"));
    await waitFor(() => {
      const names = latestMenuListProps.filteredMenu.flatMap((c: any) =>
        c.items.map((i: any) => i.name),
      );
      // Low-stock warn still available; zero-stock inventory_out is not.
      expect(names).toEqual(["Salmon Plate"]);
    });

    fireEvent.click(screen.getByText("unavailable-filter"));
    await waitFor(() => {
      const names = latestMenuListProps.filteredMenu.flatMap((c: any) =>
        c.items.map((i: any) => i.name),
      );
      expect(names).toEqual(expect.arrayContaining(["Steak Plate", "Taco Plate"]));
      expect(names).toHaveLength(2);
    });
  });

  it("includes warn-mode zero-stock (inventory_out) in the unavailable filter", async () => {
    render(<MenuBuilder businessId={1} />);

    fireEvent.click(screen.getByText("unavailable-filter"));
    await waitFor(() => {
      const names = latestMenuListProps.filteredMenu.flatMap((c: any) =>
        c.items.map((i: any) => i.name),
      );
      expect(names).toContain("Taco Plate");
    });

    // And must NOT appear under available.
    fireEvent.click(screen.getByText("available-filter"));
    await waitFor(() => {
      const names = latestMenuListProps.filteredMenu.flatMap((c: any) =>
        c.items.map((i: any) => i.name),
      );
      expect(names).not.toContain("Taco Plate");
    });
  });

  it("carries each no-id category's original index through orderability projection", async () => {
    render(<MenuBuilder businessId={1} />);

    await waitFor(() => expect(latestMenuListProps).toBeDefined());
    expect(latestMenuListProps.filteredMenu).toHaveLength(2);
    expect(latestMenuListProps.filteredMenu[0].sourceCategoryIndex).toBe(0);
    expect(latestMenuListProps.filteredMenu[1].sourceCategoryIndex).toBe(1);
  });
});
