/** @jest-environment jsdom */
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import PublicMenuDisplay from "../PublicMenuDisplay";
import { asDollars } from "@/types/money";

let mockLocale: "en" | "es" = "en";

const mockMenuCopy = {
  en: {
    "menu.allItems": "All Items",
    "menu.categoryNav": "Menu categories",
  },
  es: {
    "menu.allItems": "Todos los elementos",
    "menu.categoryNav": "Categorías del menú",
  },
} as const;

jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({
    t: (key: string) =>
      (mockMenuCopy[mockLocale] as Record<string, string>)[key] || key,
    currentLanguage: mockLocale,
  }),
}));

function item(id: string, name: string) {
  return {
    id,
    name,
    description: "",
    price: asDollars(10),
    images: [],
    options: [],
    allergens: [],
    dietary_tags: [],
    is_available: true,
    item_type: "menu_item" as const,
    menu_item_id: id,
  };
}

function renderMenu() {
  return render(
    <PublicMenuDisplay
      customUrl="test"
      businessId={1}
      businessName="Test"
      categories={[
        { name: mockLocale === "es" ? "Principales" : "Mains", description: "", items: [item("i1", "Steak")] },
        { name: mockLocale === "es" ? "Acompañamientos" : "Sides", description: "", items: [item("i2", "Bread")] },
      ]}
      offers={[]}
      bundles={[]}
      loading={false}
      designSettings={{
        primary_color: "#1a6b6a",
        secondary_color: "#2a8b8a",
        corner_radius: "medium",
        shadow_intensity: "subtle",
        font_family: "Inter",
      }}
      businessCurrencies={{ default_currency: "USD", display_currency: "USD" }}
    /> as any,
  );
}

describe("PublicMenuDisplay category tablist keyboard", () => {
  afterEach(() => {
    mockLocale = "en";
  });

  it.each([
    ["English", "en", "All Items", "Mains", "Sides"] as const,
    ["Spanish", "es", "Todos los elementos", "Principales", "Acompañamientos"] as const,
  ])(
    "implements roving tabindex and arrow/Home/End keys (%s)",
    async (_label, nextLocale, allItems, mains, sides) => {
      mockLocale = nextLocale;
      const user = userEvent.setup();
      renderMenu();

      const tablist = screen.getByRole("tablist", {
        name: mockMenuCopy[nextLocale]["menu.categoryNav"],
      });
      const tabs = within(tablist).getAllByRole("tab");
      expect(tabs).toHaveLength(3);
      expect(tabs[0]).toHaveAccessibleName(allItems);
      expect(tabs[0]).toHaveAttribute("tabindex", "0");
      expect(tabs[1]).toHaveAttribute("tabindex", "-1");
      expect(tabs[2]).toHaveAttribute("tabindex", "-1");

      tabs[0].focus();
      await user.keyboard("{ArrowRight}");
      const mainsTab = within(tablist).getByRole("tab", { name: mains });
      expect(mainsTab).toHaveFocus();
      expect(mainsTab).toHaveAttribute("aria-selected", "true");
      expect(mainsTab).toHaveAttribute("tabindex", "0");
      expect(within(tablist).getByRole("tab", { name: allItems })).toHaveAttribute(
        "tabindex",
        "-1",
      );

      await user.keyboard("{End}");
      const sidesTab = within(tablist).getByRole("tab", { name: sides });
      expect(sidesTab).toHaveFocus();
      expect(sidesTab).toHaveAttribute("aria-selected", "true");

      await user.keyboard("{Home}");
      const allTab = within(tablist).getByRole("tab", { name: allItems });
      expect(allTab).toHaveFocus();
      expect(allTab).toHaveAttribute("aria-selected", "true");
    },
  );
});
