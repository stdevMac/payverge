/** @jest-environment jsdom */
import { render } from "@testing-library/react";
import PublicMenuDisplay from "../PublicMenuDisplay";

const t = (k: string) => k;
jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({ t }),
}));

const baseProps = {
  customUrl: "test",
  businessId: 1,
  businessName: "Test",
  categories: [
    { name: "Mains", description: "", items: [
      { id: "i1", name: "Steak", description: "Grilled", price: 25, images: [], options: [], allergens: [], dietary_tags: [], is_available: true, item_type: "menu_item" as const, menu_item_id: "i1" },
    ]},
  ],
  offers: [],
  bundles: [],
  loading: false,
   
  designSettings: { primary_color: "#1a6b6a", secondary_color: "#2a8b8a", corner_radius: "medium", shadow_intensity: "subtle", font_family: "Inter" },
   
  businessCurrencies: { default_currency: "USD", display_currency: "USD" },
};

describe("PublicMenuDisplay item card", () => {
  it("does not use hover:-translate-y-2 or backdrop-blur-md", () => {
    const { container } = render(<PublicMenuDisplay {...(baseProps as any)} />);
    expect(container.innerHTML).not.toMatch(/\bhover:-translate-y-2\b/);
    expect(container.innerHTML).not.toMatch(/\bbackdrop-blur-md\b/);
  });

  it("does not use the heavy 0_8px_32px shadow literals", () => {
    const { container } = render(<PublicMenuDisplay {...(baseProps as any)} />);
    expect(container.innerHTML).not.toMatch(/0_8px_32px/);
  });
});
