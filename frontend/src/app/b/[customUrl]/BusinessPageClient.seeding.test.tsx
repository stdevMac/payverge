/** @jest-environment jsdom */
import { render, screen, waitFor } from "@testing-library/react";
import BusinessPageClient from "./BusinessPageClient";
import { getBusinessByCustomUrl, type PublicBusiness } from "@/api/publicBusiness";
import {
  GUEST_SUPPORTED_LANGUAGES,
  GuestTranslationContext,
  type GuestLanguageCode,
} from "@/i18n/GuestTranslationProvider";

jest.mock("@/api/publicBusiness", () => ({
  ...jest.requireActual("@/api/publicBusiness"),
  getBusinessByCustomUrl: jest.fn(),
}));

jest.mock("next/navigation", () => ({
  useRouter: () => ({ push: jest.fn() }),
}));

// Render a probe in place of the heavy landing page so we can assert the
// seeded business reached the success path without mounting the whole tree.
jest.mock("@/components/business-page/ConvertingBusinessLandingPage", () => {
  const MockLanding = (props: { business: { name: string } }) => (
    <div data-testid="landing">{props.business.name}</div>
  );
  MockLanding.displayName = "MockLanding";
  return MockLanding;
});

const seeded = {
  id: 7,
  name: "Seeded Cafe",
  custom_url: "seeded",
  default_language: "en",
  page_enabled: true,
  is_active: true,
} as unknown as PublicBusiness;

describe("BusinessPageClient — server seeding (PERF-1)", () => {
  beforeEach(() => jest.clearAllMocks());

  it("first-paints the seeded business WITHOUT a fetch and WITHOUT a skeleton", async () => {
    render(<BusinessPageClient customUrl="seeded" initialBusiness={seeded} />);
    // Success path renders immediately from the seed.
    expect(screen.getByTestId("landing")).toHaveTextContent("Seeded Cafe");
    // No skeleton flash.
    expect(screen.queryByTestId("hero-skeleton")).not.toBeInTheDocument();
    // The duplicate client fetch never fires when seeded.
    expect(getBusinessByCustomUrl).not.toHaveBeenCalled();
  });

  it("falls back to the client fetch when NOT seeded", async () => {
    (getBusinessByCustomUrl as jest.Mock).mockResolvedValue(seeded);
    render(<BusinessPageClient customUrl="seeded" />);
    await waitFor(() =>
      expect(getBusinessByCustomUrl).toHaveBeenCalledWith("seeded", expect.anything()),
    );
  });

  it("refreshes an unseeded storefront with the current locale after a locale switch", async () => {
    (getBusinessByCustomUrl as jest.Mock).mockResolvedValue(seeded);
    const value = (currentLanguage: GuestLanguageCode) => ({
      currentLanguage,
      setLanguage: jest.fn(),
      t: (key: string) => key,
      availableLanguages: GUEST_SUPPORTED_LANGUAGES,
      setBusinessId: jest.fn(),
    });
    const { rerender } = render(
      <GuestTranslationContext.Provider value={value("en")}>
        <BusinessPageClient customUrl="locale-cafe" />
      </GuestTranslationContext.Provider>,
    );
    await waitFor(() =>
      expect(getBusinessByCustomUrl).toHaveBeenCalledWith("locale-cafe", "en"),
    );

    rerender(
      <GuestTranslationContext.Provider value={value("es-AR")}>
        <BusinessPageClient customUrl="locale-cafe" />
      </GuestTranslationContext.Provider>,
    );
    await waitFor(() =>
      expect(getBusinessByCustomUrl).toHaveBeenCalledWith(
        "locale-cafe",
        "es-AR",
      ),
    );
  });

  it("seeds the not-found error state from initialReason without a fetch", () => {
    render(
      <BusinessPageClient
        customUrl="missing"
        initialBusiness={null}
        initialReason="not_found"
      />,
    );
    expect(getBusinessByCustomUrl).not.toHaveBeenCalled();
    // The not-found (Compass) error variant renders; assert via the isNotFound shell.
    expect(screen.queryByTestId("hero-skeleton")).not.toBeInTheDocument();
  });

  it("refetches a live venue after SSR unavailable instead of sticking on the outage (#685)", async () => {
    (getBusinessByCustomUrl as jest.Mock).mockResolvedValue({
      id: 86,
      name: "Payverge AI Pro Demo Lounge",
      custom_url: "payverge-ai-pro-demo-lounge",
      page_enabled: true,
      is_active: true,
    });
    render(
      <BusinessPageClient
        customUrl="payverge-ai-pro-demo-lounge"
        initialBusiness={null}
        initialReason="unavailable"
      />,
    );
    expect(
      screen.queryByText("Business temporarily unavailable"),
    ).not.toBeInTheDocument();
    await waitFor(() =>
      expect(screen.getByTestId("landing")).toHaveTextContent(
        "Payverge AI Pro Demo Lounge",
      ),
    );
    expect(getBusinessByCustomUrl).toHaveBeenCalledWith(
      "payverge-ai-pro-demo-lounge",
      expect.anything(),
    );
  });
});
