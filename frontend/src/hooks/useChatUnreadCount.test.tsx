/** @jest-environment jsdom */
import React from "react";
import { renderHook, waitFor, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useChatUnreadCount } from "@/hooks/useChatUnreadCount";
import { chatApi } from "@/api/chat";
import { useSSEEvents } from "@/hooks/useSSEEvents";

jest.mock("@/api/chat", () => ({
  chatApi: { listChannels: jest.fn() },
}));

jest.mock("@/hooks/useSSEEvents", () => ({
  useSSEEvents: jest.fn(),
}));

function makeWrapper() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  function wrapper({ children }: { children: React.ReactNode }) {
    return (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );
  }
  return wrapper;
}

describe("useChatUnreadCount", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (useSSEEvents as jest.Mock).mockReturnValue({
      degraded: false,
      blocked: false,
      reconnect: jest.fn(),
    });
  });

  it("sums unread across channels", async () => {
    (chatApi.listChannels as jest.Mock).mockResolvedValue({
      channels: [],
      unread: { "1": 2, "7": 3 },
      previews: {},
    });

    const { result } = renderHook(() => useChatUnreadCount(42), {
      wrapper: makeWrapper(),
    });

    await waitFor(() => expect(result.current).toBe(5));
  });

  it("returns 0 while loading", () => {
    (chatApi.listChannels as jest.Mock).mockReturnValue(new Promise(() => {}));

    const { result } = renderHook(() => useChatUnreadCount(42), {
      wrapper: makeWrapper(),
    });

    expect(result.current).toBe(0);
  });

  it("returns 0 when disabled (businessId 0)", () => {
    const { result } = renderHook(() => useChatUnreadCount(0), {
      wrapper: makeWrapper(),
    });

    expect(result.current).toBe(0);
    expect(chatApi.listChannels).not.toHaveBeenCalled();
  });

  it("fails silent to 0 on query error (queryFn throws — shared-key semantics)", async () => {
    // The queryFn must NOT swallow the error (TeamChatPanel shares this key
    // and relies on isError); the hook absorbs it by reading only `data`,
    // which stays undefined on error → 0, and never throws.
    (chatApi.listChannels as jest.Mock).mockRejectedValue(
      new Error("403 forbidden"),
    );

    const { result } = renderHook(() => useChatUnreadCount(42), {
      wrapper: makeWrapper(),
    });

    await waitFor(() => expect(chatApi.listChannels).toHaveBeenCalled());
    expect(result.current).toBe(0);
  });

  describe("SSE reconnect healing", () => {
    it("registers an onReconnect handler with the shared SSE hook", async () => {
      (chatApi.listChannels as jest.Mock).mockResolvedValue({
        channels: [],
        unread: {},
        previews: {},
      });

      renderHook(() => useChatUnreadCount(42), { wrapper: makeWrapper() });

      const options = (useSSEEvents as jest.Mock).mock.calls[0][0];
      expect(typeof options.onReconnect).toBe("function");
    });

    it("refetches the unread count immediately on reconnect (missed nudges heal)", async () => {
      (chatApi.listChannels as jest.Mock)
        .mockResolvedValueOnce({ channels: [], unread: { "1": 2 }, previews: {} })
        .mockResolvedValueOnce({ channels: [], unread: { "1": 6 }, previews: {} });

      const { result } = renderHook(() => useChatUnreadCount(42), {
        wrapper: makeWrapper(),
      });

      await waitFor(() => expect(result.current).toBe(2));

      const { onReconnect } = (useSSEEvents as jest.Mock).mock.calls[0][0];
      act(() => {
        onReconnect();
      });

      await waitFor(() => expect(result.current).toBe(6));
      expect(chatApi.listChannels).toHaveBeenCalledTimes(2);
    });

    it("cancels a pending debounce and resyncs at once on reconnect", async () => {
      jest.useFakeTimers();
      try {
        (chatApi.listChannels as jest.Mock)
          .mockResolvedValueOnce({ channels: [], unread: { "1": 2 }, previews: {} })
          .mockResolvedValueOnce({ channels: [], unread: { "1": 9 }, previews: {} });

        const { result } = renderHook(() => useChatUnreadCount(42), {
          wrapper: makeWrapper(),
        });

        await waitFor(() => expect(result.current).toBe(2));

        const { onEvent, onReconnect } = (useSSEEvents as jest.Mock).mock
          .calls[0][0];
        act(() => {
          onEvent({ type: "chat.message", data: {} });
        });
        // Debounce pending, no refetch yet.
        expect(chatApi.listChannels).toHaveBeenCalledTimes(1);

        act(() => {
          onReconnect();
        });

        await waitFor(() => expect(result.current).toBe(9));
        expect(chatApi.listChannels).toHaveBeenCalledTimes(2);

        // The pending debounce was cancelled — elapsing it triggers nothing new.
        act(() => {
          jest.advanceTimersByTime(2500);
        });
        expect(chatApi.listChannels).toHaveBeenCalledTimes(2);
      } finally {
        jest.useRealTimers();
      }
    });
  });

  describe.each(["chat.message", "chat.announcement"] as const)(
    "%s SSE nudge",
    (eventType) => {
      beforeEach(() => {
        jest.useFakeTimers();
      });

      afterEach(() => {
        jest.useRealTimers();
      });

      it("invalidates and refetches only after the 2.5s debounce", async () => {
        (chatApi.listChannels as jest.Mock)
          .mockResolvedValueOnce({
            channels: [],
            unread: { "1": 2 },
            previews: {},
          })
          .mockResolvedValueOnce({
            channels: [],
            unread: { "1": 2, "2": 9 },
            previews: {},
          });

        const { result } = renderHook(() => useChatUnreadCount(42), {
          wrapper: makeWrapper(),
        });

        await waitFor(() => expect(result.current).toBe(2));

        const onEvent = (useSSEEvents as jest.Mock).mock.calls[0][0].onEvent;
        act(() => {
          onEvent({ type: eventType, data: {} });
        });

        // Not yet — debounce hasn't elapsed.
        expect(chatApi.listChannels).toHaveBeenCalledTimes(1);

        act(() => {
          jest.advanceTimersByTime(2500);
        });

        await waitFor(() => expect(result.current).toBe(11));
        expect(chatApi.listChannels).toHaveBeenCalledTimes(2);
      });

      it("collapses a burst of nudges into a single refetch", async () => {
        (chatApi.listChannels as jest.Mock)
          .mockResolvedValueOnce({
            channels: [],
            unread: { "1": 2 },
            previews: {},
          })
          .mockResolvedValueOnce({
            channels: [],
            unread: { "1": 2, "2": 9 },
            previews: {},
          });

        const { result } = renderHook(() => useChatUnreadCount(42), {
          wrapper: makeWrapper(),
        });

        await waitFor(() => expect(result.current).toBe(2));

        const onEvent = (useSSEEvents as jest.Mock).mock.calls[0][0].onEvent;
        act(() => {
          onEvent({ type: eventType, data: {} });
          onEvent({ type: eventType, data: {} });
        });

        act(() => {
          jest.advanceTimersByTime(2500);
        });

        await waitFor(() => expect(result.current).toBe(11));
        expect(chatApi.listChannels).toHaveBeenCalledTimes(2);
      });
    },
  );
});
