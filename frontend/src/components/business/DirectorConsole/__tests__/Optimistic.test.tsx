/** @jest-environment jsdom */
import { TextEncoder as NodeTextEncoder, TextDecoder as NodeTextDecoder } from "util";
import { ReadableStream as NodeReadableStream } from "stream/web";

// jsdom lacks Web Streams + TextEncoder/Decoder; pull them from Node so the
// streaming fetch path inside `useDirectorStream` has the primitives it needs.
if (typeof (globalThis as { TextEncoder?: typeof TextEncoder }).TextEncoder === "undefined") {
  (globalThis as { TextEncoder: typeof TextEncoder }).TextEncoder =
    NodeTextEncoder as unknown as typeof TextEncoder;
}
if (typeof (globalThis as { TextDecoder?: typeof TextDecoder }).TextDecoder === "undefined") {
  (globalThis as { TextDecoder: typeof TextDecoder }).TextDecoder =
    NodeTextDecoder as unknown as typeof TextDecoder;
}
if (typeof (globalThis as { ReadableStream?: typeof ReadableStream }).ReadableStream === "undefined") {
  (globalThis as { ReadableStream: typeof ReadableStream }).ReadableStream =
    NodeReadableStream as unknown as typeof ReadableStream;
}

import React from "react";
import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => {
    if (key === "directorConsole.quickPrompts") return [];
    return key;
  },
}));

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => ({ hasAccess: true, isSuspended: false, aiConfigured: true, loading: false }),
}));

jest.mock("@/hooks/useAnalytics", () => ({
  useClickTracking: () => jest.fn(),
}));

jest.mock("@/api/directorConsole", () => ({
  askDirectorStreamURL: (businessId: string | number) =>
    `http://localhost/api/v1/inside/businesses/${businessId}/ai/director/ask/stream`,
  listDirectorThreads: jest.fn().mockResolvedValue({ threads: [] }),
  getDirectorThreadMessages: jest.fn().mockResolvedValue({ messages: [] }),
  submitDirectorFeedback: jest.fn(),
}));

// The Pre-Shift deck (mounted as the dashboard front door) pulls in the
// proactive-insights API + shared SSE EventSource, which jsdom lacks and these
// chat-behavior suites do not exercise. PreShift has its own dedicated tests
// (PreShift / PreShiftCard / insightCopy / preShiftI18n), so stub it here.
// The BriefingStrip (dashboard front door) pulls in the briefing API + shared
// SSE EventSource, which jsdom lacks and these chat-behavior suites do not
// exercise. BriefingStrip has its own dedicated tests, so stub it (and its
// drawer) here. (PreShift is no longer rendered by the dashboard.)
jest.mock("../BriefingStrip", () => ({ __esModule: true, default: () => null }));
jest.mock("../InsightsDrawer", () => ({ __esModule: true, default: () => null }));

import DirectorConsoleDashboard from "../DirectorConsoleDashboard";
import type { Business } from "@/api/business";

const business = { id: 42, ai_settings: { ai_name: "Sage" } } as unknown as Business;

// Build a duck-typed Response wrapping a ReadableStream whose chunks the test
// can push imperatively. `pull` is used (rather than `start`) so the consumer
// can request chunks on demand without us racing the controller capture.
function makeControlledStreamResponse(): {
  response: Response;
  push: (chunk: string) => void;
  close: () => void;
} {
  const encoder = new TextEncoder();
  const pending: string[] = [];
  let pullResolver: (() => void) | null = null;
  let closed = false;

  const wake = () => {
    const r = pullResolver;
    pullResolver = null;
    r?.();
  };

  const body = new ReadableStream<Uint8Array>({
    async pull(ctrl) {
      if (pending.length) {
        ctrl.enqueue(encoder.encode(pending.shift()!));
        return;
      }
      if (closed) {
        ctrl.close();
        return;
      }
      // Park until the test pushes or closes.
      await new Promise<void>((resolve) => {
        pullResolver = resolve;
      });
      if (pending.length) {
        ctrl.enqueue(encoder.encode(pending.shift()!));
      } else if (closed) {
        ctrl.close();
      }
    },
  });

  const push = (chunk: string) => {
    if (closed) return;
    pending.push(chunk);
    wake();
  };
  const close = () => {
    if (closed) return;
    closed = true;
    wake();
  };

  const response = { ok: true, status: 200, body } as unknown as Response;
  return { response, push, close };
}

describe("Director Console — optimistic render", () => {
  const originalFetch = global.fetch;

  afterEach(() => {
    global.fetch = originalFetch;
    jest.restoreAllMocks();
  });

  it("renders the user bubble + assistant placeholder before the stream resolves", async () => {
    const { response, push, close } = makeControlledStreamResponse();
    global.fetch = jest.fn().mockResolvedValue(response) as unknown as typeof fetch;

    render(<DirectorConsoleDashboard business={business} />);
    await waitFor(() => expect(screen.queryByTestId("dc-chat-column")).toBeTruthy());

    const textarea = screen.getByRole("textbox");
    await act(async () => {
      fireEvent.change(textarea, { target: { value: "Why was Tuesday slow?" } });
      fireEvent.keyDown(textarea, { key: "Enter", metaKey: true });
    });

    // Optimistic user bubble + assistant placeholder appear immediately
    // after keyDown — the synchronous tail of handleSendMessage fires
    // `setPending` before it awaits the stream.
    expect(screen.getByTestId("dc-pending-user")).toBeInTheDocument();
    expect(screen.getByTestId("dc-assistant-placeholder")).toBeInTheDocument();
    expect(
      screen.getByTestId("dc-pending-user").textContent,
    ).toContain("Why was Tuesday slow?");

    // Close the stream so the hook's read loop exits and Jest doesn't hang
    // on the dangling ReadableStream after the test returns.
    await act(async () => {
      close();
    });
  });

  it("removes the placeholder when the user aborts", async () => {
    const { response, close } = makeControlledStreamResponse();
    global.fetch = jest.fn().mockImplementation((_url: string, init?: RequestInit) => {
      // Honor the AbortController so the hook's catch path fires
      if (init?.signal) {
        init.signal.addEventListener("abort", () => close());
      }
      return Promise.resolve(response);
    }) as unknown as typeof fetch;

    render(<DirectorConsoleDashboard business={business} />);
    await waitFor(() => expect(screen.queryByTestId("dc-chat-column")).toBeTruthy());

    await act(async () => {
      fireEvent.change(screen.getByRole("textbox"), { target: { value: "hi" } });
      fireEvent.keyDown(screen.getByRole("textbox"), { key: "Enter", metaKey: true });
    });
    expect(screen.getByTestId("dc-assistant-placeholder")).toBeInTheDocument();

    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: /stop/i }));
    });
    await waitFor(() =>
      expect(screen.queryByTestId("dc-assistant-placeholder")).not.toBeInTheDocument(),
    );
    expect(screen.getByRole("textbox")).toHaveValue("hi");
  });

  it("ignores stream events that arrive after abort", async () => {
    const { listDirectorThreads, getDirectorThreadMessages } =
      jest.requireMock("@/api/directorConsole") as {
        listDirectorThreads: jest.Mock;
        getDirectorThreadMessages: jest.Mock;
      };

    const { response, push, close } = makeControlledStreamResponse();
    const fetchMock = jest.fn().mockImplementation((_url: string, init?: RequestInit) => {
      if (init?.signal) {
        init.signal.addEventListener("abort", () => close());
      }
      return Promise.resolve(response);
    });
    global.fetch = fetchMock as unknown as typeof fetch;

    render(<DirectorConsoleDashboard business={business} />);
    await waitFor(() => expect(screen.queryByTestId("dc-chat-column")).toBeTruthy());

    await act(async () => {
      fireEvent.change(screen.getByRole("textbox"), { target: { value: "hi" } });
      fireEvent.keyDown(screen.getByRole("textbox"), { key: "Enter", metaKey: true });
    });
    expect(screen.getByTestId("dc-assistant-placeholder")).toBeInTheDocument();

    // Baseline: thread loader fires once on mount. We assert no *additional*
    // calls land after abort.
    const threadsCallsBefore = listDirectorThreads.mock.calls.length;
    const messagesCallsBefore = getDirectorThreadMessages.mock.calls.length;

    // Abort before the stream emits response.complete
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: /stop/i }));
    });
    await waitFor(() =>
      expect(screen.queryByTestId("dc-assistant-placeholder")).not.toBeInTheDocument(),
    );

    // Try to push a stale response.complete after the abort — should be a no-op
    // because the underlying stream is already closed and the hook's `aborted`
    // flag has already cleared the placeholder. `push` itself is now a no-op
    // (the stream is closed), but we exercise the call for parity with the
    // previous "stale resolve" behavior.
    await act(async () => {
      push(
        'event: response.complete\ndata: {"message_id":1,"thread_id":99,"latency_ms":10,"model":"test"}\n\n',
      );
    });

    // Placeholder is still gone
    expect(screen.queryByTestId("dc-assistant-placeholder")).not.toBeInTheDocument();
    // The orphaned thread must not have been selected → no follow-up reloads.
    expect(listDirectorThreads.mock.calls.length).toBe(threadsCallsBefore);
    expect(getDirectorThreadMessages.mock.calls.length).toBe(messagesCallsBefore);
    // And fetch itself only fired once for the single send.
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});
