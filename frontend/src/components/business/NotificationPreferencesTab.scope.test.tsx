/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import NotificationPreferencesTab from "./NotificationPreferencesTab";

const mockGet = jest.fn();
const mockPut = jest.fn();
jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    get: (...args: unknown[]) => mockGet(...args),
    put: (...args: unknown[]) => mockPut(...args),
  },
}));

jest.mock("@/api/plugins", () => ({
  pluginAPI: {
    business: {
      getBusinessPlugins: jest.fn().mockResolvedValue({ plugins: [] }),
      getPluginConfig: jest.fn(),
      updatePluginConfig: jest.fn(),
    },
  },
}));

jest.mock("@/api/operationalAlerts", () => ({
  fetchOperationalAlertSettings: jest.fn().mockResolvedValue({
    enabled: true,
    browser_notifications_enabled: false,
    sound_enabled: false,
    volume: 0.5,
    repeat_interval_seconds: 30,
    event_settings: {},
  }),
  updateOperationalAlertSettings: jest.fn(),
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => key,
}));

// The real useToast returns stable callbacks. A fresh jest.fn() per render
// re-runs the settings effects every render, so they never finish loading.
jest.mock("@/contexts/ToastContext", () => {
  const toast = { showSuccess: jest.fn(), showError: jest.fn() };
  return { useToast: () => toast };
});

jest.mock("@/utils/chatSoundPrefs", () => ({
  getChatSoundPrefs: () => ({ announcementSound: true, messageSound: false }),
  saveChatSoundPrefs: jest.fn(),
}));

jest.mock("@/utils/operationalAlertSound", () => ({
  createOperationalAlertSoundEngine: () => ({ playOnce: jest.fn() }),
}));

describe("NotificationPreferencesTab storage scopes (L6-40)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockGet.mockResolvedValue({
      data: {
        address: "user@example.com",
        preferences: {
          transactional_enabled: true,
          reports_enabled: false,
          news_enabled: false,
          updates_enabled: true,
          security_enabled: true,
        },
      },
    });
  });

  it("labels account, device, business, and browser storage scopes (#268)", async () => {
    render(<NotificationPreferencesTab businessId={42} />);

    await waitFor(() => {
      expect(screen.getByTestId("notif-scope-account")).toBeInTheDocument();
    });
    expect(screen.getByTestId("notif-scope-legend")).toBeInTheDocument();
    expect(screen.getByTestId("notif-save-model-hint")).toBeInTheDocument();
    expect(screen.getByTestId("notif-scope-device")).toBeInTheDocument();
    // The browser row belongs to the operational settings section, which
    // loads independently of the account email preferences above.
    expect(await screen.findByTestId("notif-scope-browser")).toBeInTheDocument();
    expect(screen.getAllByTestId("notif-scope-business").length).toBeGreaterThanOrEqual(1);
    expect(screen.getByTestId("notif-scope-badge-account")).toBeInTheDocument();
    expect(screen.getByTestId("notif-scope-badge-browser")).toBeInTheDocument();

    expect(document.querySelector('[data-storage-scope="account"]')).toBeTruthy();
    expect(document.querySelector('[data-storage-scope="device"]')).toBeTruthy();
    expect(document.querySelector('[data-storage-scope="business"]')).toBeTruthy();
    expect(document.querySelector('[data-storage-scope="browser"]')).toBeTruthy();
  });
});
