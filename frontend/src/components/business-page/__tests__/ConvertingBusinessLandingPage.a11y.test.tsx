/** @jest-environment jsdom */
import React from "react";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import ConvertingBusinessLandingPage from "../ConvertingBusinessLandingPage";
import type { PublicBusiness } from "@/api/publicBusiness";
import { useBusinessPageData } from "@/hooks/useBusinessPageData";

jest.mock("@/hooks/useBusinessPageData", () => {
  const actual = jest.requireActual("@/hooks/useBusinessPageData") as Record<
    string,
    unknown
  >;
  return {
    ...actual,
    useBusinessPageData: jest.fn(),
  };
});

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

jest.mock("@/i18n/GuestTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/GuestTranslationProvider");
  return {
    ...actual,
    useGuestTranslation: () => ({
      t: (key: string) => key,
      currentLanguage: "en",
      setBusinessId: jest.fn(),
    }),
  };
});

jest.mock("../PublicMenuDisplay", () => ({
  __esModule: true,
  default: () => <div data-testid="public-menu">menu-body</div>,
}));

jest.mock("../BusinessAboutTab", () => ({
  __esModule: true,
  default: () => (
    <div role="tabpanel" id="tabpanel-about" aria-labelledby="tab-about" tabIndex={-1}>
      about-body
    </div>
  ),
}));

jest.mock("../BusinessDeliveryTab", () => ({
  __esModule: true,
  default: ({ deliveryLoading }: { deliveryLoading?: boolean }) => (
    <div role="tabpanel" id="tabpanel-delivery" aria-labelledby="tab-delivery" tabIndex={-1}>
      {deliveryLoading ? "delivery-loading" : "delivery-body"}
    </div>
  ),
}));

jest.mock("../BusinessReservationsTab", () => ({
  __esModule: true,
  default: ({ reservationLoading }: { reservationLoading?: boolean }) => (
    <div
      role="tabpanel"
      id="tabpanel-reservations"
      aria-labelledby="tab-reservations"
      tabIndex={-1}
    >
      {reservationLoading ? "reservations-loading" : "reservations-body"}
    </div>
  ),
}));

jest.mock("../BusinessContactTab", () => ({
  __esModule: true,
  default: () => (
    <div role="tabpanel" id="tabpanel-contact" aria-labelledby="tab-contact" tabIndex={-1}>
      contact-body
    </div>
  ),
}));

jest.mock("@/components/guest/AiWaiter", () => ({
  AiWaiter: () => null,
}));

jest.mock("@/components/guest/FloatingLanguageSelectorBusiness", () => ({
  FloatingLanguageSelectorBusiness: () => null,
}));

const useBusinessPageDataMock = useBusinessPageData as jest.MockedFunction<
  typeof useBusinessPageData
>;

const business = {
  id: 7,
  name: "Core Kitchen",
  description: "Fire-cooked plates.",
  logo: "",
  banner_images: "",
  custom_url: "core-kitchen",
  website: "",
  phone: "",
  address: { street: "", city: "", state: "", postal_code: "", country: "" },
  social_media: "",
  default_currency: "USD",
  display_currency: "USD",
  default_language: "en",
  google_business_name: "",
  google_business_url: "",
  google_place_id: "",
  google_review_link: "",
  google_reviews_enabled: false,
  show_reviews: false,
  show_operating_hours: false,
  created_at: "",
  updated_at: "",
} as unknown as PublicBusiness;

function pageData(overrides: Record<string, unknown> = {}) {
  return {
    menu: [],
    offers: [],
    bundles: [],
    itemOrderability: {},
    menuSnapshotAuthoritative: true,
    menuLoading: false,
    popularItems: [],
    googleRating: null,
    deliveryEnabled: false,
    deliveryPartnerLinks: [],
    deliverySettings: null,
    deliverySettingsLoading: false,
    reservationsEnabled: false,
    reservationPartnerLinks: [],
    reservationSettings: null,
    reservationSettingsLoading: false,
    featureTabsReady: true,
    ...overrides,
  };
}

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <ConvertingBusinessLandingPage business={business} customUrl="core-kitchen" />
    </QueryClientProvider>,
  );
}

function desktopTab(name: RegExp | string) {
  const tablist = screen.getByRole("tablist");
  return within(tablist).getByRole("tab", { name });
}

describe("ConvertingBusinessLandingPage storefront navigation", () => {
  beforeEach(() => {
    window.history.replaceState(null, "", "/b/core-kitchen");
    Element.prototype.scrollIntoView = jest.fn();
    useBusinessPageDataMock.mockReturnValue(pageData() as ReturnType<typeof useBusinessPageData>);
  });

  it("moves focus to the menu tabpanel after View Menu", async () => {
    const user = userEvent.setup();
    renderPage();

    const cta = screen.getByRole("button", { name: /view menu/i });
    await user.click(cta);

    await waitFor(() => {
      expect(document.getElementById("tabpanel-menu")).toHaveFocus();
    });
    expect(desktopTab(/menuTab/i)).toHaveAttribute("aria-selected", "true");
    expect(window.location.hash).toBe("#menu");
  });

  it("does not steal focus on a direct #menu load", () => {
    window.history.replaceState(null, "", "/b/core-kitchen#menu");
    renderPage();

    expect(desktopTab(/menuTab/i)).toHaveAttribute("aria-selected", "true");
    expect(document.activeElement).toBe(document.body);
  });

  it("renders a Delivery deep link without first selecting About while settings load", () => {
    useBusinessPageDataMock.mockReturnValue(
      pageData({
        deliverySettingsLoading: true,
        featureTabsReady: false,
      }) as ReturnType<typeof useBusinessPageData>,
    );
    window.history.replaceState(null, "", "/b/core-kitchen#delivery");

    renderPage();

    expect(desktopTab(/deliveryTab/i)).toHaveAttribute("aria-selected", "true");
    expect(desktopTab(/about/i)).toHaveAttribute("aria-selected", "false");
    expect(screen.getByText("delivery-loading")).toBeInTheDocument();
    expect(screen.queryByText("about-body")).not.toBeInTheDocument();
  });

  it("renders a Reservations deep link without first selecting About while settings load", () => {
    useBusinessPageDataMock.mockReturnValue(
      pageData({
        reservationSettingsLoading: true,
        featureTabsReady: false,
      }) as ReturnType<typeof useBusinessPageData>,
    );
    window.history.replaceState(null, "", "/b/core-kitchen#reservations");

    renderPage();

    expect(desktopTab(/reservations/i)).toHaveAttribute("aria-selected", "true");
    expect(desktopTab(/about/i)).toHaveAttribute("aria-selected", "false");
    expect(screen.getByText("reservations-loading")).toBeInTheDocument();
  });

  it("normalizes an invalid initial hash to the default tab", () => {
    window.history.replaceState(null, "", "/b/core-kitchen#not-a-section");
    renderPage();

    expect(desktopTab(/about/i)).toHaveAttribute("aria-selected", "true");
    expect(window.location.hash).toBe("#about");
    expect(screen.getByText("about-body")).toBeVisible();
  });

  it("normalizes an invalid runtime hash instead of keeping the previous tab", () => {
    useBusinessPageDataMock.mockReturnValue(
      pageData({
        reservationsEnabled: true,
        featureTabsReady: true,
      }) as ReturnType<typeof useBusinessPageData>,
    );
    window.history.replaceState(null, "", "/b/core-kitchen#reservations");
    renderPage();
    expect(desktopTab(/reservations/i)).toHaveAttribute("aria-selected", "true");

    act(() => {
      window.location.hash = "#not-a-section";
      window.dispatchEvent(new HashChangeEvent("hashchange"));
    });

    expect(desktopTab(/about/i)).toHaveAttribute("aria-selected", "true");
    expect(window.location.hash).toBe("#about");
    expect(screen.queryByText("reservations-body")).not.toBeInTheDocument();
  });

  it("keeps every tab's aria-controls target in the document", () => {
    useBusinessPageDataMock.mockReturnValue(
      pageData({
        deliveryEnabled: true,
        reservationsEnabled: true,
      }) as ReturnType<typeof useBusinessPageData>,
    );
    renderPage();

    const tabs = within(screen.getByRole("tablist")).getAllByRole("tab");
    tabs.forEach((tab) => {
      const controls = tab.getAttribute("aria-controls");
      expect(controls).toBeTruthy();
      expect(document.getElementById(controls as string)).not.toBeNull();
    });
  });
});
