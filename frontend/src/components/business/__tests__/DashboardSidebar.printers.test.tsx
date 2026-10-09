/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import DashboardSidebar from "../DashboardSidebar";

let mockIsStaffUser = false;
let mockStaffPermissions: string[] = [];

jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({
    isStaffUser: mockIsStaffUser,
    isWeb3User: !mockIsStaffUser,
    isOAuthUser: false,
    staffData: mockIsStaffUser
      ? { id: 1, role: "manager", business_id: 42 }
      : null,
  }),
}));

jest.mock("@/contexts/StaffPermissionsContext", () => ({
  useStaffPermissionsContext: () => ({
    permissions: mockStaffPermissions,
    rolePermissions: mockStaffPermissions,
    customGrants: [],
    isLoading: false,
    isError: false,
    refetch: jest.fn(),
  }),
}));

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: jest.fn(() => ({
    access: null,
    loading: false,
    error: null,
    hasAccess: true,
    isSuspended: false,
    lockState: "active" as const,
    aiConfigured: true,
    refetch: jest.fn(),
  })),
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: () => {} }),
  getTranslation: (key: string, _locale?: string) => {
    if (key === "printers.navLabel") return "Printers";
    if (key === "printers.navTitle") return "Thermal printers";
    return key;
  },
}));

jest.mock("next/image", () => ({
  __esModule: true,
  default: (props: any) => {
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

const baseProps: React.ComponentProps<typeof DashboardSidebar> = {
  business: { id: 42, name: "Test Bistro", custom_url: "test-bistro" } as any,
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

describe("DashboardSidebar — printer entry", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    window.localStorage.clear();
    mockIsStaffUser = false;
    mockStaffPermissions = [];
  });

  it("renders Printers as a dashboard tab for owners", () => {
    const setActiveTab = jest.fn();
    render(
      <DashboardSidebar
        {...baseProps}
        setActiveTab={setActiveTab}
        isStaffUser={false}
        staffData={null}
      />,
    );
    const tab = screen.getByRole("button", { name: /printers/i });
    expect(tab).toBeInTheDocument();
    fireEvent.click(tab);
    expect(setActiveTab).toHaveBeenCalledWith("printers");
  });

  it("renders the Printers tab for staff with printers:read", () => {
    mockIsStaffUser = true;
    mockStaffPermissions = ["printers:read"];
    render(
      <DashboardSidebar
        {...baseProps}
        isStaffUser
        allowedTabs={["printers"]}
        staffData={{ id: 1, role: "manager", business_id: 42 } as any}
      />,
    );
    expect(screen.getByRole("button", { name: /printers/i })).toBeInTheDocument();
  });

  it("hides the Printers tab for staff without printers:read", () => {
    mockIsStaffUser = true;
    mockStaffPermissions = ["bills:read"];
    render(
      <DashboardSidebar
        {...baseProps}
        isStaffUser
        allowedTabs={["overview"]}
        staffData={{ id: 1, role: "server", business_id: 42 } as any}
      />,
    );
    expect(screen.queryByRole("button", { name: /printers/i })).toBeNull();
  });
});
