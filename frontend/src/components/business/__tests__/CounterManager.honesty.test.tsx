/** @jest-environment jsdom */
/**
 * Counters tab honesty: operators must see that this is setup-only, not a
 * live dinner/takeaway pickup board (#172 / #239 product deferral).
 */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
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

jest.mock("@/api/counters", () => ({
  COUNTER_PREFIX_MAX_LENGTH: 5,
  getBusinessCounters: jest.fn(async () => ({
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
    ],
    business: {
      counter_enabled: true,
      counter_count: 2,
      counter_prefix: "D",
    },
  })),
  updateCounterSettings: jest.fn(),
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
        "This tab configures how many counters you have and how they are named. There is no live pickup queue or ticket-call board yet.",
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
        "Named counters",
      "businessDashboard.dashboard.counterManager.activeCounters.subtitle":
        "Labels available for walk-up bills. This is not a live pickup queue.",
      "businessDashboard.dashboard.counterManager.status.active": "Enabled",
      "businessDashboard.dashboard.counterManager.status.inactive": "Disabled",
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
    header?: { title?: string; subtitle?: string; status?: { label?: string } };
  }) => (
    <div>
      <h1>{header?.title}</h1>
      <p>{header?.subtitle}</p>
      <span>{header?.status?.label}</span>
      {children}
    </div>
  ),
}));

jest.mock("../SaveBar", () => ({
  __esModule: true,
  default: () => null,
}));

describe("CounterManager honesty treatment", () => {
  it("shows the setup-only honesty banner and does not claim a live queue", async () => {
    render(<CounterManager businessId={1} />);

    const banner = await screen.findByTestId("counter-honesty-banner");
    expect(banner).toHaveTextContent(/Setup only/i);
    expect(banner).toHaveTextContent(/no live pickup queue/i);
    expect(banner).not.toHaveTextContent(/Pickup rail|Counter-ready|call a ticket/i);

    await waitFor(() => {
      expect(screen.getByText("Counters")).toBeInTheDocument();
    });
    expect(
      screen.getByText(/Naming and count setup only/i),
    ).toBeInTheDocument();
    expect(screen.getByText("Setup on")).toBeInTheDocument();
  });
});
