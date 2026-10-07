/** @jest-environment jsdom */
/**
 * #393 — shrinking Number of Counters must not leave disabled leftover tiles
 * in the primary grid. After 2→3→2 the summary says "2 Counters"; the live
 * grid must match that count. Excess soft-disabled rows stay hidden until
 * the operator opens the Archived section.
 */
import React from "react";
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import CounterManager from "../CounterManager";
import type { Counter, CountersResponse } from "@/api/counters";

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
      "businessDashboard.dashboard.counterManager.header.statusOff":
        "Setup off",
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
      "businessDashboard.dashboard.counterManager.archivedCounters.title":
        "Archived counters",
      "businessDashboard.dashboard.counterManager.archivedCounters.subtitle":
        "Labels kept from earlier count changes. Raising the number of counters reactivates them.",
      "businessDashboard.dashboard.counterManager.archivedCounters.show":
        "Show archived ({count})",
      "businessDashboard.dashboard.counterManager.archivedCounters.hide":
        "Hide archived",
      "businessDashboard.dashboard.counterManager.status.active": "Enabled",
      "businessDashboard.dashboard.counterManager.status.inactive": "Disabled",
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
    header?: {
      title?: string;
      stats?: Array<{ label: string; value: React.ReactNode }>;
    };
  }) => (
    <div>
      <h1>{header?.title}</h1>
      {header?.stats?.map((stat) => (
        <div key={stat.label} data-testid="counter-header-stat">
          {stat.value} {stat.label}
        </div>
      ))}
      {children}
    </div>
  ),
}));

jest.mock("../SaveBar", () => ({
  __esModule: true,
  default: ({
    onSave,
    dirty,
    isSaving,
    labels,
  }: {
    onSave: () => void;
    dirty: boolean;
    isSaving: boolean;
    labels: { save: string };
  }) => (
    <button type="button" onClick={onSave} disabled={!dirty || isSaving}>
      {labels.save}
    </button>
  ),
}));

function row(
  id: number,
  number: number,
  name: string,
  isActive: boolean,
): Counter {
  return {
    id,
    business_id: 1,
    counter_number: number,
    name,
    is_active: isActive,
    created_at: "",
    updated_at: "",
  };
}

function payload(count: number, counters: Counter[]): CountersResponse {
  return {
    counters,
    business: {
      counter_enabled: true,
      counter_count: count,
      counter_prefix: "D",
    },
  };
}

const twoLive = payload(2, [row(1, 1, "D1", true), row(2, 2, "D2", true)]);
const threeLive = payload(3, [
  row(1, 1, "D1", true),
  row(2, 2, "D2", true),
  row(3, 3, "D3", true),
]);
const twoLiveOneArchived = payload(2, [
  row(1, 1, "D1", true),
  row(2, 2, "D2", true),
  row(3, 3, "D3", false),
]);
const threeLiveReactivated = payload(3, [
  row(1, 1, "D1", true),
  row(2, 2, "D2", true),
  row(3, 3, "D3", true),
]);

async function setCountAndSave(value: string) {
  const loadsBefore = mockGetBusinessCounters.mock.calls.length;
  const input = (await screen.findByTestId(
    "counter-count-input",
  )) as HTMLInputElement;
  fireEvent.change(input, { target: { value } });
  fireEvent.click(screen.getByText("Save"));
  await waitFor(() => {
    expect(mockGetBusinessCounters.mock.calls.length).toBeGreaterThan(
      loadsBefore,
    );
  });
}

describe("CounterManager inactive tiles (#393)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockUpdateCounterSettings.mockResolvedValue({ message: "ok" });
  });

  it("keeps the primary grid aligned with the configured count after 2→3→2", async () => {
    mockGetBusinessCounters
      .mockResolvedValueOnce(twoLive)
      .mockResolvedValueOnce(threeLive)
      .mockResolvedValueOnce(twoLiveOneArchived);

    render(<CounterManager businessId={1} />);

    expect(await screen.findByTestId("counter-tile-1")).toHaveTextContent("D1");
    expect(screen.getByTestId("counter-tile-2")).toHaveTextContent("D2");
    expect(screen.queryByTestId("counter-tile-3")).not.toBeInTheDocument();

    await setCountAndSave("3");
    await waitFor(() => {
      expect(mockUpdateCounterSettings).toHaveBeenCalledWith(
        1,
        expect.objectContaining({ counter_count: 3, counter_prefix: "D" }),
      );
    });
    expect(await screen.findByTestId("counter-tile-3")).toHaveTextContent("D3");
    expect(
      screen
        .getByTestId("counter-primary-grid")
        .querySelectorAll("[data-testid^='counter-tile-']"),
    ).toHaveLength(3);

    await setCountAndSave("2");
    await waitFor(() => {
      expect(mockUpdateCounterSettings).toHaveBeenLastCalledWith(
        1,
        expect.objectContaining({ counter_count: 2, counter_prefix: "D" }),
      );
    });

    const primary = await screen.findByTestId("counter-primary-grid");
    expect(within(primary).getByTestId("counter-tile-1")).toHaveTextContent(
      "D1",
    );
    expect(within(primary).getByTestId("counter-tile-2")).toHaveTextContent(
      "D2",
    );
    expect(
      within(primary).queryByTestId("counter-tile-3"),
    ).not.toBeInTheDocument();
    expect(
      primary.querySelectorAll("[data-testid^='counter-tile-']"),
    ).toHaveLength(2);
    expect(screen.getByTestId("counter-header-stat")).toHaveTextContent(
      "2 Counters",
    );
    expect(screen.queryByTestId("counter-tile-3")).not.toBeInTheDocument();
    expect(screen.queryByText("D3")).not.toBeInTheDocument();
  });

  it("hides leftover inactive rows until the archived section is opened", async () => {
    mockGetBusinessCounters.mockResolvedValue(twoLiveOneArchived);

    render(<CounterManager businessId={1} />);

    expect(
      await screen.findByTestId("counter-primary-grid"),
    ).toBeInTheDocument();
    expect(
      screen.queryByTestId("counter-archived-grid"),
    ).not.toBeInTheDocument();
    expect(screen.queryByText("D3")).not.toBeInTheDocument();

    const toggle = await screen.findByTestId("counter-archived-toggle");
    expect(toggle).toHaveTextContent("Show archived (1)");
    fireEvent.click(toggle);

    const archived = await screen.findByTestId("counter-archived-grid");
    expect(
      within(archived).getByTestId("counter-archived-tile-3"),
    ).toHaveTextContent("D3");
    expect(within(archived).getByText("Disabled")).toBeInTheDocument();
    expect(screen.getByTestId("counter-archived-toggle")).toHaveTextContent(
      "Hide archived",
    );
    expect(
      screen
        .getByTestId("counter-primary-grid")
        .querySelectorAll("[data-testid^='counter-tile-']"),
    ).toHaveLength(2);
  });

  it("returns an archived number to the primary grid when the count rises again", async () => {
    mockGetBusinessCounters
      .mockResolvedValueOnce(twoLiveOneArchived)
      .mockResolvedValueOnce(threeLiveReactivated);

    render(<CounterManager businessId={1} />);
    expect(
      await screen.findByTestId("counter-archived-toggle"),
    ).toBeInTheDocument();
    expect(screen.queryByTestId("counter-tile-3")).not.toBeInTheDocument();

    await setCountAndSave("3");
    expect(await screen.findByTestId("counter-tile-3")).toHaveTextContent("D3");
    expect(
      screen.queryByTestId("counter-archived-toggle"),
    ).not.toBeInTheDocument();
    expect(
      screen
        .getByTestId("counter-primary-grid")
        .querySelectorAll("[data-testid^='counter-tile-']"),
    ).toHaveLength(3);
  });
});
