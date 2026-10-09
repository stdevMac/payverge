/** @jest-environment jsdom */
/**
 * #192 — Counter Name Prefix must visibly drive naming: seed "Counter N"
 * tiles under prefix "D" show Apply + will-become affordances.
 */
import React from "react";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import CounterManager from "../CounterManager";

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => ({
    hasAccess: true,
    loading: false,
    error: null,
    access: null,
    isSuspended: false,
    aiConfigured: false,
  }),
}));

jest.mock("@/hooks/useDirtyForm", () => ({
  useDirtyForm: () => ({ dirty: false, markClean: jest.fn() }),
}));

jest.mock("@/hooks/useUnsavedChangesGuard", () => ({
  useUnsavedChangesGuard: () => undefined,
}));

const mockGetBusinessCounters = jest.fn();
const mockUpdateCounterSettings = jest.fn();

jest.mock("@/api/counters", () => ({
  COUNTER_PREFIX_MAX_LENGTH: 5,
  getBusinessCounters: (...args: unknown[]) => mockGetBusinessCounters(...args),
  updateCounterSettings: (...args: unknown[]) =>
    mockUpdateCounterSettings(...args),
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: () => {} }),
  getTranslation: (key: string) => {
    const map: Record<string, string> = {
      "businessDashboard.dashboard.counterManager.title": "Counters",
      "businessDashboard.dashboard.counterManager.subtitle":
        "Naming and count setup only — not a live pickup board",
      "businessDashboard.dashboard.counterManager.honesty.title":
        "Setup only — not ready for dinner service",
      "businessDashboard.dashboard.counterManager.honesty.body":
        "This tab configures counter names.",
      "businessDashboard.dashboard.counterManager.header.statusOn": "Setup on",
      "businessDashboard.dashboard.counterManager.header.statusOff": "Setup off",
      "businessDashboard.dashboard.counterManager.header.counters": "Counters",
      "businessDashboard.dashboard.counterManager.settings.title":
        "Counter Settings",
      "businessDashboard.dashboard.counterManager.settings.subtitle":
        "How many counters and what they are called",
      "businessDashboard.dashboard.counterManager.settings.counterCount":
        "Number of Counters",
      "businessDashboard.dashboard.counterManager.settings.counterCountDescription":
        "How many named counters do you want? (1-20)",
      "businessDashboard.dashboard.counterManager.settings.counterPrefix":
        "Counter Name Prefix",
      "businessDashboard.dashboard.counterManager.settings.counterPrefixDescription":
        "Used when counters are created (e.g. '{prefix}' → {prefix}1).",
      "businessDashboard.dashboard.counterManager.settings.counterPrefixPreview":
        "New names will look like {examples}",
      "businessDashboard.dashboard.counterManager.activeCounters.title":
        "Counters",
      "businessDashboard.dashboard.counterManager.activeCounters.subtitle":
        "Labels available for walk-up bills.",
      "businessDashboard.dashboard.counterManager.activeCounters.applyPrefix":
        "Apply prefix {prefix} ({count})",
      "businessDashboard.dashboard.counterManager.activeCounters.applyingPrefix":
        "Applying prefix…",
      "businessDashboard.dashboard.counterManager.activeCounters.applyPrefixHint":
        "These default names do not match prefix {prefix} yet: {examples}.",
      "businessDashboard.dashboard.counterManager.activeCounters.customNamesKept":
        "{count} custom name(s) stay as-is: {names}.",
      "businessDashboard.dashboard.counterManager.activeCounters.willBecome":
        "Will become {name}",
      "businessDashboard.dashboard.counterManager.activeCounters.customKeptBadge":
        "Custom name kept",
      "businessDashboard.dashboard.counterManager.status.active": "Enabled",
      "businessDashboard.dashboard.counterManager.status.inactive": "Disabled",
      "businessDashboard.dashboard.counterManager.success.prefixApplied":
        "Prefix applied to default counter names.",
      "businessDashboard.dashboard.counterManager.success.settingsUpdated":
        "Saved",
      "businessSettings.saveBar.save": "Save",
      "businessSettings.saveBar.saving": "Saving",
      "businessSettings.saveBar.unsaved": "Unsaved",
      "businessSettings.saveBar.auto": "Auto",
      "businessSettings.saveBar.clean": "No unsaved changes",
    };
    return map[key] ?? key;
  },
}));

jest.mock("../CounterToggle", () => ({
  CounterToggle: () => null,
}));

jest.mock("../CounterSkeleton", () => ({
  CounterSkeleton: () => <div data-testid="counter-skeleton" />,
}));

jest.mock("../premium", () => ({
  PremiumPanel: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
}));

jest.mock("../shared/DashboardTabShell", () => ({
  __esModule: true,
  default: ({
    children,
    header,
  }: {
    children: React.ReactNode;
    header?: { title?: string };
  }) => (
    <div>
      <h1>{header?.title}</h1>
      {children}
    </div>
  ),
}));

jest.mock("../SaveBar", () => ({
  __esModule: true,
  default: () => null,
}));

describe("CounterManager apply-prefix (#192)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockGetBusinessCounters.mockResolvedValue({
      counters: [
        {
          id: 1,
          business_id: 1,
          counter_number: 1,
          name: "Counter 1",
          is_active: true,
          created_at: "",
          updated_at: "",
        },
        {
          id: 2,
          business_id: 1,
          counter_number: 2,
          name: "Barra principal",
          is_active: true,
          created_at: "",
          updated_at: "",
        },
      ],
      business: {
        counter_enabled: true,
        counter_count: 2,
        counter_prefix: "D",
      },
    });
    mockUpdateCounterSettings.mockResolvedValue({ message: "ok" });
  });

  it("shows apply affordance for Counter N under prefix D and keeps custom names unavoidable", async () => {
    render(<CounterManager businessId={1} />);

    expect(await screen.findByTestId("counter-apply-prefix")).toHaveTextContent(
      /Apply prefix D \(1\)/,
    );
    expect(screen.getByTestId("counter-prefix-mismatch-banner")).toHaveTextContent(
      /Counter 1 → D1/,
    );
    expect(screen.getByTestId("counter-will-become-1")).toHaveTextContent(
      "Will become D1",
    );
    expect(screen.getByTestId("counter-custom-names-banner")).toHaveTextContent(
      /Barra principal/,
    );
    expect(screen.getByTestId("counter-custom-kept-2")).toHaveTextContent(
      "Custom name kept",
    );

    fireEvent.click(screen.getByTestId("counter-apply-prefix"));

    await waitFor(() => {
      expect(mockUpdateCounterSettings).toHaveBeenCalledWith(1, {
        counter_enabled: true,
        counter_count: 2,
        counter_prefix: "D",
      });
    });
  });
});
