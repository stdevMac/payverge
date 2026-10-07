/** @jest-environment jsdom */
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";

import NotificationPreferencesTab from "./NotificationPreferencesTab";
import { getChatSoundPrefs } from "@/utils/chatSoundPrefs";

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

// axiosInstance backs the email-notification section (GET load + PUT save).
const mockGet = jest.fn();
const mockPut = jest.fn();
jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    get: (...args: unknown[]) => mockGet(...args),
    put: (...args: unknown[]) => mockPut(...args),
  },
}));

// pluginAPI backs the Telegram section.
const mockGetBusinessPlugins = jest.fn();
const mockGetPluginConfig = jest.fn();
const mockUpdatePluginConfig = jest.fn();
jest.mock("@/api/plugins", () => ({
  pluginAPI: {
    business: {
      getBusinessPlugins: (...a: unknown[]) => mockGetBusinessPlugins(...a),
      getPluginConfig: (...a: unknown[]) => mockGetPluginConfig(...a),
      updatePluginConfig: (...a: unknown[]) => mockUpdatePluginConfig(...a),
    },
  },
}));

// operationalAlerts: resolve the operational section's load with a valid (if
// minimal) settings object so it doesn't fire its own error toast and pollute
// the showError call count. normalizeOperationalSettings fills event_settings.
jest.mock("@/api/operationalAlerts", () => ({
  fetchOperationalAlertSettings: jest.fn().mockResolvedValue({
    enabled: true,
    browser_notifications_enabled: false,
    sound_enabled: true,
    volume: 0.5,
    repeat_interval_seconds: 8,
    event_settings: {},
  }),
  updateOperationalAlertSettings: jest.fn(),
}));

// Echo translation keys so assertions stay key-based.
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => key,
}));

// Capture toast calls.
const mockShowSuccess = jest.fn();
const mockShowError = jest.fn();
jest.mock("@/contexts/ToastContext", () => ({
  useToast: () => ({ showSuccess: mockShowSuccess, showError: mockShowError }),
}));

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

interface EmailPrefs {
  transactional_enabled: boolean;
  reports_enabled: boolean;
  news_enabled: boolean;
  updates_enabled: boolean;
  security_enabled: boolean;
}

const baseEmailPrefs: EmailPrefs = {
  transactional_enabled: true,
  reports_enabled: false,
  news_enabled: false,
  updates_enabled: true,
  security_enabled: true,
};

const switchFor = (label: string): HTMLInputElement => {
  const el = screen.getByLabelText(label);
  // NextUI Switch renders a visually-hidden <input type="checkbox">.
  const input = el.closest("label")?.querySelector('input[type="checkbox"]');
  return (input ?? el) as HTMLInputElement;
};

beforeEach(() => {
  jest.clearAllMocks();
  localStorage.clear();
  mockGet.mockResolvedValue({
    data: { address: "0xabc", preferences: { ...baseEmailPrefs } },
  });
});

// ---------------------------------------------------------------------------
// Email toggle — concurrent-revert correctness (F9)
// ---------------------------------------------------------------------------

describe("handleEmailToggle — concurrent revert correctness (F9)", () => {
  // NOTE on literal concurrency: each Switch is `isDisabled={emailSaving}`,
  // so while one save is in flight the other switches are inert — a true
  // simultaneous double-click is not reachable through the rendered UI. The
  // F9 stale-closure clobber is therefore exercised here as a *sequenced*
  // scenario: field B is toggled successfully first (advancing state), then a
  // later toggle of field A fails. The old code reverted the entire prefs
  // object to a render-time snapshot, which — across re-renders — risked
  // resurrecting a pre-B value for B. The ref-based fix reverts ONLY A.
  it("preserves a previously-flipped field when a later toggle's save fails", async () => {
    // First PUT (orders) succeeds; second PUT (reports) fails.
    mockPut
      .mockResolvedValueOnce({}) // orders toggle
      .mockRejectedValueOnce(new Error("reports save failed")); // reports toggle

    render(<NotificationPreferencesTab businessId={42} />);

    const orders = await screen.findByLabelText(
      "businessSettings.notifications.ordersAndPayments",
    );
    const reportsInput = switchFor(
      "businessSettings.notifications.reportsAndAnalytics",
    );
    const ordersInput = switchFor(
      "businessSettings.notifications.ordersAndPayments",
    );

    expect(reportsInput.checked).toBe(false);
    expect(ordersInput.checked).toBe(true);

    // Flip B (orders → off) and let it persist successfully.
    await act(async () => {
      fireEvent.click(orders);
    });
    await waitFor(() => expect(ordersInput.checked).toBe(false));

    // Now flip A (reports → on); its save fails.
    const reports = screen.getByLabelText(
      "businessSettings.notifications.reportsAndAnalytics",
    );
    await act(async () => {
      fireEvent.click(reports);
      await Promise.resolve();
    });

    // A reverts ...
    await waitFor(() => expect(reportsInput.checked).toBe(false));
    // ... and the critical assertion: B's earlier flip survives A's failure.
    expect(ordersInput.checked).toBe(false);
    expect(mockShowError).toHaveBeenCalledTimes(1);

    // The failed reverting PUT must not have re-clobbered orders, and the
    // stored-but-unread flags stay at their server values.
    const lastBody = mockPut.mock.calls[mockPut.mock.calls.length - 1][1] as {
      preferences: EmailPrefs;
    };
    expect(lastBody.preferences.transactional_enabled).toBe(false);
    expect(lastBody.preferences.reports_enabled).toBe(true);
    expect(lastBody.preferences.news_enabled).toBe(false);
    expect(lastBody.preferences.updates_enabled).toBe(true);
    expect(lastBody.preferences.security_enabled).toBe(true);
  });

  it("sends the freshly-toggled value to the API (not a stale snapshot)", async () => {
    mockPut.mockResolvedValue({});
    render(<NotificationPreferencesTab businessId={42} />);

    const reports = await screen.findByLabelText(
      "businessSettings.notifications.reportsAndAnalytics",
    );
    await act(async () => {
      fireEvent.click(reports);
    });

    await waitFor(() => expect(mockPut).toHaveBeenCalled());
    const body = mockPut.mock.calls[0][1] as {
      preferences: EmailPrefs;
    };
    expect(body.preferences.reports_enabled).toBe(true);
  });

  it("reverts only the failed field and shows an error toast on a single failure", async () => {
    mockPut.mockRejectedValue(new Error("boom"));
    render(<NotificationPreferencesTab businessId={42} />);

    const reports = await screen.findByLabelText(
      "businessSettings.notifications.reportsAndAnalytics",
    );
    const reportsInput = switchFor(
      "businessSettings.notifications.reportsAndAnalytics",
    );

    await act(async () => {
      fireEvent.click(reports);
      await Promise.resolve();
    });

    await waitFor(() => expect(reportsInput.checked).toBe(false));
    // The other visible switch stays at its loaded value.
    expect(
      switchFor("businessSettings.notifications.ordersAndPayments").checked,
    ).toBe(true);
    expect(mockShowError).toHaveBeenCalledTimes(1);

    const body = mockPut.mock.calls[0][1] as { preferences: EmailPrefs };
    expect(body.preferences.news_enabled).toBe(false);
    expect(body.preferences.updates_enabled).toBe(true);
    expect(body.preferences.security_enabled).toBe(true);
    expect(body.preferences.transactional_enabled).toBe(true);
  });

  it("does not offer news, updates or security email toggles", async () => {
    render(<NotificationPreferencesTab businessId={42} />);

    await screen.findByLabelText(
      "businessSettings.notifications.ordersAndPayments",
    );
    expect(
      screen.queryByLabelText("businessSettings.notifications.platformNews"),
    ).toBeNull();
    expect(
      screen.queryByLabelText(
        "businessSettings.notifications.platformUpdates",
      ),
    ).toBeNull();
    expect(
      screen.queryByLabelText("businessSettings.notifications.securityAlerts"),
    ).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// Telegram toggle — concurrent-revert correctness (F9)
// ---------------------------------------------------------------------------

describe("handleTelegramToggle — concurrent revert correctness (F9)", () => {
  beforeEach(() => {
    mockGetBusinessPlugins.mockResolvedValue({
      plugins: [
        {
          plugin_id: 7,
          is_enabled: true,
          plugin: { name: "Telegram" },
        },
      ],
    });
    mockGetPluginConfig.mockResolvedValue({
      config: {
        notifications: {
          order_created: true,
          payment_received: true,
          reservation_created: true,
          reservation_status_changed: false,
          inventory_low_stock: false,
        },
      },
    });
  });

  // Same serialization caveat as the email section: `isDisabled={telegramSaving}`
  // blocks a literal simultaneous double-click, so F9 is exercised sequenced.
  it("preserves a previously-flipped field when a later toggle's save fails", async () => {
    mockUpdatePluginConfig
      .mockResolvedValueOnce({}) // inventory_low_stock (B) — succeeds
      .mockRejectedValueOnce(new Error("save A failed")); // reservation_status_changed (A) — fails

    render(<NotificationPreferencesTab businessId={42} telegramConnected />);

    // Wait for the telegram config to load and toggles to render.
    const lowStock = await screen.findByLabelText(
      "businessSettings.notifications.inventoryLowStock",
    );
    const resvInput = switchFor(
      "businessSettings.notifications.reservationStatusChanged",
    );
    const lowStockInput = switchFor(
      "businessSettings.notifications.inventoryLowStock",
    );

    expect(resvInput.checked).toBe(false);
    expect(lowStockInput.checked).toBe(false);

    // Flip B (inventory_low_stock → on); let it persist successfully.
    await act(async () => {
      fireEvent.click(lowStock);
    });
    await waitFor(() => expect(lowStockInput.checked).toBe(true));

    // Flip A (reservation_status_changed → on); its save fails.
    const resvChanged = screen.getByLabelText(
      "businessSettings.notifications.reservationStatusChanged",
    );
    await act(async () => {
      fireEvent.click(resvChanged);
      await Promise.resolve();
    });

    // A reverts ...
    await waitFor(() => expect(resvInput.checked).toBe(false));
    // ... and B survives A's failure.
    expect(lowStockInput.checked).toBe(true);
    expect(mockShowError).toHaveBeenCalledTimes(1);

    // The failed toggle's payload still carried B's up-to-date value.
    const lastPayload = mockUpdatePluginConfig.mock.calls[
      mockUpdatePluginConfig.mock.calls.length - 1
    ][2] as { config: { notifications: Record<string, boolean> } };
    expect(lastPayload.config.notifications.inventory_low_stock).toBe(true);
    expect(lastPayload.config.notifications.reservation_status_changed).toBe(
      true,
    );
  });

  it("sends the freshly-toggled value to the plugin config (not a stale snapshot)", async () => {
    mockUpdatePluginConfig.mockResolvedValue({});
    render(<NotificationPreferencesTab businessId={42} telegramConnected />);

    const lowStock = await screen.findByLabelText(
      "businessSettings.notifications.inventoryLowStock",
    );
    await act(async () => {
      fireEvent.click(lowStock);
    });

    await waitFor(() => expect(mockUpdatePluginConfig).toHaveBeenCalled());
    const [, pluginId, payload] = mockUpdatePluginConfig.mock.calls[0] as [
      string,
      number,
      { config: { notifications: Record<string, boolean> } },
    ];
    expect(pluginId).toBe(7);
    expect(payload.config.notifications.inventory_low_stock).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// Chat sounds (this device) — client-side localStorage opt-in toggles
// ---------------------------------------------------------------------------

describe("operational alert dinner-safe defaults (#267)", () => {
  it("clamps a legacy 8s API interval to 30s and never offers Every 8s", async () => {
    render(<NotificationPreferencesTab businessId={42} />);

    const value = await screen.findByTestId("notif-repeat-interval-value");
    expect(value).toHaveAttribute("data-seconds", "30");
    expect(value).not.toHaveAttribute("data-seconds", "8");
    expect(
      screen.getByTestId("notif-repeat-interval-floor-hint"),
    ).toBeInTheDocument();
    expect(
      screen.getByTestId("notif-sound-opt-in-hint"),
    ).toBeInTheDocument();
  });
});

describe("operational alert type rows", () => {
  it("renders a configurable row for payment_refund_review", async () => {
    render(<NotificationPreferencesTab businessId={42} />);

    // Translations are key-echoed, so the per-type row label is the i18n key.
    await waitFor(() => {
      expect(
        screen.getByText(
          "businessSettings.notifications.operationalTypes.payment_refund_review",
        ),
      ).toBeInTheDocument();
    });
  });
});

describe("chat sound prefs (per-device)", () => {
  it("renders announcement sound on and message sound off by default", async () => {
    render(<NotificationPreferencesTab businessId={42} />);

    const announcementInput = await waitFor(() =>
      switchFor("businessSettings.notifications.chatAnnouncementSound"),
    );
    const messageInput = switchFor(
      "businessSettings.notifications.chatMessageSound",
    );

    expect(announcementInput.checked).toBe(true);
    expect(messageInput.checked).toBe(false);
  });

  it("flipping the message sound toggle persists to localStorage", async () => {
    render(<NotificationPreferencesTab businessId={42} />);

    const messageToggle = await screen.findByLabelText(
      "businessSettings.notifications.chatMessageSound",
    );

    await act(async () => {
      fireEvent.click(messageToggle);
    });

    await waitFor(() => {
      expect(getChatSoundPrefs().messageSound).toBe(true);
    });
    expect(getChatSoundPrefs().announcementSound).toBe(true);
  });

  it("flipping the announcement sound toggle off persists to localStorage", async () => {
    render(<NotificationPreferencesTab businessId={42} />);

    const announcementToggle = await screen.findByLabelText(
      "businessSettings.notifications.chatAnnouncementSound",
    );

    await act(async () => {
      fireEvent.click(announcementToggle);
    });

    await waitFor(() => {
      expect(getChatSoundPrefs().announcementSound).toBe(false);
    });
  });
});
