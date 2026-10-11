/** @jest-environment jsdom */

import React from "react";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import TelegramConfig from "../TelegramConfig";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    get: jest.fn(),
    post: jest.fn(),
  },
}));

jest.mock("react-hot-toast", () => ({
  success: jest.fn(),
  error: jest.fn(),
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => key.split(".").pop() || key,
}));

jest.mock("next/image", () => ({
  __esModule: true,
  default: (props: Record<string, unknown>) => {
    // eslint-disable-next-line @next/next/no-img-element, jsx-a11y/alt-text
    return <img {...(props as any)} />;
  },
}));

const mockAxios = axiosInstance as jest.Mocked<typeof axiosInstance>;

const plugin = {
  id: 1,
  name: "telegram",
  display_name: "Telegram",
  description: "Telegram notifications",
  image: "/images/plugins/telegram.png",
  category: "integration",
  version: "1.0.0",
  features: "",
  is_enabled: true,
  config: "{}",
};

function setupDomAPIs() {
  Object.defineProperty(window, "open", {
    writable: true,
    value: jest.fn(),
  });
  Object.defineProperty(window, "ResizeObserver", {
    writable: true,
    value: class ResizeObserver {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  });
}

function renderTelegramConfig(
  overrides: Partial<React.ComponentProps<typeof TelegramConfig>> = {},
) {
  return render(
    <TelegramConfig
      businessId="42"
      plugin={plugin}
      config={{ business_name: "Payverge Test" }}
      onConfigChange={jest.fn()}
      onSave={jest.fn()}
      onCancel={jest.fn()}
      {...overrides}
    />,
  );
}

function mockGenerateToken(expiresAt = new Date(Date.now() + 15 * 60_000)) {
  mockAxios.post.mockImplementation(async (url: string) => {
    if (url.includes("/generate-token")) {
      return {
        data: {
          url: "https://t.me/PayvergeBot?start=pv_tg_test",
          expires_at: expiresAt.toISOString(),
        },
      };
    }
    return { data: {} };
  });
}

describe("TelegramConfig lifecycle", () => {
  beforeEach(() => {
    jest.useRealTimers();
    jest.clearAllMocks();
    setupDomAPIs();
  });

  it("pending state starts polling after opening Telegram", async () => {
    jest.useFakeTimers();
    const user = userEvent.setup({ advanceTimers: jest.advanceTimersByTime });
    mockAxios.get.mockResolvedValue({
      data: {
        connected: false,
        plugin_enabled: true,
        health: "pending",
      },
    });
    mockGenerateToken();

    renderTelegramConfig();

    await screen.findByText("connectToTelegramBot");
    await user.click(screen.getByText("connectToTelegramBot"));

    await act(async () => {
      jest.advanceTimersByTime(3000);
    });

    expect(mockAxios.get).toHaveBeenCalledTimes(2);
  });

  it("connected state stops polling after Telegram confirms the chat", async () => {
    jest.useFakeTimers();
    const user = userEvent.setup({ advanceTimers: jest.advanceTimersByTime });
    mockAxios.get
      .mockResolvedValueOnce({
        data: {
          connected: false,
          plugin_enabled: true,
          health: "pending",
        },
      })
      .mockResolvedValueOnce({
        data: {
          connected: true,
          plugin_enabled: true,
          health: "connected",
          chat_id: "123456789",
        },
      });
    mockGenerateToken();

    renderTelegramConfig();

    await screen.findByText("connectToTelegramBot");
    await user.click(screen.getByText("connectToTelegramBot"));

    await act(async () => {
      jest.advanceTimersByTime(3000);
    });
    await waitFor(() => expect(mockAxios.get).toHaveBeenCalledTimes(2));

    await act(async () => {
      jest.advanceTimersByTime(6000);
    });
    expect(mockAxios.get).toHaveBeenCalledTimes(2);
  });

  it("expired token state shows regenerate action", async () => {
    const expiredAt = new Date(Date.now() - 60_000);
    mockAxios.get.mockResolvedValue({
      data: {
        connected: false,
        plugin_enabled: true,
        health: "pending",
        pending_token_expires_at: expiredAt.toISOString(),
      },
    });
    mockGenerateToken(expiredAt);

    renderTelegramConfig();

    // regenerateLink paints as soon as generate-token sets connectionUrl;
    // the "expired" countdown is written in a follow-up effect on
    // connectionExpiresAt. Assert asynchronously so CI shard load cannot
    // race the sync getByText (flake, not a product regression).
    await screen.findByText("regenerateLink");
    await waitFor(() => {
      expect(
        screen.getByText((text) => text.includes("expired")),
      ).toBeInTheDocument();
    });
  });

  it("degraded state shows last error details", async () => {
    mockAxios.get.mockResolvedValue({
      data: {
        connected: true,
        plugin_enabled: true,
        health: "degraded",
        chat_id: "123456789",
        last_error: "Forbidden: bot was blocked",
        last_error_at: "2026-05-11T12:00:00Z",
      },
    });
    mockGenerateToken();

    renderTelegramConfig();

    expect(await screen.findByText("telegramDegraded")).toBeInTheDocument();
    expect(screen.getByText("Forbidden: bot was blocked")).toBeInTheDocument();
  });

  it("renders lifecycle timestamps in the business timezone, not device time", async () => {
    // 2026-05-11T12:00:00Z is 09:00 in Buenos Aires (UTC-3).
    mockAxios.get.mockResolvedValue({
      data: {
        connected: true,
        plugin_enabled: true,
        health: "connected",
        chat_id: "123456789",
        connected_at: "2026-05-11T12:00:00Z",
      },
    });
    mockGenerateToken();

    renderTelegramConfig({
      businessTimezone: "America/Argentina/Buenos_Aires",
    });

    const connectedAtRow = await screen.findByText(
      (text) => text.includes("connectedAt") && text.includes("9:00"),
    );
    // Business time (UTC-3 wall clock) present; the raw UTC 12:00 must be absent.
    expect(connectedAtRow).toBeInTheDocument();
    expect(connectedAtRow.textContent).not.toContain("12:00");
  });

  it("workforce alerts render off by default and persist into config.notifications", async () => {
    const user = userEvent.setup();
    const onConfigChange = jest.fn();
    mockAxios.get.mockResolvedValue({
      data: { connected: false, plugin_enabled: true, health: "not_connected" },
    });
    mockGenerateToken();

    renderTelegramConfig({ onConfigChange });

    // All three workforce toggles are surfaced.
    await screen.findByText("schedulePublished");
    expect(screen.getByText("shiftReminders")).toBeInTheDocument();
    expect(screen.getByText("coverageDecisions")).toBeInTheDocument();

    onConfigChange.mockClear();
    // Toggling "schedule published" writes it under the primary notifications map
    // (the namespace the backend gate reads), not notification_settings.
    const scheduleRow = screen.getByText("schedulePublished").closest("div")!
      .parentElement!;
    await user.click(scheduleRow.querySelector('[role="switch"]') as Element);

    await waitFor(() => {
      const last = onConfigChange.mock.calls.at(-1)?.[0];
      expect(last?.notifications?.schedule_published).toBe(true);
      expect(last?.notifications?.shift_reminder).toBe(false);
      expect(last?.notifications?.coverage_decided).toBe(false);
    });
  });

  it("hydrates workforce alerts from existing config.notifications", async () => {
    const onConfigChange = jest.fn();
    mockAxios.get.mockResolvedValue({
      data: { connected: false, plugin_enabled: true, health: "not_connected" },
    });
    mockGenerateToken();

    renderTelegramConfig({
      onConfigChange,
      config: {
        business_name: "Payverge Test",
        notifications: { shift_reminder: true },
      },
    });

    await waitFor(() => {
      const last = onConfigChange.mock.calls.at(-1)?.[0];
      expect(last?.notifications?.shift_reminder).toBe(true);
      expect(last?.notifications?.schedule_published).toBe(false);
    });
  });

  it("disconnect action calls the backend and refreshes status", async () => {
    const user = userEvent.setup();
    mockAxios.get
      .mockResolvedValueOnce({
        data: {
          connected: true,
          plugin_enabled: true,
          health: "connected",
          chat_id: "123456789",
        },
      })
      .mockResolvedValueOnce({
        data: {
          connected: false,
          plugin_enabled: true,
          health: "not_connected",
        },
      });
    mockAxios.post.mockResolvedValue({ data: {} });

    renderTelegramConfig();

    await screen.findByText("disconnect");
    await user.click(screen.getAllByText("disconnect")[0]);
    await screen.findByText("telegramDisconnectConfirm");
    const disconnectButtons = screen.getAllByRole("button", {
      name: "disconnect",
    });
    await user.click(disconnectButtons[disconnectButtons.length - 1]);

    await waitFor(() =>
      expect(mockAxios.post).toHaveBeenCalledWith(
        "/inside/businesses/42/plugins/telegram/disconnect",
      ),
    );
    expect(mockAxios.get).toHaveBeenCalledTimes(2);
  });
});
