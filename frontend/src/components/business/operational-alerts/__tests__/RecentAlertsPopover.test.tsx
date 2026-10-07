/** @jest-environment jsdom */
import React from "react";
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  fetchRecentOperationalAlerts as fetchRecentOperationalAlertsActual,
  type OperationalAlert,
} from "@/api/operationalAlerts";
import RecentAlertsPopover from "../RecentAlertsPopover";

let mockAlertsContext: any = null;

jest.mock("../useOperationalAlerts", () => ({
  useOptionalOperationalAlerts: () => mockAlertsContext,
}));

jest.mock("@/api/operationalAlerts", () => ({
  ...jest.requireActual("@/api/operationalAlerts"),
  fetchRecentOperationalAlerts: jest.fn(),
}));

const fetchRecentOperationalAlerts =
  fetchRecentOperationalAlertsActual as jest.Mock;

const minutesAgoIso = (minutes: number) =>
  new Date(Date.now() - minutes * 60_000).toISOString();

function makeAlert(overrides: Partial<OperationalAlert>): OperationalAlert {
  return {
    id: 1,
    business_id: 7,
    alert_type: "service_call",
    resource_type: "order",
    resource_id: 11,
    status: "open",
    priority: "normal",
    title: "Table 4 needs a waiter",
    body: "",
    claimed_by_name: null,
    last_event_at: minutesAgoIso(5),
    created_at: minutesAgoIso(5),
    updated_at: minutesAgoIso(5),
    ...overrides,
  };
}

function renderPopover() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <RecentAlertsPopover businessId={7} />
    </QueryClientProvider>,
  );
}

const openPopover = () =>
  fireEvent.click(screen.getByRole("button", { name: /recent alerts/i }));

describe("RecentAlertsPopover", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockAlertsContext = null;
  });

  it("fetches only when opened and renders title, relative time, and status chip", async () => {
    fetchRecentOperationalAlerts.mockResolvedValue({
      alerts: [
        // Both order_new so only ONE filter type is present → no filter chips
        // render, keeping the localized row titles unambiguous. Titles are
        // localized at render time from alert_type + metadata; the backend
        // English `title` is only a fallback for unknown types. (R3-AI-8)
        makeAlert({
          id: 1,
          alert_type: "order_new",
          title: "Order #7 created",
          metadata: { order_number: "7" },
          status: "open",
          created_at: minutesAgoIso(5),
        }),
        makeAlert({
          id: 2,
          alert_type: "order_new",
          title: "Order #42 created",
          metadata: { order_number: "42" },
          status: "resolved",
          created_at: minutesAgoIso(90),
        }),
      ],
    });

    renderPopover();
    expect(fetchRecentOperationalAlerts).not.toHaveBeenCalled();

    openPopover();

    // order_new with order_number metadata → localized "New order #<n>".
    expect(await screen.findByText("New order #7")).toBeInTheDocument();
    expect(screen.getByText("New order #42")).toBeInTheDocument();
    expect(fetchRecentOperationalAlerts).toHaveBeenCalledWith(7);

    // Relative times via the dashboard timeAgo vocabulary.
    expect(screen.getByText("5 min ago")).toBeInTheDocument();
    expect(screen.getByText("1h ago")).toBeInTheDocument();

    // Status chips: open = warning tone, resolved = neutral tone.
    const openChip = screen.getByText("Open");
    expect(openChip.closest("[data-status-tone]")).toHaveAttribute(
      "data-status-tone",
      "warn",
    );
    const resolvedChip = screen.getByText("Resolved");
    expect(resolvedChip.closest("[data-status-tone]")).toHaveAttribute(
      "data-status-tone",
      "neutral",
    );
  });

  it("narrows the list client-side with the type filter without refetching", async () => {
    fetchRecentOperationalAlerts.mockResolvedValue({
      alerts: [
        makeAlert({
          id: 1,
          alert_type: "bill_new",
          title: "New bill #9",
          metadata: { bill_number: "9" },
        }),
        makeAlert({
          id: 2,
          alert_type: "order_new",
          title: "Order #42 created",
          metadata: { order_number: "42" },
        }),
      ],
    });

    renderPopover();
    openPopover();

    // Row titles are localized at render time (R3-AI-8): bill_new → "New bill #9",
    // order_new → "New order #42". Neither collides with its filter chip label
    // ("New bills" / "New orders").
    expect(await screen.findByText("New bill #9")).toBeInTheDocument();
    expect(screen.getByText("New order #42")).toBeInTheDocument();

    // Chips offer "All types" plus only the types present in the data. Scope the
    // chip lookup to the filter group so it never matches a row title.
    const filterGroup = screen.getByRole("group", { name: /filter/i });
    fireEvent.click(
      within(filterGroup).getByRole("button", { name: "New bills" }),
    );

    expect(screen.getByText("New bill #9")).toBeInTheDocument();
    expect(screen.queryByText("New order #42")).not.toBeInTheDocument();

    // Back to all.
    fireEvent.click(
      within(filterGroup).getByRole("button", { name: "All types" }),
    );
    expect(screen.getByText("New order #42")).toBeInTheDocument();

    expect(fetchRecentOperationalAlerts).toHaveBeenCalledTimes(1);
  });

  it("shows the standard empty state when there is nothing in the last 7 days", async () => {
    fetchRecentOperationalAlerts.mockResolvedValue({ alerts: [] });

    renderPopover();
    openPopover();

    await waitFor(() =>
      expect(
        screen.getByText("Nothing in the last 7 days"),
      ).toBeInTheDocument(),
    );
  });

  it("opens and closes through pointer and keyboard", async () => {
    fetchRecentOperationalAlerts.mockResolvedValue({ alerts: [] });
    renderPopover();
    const trigger = screen.getByRole("button", { name: /recent alerts/i });

    fireEvent.pointerDown(trigger, { button: 0 });
    fireEvent.click(trigger);
    expect(
      await screen.findByText(/nothing in the last 7 days/i),
    ).toBeInTheDocument();

    fireEvent.keyDown(trigger, { key: "Escape" });
    await waitFor(() =>
      expect(
        screen.queryByText(/nothing in the last 7 days/i),
      ).not.toBeInTheDocument(),
    );

    fireEvent.keyDown(trigger, { key: "Enter" });
    fireEvent.keyUp(trigger, { key: "Enter" });
    expect(
      await screen.findByText(/nothing in the last 7 days/i),
    ).toBeInTheDocument();
  });

  it("shows bill context on payment rows so same-age events are distinguishable", async () => {
    fetchRecentOperationalAlerts.mockResolvedValue({
      alerts: [
        makeAlert({
          id: 21,
          alert_type: "payment_received",
          title: "Payment received",
          body: "Payment received for bill #1001",
          metadata: { bill_number: "1001", amount: "12.50", method: "card" },
          status: "resolved",
          created_at: minutesAgoIso(60 * 24 * 3),
        }),
        makeAlert({
          id: 22,
          alert_type: "payment_received",
          title: "Demo payment received",
          body: "B75-aff0f207-af2 was paid.",
          metadata: { demo: true },
          status: "resolved",
          created_at: minutesAgoIso(60 * 24 * 3),
        }),
      ],
    });

    const onNavigate = jest.fn();
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    render(
      <QueryClientProvider client={queryClient}>
        <RecentAlertsPopover businessId={7} onNavigate={onNavigate} />
      </QueryClientProvider>,
    );
    openPopover();

    expect(await screen.findByText("Payment received for bill #1001 · 12.50 · card")).toBeInTheDocument();
    expect(screen.getByText("B75-aff0f207-af2 was paid.")).toBeInTheDocument();
    const first = screen.getByRole("button", {
      name: /Payment received for bill #1001/,
    });
    fireEvent.click(first);
    expect(onNavigate).toHaveBeenCalled();
  });

  it("lets staff acknowledge and finish waiter calls from the alert list", async () => {
    const claimAlert = jest.fn().mockResolvedValue(undefined);
    const resolveAlert = jest.fn().mockResolvedValue(undefined);
    mockAlertsContext = {
      settings: null,
      audioBlocked: false,
      claimAlert,
      resolveAlert,
    };
    fetchRecentOperationalAlerts.mockResolvedValue({
      alerts: [
        makeAlert({
          id: 10,
          status: "open",
          resource_type: "table" as any,
          resource_id: 4,
          metadata: { table_name: "Table 4", reason: "water" },
        }),
        makeAlert({
          id: 11,
          status: "claimed",
          resource_type: "table" as any,
          resource_id: 5,
          claimed_by_name: "Ana",
          metadata: { table_name: "Table 5", reason: "check" },
        }),
      ],
    });

    renderPopover();
    openPopover();

    expect(await screen.findByText(/Table 4/)).toBeInTheDocument();
    expect(screen.getByText("Needs water")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Acknowledge" }));
    await waitFor(() =>
      expect(claimAlert).toHaveBeenCalledWith(10, "recent_alerts"),
    );

    fireEvent.click(screen.getByRole("button", { name: "Mark handled" }));
    await waitFor(() =>
      expect(resolveAlert).toHaveBeenCalledWith(11, "handled"),
    );
  });

  it("paints an opaque white panel so alert text does not overlay the sidebar", async () => {
    fetchRecentOperationalAlerts.mockResolvedValue({ alerts: [] });
    renderPopover();
    openPopover();
    const chrome = await screen.findByTestId("recent-alerts-chrome");
    expect(chrome.className).toMatch(/bg-white/);
    const trigger = screen.getByTestId("recent-alerts-trigger");
    expect(trigger.getAttribute("title")).toBeNull();
    expect(trigger).toHaveAttribute("data-open", "true");
  });

  it("dims sidebar labels with a backdrop scrim while Alertas recientes is open (#620)", async () => {
    fetchRecentOperationalAlerts.mockResolvedValue({ alerts: [] });
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    render(
      <div>
        <nav>
          <a href="/ai-waiter">Camarero IA</a>
        </nav>
        <QueryClientProvider client={queryClient}>
          <RecentAlertsPopover businessId={7} placement="top-start" />
        </QueryClientProvider>
      </div>,
    );
    expect(screen.getByText("Camarero IA")).toBeInTheDocument();
    expect(screen.queryByTestId("recent-alerts-backdrop")).not.toBeInTheDocument();

    openPopover();
    await screen.findByTestId("recent-alerts-chrome");

    const backdrop = await screen.findByTestId("recent-alerts-backdrop");
    expect(backdrop).toHaveAttribute("aria-hidden", "true");
    expect(backdrop.className).toMatch(/fixed/);
    expect(backdrop.className).toMatch(/inset-0/);
    expect(backdrop.className).toMatch(/bg-ink-950\/40|bg-black\/40/);
    expect(backdrop.className).toMatch(/z-\[60\]/);
    expect(screen.getByText("Camarero IA")).toBeInTheDocument();
  });
});
