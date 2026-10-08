/** @jest-environment jsdom */
import React from "react";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import QRCode from "qrcode";
import WhatsAppChannelStatus, {
  WHATSAPP_QR_MAX_FAILURES,
  WHATSAPP_QR_POLL_MS,
  formatWhatsAppError,
  formatWhatsAppStatusLabel,
} from "../WhatsAppChannelStatus";
import { axiosInstance } from "@/api/tools/instance";

// Instance gate is covered in components/instance featureGating.test.tsx;
// here the install is treated as WhatsApp-capable.
jest.mock("@/hooks/useInstance", () => ({
  useInstance: () => ({ isOff: () => false }),
}));

jest.mock("qrcode", () => ({
  __esModule: true,
  default: {
    toDataURL: jest.fn(async (text: string) => `data:image/png;base64,${text}`),
  },
}));

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    get: jest.fn(),
    post: jest.fn(),
  },
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  // Matches real getTranslation(key, locale, params) signature.
  getTranslation: (key: string, _locale?: string, vars?: Record<string, string | number>) => {
    const leaf = key.startsWith("aiWaiterDashboard.")
      ? key.slice("aiWaiterDashboard.".length)
      : key;
    const map: Record<string, string> = {
      "whatsapp.title": "WhatsApp channel",
      "whatsapp.subtitle": "Connection status",
      "whatsapp.status.connected": "Connected",
      "whatsapp.status.degraded": "Degraded — reconnecting",
      "whatsapp.status.disconnected": "Disconnected",
      "whatsapp.status.pairing": "Pairing",
      "whatsapp.lastConnected": `Last connected ${vars?.time || ""}`,
      "whatsapp.retryAt": `Next retry ${vars?.time || ""}`,
      "whatsapp.errors.device_missing": "Saved device is no longer available",
      "whatsapp.errors.fallback": "Channel needs attention",
      "whatsapp.connect": "Connect WhatsApp",
      "whatsapp.reconnect": "Reconnect",
      "whatsapp.disconnect": "Disconnect",
      "whatsapp.notEnabled": "WhatsApp is installed on this server but not turned on.",
      "whatsapp.actionFailed": "Could not update the WhatsApp channel. Try again.",
      "whatsapp.startFailed": "WhatsApp is turned on for this server but could not start.",
      "whatsapp.qrAlt": "WhatsApp pairing QR code",
      "whatsapp.scanHint": "Open WhatsApp, Linked devices, and scan this code.",
    };
    return map[leaf] || map[key] || key;
  },
}));

jest.mock("@nextui-org/react", () => {
  const ReactActual = require("react");
  return {
    Button: ({ children, onPress, isLoading }: any) =>
      ReactActual.createElement(
        "button",
        { type: "button", disabled: isLoading, onClick: onPress },
        children,
      ),
    Chip: ({ children }: any) => ReactActual.createElement("span", null, children),
    Spinner: () => ReactActual.createElement("div", { "data-testid": "spinner" }),
  };
});

const mockedAxios = axiosInstance as jest.Mocked<typeof axiosInstance>;

// Server built with `-tags whatsapp` and WHATSAPP_ENABLED=true.
const RUNNING = { built: true, enabled: true } as const;

describe("WhatsAppChannelStatus helpers", () => {
  it("maps lifecycle statuses and error codes to localized copy", () => {
    const t = (key: string) =>
      ({
        "whatsapp.status.degraded": "Degraded — reconnecting",
        "whatsapp.errors.device_missing": "Saved device is no longer available",
        "whatsapp.errors.fallback": "Channel needs attention",
      })[key] || key;
    expect(formatWhatsAppStatusLabel("degraded", t)).toBe("Degraded — reconnecting");
    expect(formatWhatsAppError("device_missing", t)).toBe("Saved device is no longer available");
    expect(formatWhatsAppError("unknown_code", t)).toBe("Channel needs attention");
  });
});

describe("WhatsAppChannelStatus", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("shows connected state with last-connected time", async () => {
    mockedAxios.get.mockResolvedValueOnce({
      data: {
        status: "connected",
        last_connected_at: "2026-07-18T12:00:00Z",
        ...RUNNING,
      },
    });
    render(<WhatsAppChannelStatus businessId={7} locale="en" />);
    expect(await screen.findByText("Connected")).toBeInTheDocument();
    // L4-3: locale-aware datetime, not raw ISO.
    expect(screen.getByText(/Last connected/)).toBeInTheDocument();
    expect(screen.queryByText(/2026-07-18T12:00:00Z/)).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Disconnect/i })).toBeInTheDocument();
  });

  it("shows degraded reason and reconnect action", async () => {
    mockedAxios.get.mockResolvedValueOnce({
      data: {
        status: "degraded",
        last_error_code: "device_missing",
        retry_at: "2026-07-18T13:00:00Z",
        ...RUNNING,
      },
    });
    render(<WhatsAppChannelStatus businessId={7} locale="en" />);
    expect(await screen.findByText("Degraded — reconnecting")).toBeInTheDocument();
    expect(screen.getByText("Saved device is no longer available")).toBeInTheDocument();
    expect(screen.getByText(/Next retry/)).toBeInTheDocument();
    expect(screen.queryByText(/2026-07-18T13:00:00Z/)).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Reconnect/i })).toBeInTheDocument();
  });

  it("offers connect when disconnected", async () => {
    mockedAxios.get.mockResolvedValueOnce({ data: { status: "disconnected", ...RUNNING } });
    render(<WhatsAppChannelStatus businessId={7} locale="en" />);
    expect(await screen.findByText("Disconnected")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Connect WhatsApp/i })).toBeInTheDocument();
  });

  it("requires a locale prop (no silent English default) and formats times for that locale (L4-3)", async () => {
    mockedAxios.get.mockResolvedValueOnce({
      data: {
        status: "connected",
        last_connected_at: "2026-08-04T15:30:00Z",
        ...RUNNING,
      },
    });
    render(<WhatsAppChannelStatus businessId={7} locale="es" />);
    expect(await screen.findByText("Connected")).toBeInTheDocument();
    // Must not leak the raw ISO wire value into the card.
    expect(screen.queryByText(/2026-08-04T15:30:00Z/)).not.toBeInTheDocument();
    const body = document.body.textContent || "";
    expect(body).toMatch(/Last connected/);
  });
});

// The default backend image compiles WhatsApp out (GPL-3.0 libsignal lives
// behind the `whatsapp` build tag); the card must not exist there.
describe("WhatsAppChannelStatus build gating", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  // Flush the status promise (and its finally) so React commits the answer.
  async function settle(): Promise<void> {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
  }

  async function renderAndSettle(): Promise<HTMLElement> {
    const { container } = render(<WhatsAppChannelStatus businessId={7} locale="en" />);
    await settle();
    expect(mockedAxios.get).toHaveBeenCalledWith("/inside/businesses/7/whatsapp/status");
    return container;
  }

  it("renders nothing on a default (untagged) server", async () => {
    mockedAxios.get.mockResolvedValueOnce({
      data: { status: "disconnected", enabled: false, built: false },
    });
    const container = await renderAndSettle();
    expect(container).toBeEmptyDOMElement();
    expect(screen.queryByTestId("whatsapp-channel-status")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Connect WhatsApp/i })).not.toBeInTheDocument();
  });

  it("renders nothing when the backend predates the built field", async () => {
    mockedAxios.get.mockResolvedValueOnce({ data: { status: "disconnected" } });
    const container = await renderAndSettle();
    expect(container).toBeEmptyDOMElement();
  });

  it("renders nothing while the first status request is in flight or after it fails", async () => {
    let reject: (err: Error) => void = () => {};
    mockedAxios.get.mockReturnValueOnce(
      new Promise((_, r) => {
        reject = r;
      }) as any,
    );
    const { container } = render(<WhatsAppChannelStatus businessId={7} locale="en" />);
    expect(container).toBeEmptyDOMElement();
    reject(new Error("network"));
    await settle();
    expect(mockedAxios.get).toHaveBeenCalledTimes(1);
    expect(container).toBeEmptyDOMElement();
  });

  it("explains a tagged build without WHATSAPP_ENABLED and offers no actions", async () => {
    mockedAxios.get.mockResolvedValueOnce({
      data: { status: "disconnected", enabled: false, built: true },
    });
    render(<WhatsAppChannelStatus businessId={7} locale="en" />);
    expect(await screen.findByTestId("whatsapp-not-enabled")).toHaveTextContent(
      "WhatsApp is installed on this server but not turned on.",
    );
    expect(screen.getByTestId("whatsapp-channel-status")).toBeInTheDocument();
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("surfaces a failed connect instead of an unhandled rejection", async () => {
    mockedAxios.get.mockResolvedValue({ data: { status: "disconnected", ...RUNNING } });
    mockedAxios.post.mockRejectedValueOnce(new Error("boom"));
    render(<WhatsAppChannelStatus businessId={7} locale="en" />);
    fireEvent.click(await screen.findByRole("button", { name: /Connect WhatsApp/i }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Could not update the WhatsApp channel. Try again.",
    );
    expect(mockedAxios.post).toHaveBeenCalledWith("/inside/businesses/7/whatsapp/connect");
    await waitFor(() => expect(mockedAxios.get).toHaveBeenCalledTimes(2));
  });
});

// The connect response and GET /whatsapp/qr carry the code the operator must
// scan; before this the card dropped it and pairing could not be finished.
describe("WhatsAppChannelStatus pairing", () => {
  const STATUS_URL = "/inside/businesses/7/whatsapp/status";
  const QR_URL = "/inside/businesses/7/whatsapp/qr";
  const toDataURL = (QRCode as unknown as { toDataURL: jest.Mock }).toDataURL;

  type Route = () => Promise<{ data: unknown }>;
  function routeGets(routes: Record<string, Route>) {
    mockedAxios.get.mockImplementation(((url: string) => {
      const route = routes[url];
      return route ? route() : Promise.reject(new Error(`unexpected GET ${url}`));
    }) as any);
  }

  // Fake timers from the start so the poll interval is created under them;
  // testing-library's findBy/waitFor advance them while they wait.
  beforeEach(() => {
    jest.useFakeTimers();
    jest.clearAllMocks();
    mockedAxios.get.mockReset();
    mockedAxios.post.mockReset();
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  async function nextPoll(times = 1): Promise<void> {
    await act(async () => {
      jest.advanceTimersByTime(WHATSAPP_QR_POLL_MS * times);
    });
  }

  it("shows the QR returned by connect, then the latest rotated code from /qr", async () => {
    let status = "disconnected";
    const qrCodes = ["code-B", "code-C"];
    routeGets({
      [STATUS_URL]: async () => ({ data: { status, ...RUNNING } }),
      [QR_URL]: async () => ({ data: { status: "pairing", qr_code: qrCodes[0] } }),
    });
    mockedAxios.post.mockImplementation((async () => {
      status = "pairing";
      return { data: { status: "scanning", qr_code: "code-A" } };
    }) as any);

    render(<WhatsAppChannelStatus businessId={7} locale="en" />);
    fireEvent.click(await screen.findByRole("button", { name: /Connect WhatsApp/i }));

    expect(mockedAxios.post).toHaveBeenCalledWith("/inside/businesses/7/whatsapp/connect");
    await waitFor(() => expect(toDataURL).toHaveBeenCalledWith("code-A", expect.anything()));
    const qr = await screen.findByTestId("whatsapp-qr");
    expect(qr).toHaveAttribute("alt", "WhatsApp pairing QR code");
    expect(screen.getByText("Open WhatsApp, Linked devices, and scan this code.")).toBeInTheDocument();
    await waitFor(() =>
      expect(screen.getByTestId("whatsapp-qr")).toHaveAttribute(
        "src",
        "data:image/png;base64,code-B",
      ),
    );

    // whatsmeow rotates the code; the card follows it on the next poll.
    qrCodes.shift();
    await nextPoll();
    await waitFor(() =>
      expect(screen.getByTestId("whatsapp-qr")).toHaveAttribute(
        "src",
        "data:image/png;base64,code-C",
      ),
    );
  });

  it("drops the QR and re-reads status once pairing ends", async () => {
    let status = "pairing";
    let qr: { status: string; qr_code?: string } = { status: "pairing", qr_code: "code-A" };
    routeGets({
      [STATUS_URL]: async () => ({ data: { status, ...RUNNING } }),
      [QR_URL]: async () => ({ data: qr }),
    });

    render(<WhatsAppChannelStatus businessId={7} locale="en" />);
    expect(await screen.findByTestId("whatsapp-qr")).toBeInTheDocument();

    // The phone scanned: /qr has no code and the device is connected.
    status = "connected";
    qr = { status: "connected" };
    await nextPoll();

    expect(await screen.findByText("Connected")).toBeInTheDocument();
    expect(screen.queryByTestId("whatsapp-qr")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Disconnect/i })).toBeInTheDocument();
  });

  // The shared axios interceptor rejects with a sanitized Error carrying the
  // HTTP status on both `status` and `response.status`.
  function httpError(status?: number): Error {
    return Object.assign(new Error(status ? `HTTP ${status}` : "Network Error"), {
      status,
      response: status ? { status } : undefined,
    });
  }

  // 401/403: this viewer may not read the code, or the business is suspended
  // (the route sits behind RequireOperationalBusiness); 404: not mounted.
  it.each([401, 403, 404])(
    "stops polling /qr at once on a definitive %i",
    async (refusal) => {
      routeGets({
        [STATUS_URL]: async () => ({ data: { status: "pairing", ...RUNNING } }),
        [QR_URL]: async () => Promise.reject(httpError(refusal)),
      });

      render(<WhatsAppChannelStatus businessId={7} locale="en" />);
      expect(await screen.findByText("Pairing")).toBeInTheDocument();
      // Background poll: the card handles failures itself, no global toast.
      await waitFor(() =>
        expect(mockedAxios.get).toHaveBeenCalledWith(QR_URL, { _skipErrorToast: true }),
      );

      await nextPoll(3);

      expect(mockedAxios.get.mock.calls.filter(([url]) => url === QR_URL)).toHaveLength(1);
      expect(screen.queryByTestId("whatsapp-qr")).not.toBeInTheDocument();
      expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    },
  );

  it("gives up after repeated transient /qr failures, says so and re-reads status", async () => {
    let statusCalls = 0;
    let qrCalls = 0;
    routeGets({
      [STATUS_URL]: async () => {
        statusCalls += 1;
        return { data: { status: "pairing", ...RUNNING } };
      },
      [QR_URL]: async () => {
        qrCalls += 1;
        if (qrCalls === 1) return { data: { status: "pairing", qr_code: "code-A" } };
        throw httpError(qrCalls % 2 === 0 ? 503 : undefined);
      },
    });

    render(<WhatsAppChannelStatus businessId={7} locale="en" />);
    expect(await screen.findByTestId("whatsapp-qr")).toBeInTheDocument();
    expect(statusCalls).toBe(1);

    // One short of the cap: still polling, last code still on screen.
    await nextPoll(WHATSAPP_QR_MAX_FAILURES - 1);
    expect(qrCalls).toBe(WHATSAPP_QR_MAX_FAILURES);
    expect(screen.getByTestId("whatsapp-qr")).toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();

    await nextPoll();
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Could not update the WhatsApp channel. Try again.",
    );
    expect(screen.queryByTestId("whatsapp-qr")).not.toBeInTheDocument();
    await waitFor(() => expect(statusCalls).toBe(2));

    await nextPoll(3);
    expect(qrCalls).toBe(WHATSAPP_QR_MAX_FAILURES + 1);
  });

  it("keeps the code and retries /qr after a transient failure", async () => {
    let qrCalls = 0;
    routeGets({
      [STATUS_URL]: async () => ({ data: { status: "pairing", ...RUNNING } }),
      [QR_URL]: async () => {
        qrCalls += 1;
        if (qrCalls === 2) throw httpError();
        if (qrCalls === 3) throw httpError(502);
        return { data: { status: "pairing", qr_code: qrCalls === 1 ? "code-A" : "code-B" } };
      },
    });

    render(<WhatsAppChannelStatus businessId={7} locale="en" />);
    await waitFor(() =>
      expect(screen.getByTestId("whatsapp-qr")).toHaveAttribute(
        "src",
        "data:image/png;base64,code-A",
      ),
    );

    // A network blip and a 502 leave the last code on screen.
    await nextPoll();
    await nextPoll();
    expect(qrCalls).toBe(3);
    expect(screen.getByTestId("whatsapp-qr")).toHaveAttribute(
      "src",
      "data:image/png;base64,code-A",
    );

    // ...and the next poll picks up the rotated code.
    await nextPoll();
    await waitFor(() =>
      expect(screen.getByTestId("whatsapp-qr")).toHaveAttribute(
        "src",
        "data:image/png;base64,code-B",
      ),
    );
  });

  it("says the channel failed to start when WHATSAPP_ENABLED is already set", async () => {
    mockedAxios.get.mockResolvedValueOnce({
      data: { status: "disconnected", built: true, enabled: false, requested: true },
    });
    render(<WhatsAppChannelStatus businessId={7} locale="en" />);
    expect(await screen.findByTestId("whatsapp-start-failed")).toHaveTextContent(
      "WhatsApp is turned on for this server but could not start.",
    );
    expect(screen.queryByTestId("whatsapp-not-enabled")).not.toBeInTheDocument();
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });
});
