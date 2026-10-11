/** @jest-environment jsdom */
import { render, screen } from "@testing-library/react";
import PublicMenuDisplay from "../PublicMenuDisplay";

const t = (key: string, values?: Record<string, unknown>) =>
  key === "accessibility.openItem" ? `Open ${values?.name}` : key;
jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({ t }),
}));

// Copied from PublicMenuDisplay.itemCard.test.tsx so the card actually renders
// and the className assertions execute (full item shape + required top-level props).
const baseProps = {
  customUrl: "test",
  businessId: 1,
  businessName: "Test",
  categories: [
    { name: "Mains", description: "", items: [
      { id: "i1", name: "Burger", description: "Grilled", price: 25, images: [], options: [], allergens: [], dietary_tags: [], is_available: true, item_type: "menu_item" as const, menu_item_id: "i1" },
    ]},
  ],
  offers: [],
  bundles: [],
  loading: false,
  designSettings: { primary_color: "#1a6b6a", secondary_color: "#2a8b8a", corner_radius: "medium", shadow_intensity: "subtle", font_family: "Inter", menu_layout: "grid" },
  businessCurrencies: { default_currency: "USD", display_currency: "USD" },
};

describe("PublicMenuDisplay — premium menu-card hover (BEAUTY-3)", () => {
  it("the menu card has a lift + shadow hover, a motion-reduce guard, and keeps the focus-visible ring", () => {
    render(<PublicMenuDisplay {...(baseProps as any)} />);
    const details = screen.getByRole("button", { name: "Open Burger" });
    const card = details.closest("article");
    expect(card).not.toBeNull();
    // Premium hover: lift + shadow elevation + smooth transition.
    expect(card!.className).toContain("hover:-translate-y-0.5");
    expect(card!.className).toContain("hover:shadow-lg");
    expect(card!.className).toContain("transition-all");
    // a11y: reduced-motion suppresses the transform.
    expect(card!.className).toContain("motion-reduce:transform-none");
    // Preserved bars: group + focus-visible ring + radius/shadow tokens.
    expect(card!.className).toContain("group");
    expect(details.className).toContain("focus-visible:ring-brand");
  });

  it("the menu item image zooms on card hover and is motion-reduce gated (BEAUTY-3)", () => {
    const { container } = render(<PublicMenuDisplay {...(baseProps as any)} />);
    // MenuItemMedia renders MenuItemNoMediaHeader when there are no loadable
    // images. To exercise the zoomed <img>, give the item a real image URL.
    // (next/image renders an <img>; jsdom does not load it, which is fine —
    // we assert on the className, not the pixels.)
    const withImage = {
      ...baseProps,
      categories: [
        { name: "Mains", description: "", items: [
          { ...(baseProps.categories[0].items[0]), images: ["https://example.com/burger.jpg"] },
        ]},
      ],
    };
    const { container: c2 } = render(<PublicMenuDisplay {...(withImage as any)} />);
    const img = c2.querySelector('img[alt="Burger"]') as HTMLElement | null;
    expect(img).not.toBeNull();
    // Slow zoom on group-hover, clipped by the media container's overflow-hidden.
    expect(img!.className).toContain("group-hover:scale-[1.04]");
    expect(img!.className).toContain("transition-transform");
    // a11y: reduced-motion suppresses the zoom too.
    expect(img!.className).toContain("motion-reduce:transform-none");
    // Phase-3 invariant: next/image is NOT unoptimized.
    expect(container.innerHTML).not.toMatch(/unoptimized/);
  });

  it("list mode still injects the Phase-2 menuCardClass (md:flex)", () => {
    render(
      <PublicMenuDisplay
        {...(baseProps as any)}
        designSettings={{ ...baseProps.designSettings, menu_layout: "list" }}
      />,
    );
    const details = screen.getByRole("button", { name: "Open Burger" });
    expect(details.closest("article")?.className).toContain("md:flex");
  });
});
