/** @jest-environment jsdom */
import { render, screen } from "@testing-library/react";
import BusinessPageClient from "./BusinessPageClient";
import type { PublicBusiness } from "@/api/publicBusiness";
import type { MenuCategory } from "@/api/business";

jest.mock("@/api/publicBusiness", () => ({
  ...jest.requireActual("@/api/publicBusiness"),
  getBusinessByCustomUrl: jest.fn(),
}));

jest.mock("next/navigation", () => ({
  useRouter: () => ({ push: jest.fn() }),
}));

// Keep the heavy landing page out of the tree — the SEO menu block is a
// sibling of it, not part of it.
jest.mock("@/components/business-page/ConvertingBusinessLandingPage", () => {
  const MockLanding = () => <div data-testid="landing" />;
  MockLanding.displayName = "MockLanding";
  return MockLanding;
});

const seededBusiness = {
  id: 7,
  name: "Seeded Cafe",
  custom_url: "seeded",
  default_language: "en",
  display_currency: "USD",
  page_enabled: true,
  is_active: true,
} as unknown as PublicBusiness;

const seededMenu = [
  {
    id: "cat-1",
    name: "Pizzas",
    description: "Wood fired",
    items: [
      {
        id: "item-1",
        name: "Margherita",
        description: "San Marzano tomatoes",
        price: 12.5,
        is_available: true,
      },
    ],
  },
] as unknown as MenuCategory[];

describe("BusinessPageClient — crawlable SEO menu block (SEO-0.1)", () => {
  it("renders the server-seeded menu as sr-only crawlable HTML", () => {
    render(
      <BusinessPageClient
        customUrl="seeded"
        initialBusiness={seededBusiness}
        initialMenuCategories={seededMenu}
      />,
    );

    const section = screen.getByTestId("storefront-seo-menu");
    expect(section).toHaveClass("sr-only");
    expect(section).toHaveAttribute("aria-hidden", "true");
    expect(section).toHaveTextContent("Pizzas");
    expect(section).toHaveTextContent("Margherita");
    expect(section).toHaveTextContent("San Marzano tomatoes");
    expect(section).toHaveTextContent("12.50");
  });

  it("renders nothing when no menu seed was provided", () => {
    render(
      <BusinessPageClient
        customUrl="seeded"
        initialBusiness={seededBusiness}
      />,
    );
    expect(screen.queryByTestId("storefront-seo-menu")).not.toBeInTheDocument();
  });

  it("renders nothing for an empty menu seed", () => {
    render(
      <BusinessPageClient
        customUrl="seeded"
        initialBusiness={seededBusiness}
        initialMenuCategories={[]}
      />,
    );
    expect(screen.queryByTestId("storefront-seo-menu")).not.toBeInTheDocument();
  });
});
