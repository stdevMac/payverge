/** @jest-environment jsdom */
import { render, screen, fireEvent, waitFor } from "@testing-library/react";

import NotificationPreferencesTab from "./NotificationPreferencesTab";

// ---------------------------------------------------------------------------
// Mocks — mirrors NotificationPreferencesTab.test.tsx plumbing so fetches
// resolve without polluting the toast/error call counts.
// ---------------------------------------------------------------------------

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
    sound_enabled: true,
    volume: 0.5,
    repeat_interval_seconds: 8,
    event_settings: {},
  }),
  updateOperationalAlertSettings: jest.fn(),
}));

// Use the REAL getTranslation (English message JSON) rather than echoing
// keys — the preview button's aria-label is built via string interpolation
// ({{type}} -> per-event-type label), and the real strings are what let this
// test distinguish "preview order_new sound" from the other six per-type
// buttons. Only `useSimpleLocale` is mocked, since it needs a context
// provider this test doesn't set up.
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  ...jest.requireActual("@/i18n/SimpleTranslationProvider"),
  useSimpleLocale: () => ({ locale: "en" }),
}));

const mockShowSuccess = jest.fn();
const mockShowError = jest.fn();
jest.mock("@/contexts/ToastContext", () => ({
  useToast: () => ({ showSuccess: mockShowSuccess, showError: mockShowError }),
}));

const mockPlayOnce = jest.fn().mockResolvedValue(undefined);
jest.mock("@/utils/operationalAlertSound", () => ({
  ...jest.requireActual("@/utils/operationalAlertSound"),
  createOperationalAlertSoundEngine: () => ({
    // Deferred through a wrapper so the reference to `mockPlayOnce` isn't
    // read until a preview button is actually clicked — the real component
    // creates its sound engine eagerly at module scope, which would
    // otherwise read this before the `const mockPlayOnce = ...` below runs.
    playOnce: (...args: Parameters<typeof mockPlayOnce>) =>
      mockPlayOnce(...args),
    unlock: jest
      .fn()
      .mockResolvedValue({ unlocked: true, blocked: false, repeating: false }),
    startRepeating: jest.fn(),
    stopRepeating: jest.fn(),
    isRepeating: () => false,
    getState: () => ({ unlocked: true, blocked: false, repeating: false }),
  }),
}));

beforeEach(() => {
  jest.clearAllMocks();
  mockGet.mockResolvedValue({
    data: {
      address: "0xabc",
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

describe("operational alert per-type sound preview", () => {
  it("plays the live alert sound for that type when the preview button is clicked", async () => {
    render(<NotificationPreferencesTab businessId={42} />);

    const button = await screen.findByRole("button", {
      name: /preview.*new orders.*sound/i,
    });
    fireEvent.click(button);

    await waitFor(() =>
      expect(mockPlayOnce).toHaveBeenCalledWith(
        expect.objectContaining({ alertTypes: ["order_new"] }),
      ),
    );
  });
});
