/** @jest-environment jsdom */
import { fireEvent, render, screen } from "@testing-library/react";
import { CommandPaletteProvider } from "../CommandPaletteProvider";
import {
  resetInstanceCacheForTests,
  setInstanceForTests,
} from "@/hooks/useInstance";
import { parseInstanceInfo } from "@/lib/instance/instanceInfo";

// Operator-tier translation provider: echo the leaf key so assertions are stable.
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => key.split(".").pop(),
}));

// Fully-entitled business so no tab is locked/hidden.
jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => ({
    hasAccess: true,
    isSuspended: false,
    aiConfigured: true,
    loading: false,
  }),
}));

// Task 35: record indexers — resolve empty so tests don't hang on open handles.
jest.mock("@/api/bills", () => ({
  getBusinessBills: jest.fn(async () => ({ bills: [] })),
}));
jest.mock("@/api/staff", () => ({
  getBusinessStaff: jest.fn(async () => ({ staff: [], pending_invitations: [] })),
}));
jest.mock("@/api/business", () => {
  const actual = jest.requireActual("@/api/business");
  return {
    ...actual,
    getMenu: jest.fn(async () => ({ categories: [] })),
    getTablesWithStatus: jest.fn(async () => ({ tables: [] })),
  };
});

const business = {
  id: 7,
  name: "Test Bistro",
  custom_url: "test-bistro",
  business_page_enabled: true,
} as never;

function renderProvider() {
  const setActiveTab = jest.fn();
  const setSidebarOpen = jest.fn();
  render(
    <CommandPaletteProvider
      business={business}
      activeTab="overview"
      setActiveTab={setActiveTab}
      setSidebarOpen={setSidebarOpen}
      allowedTabs={[]}
      isStaffUser={false}
      onStartTutorial={() => {}}
    >
      <div>dashboard content</div>
    </CommandPaletteProvider>,
  );
  return { setActiveTab, setSidebarOpen };
}

describe("CommandPaletteProvider", () => {
  beforeEach(() => window.localStorage.clear());

  it("opens on Ctrl/⌘+K and closes on a second press", () => {
    renderProvider();
    expect(screen.queryByRole("combobox")).not.toBeInTheDocument();

    fireEvent.keyDown(window, { key: "k", ctrlKey: true });
    expect(screen.getByRole("combobox")).toBeInTheDocument();

    fireEvent.keyDown(window, { key: "k", ctrlKey: true });
    expect(screen.queryByRole("combobox")).not.toBeInTheDocument();
  });

  it("navigates and records a recent when a section is chosen", () => {
    const { setActiveTab, setSidebarOpen } = renderProvider();
    fireEvent.keyDown(window, { key: "k", metaKey: true });

    const input = screen.getByRole("combobox");
    fireEvent.change(input, { target: { value: "bills" } });
    fireEvent.keyDown(input, { key: "Enter" });

    expect(setActiveTab).toHaveBeenCalledWith("bills");
    expect(setSidebarOpen).toHaveBeenCalledWith(false);
    // Palette closes after dispatch.
    expect(screen.queryByRole("combobox")).not.toBeInTheDocument();
    // Recent persisted under the per-business key.
    expect(window.localStorage.getItem("payverge_cmdk_recent:7")).toContain("bills");
  });

  it("finds cash-register by keyword and navigates to it", () => {
    const { setActiveTab, setSidebarOpen } = renderProvider();
    fireEvent.keyDown(window, { key: "k", metaKey: true });

    const input = screen.getByRole("combobox");
    fireEvent.change(input, { target: { value: "reconciliation" } });
    fireEvent.keyDown(input, { key: "Enter" });

    expect(setActiveTab).toHaveBeenCalledWith("cash-register");
    expect(setSidebarOpen).toHaveBeenCalledWith(false);
  });

  it("offers storefront quick actions for a published business", () => {
    renderProvider();
    fireEvent.keyDown(window, { key: "k", metaKey: true });
    // Action labels come from the (mocked) leaf keys.
    expect(screen.getByText("viewStorefront")).toBeInTheDocument();
    expect(screen.getByText("copyStorefrontLink")).toBeInTheDocument();
  });

  describe("instance AI gate", () => {
    afterEach(() => resetInstanceCacheForTests());
    const withAi = (ai: boolean) =>
      setInstanceForTests(
        parseInstanceInfo({
          registration_mode: "invite",
          features: { ai },
        }),
      );

    it("never offers the AI tabs when the instance has no LLM provider", () => {
      withAi(false);
      renderProvider();
      fireEvent.keyDown(window, { key: "k", metaKey: true });
      fireEvent.change(screen.getByRole("combobox"), { target: { value: "ai" } });
      expect(screen.queryByText("aiWaiter")).not.toBeInTheDocument();
      expect(screen.queryByText("directorConsole")).not.toBeInTheDocument();
    });

    it("offers them when the instance confirms AI is configured", () => {
      withAi(true);
      renderProvider();
      fireEvent.keyDown(window, { key: "k", metaKey: true });
      fireEvent.change(screen.getByRole("combobox"), { target: { value: "ai" } });
      expect(screen.getByText("aiWaiter")).toBeInTheDocument();
    });
  });
});
