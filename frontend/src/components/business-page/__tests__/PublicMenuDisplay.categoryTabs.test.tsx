/** @jest-environment jsdom */
import { fireEvent, render, screen } from "@testing-library/react";
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
      { id: "i1", name: "Steak", description: "", price: 25, images: [], options: [], allergens: [], dietary_tags: [], is_available: true, item_type: "menu_item" as const, menu_item_id: "i1" },
    ]},
    { name: "Sides", description: "", items: [] },
  ],
  offers: [],
  bundles: [],
  loading: false,
   
  designSettings: { primary_color: "#1a6b6a", secondary_color: "#2a8b8a", corner_radius: "medium", shadow_intensity: "subtle", font_family: "Inter" },
   
  businessCurrencies: { default_currency: "USD", display_currency: "USD" },
};

describe("PublicMenuDisplay category tabs", () => {
  it("does not use scale-105, font-title text-white, or uppercase tracking-[0.1em]", () => {
    const { container } = render(<PublicMenuDisplay {...(baseProps as any)} />);
    expect(container.innerHTML).not.toMatch(/\bscale-105\b/);
    expect(container.innerHTML).not.toMatch(/font-title text-white shadow-xl/);
    expect(container.innerHTML).not.toMatch(/uppercase tracking-\[0\.1em\]/);
  });

  it("renders category buttons with role=tab", () => {
    const { container } = render(<PublicMenuDisplay {...(baseProps as any)} />);
    expect(container.querySelectorAll('[role="tab"]').length).toBeGreaterThanOrEqual(2);
  });

  it("connects keyboard-reachable category tabs to the shared item tabpanel", () => {
    render(<PublicMenuDisplay {...(baseProps as any)} />);

    const tabs = screen.getAllByRole("tab");
    const panel = screen.getByRole("tabpanel");
    tabs.forEach((tab) => {
      expect(tab.id).not.toBe("");
      expect(tab).toHaveAttribute("aria-controls", panel.id);
    });
    const selected = tabs.filter((tab) => tab.getAttribute("aria-selected") === "true");
    expect(selected).toHaveLength(1);
    expect(selected[0]).toHaveAttribute("tabindex", "0");
    tabs
      .filter((tab) => tab !== selected[0])
      .forEach((tab) => expect(tab).toHaveAttribute("tabindex", "-1"));
    expect(panel).toHaveAttribute("aria-labelledby", tabs[0].id);

    fireEvent.click(screen.getByRole("tab", { name: "Mains" }));
    expect(panel).toHaveAttribute(
      "aria-labelledby",
      screen.getByRole("tab", { name: "Mains" }).id,
    );
  });

  it("keeps the last category selected instead of resetting it to all items", () => {
    render(<PublicMenuDisplay {...(baseProps as any)} />);

    const sides = screen.getByRole("tab", { name: "Sides" });
    fireEvent.click(sides);

    expect(sides).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tab", { name: "menu.allItems" })).toHaveAttribute(
      "aria-selected",
      "false",
    );
  });
});
