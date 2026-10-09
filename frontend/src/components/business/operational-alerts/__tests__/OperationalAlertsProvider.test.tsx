/** @jest-environment jsdom */
import React from "react";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  claimOperationalAlert,
  resolveOperationalAlert,
  fetchOperationalAlerts,
  fetchOperationalAlertSettings,
  OperationalAlertConflictError,
  type OperationalAlert,
  type OperationalAlertSettings,
} from "@/api/operationalAlerts";
import { OperationalAlertsProvider } from "../OperationalAlertsProvider";
import { useOperationalAlerts } from "../useOperationalAlerts";
import { getChatSoundPrefs } from "@/utils/chatSoundPrefs";
import { queryKeys } from "@/api/queryKeys";

declare global {
  var __lastAlertSSEOptions:
    | {
        businessId: number;
        enabled: boolean;
        onEvent: (event: {
          type: string;
          data: Record<string, unknown>;
        }) => void;
        onReconnect?: () => void;
      }
    | undefined;
}

jest.mock("@/api/operationalAlerts", () => ({
  // Real error class so the provider's `instanceof` 409 branch works.
  OperationalAlertConflictError: jest.requireActual("@/api/operationalAlerts")
    .OperationalAlertConflictError,
  fetchOperationalAlerts: jest.fn(),
  fetchOperationalAlertSettings: jest.fn(),
  claimOperationalAlert: jest.fn(),
  resolveOperationalAlert: jest.fn(),
}));

jest.mock("@/hooks/useSSEEvents", () => ({
  useSSEEvents: jest.fn((options) => {
    globalThis.__lastAlertSSEOptions = options;
    return { degraded: false, blocked: false, reconnect: jest.fn() };
  }),
}));

const mockSoundEngine = {
  unlock: jest.fn(async () => ({
    unlocked: true,
    blocked: false,
    repeating: false,
  })),
  playOnce: jest.fn(async () => undefined),
  startRepeating: jest.fn(),
  stopRepeating: jest.fn(),
  isRepeating: jest.fn(() => false),
  getState: jest.fn(() => ({
    unlocked: false,
    blocked: false,
    repeating: false,
  })),
};

jest.mock("@/utils/operationalAlertSound", () => ({
  createOperationalAlertSoundEngine: jest.fn(() => mockSoundEngine),
  getLocalAlertSoundOverrides: jest.fn(() => ({ muted: false, volume: null })),
}));

jest.mock("@/utils/chatSoundPrefs", () => ({
  getChatSoundPrefs: jest.fn(() => ({
    announcementSound: true,
    messageSound: false,
  })),
}));

const mockLeadershipRef: { current: boolean } = { current: true };
// Captures the provider's onAcquire callback so tests can simulate this tab
// inheriting Web Locks leadership (previous leader tab closed).
const mockOnAcquire: { current: (() => void) | undefined } = {
  current: undefined,
};

jest.mock("@/hooks/useSoundLeadership", () => ({
  useSoundLeadership: jest.fn((_bid: number, onAcquire?: () => void) => {
    mockOnAcquire.current = onAcquire;
    return mockLeadershipRef;
  }),
}));

const mockShowInfo = jest.fn();
const mockShowWarning = jest.fn();

jest.mock("@/contexts/ToastContext", () => ({
  useToast: jest.fn(() => ({
    showToast: jest.fn(),
    showSuccess: jest.fn(),
    showError: jest.fn(),
    showWarning: mockShowWarning,
    showInfo: mockShowInfo,
  })),
}));

const mockGetChatSoundPrefs = getChatSoundPrefs as jest.MockedFunction<
  typeof getChatSoundPrefs
>;

const mockFetchOperationalAlerts =
  fetchOperationalAlerts as jest.MockedFunction<typeof fetchOperationalAlerts>;
const mockFetchOperationalAlertSettings =
  fetchOperationalAlertSettings as jest.MockedFunction<
    typeof fetchOperationalAlertSettings
  >;
const mockClaimOperationalAlert = claimOperationalAlert as jest.MockedFunction<
  typeof claimOperationalAlert
>;
const mockResolveOperationalAlert =
  resolveOperationalAlert as jest.MockedFunction<
    typeof resolveOperationalAlert
  >;

const settings: OperationalAlertSettings = {
  enabled: true,
  browser_notifications_enabled: false,
  sound_enabled: true,
  volume: 0.6,
  repeat_interval_seconds: 10,
  event_settings: {
    order_new: {
      enabled: true,
      repeating: true,
      sound_enabled: true,
      repeat_interval_seconds: 10,
    },
    kitchen_order_ready: {
      enabled: true,
      repeating: true,
      sound_enabled: true,
      repeat_interval_seconds: 10,
    },
    reservation_new: {
      enabled: true,
      repeating: false,
      sound_enabled: true,
      repeat_interval_seconds: null,
    },
    reservation_approval: {
      enabled: true,
      repeating: true,
      sound_enabled: true,
      repeat_interval_seconds: 10,
    },
    delivery_new: {
      enabled: true,
      repeating: true,
      sound_enabled: true,
      repeat_interval_seconds: 10,
    },
    bill_new: {
      enabled: true,
      repeating: false,
      sound_enabled: true,
      repeat_interval_seconds: null,
    },
    payment_requested: {
      enabled: true,
      repeating: false,
      sound_enabled: true,
      repeat_interval_seconds: null,
    },
    payment_received: {
      enabled: true,
      repeating: false,
      sound_enabled: true,
      repeat_interval_seconds: null,
    },
    payment_refund_review: {
      enabled: true,
      repeating: false,
      sound_enabled: true,
      repeat_interval_seconds: null,
    },
    service_call: {
      enabled: true,
      repeating: true,
      sound_enabled: true,
      repeat_interval_seconds: 10,
    },
    ai_takeover: {
      enabled: true,
      repeating: true,
      sound_enabled: true,
      repeat_interval_seconds: 10,
    },
  },
};

const makeAlert = (
  overrides: Partial<OperationalAlert> = {},
): OperationalAlert => ({
  id: 1,
  business_id: 42,
  alert_type: "order_new",
  resource_type: "order",
  resource_id: 77,
  status: "open",
  priority: "normal",
  title: "New order",
  body: "Order 77",
  claimed_by_staff_id: null,
  claimed_by_user_id: null,
  claimed_by_name: null,
  claimed_at: null,
  resolved_at: null,
  snoozed_until: null,
  last_event_at: "2026-06-04T12:00:00Z",
  metadata: null,
  created_at: "2026-06-04T12:00:00Z",
  updated_at: "2026-06-04T12:00:00Z",
  ...overrides,
});

const Probe = () => {
  const { counts, claimByResource, resolveAlert, settings } =
    useOperationalAlerts();

  return (
    <div>
      <span data-testid="bills-count">{counts.bills}</span>
      <span data-testid="reservations-count">{counts.reservations}</span>
      <span data-testid="tables-count">{counts.tables}</span>
      <span data-testid="settings-loaded">{settings ? "yes" : "no"}</span>
      <button onClick={() => claimByResource("order", 77, "row_click")}>
        claim order
      </button>
      <button onClick={() => resolveAlert(1, "handled")}>resolve alert</button>
    </div>
  );
};

const renderProvider = () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });

  const view = render(
    <QueryClientProvider client={client}>
      <OperationalAlertsProvider businessId={42} enabled>
        <Probe />
      </OperationalAlertsProvider>
    </QueryClientProvider>,
  );
  return { ...view, client };
};

describe("OperationalAlertsProvider", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    globalThis.__lastAlertSSEOptions = undefined;
    mockLeadershipRef.current = true;
    mockOnAcquire.current = undefined;
    mockGetChatSoundPrefs.mockReturnValue({
      announcementSound: true,
      messageSound: false,
    });
    mockFetchOperationalAlerts.mockResolvedValue({ alerts: [makeAlert()] });
    mockFetchOperationalAlertSettings.mockResolvedValue(settings);
    mockClaimOperationalAlert.mockResolvedValue(
      makeAlert({ status: "claimed", claimed_by_name: "Staff" }),
    );
    mockResolveOperationalAlert.mockResolvedValue(
      makeAlert({ status: "resolved", resolved_at: "2026-06-04T12:01:00Z" }),
    );
  });

  it("exposes counts, with an order_new alert incrementing bills count", async () => {
    renderProvider();

    await waitFor(() => {
      expect(screen.getByTestId("bills-count")).toHaveTextContent("1");
    });
  });

  it("exposes counts, with a reservation_approval alert incrementing reservations count", async () => {
    mockFetchOperationalAlerts.mockResolvedValue({
      alerts: [
        makeAlert({
          id: 3,
          alert_type: "reservation_approval",
          resource_type: "reservation",
          resource_id: 9,
          priority: "urgent",
          title: "Reservation approval needed",
          body: "Table for 4",
        }),
      ],
    });

    renderProvider();

    await waitFor(() => {
      expect(screen.getByTestId("reservations-count")).toHaveTextContent("1");
    });
  });

  it("surfaces active service calls in the tables count", async () => {
    mockFetchOperationalAlerts.mockResolvedValue({
      alerts: [
        makeAlert({
          alert_type: "service_call",
          resource_type: "table" as any,
          resource_id: 9,
          title: "Table 9 needs a waiter",
          metadata: { table_name: "Table 9", reason: "check" },
          last_event_at: new Date().toISOString(),
          created_at: new Date().toISOString(),
        }),
      ],
    });

    renderProvider();

    await waitFor(() => {
      expect(screen.getByTestId("tables-count")).toHaveTextContent("1");
    });
  });

  it("does not count service calls older than the seating SLA", async () => {
    const dayAgo = new Date(Date.now() - 26 * 60 * 60_000).toISOString();
    mockFetchOperationalAlerts.mockResolvedValue({
      alerts: [
        makeAlert({
          alert_type: "service_call",
          resource_type: "table" as any,
          resource_id: 1,
          title: "Table 1 needs water",
          metadata: { table_name: "Table 1", reason: "water" },
          last_event_at: dayAgo,
          created_at: dayAgo,
        }),
      ],
    });

    renderProvider();

    await waitFor(() => {
      expect(screen.getByTestId("settings-loaded")).toHaveTextContent("yes");
    });
    expect(screen.getByTestId("tables-count")).toHaveTextContent("0");
  });

  it("still counts a day-old unpaid check-please the API still returned", async () => {
    const dayAgo = new Date(Date.now() - 26 * 60 * 60_000).toISOString();
    mockFetchOperationalAlerts.mockResolvedValue({
      alerts: [
        makeAlert({
          alert_type: "service_call",
          resource_type: "table" as any,
          resource_id: 1,
          title: "Table 1 needs the check",
          metadata: { table_name: "Table 1", reason: "check" },
          last_event_at: dayAgo,
          created_at: dayAgo,
        }),
      ],
    });

    renderProvider();

    await waitFor(() => {
      expect(screen.getByTestId("tables-count")).toHaveTextContent("1");
    });
  });

  it("resolves an alert, removes it from active counts, and refreshes recent history", async () => {
    const { client } = renderProvider();
    client.setQueryData(queryKeys.alerts.recent("42"), {
      alerts: [makeAlert()],
    });
    const invalidate = jest.spyOn(client, "invalidateQueries");

    await waitFor(() => {
      expect(screen.getByTestId("bills-count")).toHaveTextContent("1");
    });

    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "resolve alert" }));
    });

    expect(mockResolveOperationalAlert).toHaveBeenCalledWith(42, 1, "handled");
    expect(screen.getByTestId("bills-count")).toHaveTextContent("0");
    expect(
      client.getQueryData<{ alerts: OperationalAlert[] }>(
        queryKeys.alerts.recent("42"),
      )?.alerts[0].status,
    ).toBe("resolved");
    expect(invalidate).toHaveBeenCalledWith({
      queryKey: expect.arrayContaining(["alerts"]),
    });
  });

  it("patches alert.claimed SSE state without removing the claimed alert from counts", async () => {
    renderProvider();

    await waitFor(() => {
      expect(screen.getByTestId("bills-count")).toHaveTextContent("1");
    });

    act(() => {
      globalThis.__lastAlertSSEOptions?.onEvent({
        type: "alert.claimed",
        data: makeAlert({
          status: "claimed",
          claimed_by_name: "Server",
        }) as unknown as Record<string, unknown>,
      });
    });

    expect(screen.getByTestId("bills-count")).toHaveTextContent("1");
  });

  it('claimByResource("order", 77, "row_click") calls claimOperationalAlert(42, 1, "row_click")', async () => {
    const { client } = renderProvider();
    client.setQueryData(queryKeys.alerts.recent("42"), {
      alerts: [makeAlert()],
    });

    await waitFor(() => {
      expect(screen.getByTestId("bills-count")).toHaveTextContent("1");
    });

    await act(async () => {
      screen.getByRole("button", { name: "claim order" }).click();
    });

    expect(mockClaimOperationalAlert).toHaveBeenCalledWith(42, 1, "row_click");
    expect(
      client.getQueryData<{ alerts: OperationalAlert[] }>(
        queryKeys.alerts.recent("42"),
      )?.alerts[0].status,
    ).toBe("claimed");
  });

  it("removes alert from counts on a resolved SSE event", async () => {
    renderProvider();

    await waitFor(() => {
      expect(screen.getByTestId("bills-count")).toHaveTextContent("1");
    });

    act(() => {
      globalThis.__lastAlertSSEOptions?.onEvent({
        type: "alert.resolved",
        data: makeAlert({
          status: "resolved",
          resolved_at: "2026-06-04T12:01:00Z",
        }) as unknown as Record<string, unknown>,
      });
    });

    expect(screen.getByTestId("bills-count")).toHaveTextContent("0");
  });

  it("plays one-shot sound for non-repeating created alerts", async () => {
    renderProvider();

    await waitFor(() => {
      expect(screen.getByTestId("bills-count")).toHaveTextContent("1");
    });

    act(() => {
      globalThis.__lastAlertSSEOptions?.onEvent({
        type: "alert.created",
        data: makeAlert({
          id: 2,
          alert_type: "payment_received",
          resource_type: "payment",
          resource_id: 2,
          title: "Payment received",
        }) as unknown as Record<string, unknown>,
      });
    });

    await waitFor(() => {
      expect(mockSoundEngine.playOnce).toHaveBeenCalledWith({
        alertTypes: ["payment_received"],
        volume: settings.volume,
        emphasize: false,
      });
    });
  });

  it("non-leader tab does not play one-shot sounds on alert.created", async () => {
    mockLeadershipRef.current = false;
    renderProvider();

    await waitFor(() => {
      expect(screen.getByTestId("bills-count")).toHaveTextContent("1");
    });

    act(() => {
      globalThis.__lastAlertSSEOptions?.onEvent({
        type: "alert.created",
        data: makeAlert({
          id: 2,
          alert_type: "payment_received",
          resource_type: "payment",
          resource_id: 2,
          title: "Payment received",
          metadata: { bill_number: "9" },
        }) as unknown as Record<string, unknown>,
      });
    });

    // Visual path still runs even for a non-leader tab. Toast title/body are
    // localized at render time from alert_type + metadata. (R3-AI-8)
    await waitFor(() => {
      expect(mockShowInfo).toHaveBeenCalledWith(
        "Payment received",
        "Payment received for bill #9",
      );
    });

    expect(mockSoundEngine.playOnce).not.toHaveBeenCalled();
  });

  it("leader tab still plays one-shot sounds on alert.created", async () => {
    mockLeadershipRef.current = true;
    renderProvider();

    await waitFor(() => {
      expect(screen.getByTestId("bills-count")).toHaveTextContent("1");
    });

    act(() => {
      globalThis.__lastAlertSSEOptions?.onEvent({
        type: "alert.created",
        data: makeAlert({
          id: 2,
          alert_type: "payment_received",
          resource_type: "payment",
          resource_id: 2,
          title: "Payment received",
        }) as unknown as Record<string, unknown>,
      });
    });

    await waitFor(() => {
      expect(mockSoundEngine.playOnce).toHaveBeenCalledWith({
        alertTypes: ["payment_received"],
        volume: settings.volume,
        emphasize: false,
      });
    });
  });

  it("non-leader tab does not post browser notifications", async () => {
    mockLeadershipRef.current = false;
    const notificationSpy = jest.fn();
    class MockNotification {
      static permission = "granted";
      constructor(...args: unknown[]) {
        notificationSpy(...args);
      }
    }
    const originalNotification = (
      window as unknown as { Notification?: unknown }
    ).Notification;
    Object.defineProperty(window, "Notification", {
      value: MockNotification,
      configurable: true,
      writable: true,
    });

    try {
      mockFetchOperationalAlertSettings.mockResolvedValue({
        ...settings,
        browser_notifications_enabled: true,
      });

      renderProvider();

      await waitFor(() => {
        expect(screen.getByTestId("bills-count")).toHaveTextContent("1");
      });

      act(() => {
        globalThis.__lastAlertSSEOptions?.onEvent({
          type: "alert.created",
          data: makeAlert({
            id: 2,
            alert_type: "reservation_new",
            resource_type: "reservation",
            resource_id: 2,
            title: "New reservation",
          }) as unknown as Record<string, unknown>,
        });
      });

      expect(notificationSpy).not.toHaveBeenCalled();
    } finally {
      Object.defineProperty(window, "Notification", {
        value: originalNotification,
        configurable: true,
        writable: true,
      });
    }
  });

  it("alert.created with priority urgent shows a warning toast", async () => {
    renderProvider();

    await waitFor(() => {
      expect(screen.getByTestId("bills-count")).toHaveTextContent("1");
    });

    act(() => {
      globalThis.__lastAlertSSEOptions?.onEvent({
        type: "alert.created",
        data: makeAlert({
          id: 2,
          alert_type: "reservation_approval",
          resource_type: "reservation",
          resource_id: 9,
          priority: "urgent",
          title: "Reservation approval needed",
          body: "Table for 4",
          metadata: { customer_name: "Alex" },
        }) as unknown as Record<string, unknown>,
      });
    });

    // Toast title/body are localized at render time from alert_type + metadata:
    // reservation_approval → "Reservation needs approval" / "Reservation from Alex".
    await waitFor(() => {
      expect(mockShowWarning).toHaveBeenCalledWith(
        "Reservation needs approval",
        "Reservation from Alex",
      );
    });
    expect(mockShowInfo).not.toHaveBeenCalled();
  });

  it("alert.created with normal priority shows an info toast", async () => {
    renderProvider();

    await waitFor(() => {
      expect(screen.getByTestId("bills-count")).toHaveTextContent("1");
    });

    act(() => {
      globalThis.__lastAlertSSEOptions?.onEvent({
        type: "alert.created",
        data: makeAlert({
          id: 2,
          alert_type: "payment_received",
          resource_type: "payment",
          resource_id: 2,
          priority: "normal",
          title: "Payment received",
          metadata: { bill_number: "9" },
        }) as unknown as Record<string, unknown>,
      });
    });

    await waitFor(() => {
      expect(mockShowInfo).toHaveBeenCalledWith(
        "Payment received",
        "Payment received for bill #9",
      );
    });
    expect(mockShowWarning).not.toHaveBeenCalled();
  });

  it("urgent alert passes emphasize to the one-shot sound", async () => {
    renderProvider();

    await waitFor(() => {
      expect(screen.getByTestId("bills-count")).toHaveTextContent("1");
    });

    act(() => {
      globalThis.__lastAlertSSEOptions?.onEvent({
        type: "alert.created",
        // reservation_new is non-repeating in the test settings, so it takes
        // the one-shot playOnce() path (reservation_approval repeats).
        data: makeAlert({
          id: 2,
          alert_type: "reservation_new",
          resource_type: "reservation",
          resource_id: 9,
          priority: "urgent",
          title: "Urgent reservation",
          body: "Table for 4",
        }) as unknown as Record<string, unknown>,
      });
    });

    await waitFor(() => {
      expect(mockSoundEngine.playOnce).toHaveBeenCalledWith(
        expect.objectContaining({ emphasize: true }),
      );
    });
  });

  it("first user gesture unlocks the audio engine once", async () => {
    renderProvider();

    await waitFor(() => {
      expect(screen.getByTestId("bills-count")).toHaveTextContent("1");
    });

    mockSoundEngine.unlock.mockClear();

    act(() => {
      fireEvent.pointerDown(window);
    });

    await waitFor(() => {
      expect(mockSoundEngine.unlock).toHaveBeenCalledTimes(1);
    });

    act(() => {
      fireEvent.pointerDown(window);
      fireEvent.keyDown(window);
    });

    expect(mockSoundEngine.unlock).toHaveBeenCalledTimes(1);
  });

  it("chat.announcement plays sound when leader and announcementSound is on (default)", async () => {
    renderProvider();

    await waitFor(() => {
      expect(screen.getByTestId("bills-count")).toHaveTextContent("1");
    });

    act(() => {
      globalThis.__lastAlertSSEOptions?.onEvent({
        type: "chat.announcement",
        data: { channel_id: 5 },
      });
    });

    await waitFor(() => {
      expect(mockSoundEngine.playOnce).toHaveBeenCalledWith(
        expect.objectContaining({ alertTypes: [] }),
      );
    });
  });

  it("chat.message does not play sound by default (messageSound off)", async () => {
    renderProvider();

    await waitFor(() => {
      expect(screen.getByTestId("bills-count")).toHaveTextContent("1");
    });

    act(() => {
      globalThis.__lastAlertSSEOptions?.onEvent({
        type: "chat.message",
        data: { channel_id: 5 },
      });
    });

    expect(mockSoundEngine.playOnce).not.toHaveBeenCalled();
  });

  it("chat.message plays sound when messageSound opt-in is on", async () => {
    mockGetChatSoundPrefs.mockReturnValue({
      announcementSound: true,
      messageSound: true,
    });

    renderProvider();

    await waitFor(() => {
      expect(screen.getByTestId("bills-count")).toHaveTextContent("1");
    });

    act(() => {
      globalThis.__lastAlertSSEOptions?.onEvent({
        type: "chat.message",
        data: { channel_id: 5 },
      });
    });

    await waitFor(() => {
      expect(mockSoundEngine.playOnce).toHaveBeenCalledWith(
        expect.objectContaining({ alertTypes: [] }),
      );
    });
  });

  it("non-leader tab does not play sound for chat.announcement", async () => {
    mockLeadershipRef.current = false;
    renderProvider();

    await waitFor(() => {
      expect(screen.getByTestId("bills-count")).toHaveTextContent("1");
    });

    act(() => {
      globalThis.__lastAlertSSEOptions?.onEvent({
        type: "chat.announcement",
        data: { channel_id: 5 },
      });
    });

    expect(mockSoundEngine.playOnce).not.toHaveBeenCalled();
  });

  it("chat events never fall through to alert state patching", async () => {
    renderProvider();

    await waitFor(() => {
      expect(screen.getByTestId("bills-count")).toHaveTextContent("1");
    });

    act(() => {
      globalThis.__lastAlertSSEOptions?.onEvent({
        type: "chat.announcement",
        data: { channel_id: 5 },
      });
    });

    // Bills count unaffected — chat events must never be treated as alerts.
    expect(screen.getByTestId("bills-count")).toHaveTextContent("1");
    expect(mockShowInfo).not.toHaveBeenCalled();
    expect(mockShowWarning).not.toHaveBeenCalled();
  });

  it("does not toast alert.created for a type disabled in notification preferences", async () => {
    mockFetchOperationalAlertSettings.mockResolvedValue({
      ...settings,
      event_settings: {
        ...settings.event_settings,
        payment_received: {
          ...settings.event_settings.payment_received,
          enabled: false,
        },
      },
    });

    renderProvider();

    await waitFor(() => {
      expect(screen.getByTestId("settings-loaded")).toHaveTextContent("yes");
    });

    act(() => {
      globalThis.__lastAlertSSEOptions?.onEvent({
        type: "alert.created",
        data: makeAlert({
          id: 2,
          alert_type: "payment_received",
          resource_type: "payment",
          resource_id: 2,
          title: "Payment received",
          metadata: { bill_number: "9" },
        }) as unknown as Record<string, unknown>,
      });
    });

    expect(mockShowInfo).not.toHaveBeenCalled();
    expect(mockShowWarning).not.toHaveBeenCalled();
    // The alert still lands in state (badges/counts are data, not pings).
    expect(screen.getByTestId("bills-count")).toHaveTextContent("1");
  });

  it("does not toast any alert.created when alerts are globally disabled", async () => {
    mockFetchOperationalAlertSettings.mockResolvedValue({
      ...settings,
      enabled: false,
    });

    renderProvider();

    await waitFor(() => {
      expect(screen.getByTestId("settings-loaded")).toHaveTextContent("yes");
    });

    act(() => {
      globalThis.__lastAlertSSEOptions?.onEvent({
        type: "alert.created",
        data: makeAlert({
          id: 2,
          alert_type: "payment_received",
          resource_type: "payment",
          resource_id: 2,
        }) as unknown as Record<string, unknown>,
      });
    });

    expect(mockShowInfo).not.toHaveBeenCalled();
    expect(mockShowWarning).not.toHaveBeenCalled();
  });

  it("invalidates the recent-alerts feed on alert.* SSE frames", async () => {
    const { client } = renderProvider();

    await waitFor(() => {
      expect(screen.getByTestId("bills-count")).toHaveTextContent("1");
    });

    const invalidateSpy = jest.spyOn(client, "invalidateQueries");

    act(() => {
      globalThis.__lastAlertSSEOptions?.onEvent({
        type: "alert.resolved",
        data: makeAlert({
          status: "resolved",
          resolved_at: "2026-06-04T12:01:00Z",
        }) as unknown as Record<string, unknown>,
      });
    });

    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: ["alerts", "42", "recent"] }),
    );
  });

  it("409 claim conflict shows an 'already claimed' toast and refetches instead of throwing", async () => {
    mockClaimOperationalAlert.mockRejectedValue(
      new OperationalAlertConflictError("Sam", "claimed"),
    );

    const { client } = renderProvider();

    await waitFor(() => {
      expect(screen.getByTestId("bills-count")).toHaveTextContent("1");
    });

    const invalidateSpy = jest.spyOn(client, "invalidateQueries");

    await act(async () => {
      screen.getByRole("button", { name: "claim order" }).click();
    });

    expect(mockShowInfo).toHaveBeenCalledWith("Already claimed by Sam");
    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        queryKey: expect.arrayContaining(["alerts", "42", "active"]),
      }),
    );
  });

  it("takes over repeating alarms when this tab acquires sound leadership", async () => {
    // Start as a non-leader tab with an open repeating alert (order_new).
    mockLeadershipRef.current = false;

    renderProvider();

    await waitFor(() => {
      expect(screen.getByTestId("settings-loaded")).toHaveTextContent("yes");
      expect(screen.getByTestId("bills-count")).toHaveTextContent("1");
    });

    expect(mockSoundEngine.startRepeating).not.toHaveBeenCalled();
    expect(mockOnAcquire.current).toBeDefined();

    // The leader tab closes; the browser hands this tab the Web Lock.
    await act(async () => {
      mockLeadershipRef.current = true;
      mockOnAcquire.current?.();
    });

    await waitFor(() => {
      expect(mockSoundEngine.startRepeating).toHaveBeenCalledWith(
        expect.objectContaining({ alertTypes: ["order_new"] }),
      );
    });
  });
});
