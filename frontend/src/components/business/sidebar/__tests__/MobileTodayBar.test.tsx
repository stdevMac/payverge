/** @jest-environment jsdom */
/**
 * M4 — mobile TODAY bottom bar: the three service rails (overview, kitchen,
 * reservations) reachable with one thumb tap on phones, where the sidebar is
 * hidden behind a hamburger. Filtered by allowedTabs, lock-aware, badged with
 * the same useRailBadges rules as the desktop rail.
 */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { MobileTodayBar } from "../MobileTodayBar";
import { prefetchTab } from "../../tabs/tabRegistry";
import type { AccessState } from "../../commandPalette/tabAccess";

jest.mock("../../tabs/tabRegistry", () => {
  const { Home, ChefHat, Calendar } = jest.requireActual("lucide-react");
  return {
    TAB_REGISTRY: {
      overview: { icon: Home },
      kitchen: { icon: ChefHat },
      reservations: { icon: Calendar },
    },
    prefetchTab: jest.fn(() => Promise.resolve()),
  };
});

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: () => {} }),
  getTranslation: (key: string) => {
    const leaf = key.replace(/^businessDashboard\./, "");
    const labels: Record<string, string> = {
      "tabs.overview": "Overview",
      "tabs.kitchen": "Kitchen",
      "tabs.reservations": "Reservations",
      "mobileNav.label": "Today",
      "locked.short": "Locked",
    };
    return labels[leaf] ?? key;
  },
}));

jest.mock("@/hooks/useFailedInvoiceCount", () => ({
  useFailedInvoiceCount: jest.fn(() => 0),
}));
jest.mock("@/hooks/useChatUnreadCount", () => ({
  useChatUnreadCount: jest.fn(() => 0),
}));

jest.mock("framer-motion", () => ({
  motion: {
    span: ({ children, ...rest }: any) => <span {...rest}>{children}</span>,
  },
}));
jest.mock("../../premium/useReducedDashboardMotion", () => ({
  useReducedDashboardMotion: () => true,
}));

const ACCESS: AccessState = {
  loading: false,
  isSuspended: false,
};

function renderBar(
  overrides: Partial<React.ComponentProps<typeof MobileTodayBar>> = {},
) {
  const props: React.ComponentProps<typeof MobileTodayBar> = {
    businessId: 7,
    activeTab: "overview",
    onNavigate: jest.fn(),
    allowedTabs: [],
    accessState: ACCESS,
    isStaffUser: false,
    hidden: false,
    ...overrides,
  };
  render(<MobileTodayBar {...props} />);
  return props;
}

beforeEach(() => jest.clearAllMocks());

describe("MobileTodayBar — rails", () => {
  it("renders the three TODAY tabs with a nav landmark", () => {
    renderBar();
    const nav = screen.getByRole("navigation", { name: "Today" });
    expect(nav).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Overview/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Kitchen/ })).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /Reservations/ }),
    ).toBeInTheDocument();
  });

  it("is hidden on desktop (lg:hidden) and pads for the iPhone home indicator", () => {
    renderBar();
    const nav = screen.getByRole("navigation", { name: "Today" });
    expect(nav.className).toContain("lg:hidden");
    expect(nav.className).toContain("pb-[env(safe-area-inset-bottom)]");
  });

  it("filters rails by allowedTabs", () => {
    renderBar({ allowedTabs: ["kitchen"] });
    expect(screen.getByRole("button", { name: /Kitchen/ })).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /Overview/ }),
    ).not.toBeInTheDocument();
  });

  it("marks the active tab with aria-current", () => {
    renderBar({ activeTab: "kitchen" });
    expect(screen.getByRole("button", { name: /Kitchen/ })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(
      screen.getByRole("button", { name: /Overview/ }),
    ).not.toHaveAttribute("aria-current");
  });

  it("navigates on tap", () => {
    const props = renderBar();
    fireEvent.click(screen.getByRole("button", { name: /Reservations/ }));
    expect(props.onNavigate).toHaveBeenCalledWith("reservations");
  });

  it("warms the rail chunk on touchstart (M1, mobile has no hover)", () => {
    renderBar();
    fireEvent.touchStart(screen.getByRole("button", { name: /Kitchen/ }));
    expect(prefetchTab).toHaveBeenCalledWith("kitchen");
  });
});

describe("MobileTodayBar — badges and locks", () => {
  it("shows the kitchen cook-queue badge from the shared rules (#792)", () => {
    // Tickets waiting on the cook = pending + approved.
    renderBar({
      globalOrders: {
        1: [{ status: "approved" }, { status: "pending" }],
        2: [{ status: "approved" }, { status: "in_kitchen" }],
      },
    });
    expect(screen.getByTestId("mobile-today-badge-kitchen")).toHaveTextContent(
      "3",
    );
  });

  it("renders a lock indicator when the access locks a rail", () => {
    renderBar({
      accessState: { ...ACCESS, isSuspended: true },
    });
    expect(
      screen.getAllByLabelText("Locked").length,
    ).toBeGreaterThan(0);
  });

  it("renders nothing while the tutorial owns the chrome", () => {
    renderBar({ hidden: true });
    expect(
      screen.queryByRole("navigation", { name: "Today" }),
    ).not.toBeInTheDocument();
  });
});
