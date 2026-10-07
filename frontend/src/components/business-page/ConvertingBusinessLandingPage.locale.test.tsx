/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import ConvertingBusinessLandingPage from "./ConvertingBusinessLandingPage";
import {
  GuestTranslationProvider,
  useGuestTranslation,
} from "@/i18n/GuestTranslationProvider";
import { getBusinessByCustomUrl, type PublicBusiness } from "@/api/publicBusiness";

jest.mock("@/api/publicBusiness", () => ({
  ...jest.requireActual("@/api/publicBusiness"),
  getBusinessByCustomUrl: jest.fn(),
}));

jest.mock("@/hooks/useBusinessPageData", () => ({
  shouldPollStorefrontMenu: (tab: string | null) => tab === "menu",
  useBusinessPageData: () => ({
    menu: [],
    offers: [],
    bundles: [],
    itemOrderability: {},
    menuSnapshotAuthoritative: true,
    menuLoading: false,
    googleRating: null,
    deliveryEnabled: false,
    deliveryPartnerLinks: [],
    deliverySettings: null,
    deliverySettingsLoading: false,
    reservationsEnabled: false,
    reservationPartnerLinks: [],
    reservationSettings: null,
  }),
}));

jest.mock("@/hooks/useHashTabs", () => ({
  useHashTabs: () => ({ activeTab: "about", changeTab: jest.fn() }),
}));

jest.mock("@/hooks/useFulfillmentContext", () => ({
  useFulfillmentContext: () => ({
    context: null,
    setContext: jest.fn(),
    clearContext: jest.fn(),
  }),
}));

jest.mock("@/hooks/useBusinessOpenStatus", () => ({
  useBusinessOpenStatus: () => ({ isOpen: true }),
}));

jest.mock("./BusinessHeroSection", () => ({
  __esModule: true,
  default: ({ business }: { business: { description?: string } }) => (
    <p>{business.description}</p>
  ),
}));

jest.mock("./BusinessTabNavigation", () => ({
  __esModule: true,
  default: () => <nav>tabs</nav>,
}));

jest.mock("./BusinessAboutTab", () => ({
  __esModule: true,
  default: ({ business }: { business: { about_story?: string } }) => (
    <p>{business.about_story}</p>
  ),
}));

jest.mock("./BusinessContactTab", () => ({
  __esModule: true,
  default: () => null,
}));
jest.mock("./BusinessDeliveryTab", () => ({
  __esModule: true,
  default: () => null,
}));
jest.mock("./BusinessReservationsTab", () => ({
  __esModule: true,
  default: () => null,
}));
jest.mock("./PublicMenuDisplay", () => ({
  __esModule: true,
  default: () => null,
}));
jest.mock("@/components/guest/FloatingLanguageSelectorBusiness", () => ({
  FloatingLanguageSelectorBusiness: () => null,
}));
jest.mock("@/components/guest/AiWaiter", () => ({
  AiWaiter: () => null,
}));

const spanishSeed = {
  id: 11,
  name: "Demo Kitchen",
  custom_url: "demo-kitchen",
  default_language: "en",
  description: "Un restaurante modelo de Payverge con datos de demostración.",
  welcome_message: "Bienvenidos. El equipo de demostración está listo.",
  about_story: "Un entorno de demostración realista de Payverge.",
  special_features: [],
  gallery_images: [{ caption: "Galería de demostración 1" }],
  business_languages: [
    { language_code: "en", is_default: true },
    { language_code: "es", is_default: false },
  ],
  supported_languages: [],
  page_enabled: true,
  is_active: true,
} as unknown as PublicBusiness;

const englishLive = {
  ...spanishSeed,
  description: "A Payverge model restaurant with realistic demo data.",
  welcome_message: "Welcome. The demo team is ready to serve you.",
  about_story: "A realistic Payverge demo environment.",
  gallery_images: [{ caption: "Demo gallery 1" }],
} as unknown as PublicBusiness;

function SwitchToEnglish() {
  const { setLanguage } = useGuestTranslation();
  return (
    <button type="button" onClick={() => setLanguage("en")}>
      switch-to-en
    </button>
  );
}

describe("ConvertingBusinessLandingPage locale switch (#400)", () => {
  beforeEach(() => {
    (getBusinessByCustomUrl as jest.Mock).mockReset();
    (getBusinessByCustomUrl as jest.Mock).mockImplementation(
      async (_url: string, language?: string) =>
        language === "en" ? englishLive : spanishSeed,
    );
  });

  it("reloads English authored fields instead of keeping the Spanish SSR seed", async () => {
    const user = userEvent.setup();
    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });

    render(
      <QueryClientProvider client={qc}>
        <GuestTranslationProvider initialLanguage="es" preferInitialLanguage>
          <SwitchToEnglish />
          <ConvertingBusinessLandingPage
            business={spanishSeed}
            customUrl="demo-kitchen"
            seedLanguage="es"
          />
        </GuestTranslationProvider>
      </QueryClientProvider>,
    );

    expect(
      screen.getByText(
        "Un restaurante modelo de Payverge con datos de demostración.",
      ),
    ).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "switch-to-en" }));

    await waitFor(() => {
      expect(getBusinessByCustomUrl).toHaveBeenCalledWith(
        "demo-kitchen",
        "en",
      );
    });
    await waitFor(() => {
      expect(
        screen.getByText(
          "A Payverge model restaurant with realistic demo data.",
        ),
      ).toBeInTheDocument();
    });
    expect(
      screen.queryByText(
        "Un restaurante modelo de Payverge con datos de demostración.",
      ),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByText("Un entorno de demostración realista de Payverge."),
    ).not.toBeInTheDocument();
  });
});
