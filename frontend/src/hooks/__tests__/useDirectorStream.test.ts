/** @jest-environment jsdom */
import { TextEncoder as NodeTextEncoder, TextDecoder as NodeTextDecoder } from "util";
import { ReadableStream as NodeReadableStream } from "stream/web";

// jsdom omits Web Streams + TextEncoder/Decoder; pull them in from Node so the
// fetch+ReadableStream pipeline under test has the primitives it needs. We
// avoid the global `Response` constructor (also missing in jsdom) by returning
// a minimal duck-typed response from the mocked fetch.
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

import { renderHook, act, waitFor } from "@testing-library/react";
import { useDirectorStream } from "../useDirectorStream";

// Mock the global fetch with a manually-controlled ReadableStream wrapped in
// a duck-typed Response (ok / status / body) — enough surface for the hook.
function makeStreamResponse(chunks: string[]) {
  const encoder = new TextEncoder();
  let i = 0;
  const body = new ReadableStream<Uint8Array>({
    pull(ctrl) {
      if (i >= chunks.length) {
        ctrl.close();
        return;
      }
      ctrl.enqueue(encoder.encode(chunks[i++]));
    },
  });
  return { ok: true, status: 200, body } as unknown as Response;
}

describe("useDirectorStream", () => {
  beforeEach(() => {
    (global as { fetch?: typeof fetch }).fetch = jest.fn();
  });

  it("parses SSE events and surfaces them via state", async () => {
    (global.fetch as jest.Mock).mockResolvedValue(
      makeStreamResponse([
        "event: tool.call.started\ndata: {\"name\":\"get_revenue_summary\",\"human_label\":\"Reading revenue\",\"args\":{}}\n\n",
        "event: tool.call.completed\ndata: {\"name\":\"get_revenue_summary\",\"summary\":\"$1,500 · 50 tx\",\"duration_ms\":412,\"success\":true}\n\n",
        "event: response.complete\ndata: {\"message_id\":1,\"latency_ms\":2000,\"model\":\"test\",\"thread_id\":7}\n\n",
      ]),
    );

    const { result } = renderHook(() => useDirectorStream());
    await act(async () => {
      await result.current.start({
        url: "/api/v1/inside/businesses/1/ai/director/ask/stream",
        body: { message: "why slow?", locale: "en" },
      });
    });

    await waitFor(() => expect(result.current.toolCalls.length).toBeGreaterThan(0));
    expect(result.current.toolCalls[0].name).toBe("get_revenue_summary");
    expect(result.current.complete).toEqual(
      expect.objectContaining({ message_id: 1, thread_id: 7 }),
    );
  });

  it("aborts in-flight stream via AbortController", async () => {
    (global.fetch as jest.Mock).mockImplementation(() => new Promise(() => {}));
    const { result } = renderHook(() => useDirectorStream());
    await act(async () => {
      result.current.start({ url: "/x", body: {} });
      result.current.abort();
    });
    expect(result.current.aborted).toBe(true);
  });
});
