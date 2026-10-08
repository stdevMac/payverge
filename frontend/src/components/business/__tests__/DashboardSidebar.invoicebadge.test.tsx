/** @jest-environment jsdom */
import React from "react";
import { render, screen, within } from "@testing-library/react";
import DashboardSidebar from "../DashboardSidebar";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";
import { useFailedInvoiceCount } from "@/hooks/useFailedInvoiceCount";
import { useChatUnreadCount } from "@/hooks/useChatUnreadCount";

jest.mock("@/hooks/useBusinessAccess", () => ({ useBusinessAccess: jest.fn() }));
jest.mock("@/hooks/useFailedInvoiceCount", () => ({
  useFailedInvoiceCount: jest.fn(),
}));
// The sidebar now also calls useChatUnreadCount (react-query + SSE) — mock
// at the hook level so these tests don't need a QueryClientProvider/EventSource.
jest.mock("@/hooks/useChatUnreadCount", () => ({
  useChatUnreadCount: jest.fn(),
}));
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: () => {} }),
  getTranslation: (key: string) => key,
}));
jest.mock("next/image", () => ({
  __esModule: true,
  // eslint-disable-next-line @next/next/no-img-element, jsx-a11y/alt-text
  default: (props: any) => <img {...props} />,
}));

const tier = {
  access: null,
  loading: false,
  error: null,
  hasAccess: true,
  isSuspended: false,
  lockState: "active" as const,
  aiConfigured: true,
  refetch: jest.fn(),
};

function renderSidebar() {
  return render(
    <DashboardSidebar
      business={{ id: 42, name: "Test Bistro", custom_url: "tb" } as any}
      activeTab="cash-register"
      setActiveTab={jest.fn()}
      sidebarOpen={true}
      setSidebarOpen={jest.fn()}
      allowedTabs={[]}
      isStaffUser={false}
      staffData={null}
      globalOrders={{}}
      upcomingReservations={[]}
    />,
  );
}

/**
 * #726 removed the expanded-row hover bubble, so rows are no longer findable by
 * their description tail. Match the visible label instead — that is the only
 * copy an operator reads on an expanded rail.
 */
const findRowByDesc = (leaf: string) =>
  screen
    .getAllByRole("button")
    .find((b) => within(b).queryByText(`businessDashboard.tabs.${leaf}`));

describe("DashboardSidebar — failed-invoice nav badge", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    window.localStorage.clear();
    (useBusinessAccess as jest.Mock).mockReturnValue(tier);
    (useChatUnreadCount as jest.Mock).mockReturnValue(0);
  });

  it("renders and highlights the Caja tab when cash-register is active", () => {
    (useFailedInvoiceCount as jest.Mock).mockReturnValue(0);
    renderSidebar();

    const cajaButton = findRowByDesc("cashRegister");

    expect(cajaButton).toBeTruthy();
    expect(cajaButton).toHaveAttribute("aria-current", "page");
    expect(cajaButton!.textContent).toContain(
      "businessDashboard.tabs.cashRegister",
    );
  });

  it("badges the Accounting entry with the failed-invoice count", () => {
    (useFailedInvoiceCount as jest.Mock).mockReturnValue(2);
    renderSidebar();
    // Task 32: Accounting lives under the Finance group (primary rail), not MORE.
    const accountingButton = findRowByDesc("accounting");
    expect(accountingButton).toBeTruthy();
    expect(accountingButton!.textContent).toContain("2");
  });

  it("shows no count when there are no failed invoices", () => {
    (useFailedInvoiceCount as jest.Mock).mockReturnValue(0);
    renderSidebar();
    const accountingButton = findRowByDesc("accounting");
    expect(accountingButton).toBeTruthy();
    expect(accountingButton!.textContent).not.toContain("2");
  });

  it("badges the Team entry with the unread chat count", () => {
    (useFailedInvoiceCount as jest.Mock).mockReturnValue(0);
    (useChatUnreadCount as jest.Mock).mockReturnValue(3);
    renderSidebar();
    const teamButton = findRowByDesc("staff");
    expect(teamButton).toBeTruthy();
    expect(teamButton!.textContent).toContain("3");
  });
});
