/** @jest-environment jsdom */
/**
 * F21: public storefront menu items need a native, named details control so
 * keyboard activation works without turning the entire card into a button.
 */
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import PublicMenuDisplay from "../PublicMenuDisplay";

jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({
    t: (key: string, values?: Record<string, unknown>) =>
      key === "accessibility.openItem" ? `Open ${values?.name}` : key,
  }),
}));

// Render CurrencyPrice as its amount to keep the test focused.
jest.mock("@/components/common/CurrencyConverter", () => ({
  __esModule: true,
  CurrencyPrice: ({ amount }: { amount: number }) => <span>{amount}</span>,
}));

const baseProps = {
  customUrl: "test",
  businessId: 1,
  businessName: "Test",
  categories: [
    {
      name: "Mains",
      description: "",
      items: [
        {
          id: "i1",
          name: "Grilled Steak",
          description: "Char-grilled",
          price: 25,
          images: [],
          options: [],
          allergens: [],
          dietary_tags: [],
          is_available: true,
          item_type: "menu_item" as const,
          menu_item_id: "i1",
        },
      ],
    },
  ],
  offers: [],
  bundles: [],
  loading: false,
  designSettings: {
    primary_color: "#1a6b6a",
    secondary_color: "#2a8b8a",
    corner_radius: "medium",
    shadow_intensity: "subtle",
    font_family: "Inter",
  },
  businessCurrencies: { default_currency: "USD", display_currency: "USD" },
};

describe("PublicMenuDisplay item card keyboard access (F21)", () => {
  it("exposes a native details button with a contextual accessible name", () => {
    render(<PublicMenuDisplay {...(baseProps as any)} />);
    const details = screen.getByRole("button", { name: "Open Grilled Steak" });
    expect(details.tagName).toBe("BUTTON");
    expect(details.closest("article")).not.toHaveAttribute("role", "button");
  });

  it("opens the item modal through keyboard activation", async () => {
    const user = userEvent.setup();
    render(<PublicMenuDisplay {...(baseProps as any)} />);
    const details = screen.getByRole("button", { name: "Open Grilled Steak" });

    details.focus();
    await user.keyboard("{Enter}");

    expect(
      screen.getByRole("dialog", { name: "Grilled Steak" }),
    ).toHaveAttribute("aria-modal", "true");
  });
});
