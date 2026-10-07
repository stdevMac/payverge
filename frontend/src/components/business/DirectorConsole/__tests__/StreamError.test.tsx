/** @jest-environment jsdom */
import { TextEncoder as NodeTextEncoder, TextDecoder as NodeTextDecoder } from "util";
import { ReadableStream as NodeReadableStream } from "stream/web";
if (typeof (globalThis as any).TextEncoder === "undefined") (globalThis as any).TextEncoder = NodeTextEncoder;
if (typeof (globalThis as any).TextDecoder === "undefined") (globalThis as any).TextDecoder = NodeTextDecoder;
if (typeof (globalThis as any).ReadableStream === "undefined") (globalThis as any).ReadableStream = NodeReadableStream;

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => { if (key === "directorConsole.quickPrompts") return []; return key; },
}));
jest.mock("@/hooks/useBusinessAccess", () => ({ useBusinessAccess: () => ({ hasAccess: true, isSuspended: false, aiConfigured: true, loading: false }) }));
jest.mock("@/hooks/useAnalytics", () => ({ useClickTracking: () => jest.fn() }));
jest.mock("@/api/directorConsole", () => ({
  askDirectorStreamURL: (id: string | number) => `http://localhost/api/v1/inside/businesses/${id}/ai/director/ask/stream`,
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
const business = { id: 4, ai_settings: { ai_name: "Sage" } } as unknown as Business;

describe("Director Console stream error UX", () => {
  const originalFetch = global.fetch;
  afterEach(() => { global.fetch = originalFetch; jest.restoreAllMocks(); });

  it("clears the placeholder, shows an error banner, and restores the composer on stream error", async () => {
    const enc = new TextEncoder();
    const body = new ReadableStream<Uint8Array>({
      start(c) {
        c.enqueue(enc.encode('event: error\ndata: {"message":"upstream exploded"}\n\n'));
        c.close();
      },
    });
    global.fetch = jest.fn().mockResolvedValue({ ok: true, status: 200, body } as unknown as Response) as unknown as typeof fetch;

    render(<DirectorConsoleDashboard business={business} />);
    await waitFor(() => expect(screen.queryByTestId("dc-chat-column")).toBeTruthy());

    fireEvent.change(screen.getByRole("textbox"), { target: { value: "why slow tuesday" } });
    fireEvent.keyDown(screen.getByRole("textbox"), { key: "Enter", metaKey: true });

    await waitFor(() => expect(screen.queryByTestId("dc-assistant-placeholder")).not.toBeInTheDocument());
    await waitFor(() => expect((screen.getByRole("textbox") as HTMLTextAreaElement).value).toBe("why slow tuesday"));
  });
});
