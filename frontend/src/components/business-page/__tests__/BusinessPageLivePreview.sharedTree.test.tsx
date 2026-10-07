/** @jest-environment jsdom */
/**
 * Regression guard for #591: the Business Page live preview must render the
 * SAME storefront tree as `/b/<slug>`, not a hand-rolled replica.
 *
 * The old preview was a second implementation that had drifted — no
 * `storefront-theme` scope (so brand colors never applied), no announcement
 * bar, no tab navigation, no footer, no language selector, hardcoded English
 * copy and a permanently "Closed" hero. These tests assert the shared
 * components are really mounted, and that the preview-only concessions
 * (no URL writes, no AI waiter, no guest fulfillment context) hold.
 *
 * Only the tab BODIES and the network hook are mocked — the storefront shell
 * (theme style, announcement bar, hero, tab navigation, footer, language
 * selector) renders for real, because that shell is what drifted.
 */
import React from "react";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import fs from "fs";
import path from "path";
import BusinessPageLivePreview from "../BusinessPageLivePreview";
import type { BusinessPageLivePreviewModel } from "../previewStorefrontBusiness";
import type { BusinessDesignSettings } from "@/api/business";
import { useBusinessPageData } from "@/hooks/useBusinessPageData";
import { useFulfillmentContext } from "@/hooks/useFulfillmentContext";

jest.mock("@/hooks/useBusinessPageData", () => {
  const actual = jest.requireActual("@/hooks/useBusinessPageData") as Record<
    string,
    unknown
  >;
  return { ...actual, useBusinessPageData: jest.fn() };
});

jest.mock("@/hooks/useFulfillmentContext", () => ({
  useFulfillmentContext: jest.fn(() => ({
    context: null,
    setContext: jest.fn(),
    clearContext: jest.fn(),
  })),
}));

// Tab bodies are stubbed: this suite is about the shared storefront SHELL.
jest.mock("../BusinessAboutTab", () => ({
  __esModule: true,
  default: () => (
    <div role="tabpanel" id="tabpanel-about" aria-labelledby="tab-about" tabIndex={-1}>
      about-body
    </div>
  ),
}));
jest.mock("../PublicMenuDisplay", () => ({
  __esModule: true,
  default: () => <div data-testid="public-menu">menu-body</div>,
}));
jest.mock("../BusinessDeliveryTab", () => ({
  __esModule: true,
  default: () => (
    <div role="tabpanel" id="tabpanel-delivery" aria-labelledby="tab-delivery" tabIndex={-1}>
      delivery-body
    </div>
  ),
}));
jest.mock("../BusinessReservationsTab", () => ({
  __esModule: true,
  default: () => (
    <div
      role="tabpanel"
      id="tabpanel-reservations"
      aria-labelledby="tab-reservations"
      tabIndex={-1}
    >
      reservations-body
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
  AiWaiter: () => <div data-testid="ai-waiter">ai-waiter</div>,
}));

const useBusinessPageDataMock = useBusinessPageData as jest.MockedFunction<
  typeof useBusinessPageData
>;
const useFulfillmentContextMock = useFulfillmentContext as jest.MockedFunction<
  typeof useFulfillmentContext
>;

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
    deliveryEnabled: true,
    deliveryPartnerLinks: [],
    deliverySettings: null,
    deliverySettingsLoading: false,
    reservationsEnabled: true,
    reservationPartnerLinks: [],
    reservationSettings: null,
    reservationSettingsLoading: false,
    featureTabsReady: true,
    ...overrides,
  } as unknown as ReturnType<typeof useBusinessPageData>;
}

/** Local YYYY-MM-DD `days` from today — the announcement window is today..+7d. */
function localDateKey(days: number): string {
  const d = new Date();
  d.setDate(d.getDate() + days);
  const month = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${d.getFullYear()}-${month}-${day}`;
}

/** Open every day, all day, so the hero "Open Now" pill is time-independent. */
const ALWAYS_OPEN_HOURS = Array.from({ length: 7 }, (_, day) => ({
  id: day + 1,
  business_id: 85,
  day_of_week: day,
  open_time: "00:00",
  close_time: "23:59",
  is_closed: false,
  created_at: "",
  updated_at: "",
}));

const DRAFT_DESIGN: BusinessDesignSettings = {
  primary_color: "#ff5722",
  secondary_color: "#3d5afe",
  font_family: "Serif",
  corner_radius: "large",
  shadow_intensity: "subtle",
  background_pattern: "none",
  pattern_opacity: 0.2,
  show_images: true,
  show_descriptions: true,
} as unknown as BusinessDesignSettings;

const baseModel: BusinessPageLivePreviewModel = {
  businessId: 85,
  name: "Core Kitchen",
  description: "Fire-cooked plates.",
  phone: "+54 11 5555 0101",
  customUrl: "payverge-core-demo-kitchen",
  address: {
    street: "42 Harbor Ave",
    city: "Buenos Aires",
    state: "",
    postal_code: "1425",
    country: "AR",
  },
  operatingHours: ALWAYS_OPEN_HOURS,
  operatingExceptions: [
    {
      id: 9,
      business_id: 85,
      exception_date: localDateKey(3),
      is_closed: true,
      label: "Kitchen refit",
      created_at: "",
      updated_at: "",
    },
  ],
  showOperatingHours: true,
  businessLanguages: [
    { language_code: "en", is_default: true, is_active: true },
    { language_code: "es", is_default: false, is_active: true },
  ] as unknown as BusinessPageLivePreviewModel["businessLanguages"],
  supportedLanguages: [
    { code: "en", name: "English" },
    { code: "es", name: "Spanish" },
  ] as unknown as BusinessPageLivePreviewModel["supportedLanguages"],
};

function renderPreview(
  overrides: Partial<BusinessPageLivePreviewModel> = {},
  design: Partial<BusinessDesignSettings> = {},
) {
  return render(
    <BusinessPageLivePreview
      model={{ ...baseModel, ...overrides }}
      designSettings={{ ...DRAFT_DESIGN, ...design }}
      operatorLocale="en"
    />,
  );
}

/** Every stylesheet styled-jsx injected, plus anything rendered inline. */
function collectedCss(container: HTMLElement): string {
  const fromDocument = Array.from(document.querySelectorAll("style"))
    .map((el) => el.textContent || "")
    .join("\n");
  const fromContainer = Array.from(container.querySelectorAll("style"))
    .map((el) => el.textContent || "")
    .join("\n");
  return `${fromDocument}\n${fromContainer}`;
}

describe("BusinessPageLivePreview — renders the shared storefront tree (#591)", () => {
  beforeEach(() => {
    window.history.replaceState(null, "", "/business/85/dashboard");
    Element.prototype.scrollIntoView = jest.fn();
    useBusinessPageDataMock.mockReturnValue(pageData());
    useFulfillmentContextMock.mockReturnValue({
      context: null,
      setContext: jest.fn(),
      clearContext: jest.fn(),
    } as unknown as ReturnType<typeof useFulfillmentContext>);
  });

  it("mounts the storefront root with the storefront-theme scope", () => {
    const { container } = renderPreview();
    const root = container.querySelector('[data-storefront-preview="true"]');
    expect(root).not.toBeNull();
    // Brand CSS variables only apply inside this class — its absence was the
    // reason draft colors never painted in the old replica.
    expect(root).toHaveClass("storefront-theme");
    // Embedded render: no second <main>, no duplicate landmark ids.
    expect(root!.tagName).toBe("DIV");
    expect(document.getElementById("main-content")).toBeNull();
    expect(document.getElementById("tab-content")).toBeNull();
  });

  it("applies the draft brand colors through StorefrontThemeStyle", () => {
    const { container } = renderPreview();
    const css = collectedCss(container);
    expect(css).toContain(".storefront-theme");
    expect(css.replace(/\s+/g, "")).toContain("--primary:#ff5722");
    expect(css.replace(/\s+/g, "")).toContain("--secondary:#3d5afe");
  });

  it("renders the announcement bar for an upcoming operating exception", () => {
    renderPreview();
    const bar = screen.getByTestId("storefront-announcement-bar");
    expect(within(bar).getByText("Kitchen refit")).toBeInTheDocument();
  });

  it("renders the sticky storefront tab navigation with every feature tab", () => {
    renderPreview();
    const tablist = screen.getByRole("tablist", { hidden: true });
    for (const label of [
      "About",
      "Menu",
      "Delivery",
      "Reservations",
      "Contact",
    ]) {
      expect(within(tablist).getByRole("tab", { name: label, hidden: true })).toBeInTheDocument();
    }
    expect(within(tablist).getByRole("tab", { name: "About", hidden: true })).toHaveAttribute(
      "aria-selected",
      "true",
    );
  });

  it("renders the hero with the draft name and the real open status", () => {
    renderPreview();
    expect(
      screen.getByRole("heading", { level: 1, name: "Core Kitchen", hidden: true }),
    ).toBeInTheDocument();
    // Always-open hours: the old replica hardcoded a "Closed" pill.
    expect(screen.getAllByText("Open Now").length).toBeGreaterThan(0);
  });

  it("renders the storefront footer with the draft NAP and hours", () => {
    renderPreview();
    expect(screen.getByTestId("footer-contact")).toBeInTheDocument();
    expect(screen.getByTestId("footer-hours")).toBeInTheDocument();
    expect(screen.getByText("+54 11 5555 0101")).toBeInTheDocument();
  });

  it("renders the language selector when the storefront is multilingual", () => {
    renderPreview();
    const tablist = screen.getByRole("tablist", { hidden: true });
    // Inline (tab bar) selector only — the fixed mobile pill would escape the
    // device frame, so it is suppressed in preview.
    const selectors = screen.getAllByRole("button", { name: /language/i, hidden: true });
    expect(selectors.length).toBe(1);
    expect(tablist.closest("nav, header, div")).not.toBeNull();
  });

  it("reads the real storefront data for the real business id", () => {
    renderPreview();
    const [businessId, customUrl, language] =
      useBusinessPageDataMock.mock.calls[0];
    expect(businessId).toBe(85);
    expect(customUrl).toBe("payverge-core-demo-kitchen");
    expect(language).toBe("en");
  });

  it("renders localized storefront copy for a Spanish storefront", async () => {
    // The old replica was hardcoded English regardless of the storefront's
    // default language; the guest bundle loads asynchronously.
    renderPreview({ defaultLanguage: "es" });
    expect(
      await screen.findByRole("tab", { name: "Menú", hidden: true }),
    ).toBeInTheDocument();
    const tablist = screen.getByRole("tablist", { hidden: true });
    expect(
      within(tablist).getByRole("tab", { name: "Contacto", hidden: true }),
    ).toBeInTheDocument();
    expect(screen.getAllByText("Abierto Ahora").length).toBeGreaterThan(0);
  });

  it("renders es-AR (voseo) copy when the storefront defaults to it", async () => {
    renderPreview({ defaultLanguage: "es-AR" });
    expect(
      await screen.findByRole("tab", { name: "Menú", hidden: true }),
    ).toBeInTheDocument();
    const [, , language] =
      useBusinessPageDataMock.mock.calls[
        useBusinessPageDataMock.mock.calls.length - 1
      ];
    expect(language).toBe("es-AR");
  });
});

describe("BusinessPageLivePreview — preview-only concessions (#591)", () => {
  beforeEach(() => {
    window.history.replaceState(null, "", "/business/85/dashboard");
    Element.prototype.scrollIntoView = jest.fn();
    useBusinessPageDataMock.mockReturnValue(pageData());
    useFulfillmentContextMock.mockReturnValue({
      context: null,
      setContext: jest.fn(),
      clearContext: jest.fn(),
    } as unknown as ReturnType<typeof useFulfillmentContext>);
  });

  it("never writes the dashboard URL when a tab is switched", async () => {
    const user = userEvent.setup();
    renderPreview();
    const before = window.location.href;

    const tablist = screen.getByRole("tablist", { hidden: true });
    await user.click(within(tablist).getByRole("tab", { name: "Menu", hidden: true }));

    expect(screen.getByTestId("public-menu")).toBeInTheDocument();
    expect(window.location.href).toBe(before);
    expect(window.location.hash).toBe("");
  });

  it("does not mount the AI waiter", () => {
    renderPreview();
    expect(screen.queryByTestId("ai-waiter")).toBeNull();
  });

  it("does not read the operator's own guest fulfillment context", () => {
    renderPreview();
    expect(useFulfillmentContextMock).toHaveBeenCalledWith(0);
  });

  it("is hidden from the dashboard accessibility tree", () => {
    const { container } = renderPreview();
    expect(screen.getByTestId("business-page-live-preview")).toHaveAttribute(
      "aria-hidden",
      "true",
    );
    expect(container.querySelector('[data-storefront-preview="true"]')).not.toBeNull();
  });
});

describe("BusinessPageLivePreview — no second storefront implementation (#591)", () => {
  it("delegates to the live landing page instead of re-implementing it", () => {
    const source = fs.readFileSync(
      path.join(__dirname, "..", "BusinessPageLivePreview.tsx"),
      "utf8",
    );
    expect(source).toContain('from "./ConvertingBusinessLandingPage"');
    expect(source).toContain("previewMode");
    // The replica's own storefront markup and fixture menu must not come back.
    expect(source).not.toContain("PREVIEW_MENU_ITEMS");
    expect(source).not.toContain("buildPublicBusiness");
    expect(source).not.toContain("./BusinessHeroSection");
    expect(source).not.toContain("./PublicMenuDisplay");
    expect(source).not.toContain("./BusinessFooter");
  });
});
