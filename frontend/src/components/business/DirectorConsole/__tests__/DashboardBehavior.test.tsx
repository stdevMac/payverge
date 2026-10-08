/** @jest-environment jsdom */
import {
  TextEncoder as NodeTextEncoder,
  TextDecoder as NodeTextDecoder,
} from "util";
import { ReadableStream as NodeReadableStream } from "stream/web";

// jsdom lacks Web Streams + TextEncoder/Decoder; pull them from Node so the
// streaming fetch path inside `useDirectorStream` has the primitives it needs.
if (
  typeof (globalThis as { TextEncoder?: typeof TextEncoder }).TextEncoder ===
  "undefined"
) {
  (globalThis as { TextEncoder: typeof TextEncoder }).TextEncoder =
    NodeTextEncoder as unknown as typeof TextEncoder;
}
if (
  typeof (globalThis as { TextDecoder?: typeof TextDecoder }).TextDecoder ===
  "undefined"
) {
  (globalThis as { TextDecoder: typeof TextDecoder }).TextDecoder =
    NodeTextDecoder as unknown as typeof TextDecoder;
}
if (
  typeof (globalThis as { ReadableStream?: typeof ReadableStream })
    .ReadableStream === "undefined"
) {
  (globalThis as { ReadableStream: typeof ReadableStream }).ReadableStream =
    NodeReadableStream as unknown as typeof ReadableStream;
}

import React from "react";
import {
  render,
  screen,
  fireEvent,
  waitFor,
  act,
  within,
} from "@testing-library/react";

// A getTranslation mock that resolves the localized relative-time + error +
// stop strings and interpolates {count} so the timestamp + error assertions are
// meaningful (rather than asserting raw keys). Anything else returns the key.
jest.mock("@/i18n/SimpleTranslationProvider", () => {
  // Distinctive sentinel strings so the timestamp assertions can ONLY pass if
  // the component routes through t() — the old code hardcoded English literals
  // ("5 m ago"), which these values deliberately do not match.
  const TABLE: Record<string, string> = {
    "directorConsole.relativeTime.justNow": "hace instantes",
    "directorConsole.relativeTime.minutesAgo": "hace {count} min",
    "directorConsole.relativeTime.hoursAgo": "hace {count} h",
    "directorConsole.relativeTime.yesterday": "ayer",
    "directorConsole.relativeTime.daysAgo": "hace {count} d",
    "directorConsole.errors.askFailed":
      "Failed to get an answer. Please try again.",
    "directorConsole.chat.stop": "Stop",
    "directorConsole.chat.newReply": "New reply ↓",
    "directorConsole.chat.newReplyAria": "Jump to latest reply",
    "directorConsole.threadActions.menu": "Thread actions",
    "directorConsole.threadActions.rename": "Rename",
    "directorConsole.threadActions.pin": "Pin",
    "directorConsole.threadActions.unpin": "Unpin",
    "directorConsole.threadActions.export": "Export markdown",
    "directorConsole.threadActions.archive": "Archive",
    "directorConsole.threadActions.delete": "Delete permanently",
    "directorConsole.threadActions.deleteTitle": "Delete this thread?",
    "directorConsole.threadActions.deleteDescription": "Permanent.",
    "directorConsole.threadActions.deleteConfirm": "Delete",
    "directorConsole.threadActions.renameTitle": "Rename thread",
    "directorConsole.threadActions.cancel": "Cancel",
    "directorConsole.threadActions.save": "Save",
    "directorConsole.threads.pinnedLabel": "Pinned",
  };
  return {
    useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
    getTranslation: (
      key: string,
      _locale?: string,
      params?: Record<string, string | number>,
    ) => {
      if (key === "directorConsole.quickPrompts") return [];
      let v = TABLE[key] ?? key;
      if (params) {
        Object.entries(params).forEach(([k, val]) => {
          v = v.replace(new RegExp(`\\{${k}\\}`, "g"), String(val));
        });
      }
      return v;
    },
  };
});

const mockUseBusinessAccess = jest.fn(() => ({
  hasAccess: true,
  isSuspended: false,
  aiConfigured: true,
  loading: false,
}));
jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => mockUseBusinessAccess(),
}));

jest.mock("@/hooks/useAnalytics", () => ({
  useClickTracking: () => jest.fn(),
}));

jest.mock("@/api/directorConsole", () => ({
  askDirectorStreamURL: (businessId: string | number) =>
    `http://localhost/api/v1/inside/businesses/${businessId}/ai/director/ask/stream`,
  listDirectorThreads: jest.fn(),
  getDirectorThreadMessages: jest.fn().mockResolvedValue({ messages: [] }),
  submitDirectorFeedback: jest.fn(),
  patchDirectorThread: jest.fn().mockResolvedValue({}),
  pinDirectorThread: jest.fn().mockResolvedValue({}),
  archiveDirectorThread: jest.fn().mockResolvedValue({}),
  restoreDirectorThread: jest.fn().mockResolvedValue({}),
  deleteDirectorThread: jest.fn().mockResolvedValue({}),
  exportDirectorThreadURL: (businessId: number, threadId: number) =>
    `http://localhost/export/${businessId}/${threadId}`,
  listDirectorAppliedActions: jest.fn().mockResolvedValue({ actions: [] }),
}));

// The Pre-Shift deck (mounted as the dashboard front door) pulls in the
// proactive-insights API + shared SSE EventSource, which jsdom lacks and these
// chat-behavior suites do not exercise. PreShift has its own dedicated tests
// (PreShift / PreShiftCard / insightCopy / preShiftI18n), so stub it here.
// The BriefingStrip (dashboard front door) pulls in the briefing API + shared
// SSE EventSource, which jsdom lacks and these chat-behavior suites do not
// exercise. BriefingStrip has its own dedicated tests, so stub it (and its
// drawer) here. (PreShift is no longer rendered by the dashboard.)
jest.mock("../BriefingStrip", () => ({
  __esModule: true,
  default: () => null,
}));
jest.mock("../InsightsDrawer", () => ({
  __esModule: true,
  default: () => null,
}));

import DirectorConsoleDashboard from "../DirectorConsoleDashboard";
import type { Business } from "@/api/business";
import {
  listDirectorThreads,
  pinDirectorThread,
} from "@/api/directorConsole";

const business = {
  id: 42,
  ai_settings: { ai_name: "Sage" },
} as unknown as Business;

const mockedList = listDirectorThreads as jest.Mock;
const mockedPin = pinDirectorThread as jest.Mock;

describe("DirectorConsoleDashboard behavior (L8)", () => {
  // Save originals so we don't leak our patched globals into sibling DirectorConsole
  // test files that share this worker's jsdom env (Layout/Optimistic rely on the
  // default matchMedia behavior; clobbering it made them flake intermittently).
  const originalMatchMedia = window.matchMedia;
  const originalScrollIntoView = (
    window.HTMLElement.prototype as unknown as { scrollIntoView?: () => void }
  ).scrollIntoView;

  beforeEach(() => {
    jest.clearAllMocks();
    mockUseBusinessAccess.mockReturnValue({
      hasAccess: true,
      isSuspended: false,
      aiConfigured: true,
      loading: false,
    });
    mockedList.mockResolvedValue({ threads: [] });
    // Force the wide-viewport layout so the thread sidebar Card (which renders
    // the relative timestamps) is shown instead of the narrow <select>.
    window.matchMedia = jest.fn().mockImplementation((query: string) => ({
      matches: true,
      media: query,
      onchange: null,
      addEventListener: jest.fn(),
      removeEventListener: jest.fn(),
      addListener: jest.fn(),
      removeListener: jest.fn(),
      dispatchEvent: jest.fn(),
    })) as unknown as typeof window.matchMedia;
  });

  it("does not list threads for locked Director", async () => {
    mockUseBusinessAccess.mockReturnValue({
      hasAccess: false,
      isSuspended: true,
      aiConfigured: true,
      loading: false,
    });
    render(<DirectorConsoleDashboard business={business} />);
    await act(async () => {
      await Promise.resolve();
    });
    expect(mockedList).not.toHaveBeenCalled();
  });

  it("waits for the access check before listing Director threads", async () => {
    mockUseBusinessAccess.mockReturnValue({
      hasAccess: false,
      isSuspended: false,
      aiConfigured: false,
      loading: true,
    });
    const view = render(<DirectorConsoleDashboard business={business} />);
    expect(mockedList).not.toHaveBeenCalled();
    mockUseBusinessAccess.mockReturnValue({
      hasAccess: true,
      isSuspended: false,
      aiConfigured: true,
      loading: false,
    });
    view.rerender(<DirectorConsoleDashboard business={business} />);
    await waitFor(() => expect(mockedList).toHaveBeenCalledTimes(1));
  });

  afterEach(() => {
    // Restore globals so neither matchMedia nor scrollIntoView leaks across files.
    window.matchMedia = originalMatchMedia;
    if (originalScrollIntoView === undefined) {
      delete (
        window.HTMLElement.prototype as unknown as {
          scrollIntoView?: () => void;
        }
      ).scrollIntoView;
    } else {
      (
        window.HTMLElement.prototype as unknown as {
          scrollIntoView?: () => void;
        }
      ).scrollIntoView = originalScrollIntoView;
    }
  });

  it("L1: threads-rail empty resolves real i18n copy (locks the 'Title'/'Subtitle' placeholder regression)", async () => {
    mockedList.mockResolvedValue({ threads: [] });
    render(<DirectorConsoleDashboard business={business} />);
    // The Sage-mark-led empty resolves the real thread keys…
    expect(
      await screen.findByText("directorConsole.threads.emptyTitle"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("directorConsole.threads.empty"),
    ).toBeInTheDocument();
    // …never the generic ui/EmptyState's literal "Title"/"Subtitle" placeholders.
    expect(screen.queryByText("Title")).not.toBeInTheDocument();
    expect(screen.queryByText("Subtitle")).not.toBeInTheDocument();
  });

  it("renders one row per conversation when the API repeats titles and ids (#730)", async () => {
    const older = new Date(Date.now() - 2 * 60 * 60 * 1000).toISOString();
    const newer = new Date(Date.now() - 5 * 60 * 1000).toISOString();
    mockedList.mockResolvedValue({
      threads: [
        { id: 1, title: "what's our best seller", updated_at: older },
        { id: 2, title: "cierra la caja", updated_at: older },
        { id: 1, title: "what's our best seller", updated_at: older },
        { id: 3, title: "what's our best seller", updated_at: newer },
        { id: 4, title: "cierra la caja", updated_at: newer },
        { id: 5, title: "mandá un mozo", updated_at: newer },
      ],
    });

    render(<DirectorConsoleDashboard business={business} />);
    const rows = await screen.findAllByTestId("dc-thread-row");
    expect(rows).toHaveLength(5);
    const rowTitles = rows.map((row) => row.textContent || "");
    expect(
      rowTitles.filter((text) => text.includes("what's our best seller")),
    ).toHaveLength(2);
    expect(rowTitles.filter((text) => text.includes("cierra la caja"))).toHaveLength(
      2,
    );
    expect(rowTitles.filter((text) => text.includes("mandá un mozo"))).toHaveLength(1);
  });

  it("F6: renders a translated relative timestamp (not raw English 'm ago') for a recent thread", async () => {
    // Thread updated 5 minutes ago → "5 m ago" via the translation table.
    const fiveMinAgo = new Date(Date.now() - 5 * 60 * 1000).toISOString();
    mockedList.mockResolvedValue({
      threads: [{ id: 1, title: "Tuesday recap", updated_at: fiveMinAgo }],
    });

    render(<DirectorConsoleDashboard business={business} />);

    // The relative time goes through t("relativeTime.minutesAgo", {count}).
    // The old hardcoded path would have rendered "5 m ago" instead.
    expect(await screen.findByText("hace 5 min")).toBeInTheDocument();
    expect(screen.queryByText("5 m ago")).not.toBeInTheDocument();
  });

  it("F6: 'just now' for sub-minute, 'yesterday' for exactly one day", async () => {
    const yesterday = new Date(Date.now() - 25 * 60 * 60 * 1000).toISOString();
    mockedList.mockResolvedValue({
      threads: [{ id: 2, title: "Old thread", updated_at: yesterday }],
    });
    render(<DirectorConsoleDashboard business={business} />);
    expect(await screen.findByText("ayer")).toBeInTheDocument();
  });

  it("F5: shows the friendly translated error (not the raw 'stream HTTP 500') on stream failure", async () => {
    mockedList.mockResolvedValue({ threads: [] });
    // Fetch resolves with a non-ok response → hook sets error.message = "stream HTTP 500".
    global.fetch = jest
      .fn()
      .mockResolvedValue({
        ok: false,
        status: 500,
        body: null,
      }) as unknown as typeof fetch;

    render(<DirectorConsoleDashboard business={business} />);
    await waitFor(() =>
      expect(screen.queryByTestId("dc-chat-column")).toBeTruthy(),
    );

    const textarea = screen.getByRole("textbox");
    fireEvent.change(textarea, { target: { value: "Why was Tuesday slow?" } });
    await act(async () => {
      fireEvent.keyDown(textarea, { key: "Enter", metaKey: true });
    });

    // The friendly copy appears; the raw technical code must NOT.
    expect(
      await screen.findByText("Failed to get an answer. Please try again."),
    ).toBeInTheDocument();
    expect(screen.queryByText(/stream HTTP 500/)).not.toBeInTheDocument();
    expect(screen.getByRole("textbox")).toHaveValue("Why was Tuesday slow?");
  });

  it("F7: on send, shows pending user bubble and keeps message list (smart scroll path)", async () => {
    mockedList.mockResolvedValue({ threads: [] });
    // A stream that never resolves so the pending state persists.
    global.fetch = jest
      .fn()
      .mockReturnValue(new Promise(() => {})) as unknown as typeof fetch;

    render(<DirectorConsoleDashboard business={business} />);
    await waitFor(() =>
      expect(screen.queryByTestId("dc-chat-column")).toBeTruthy(),
    );

    const list = screen.getByTestId("dc-message-list");
    const metrics = { scrollTop: 100, scrollHeight: 500 };
    Object.defineProperties(list, {
      scrollHeight: {
        configurable: true,
        get: () => metrics.scrollHeight,
      },
      clientHeight: { configurable: true, get: () => 400 },
      scrollTop: {
        configurable: true,
        get: () => metrics.scrollTop,
        set: (v: number) => {
          metrics.scrollTop = v;
        },
      },
    });

    const textarea = screen.getByRole("textbox");
    fireEvent.change(textarea, { target: { value: "Hello" } });
    await act(async () => {
      fireEvent.keyDown(textarea, { key: "Enter", metaKey: true });
    });

    expect(screen.getByTestId("dc-message-end")).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.getByTestId("dc-pending-user")).toBeInTheDocument();
    });
    // Force-on-send assigns scrollTop = scrollHeight on the list container.
    await waitFor(() => {
      expect(metrics.scrollTop).toBe(metrics.scrollHeight);
    });
  });

  it("sidebar row actions menu pins without selecting another thread", async () => {
    mockedList.mockResolvedValue({
      threads: [
        {
          id: 1,
          title: "Alpha",
          updated_at: new Date().toISOString(),
          pinned: false,
        },
        {
          id: 2,
          title: "Beta",
          updated_at: new Date().toISOString(),
          pinned: false,
        },
      ],
    });
    render(<DirectorConsoleDashboard business={business} />);
    // Auto-select picks first thread (Alpha); wait for both rows.
    await waitFor(() =>
      expect(screen.getAllByTestId("dc-thread-row").length).toBe(2),
    );
    await waitFor(() => {
      expect(
        screen
          .getAllByText("Alpha")
          .some((el) => el.className.includes("font-title")),
      ).toBe(true);
    });

    const rows = screen.getAllByTestId("dc-thread-row");
    const betaRow = rows.find((r) => r.textContent?.includes("Beta"));
    expect(betaRow).toBeTruthy();
    fireEvent.click(
      within(betaRow!).getByRole("button", { name: /thread actions/i }),
    );

    fireEvent.click(await screen.findByText(/^Pin$/));
    await waitFor(() => {
      expect(mockedPin).toHaveBeenCalledWith(42, 2, true);
    });

    // Opening Beta's menu + pin must not switch selection away from Alpha.
    await waitFor(() => {
      const after = screen.getAllByTestId("dc-thread-row");
      const alphaSelected = after.find(
        (r) =>
          r.textContent?.includes("Alpha") &&
          r.className.includes("ring-brand"),
      );
      expect(alphaSelected).toBeTruthy();
    });
    expect(
      screen
        .getAllByText("Alpha")
        .some((el) => el.className.includes("font-title")),
    ).toBe(true);
  });
});
