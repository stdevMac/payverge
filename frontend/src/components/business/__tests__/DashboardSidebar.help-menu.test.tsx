/**
 * @jest-environment jsdom
 *
 * #61 — sidebar footer is one labeled Help control. No floating Help FAB
 * lives in the sidebar itself.
 */
import React from "react";
import { act, fireEvent, render, screen, within } from "@testing-library/react";
import DashboardSidebar from "../DashboardSidebar";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: jest.fn(),
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: () => {} }),
  getTranslation: (key: string) => {
    const leaf = key.replace(/^businessDashboard\./, "");
    const labels: Record<string, string> = {
      "tabs.overview": "Overview",
      "tabs.bills": "Bills",
      "tabs.kitchen": "Kitchen",
      "tabs.reservations": "Reservations",
      "tabs.settings": "Settings",
      "footer.helpMenu": "Help",
      "footer.askAssistant": "Ask assistant",
      "footer.requestHelp": "Request Help",
      "footer.urgentSupport": "Urgent Support",
      "footer.bookCall": "Book a Call",
      "footer.language": "Language",
      "tutorial.takeATour": "Take a tour",
      "sidebarGroups.setup": "Setup",
      "sidebarGroups.operations": "Operations",
      "sidebarGroups.ai": "AI",
      "sidebarGroups.management": "Management",
      "sidebarGroups.finance": "Finance",
      "sidebar.badgeCount": "{count} in {label}",
      "sidebar.billsAlertBadge": "{count} active bills",
      "sidebar.billsAlertBadge_one": "{count} active bill",
      "sidebar.billsAlertBadge_other": "{count} active bills",
      "sidebar.kitchenReadyBadge": "{count} kitchen-ready orders",
      "sidebar.kitchenReadyBadge_one": "{count} kitchen-ready order",
      "sidebar.kitchenReadyBadge_other": "{count} kitchen-ready orders",
      "locked.short": "Locked",
      "locked.requires": "Locked — requires {plan}",
    };
    return labels[leaf] ?? key;
  },
}));

jest.mock("next/image", () => ({
  __esModule: true,
  default: (props: Record<string, unknown>) => {
    // eslint-disable-next-line @next/next/no-img-element, jsx-a11y/alt-text
    return <img {...props} />;
  },
}));

jest.mock("@/hooks/useFailedInvoiceCount", () => ({
  useFailedInvoiceCount: jest.fn(() => 0),
}));
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

describe("DashboardSidebar — Help menu footer (#61)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    window.localStorage.clear();
    (useBusinessAccess as jest.Mock).mockReturnValue(mockTierDefaults);
    Element.prototype.scrollIntoView = jest.fn();
  });

  it("renders a labeled Help control and no competing floating Help FAB", () => {
    render(
      <DashboardSidebar
        business={{ id: 51, name: "AI Pro Lounge", custom_url: "ai-pro" } as any}
        activeTab="overview"
        setActiveTab={jest.fn()}
        sidebarOpen
        setSidebarOpen={jest.fn()}
        onStartTutorial={jest.fn()}
        onOpenOpsAssistant={jest.fn()}
      />,
    );

    const help = screen.getByTestId("sidebar-help-menu");
    expect(help).toHaveAccessibleName("Help");
    expect(within(help).getByText("Help")).toBeInTheDocument();
    expect(screen.queryByTestId("ops-assistant-fab")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /^fabLabel$/i }),
    ).not.toBeInTheDocument();
  });

  it("sends Request Help to the project support link and hides unset channels", async () => {
    render(
      <DashboardSidebar
        business={{ id: 51, name: "AI Pro Lounge", custom_url: "ai-pro" } as any}
        activeTab="overview"
        setActiveTab={jest.fn()}
        sidebarOpen
        setSidebarOpen={jest.fn()}
        onStartTutorial={jest.fn()}
        onOpenOpsAssistant={jest.fn()}
      />,
    );

    const help = screen.getByTestId("sidebar-help-menu");
    await act(async () => {
      fireEvent.pointerDown(help, { button: 0, pointerType: "mouse" });
      fireEvent.pointerUp(help, { button: 0, pointerType: "mouse" });
      fireEvent.click(help);
    });

    const requestHelp = await screen.findByText("Request Help");
    expect(requestHelp.closest("a")).toHaveAttribute(
      "href",
      "https://github.com/stdevMac/payverge/issues",
    );
    // A stock install ships no phone, chat handle or booking calendar.
    expect(screen.queryByText("Urgent Support")).not.toBeInTheDocument();
    expect(screen.queryByText("Book a Call")).not.toBeInTheDocument();
    expect(document.body.innerHTML).not.toMatch(
      /wa\.me|calendar\.app\.google|t\.me\/payverge_io/,
    );
  });

  it("shows Globe + a visible locale code on the footer language switcher (#593)", () => {
    render(
      <DashboardSidebar
        business={{ id: 51, name: "AI Pro Lounge", custom_url: "ai-pro" } as any}
        activeTab="overview"
        setActiveTab={jest.fn()}
        sidebarOpen
        setSidebarOpen={jest.fn()}
        onStartTutorial={jest.fn()}
        onOpenOpsAssistant={jest.fn()}
      />,
    );

    const language = screen.getByRole("button", { name: /change language/i });
    expect(within(language).getByText("EN")).toBeInTheDocument();
    expect(language.querySelector("svg.lucide-globe")).not.toBeNull();
    expect(language.textContent ?? "").not.toMatch(/🇺🇸|🇪🇸|🇦🇷|🌐/);
  });
});
