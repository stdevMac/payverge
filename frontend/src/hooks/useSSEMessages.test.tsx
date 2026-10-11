/** @jest-environment jsdom */
import { act, renderHook, waitFor } from "@testing-library/react";
import { ReadableStream } from "stream/web";
import { TextEncoder } from "util";
import { useSSEMessages } from "@/hooks/useSSEMessages";

type MockStream = {
  response: Response;
  emit: (chunk: string) => void;
  emitBytes: (chunk: Uint8Array) => void;
  close: () => void;
  fail: (error?: Error) => void;
  cancelled: jest.Mock;
};

const encoder = new TextEncoder();

function mockStream(): MockStream {
  let controller!: ReadableStreamDefaultController<Uint8Array>;
  let closed = false;
  const cancelled = jest.fn(() => {
    closed = true;
  });
  const body = new ReadableStream<Uint8Array>({
    start(next) {
      controller = next;
    },
    cancel: cancelled,
  });
  return {
    response: { ok: true, status: 200, body } as unknown as Response,
    emit(chunk) {
      if (!closed) controller.enqueue(encoder.encode(chunk));
    },
    emitBytes(chunk) {
      if (!closed) controller.enqueue(chunk);
    },
    close() {
      if (!closed) {
        closed = true;
        controller.close();
      }
    },
    fail(error = new Error("stream failed")) {
      if (!closed) {
        closed = true;
        controller.error(error);
      }
    },
    cancelled,
  };
}

const flush = async () => {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
};

describe("useSSEMessages", () => {
  const originalFetch = global.fetch;
  let fetchMock: jest.MockedFunction<typeof fetch>;

  beforeEach(() => {
    jest.useFakeTimers();
    fetchMock = jest.fn();
    global.fetch = fetchMock;
  });

  afterEach(() => {
    jest.useRealTimers();
    global.fetch = originalFetch;
  });

  it("uses only the in-memory bearer header and omits cookies, redirects, caches, and URL tokens", async () => {
    const stream = mockStream();
    fetchMock.mockResolvedValueOnce(stream.response);
    renderHook(() =>
      useSSEMessages({
        businessId: 5,
        sessionToken: "token-A",
        enabled: true,
        onMessage: jest.fn(),
        onFallback: jest.fn(),
      }),
    );

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    const [url, init] = fetchMock.mock.calls[0];
    expect(String(url)).toContain("/ai-waiter/5/stream");
    expect(String(url)).not.toContain("token-A");
    expect(String(url)).not.toContain("session_token");
    expect(init).toMatchObject({
      credentials: "omit",
      redirect: "error",
      cache: "no-store",
      headers: {
        Accept: "text/event-stream",
        Authorization: "Bearer token-A",
      },
    });
    expect(init?.signal).toBeInstanceOf(AbortSignal);
  });

  it("incrementally parses CRLF, split chunks, comments, and multiline message.created data", async () => {
    const stream = mockStream();
    fetchMock.mockResolvedValueOnce(stream.response);
    const onMessage = jest.fn();
    renderHook(() =>
      useSSEMessages({
        businessId: 5,
        sessionToken: "token-A",
        enabled: true,
        onMessage,
        onFallback: jest.fn(),
      }),
    );
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));

    act(() => {
      stream.emit(": ping\r\nevent: connec");
      stream.emit("ted\r\ndata: {}\r\n\r\nevent: ignored\r\ndata: {}\r\n\r\n");
      stream.emit("event: message.created\r\ndata: {not-json}\r\n\r\n");
      stream.emit('event: message.created\r\ndata: {"id":42,\r\n');
      const tail = encoder.encode(
        'data: "role":"assistant","content":"ready ☕","createdAt":1717000000,"response_v2":{"version":2,"response_id":"stream-42"}}\r\n\r\n',
      );
      const split = tail.findIndex((byte) => byte === 0xe2) + 1;
      stream.emitBytes(tail.slice(0, split));
      stream.emitBytes(tail.slice(split));
    });
    await flush();

    expect(onMessage).toHaveBeenCalledTimes(1);
    expect(onMessage).toHaveBeenCalledWith({
      id: 42,
      role: "assistant",
      content: "ready ☕",
      createdAt: 1717000000,
      response_v2: { version: 2, response_id: "stream-42" },
    });
  });

  it("aborts token A, reconnects with token B, and ignores queued token-A chunks", async () => {
    const streamA = mockStream();
    const streamB = mockStream();
    fetchMock
      .mockResolvedValueOnce(streamA.response)
      .mockResolvedValueOnce(streamB.response);
    const onMessage = jest.fn();
    const { rerender } = renderHook(
      ({ token }: { token: string }) =>
        useSSEMessages({
          businessId: 5,
          sessionToken: token,
          enabled: true,
          onMessage,
          onFallback: jest.fn(),
        }),
      { initialProps: { token: "token-A" } },
    );
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    const oldSignal = fetchMock.mock.calls[0][1]?.signal as AbortSignal;

    rerender({ token: "token-B" });
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    expect(oldSignal.aborted).toBe(true);
    expect(fetchMock.mock.calls[1][1]?.headers).toEqual(
      expect.objectContaining({ Authorization: "Bearer token-B" }),
    );

    act(() => {
      streamA.emit(
        'event: message.created\ndata: {"id":1,"role":"assistant","content":"stale","createdAt":1}\n\n',
      );
      streamB.emit(
        'event: message.created\ndata: {"id":2,"role":"assistant","content":"current","createdAt":2}\n\n',
      );
    });
    await flush();
    expect(onMessage).toHaveBeenCalledTimes(1);
    expect(onMessage).toHaveBeenCalledWith(
      expect.objectContaining({ id: 2, content: "current" }),
    );
  });

  it("reconnects with the same bearer then falls back after two stream failures", async () => {
    const first = mockStream();
    const second = mockStream();
    fetchMock
      .mockResolvedValueOnce(first.response)
      .mockResolvedValueOnce(second.response);
    const onFallback = jest.fn();
    renderHook(() =>
      useSSEMessages({
        businessId: 5,
        sessionToken: "token-A",
        enabled: true,
        onMessage: jest.fn(),
        onFallback,
      }),
    );
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));

    act(() => first.fail());
    await flush();
    expect(onFallback).not.toHaveBeenCalled();
    await act(async () => {
      jest.advanceTimersByTime(5_000);
      await Promise.resolve();
    });
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    expect(fetchMock.mock.calls[1][1]?.headers).toEqual(
      expect.objectContaining({ Authorization: "Bearer token-A" }),
    );

    act(() => second.fail());
    await flush();
    expect(onFallback).toHaveBeenCalledTimes(1);
  });

  it("does not let immediate connected-plus-EOF loops reset the bounded failure count", async () => {
    const first = mockStream();
    const second = mockStream();
    fetchMock
      .mockResolvedValueOnce(first.response)
      .mockResolvedValueOnce(second.response);
    const onFallback = jest.fn();
    renderHook(() =>
      useSSEMessages({
        businessId: 5,
        sessionToken: "token-A",
        enabled: true,
        onMessage: jest.fn(),
        onFallback,
      }),
    );
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    act(() => {
      first.emit("event: connected\ndata: {}\n\n");
      first.close();
    });
    await flush();
    await act(async () => {
      jest.advanceTimersByTime(5_000);
      await Promise.resolve();
    });
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    act(() => {
      second.emit("event: connected\ndata: {}\n\n");
      second.close();
    });
    await flush();
    expect(onFallback).toHaveBeenCalledTimes(1);
  });

  it("cancels non-ok bodies and treats non-ok and bodyless responses as bounded failures", async () => {
    const unauthorized = mockStream();
    fetchMock
      .mockResolvedValueOnce({
        ok: false,
        status: 401,
        body: unauthorized.response.body,
      } as Response)
      .mockResolvedValueOnce({ ok: true, status: 200, body: null } as Response);
    const onFallback = jest.fn();
    renderHook(() =>
      useSSEMessages({
        businessId: 5,
        sessionToken: "token-A",
        enabled: true,
        onMessage: jest.fn(),
        onFallback,
      }),
    );
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    await flush();
    expect(unauthorized.cancelled).toHaveBeenCalled();
    await act(async () => {
      jest.advanceTimersByTime(5_000);
      await Promise.resolve();
    });
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    await flush();
    expect(onFallback).toHaveBeenCalledTimes(1);
  });

  it("bounds unterminated event buffers and falls back after repeated oversized streams", async () => {
    const first = mockStream();
    const second = mockStream();
    fetchMock
      .mockResolvedValueOnce(first.response)
      .mockResolvedValueOnce(second.response);
    const onFallback = jest.fn();
    renderHook(() =>
      useSSEMessages({
        businessId: 5,
        sessionToken: "token-A",
        enabled: true,
        onMessage: jest.fn(),
        onFallback,
      }),
    );
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    act(() =>
      first.emit(`event: message.created\ndata: ${"x".repeat(200_000)}`),
    );
    await flush();
    await act(async () => {
      jest.advanceTimersByTime(5_000);
      await Promise.resolve();
    });
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    act(() =>
      second.emit(`event: message.created\ndata: ${"x".repeat(200_000)}`),
    );
    await flush();
    expect(onFallback).toHaveBeenCalledTimes(1);
  });

  it("accepts a coalesced chunk larger than the buffer cap when each complete event is bounded", async () => {
    const stream = mockStream();
    fetchMock.mockResolvedValueOnce(stream.response);
    const onMessage = jest.fn();
    renderHook(() =>
      useSSEMessages({
        businessId: 5,
        sessionToken: "token-A",
        enabled: true,
        onMessage,
        onFallback: jest.fn(),
      }),
    );
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    const ignored = `event: ignored\ndata: ${"x".repeat(1_000)}\n\n`;
    act(() =>
      stream.emit(
        ignored.repeat(140) +
          'event: message.created\ndata: {"id":9,"role":"assistant","content":"still readable","createdAt":9}\n\n',
      ),
    );
    await flush();
    expect(onMessage).toHaveBeenCalledWith(
      expect.objectContaining({ id: 9, content: "still readable" }),
    );
  });

  it("aborts without fallback when disabled or unmounted and never connects without a token", async () => {
    const stream = mockStream();
    fetchMock.mockResolvedValueOnce(stream.response);
    const onFallback = jest.fn();
    const { rerender, unmount } = renderHook(
      ({ enabled, token }: { enabled: boolean; token: string }) =>
        useSSEMessages({
          businessId: 5,
          sessionToken: token,
          enabled,
          onMessage: jest.fn(),
          onFallback,
        }),
      { initialProps: { enabled: true, token: "token-A" } },
    );
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    const signal = fetchMock.mock.calls[0][1]?.signal as AbortSignal;
    rerender({ enabled: false, token: "token-A" });
    expect(signal.aborted).toBe(true);
    await flush();
    expect(stream.cancelled).toHaveBeenCalled();
    act(() => jest.advanceTimersByTime(60_000));
    expect(onFallback).not.toHaveBeenCalled();
    unmount();

    renderHook(() =>
      useSSEMessages({
        businessId: 5,
        sessionToken: "",
        enabled: true,
        onMessage: jest.fn(),
        onFallback,
      }),
    );
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});
