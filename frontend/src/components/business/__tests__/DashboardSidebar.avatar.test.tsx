/** @jest-environment jsdom */
/**
 * Task 2.4 — sidebar business avatar must not flicker on tab nav.
 *
 * Two behaviors under test:
 *
 *   1. When `business.logo` is null/missing, the sidebar must render an
 *      initials placeholder so the avatar slot is never empty / never
 *      pops a broken-image icon. Initials are derived from the first and
 *      last words of the business name (e.g. "Mara AI Lounge" -> "ML").
 *
 *   2. When the logo URL IS present, the <img> must be rendered so the
 *      consumer of the sidebar (parent dashboard) can rely on the React
 *      Query layer caching it across tab nav.
 */
import React from "react";
import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import DashboardSidebar from "../DashboardSidebar";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";
import { useOptionalOperationalAlerts } from "../operational-alerts/useOperationalAlerts";

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: jest.fn(),
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const { getTranslation } = jest.requireActual("@/i18n/getTranslation");
  return {
    useSimpleLocale: () => ({ locale: "en", setLocale: () => {} }),
    getTranslation,
  };
});

jest.mock("../operational-alerts/useOperationalAlerts", () => ({
  useOptionalOperationalAlerts: jest.fn(),
}));

// Next/Image is DOM-hostile in node testing; stub to a plain img.
jest.mock("next/image", () => ({
  __esModule: true,
  default: (props: any) => {
    // eslint-disable-next-line @next/next/no-img-element, jsx-a11y/alt-text
    return <img {...props} />;
  },
}));

// The sidebar now calls useChatUnreadCount, which subscribes to the SSE
// stream (jsdom has no EventSource) — mock at the hook level so this test's
// real QueryClientProvider isn't required to also fake a live connection.
jest.mock("@/hooks/useChatUnreadCount", () => ({
  useChatUnreadCount: jest.fn(() => 0),
}));

const mockTierDefaults = {
  access: null,
  loading: false,
  error: null,
  hasAccess: true,
  isSuspended: false,
  lockState: "active" as const,
  aiConfigured: true,
  refetch: jest.fn(),
};

function withQuery(children: React.ReactNode) {
  const c = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return <QueryClientProvider client={c}>{children}</QueryClientProvider>;
}

function renderSidebar(
  overrides: Partial<React.ComponentProps<typeof DashboardSidebar>> = {},
) {
  const defaults: React.ComponentProps<typeof DashboardSidebar> = {
    business: {
      id: 1,
      name: "Mara AI Lounge",
      logo: "",
      custom_url: "mara-ai-lounge",
    } as any,
    activeTab: "overview",
    setActiveTab: jest.fn(),
    sidebarOpen: true,
    setSidebarOpen: jest.fn(),
    allowedTabs: [],
    isStaffUser: false,
    staffData: null,
    globalOrders: {},
    upcomingReservations: [],
    tutorialOpen: false,
    tutorialTabKey: null,
    tutorialTarget: null,
    onStartTutorial: jest.fn(),
  };
  return render(withQuery(<DashboardSidebar {...defaults} {...overrides} />));
}

describe("DashboardSidebar — business avatar", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    window.localStorage.clear();
    (useBusinessAccess as jest.Mock).mockReturnValue(mockTierDefaults);
    (useOptionalOperationalAlerts as jest.Mock).mockReturnValue(null);
  });

  it("renders initials placeholder when business has no logo", () => {
    renderSidebar({
      business: {
        id: 1,
        name: "Mara AI Lounge",
        logo: "",
        custom_url: "mara-ai-lounge",
      } as any,
    });

    // First + last word initials -> "ML" for "Mara AI Lounge".
    expect(screen.getByText("ML")).toBeInTheDocument();
  });

  it("renders a single-letter initial for one-word names", () => {
    renderSidebar({
      business: {
        id: 2,
        name: "Payverge",
        logo: "",
        custom_url: "payverge",
      } as any,
    });

    expect(screen.getByText("P")).toBeInTheDocument();
  });

  it("renders an <img> with the logo URL when present and skips the initials placeholder", () => {
    renderSidebar({
      business: {
        id: 3,
        name: "Mara AI Lounge",
        logo: "https://cdn.example.com/logo.png",
        custom_url: "mara-ai-lounge",
      } as any,
    });

    const img = screen.getByAltText("Mara AI Lounge") as HTMLImageElement;
    expect(img).toBeInTheDocument();
    expect(img.src).toContain("https://cdn.example.com/logo.png");
    // When the logo renders, the initials placeholder is gone.
    expect(screen.queryByText("ML")).toBeNull();
  });

  it("lets a long expanded-sidebar business name wrap to two lines", () => {
    renderSidebar({
      business: {
        id: 4,
        name: "Payverge Core Demo Kitchen",
        logo: "",
        custom_url: "payverge-core-demo-kitchen",
      } as any,
    });

    const heading = screen.getByRole("heading", {
      name: "Payverge Core Demo Kitchen",
    });
    expect(heading.className).toContain("line-clamp-2");
    expect(heading.className).not.toContain("truncate");
  });

  it("names the Tables badge as active service calls without implying occupancy", () => {
    (useOptionalOperationalAlerts as jest.Mock).mockReturnValue({
      counts: {
        bills: 0,
        kitchen: 0,
        reservations: 0,
        delivery: 0,
        tables: 1,
      },
    });

    renderSidebar();

    const tablesButton = screen.getByRole("button", {
      name: "Tables Active service calls: 1",
    });
    expect(tablesButton).toBeInTheDocument();
    expect(tablesButton).toHaveTextContent("1");
    expect(
      screen.queryByRole("button", { name: "Tables 1 in Tables" }),
    ).not.toBeInTheDocument();
  });

  it("names the Tables badge as occupied tables when leftover kitchen occupies the floor (#774)", () => {
    (useOptionalOperationalAlerts as jest.Mock).mockReturnValue({
      counts: {
        bills: 0,
        kitchen: 0,
        reservations: 0,
        delivery: 0,
        tables: 1,
      },
    });

    renderSidebar({ occupiedTablesCount: 5 });

    const tablesButton = screen.getByRole("button", {
      name: "Tables 5 occupied tables",
    });
    expect(tablesButton).toBeInTheDocument();
    expect(tablesButton).toHaveTextContent("5");
    expect(
      screen.queryByRole("button", { name: "Tables Active service calls: 1" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Tables Active service calls: 5" }),
    ).not.toBeInTheDocument();
  });
});
