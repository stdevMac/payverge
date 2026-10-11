/** @jest-environment jsdom */
import React from "react";
import { ReadableStream } from "stream/web";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { AiWaiter } from "@/components/guest/AiWaiter";
import { axiosInstance } from "../../../api";
import { createAiWaiterSession } from "@/api/aiWaiter";

jest.mock("../../../api", () => ({
  axiosInstance: { get: jest.fn(), post: jest.fn() },
}));
jest.mock("react-hot-toast", () => ({
  toast: { success: jest.fn(), error: jest.fn() },
}));
jest.mock("../aiWaiterCopy", () => ({
  getStarterQuestions: jest.fn().mockResolvedValue([]),
}));
jest.mock("@/api/aiWaiter", () => ({
  createAiWaiterSession: jest.fn(),
  isSessionUnknownError: (e: any) =>
    e?.response?.status === 404 &&
    e?.response?.data?.code === "session_unknown",
  classifyChatError: (e: any) => {
    if (
      e?.response?.status === 404 &&
      e?.response?.data?.code === "session_unknown"
    )
      return "session_unknown";
    if (e?.response?.status === 429) return "rate_limited";
    if (e?.response?.status === 413) return "too_long";
    if (e?.response?.data?.code === "over_budget") return "over_budget";
    return "generic";
  },
}));
jest.mock("framer-motion", () => {
  const R = require("react");
  const M = R.forwardRef(({ children, ...p }: any, ref: any) => {
    const { initial, animate, exit, transition, whileHover, whileTap, ...d } =
      p;
    return R.createElement("div", { ...d, ref }, children);
  });
  return {
    AnimatePresence: ({ children }: any) =>
      R.createElement(R.Fragment, null, children),
    motion: new Proxy({}, { get: () => M }),
    useReducedMotion: () => false,
  };
});
jest.mock("@nextui-org/react", () => {
  const R = require("react");
  return {
    Modal: ({ children, isOpen }: any) =>
      isOpen
        ? R.createElement(
            "div",
            { role: "dialog", "aria-modal": "true" },
            children,
          )
        : null,
    ModalContent: ({ children, ...p }: any) =>
      R.createElement("div", p, children),
    ModalBody: ({ children, ...p }: any) => R.createElement("div", p, children),
    ModalFooter: ({ children, ...p }: any) =>
      R.createElement("div", p, children),
    Card: ({ children, ...p }: any) => <div {...p}>{children}</div>,
    CardBody: ({ children, ...p }: any) => <div {...p}>{children}</div>,
    ScrollShadow: R.forwardRef(({ children }: any, ref: any) => (
      <div ref={ref}>{children}</div>
    )),
    Image: ({ alt }: any) => <div aria-label={alt} />,
    Input: R.forwardRef(
      (
        { placeholder, value, onValueChange, onKeyDown, endContent }: any,
        ref: any,
      ) => (
        <label>
          <input
            ref={ref}
            placeholder={placeholder}
            value={value}
            onChange={(e) => onValueChange?.(e.target.value)}
            onKeyDown={onKeyDown}
          />
          {endContent}
        </label>
      ),
    ),
    Button: ({
      children,
      onPress,
      isDisabled,
      title,
      "aria-label": al,
    }: any) => (
      <button
        type="button"
        disabled={isDisabled}
        onClick={onPress}
        title={title}
        aria-label={al}
      >
        {children}
      </button>
    ),
  };
});

const mockedAxios = axiosInstance as jest.Mocked<typeof axiosInstance>;
const mockedCreate = createAiWaiterSession as jest.Mock;
const originalFetch = global.fetch;

beforeEach(() => {
  const body = new ReadableStream<Uint8Array>({
    start() {
      // Keep the authenticated stream open until the component aborts it.
    },
  });
  global.fetch = jest.fn().mockResolvedValue({
    ok: true,
    status: 200,
    body,
  }) as jest.MockedFunction<typeof fetch>;
});

afterAll(() => {
  global.fetch = originalFetch;
});

function deferred<T = unknown>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

describe("AiWaiter session-token flow", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    localStorage.clear();
    mockedAxios.get.mockResolvedValue({ data: [] });
    mockedCreate.mockResolvedValue({
      session_token: "srv-token-1",
      greeting: "Hi, AI here",
      expires_at: "2999-01-01T00:00:00Z",
    });
  });

  it("creates a server session on open and reads messages via the cookie, not a URL token", async () => {
    render(
      <AiWaiter
        businessId={3}
        businessName="Cafe"
        menuData={[]}
        onAddToCart={jest.fn()}
        tableCode="T9"
      />,
    );
    fireEvent.click(screen.getAllByRole("button")[0]);
    // The server crypto session is created (not a client Math.random token).
    await waitFor(() =>
      expect(mockedCreate).toHaveBeenCalledWith(3, {
        table_code: "T9",
        mode: "ordering",
        language: "en",
      }),
    );
    await waitFor(() => {
      const call = mockedAxios.get.mock.calls.find((c) =>
        String(c[0]).includes("/ai-waiter/3/messages"),
      );
      expect(call).toBeTruthy();
      // Token must NOT ride the URL — it authenticates from the HttpOnly
      // pv_ai_waiter_session cookie (axiosInstance withCredentials). A URL token
      // would leak into proxy/CDN access logs.
      expect(
        (call?.[1]?.params as Record<string, unknown> | undefined)
          ?.session_token,
      ).toBeUndefined();
      // Conversation reads must bypass the 5-min GET cache.
      expect(call?.[1]?._useCache).toBe(false);
    });
  });

  it("does NOT persist the session token to localStorage (XSS-stealable bearer credential)", async () => {
    render(
      <AiWaiter
        businessId={3}
        businessName="Cafe"
        menuData={[]}
        onAddToCart={jest.fn()}
        tableCode="T9"
      />,
    );
    fireEvent.click(screen.getAllByRole("button")[0]);
    await waitFor(() => expect(mockedCreate).toHaveBeenCalledTimes(1));

    // The raw session token is a replayable bearer credential; it must live in
    // memory only. SSE uses it directly from memory while history continues to
    // authenticate with the HttpOnly cookie. Persisting it to localStorage lets
    // any XSS dump and replay it.
    expect(localStorage.getItem("ai_session_3_ordering_T9")).toBeNull();
    for (let i = 0; i < localStorage.length; i++) {
      const key = localStorage.key(i)!;
      expect(localStorage.getItem(key) ?? "").not.toContain("srv-token-1");
    }
  });

  it("re-creates the session + shows a new-conversation notice on session_unknown", async () => {
    mockedCreate.mockResolvedValueOnce({
      session_token: "srv-token-1",
      greeting: "Hi",
      expires_at: "2999-01-01T00:00:00Z",
    });
    render(
      <AiWaiter
        businessId={3}
        businessName="Cafe"
        menuData={[]}
        onAddToCart={jest.fn()}
        tableCode="T9"
      />,
    );
    fireEvent.click(screen.getAllByRole("button")[0]);
    await waitFor(() => expect(mockedCreate).toHaveBeenCalledTimes(1));

    mockedAxios.post.mockRejectedValueOnce({
      response: { status: 404, data: { code: "session_unknown" } },
    });
    const recreated = deferred<{
      session_token: string;
      greeting: string;
      expires_at: string;
    }>();
    mockedCreate.mockReturnValueOnce(recreated.promise);

    const input = await screen.findByPlaceholderText(
      /Ask me anything about the menu/i,
    );
    fireEvent.change(input, { target: { value: "hi" } });
    fireEvent.keyDown(input, { key: "Enter" });

    await waitFor(() => expect(mockedCreate).toHaveBeenCalledTimes(2));
    expect(screen.getByRole("button", { name: /sending/i })).toBeDisabled();
    fireEvent.change(input, { target: { value: "must wait" } });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(mockedAxios.post).toHaveBeenCalledTimes(1);
    await act(async () =>
      recreated.resolve({
        session_token: "srv-token-2",
        greeting: "Hi again",
        expires_at: "2999-01-01T00:00:00Z",
      }),
    );
    expect(
      await screen.findByText(/Starting a new conversation/i),
    ).toBeInTheDocument();
  });

  it("does not strand the chat in a loading state when send-time session creation fails", async () => {
    // Session creation fails on open (bootstrap swallows it, token stays null)
    // and again at send time. The send-time ensureSession() must be covered by
    // the handler's try/catch so the error is classified and isLoading clears —
    // previously it threw before the try and left the chat permanently disabled.
    mockedCreate.mockRejectedValue({ response: { status: 429 } });
    render(
      <AiWaiter
        businessId={3}
        businessName="Cafe"
        menuData={[]}
        onAddToCart={jest.fn()}
        tableCode="T9"
      />,
    );
    fireEvent.click(screen.getAllByRole("button")[0]);
    await waitFor(() => expect(mockedCreate).toHaveBeenCalled());

    const input = await screen.findByPlaceholderText(
      /Ask me anything about the menu/i,
    );
    fireEvent.change(input, { target: { value: "hi" } });
    fireEvent.keyDown(input, { key: "Enter" });

    // The failure is classified and surfaced (rate-limited copy) instead of an
    // unhandled rejection that leaves the chat stuck loading forever.
    expect(
      await screen.findByText(/sending messages a little fast/i),
    ).toBeInTheDocument();
  });

  it("does not log bootstrap failures after the dialog unmounts", async () => {
    const consoleErrorSpy = jest
      .spyOn(console, "error")
      .mockImplementation(() => {});
    const pending = deferred();
    mockedCreate.mockReturnValueOnce(pending.promise);

    const { unmount } = render(
      <AiWaiter
        businessId={3}
        businessName="Cafe"
        menuData={[]}
        onAddToCart={jest.fn()}
        tableCode="T9"
      />,
    );
    fireEvent.click(screen.getAllByRole("button")[0]);
    await waitFor(() => expect(mockedCreate).toHaveBeenCalledTimes(1));

    unmount();
    await act(async () => {
      pending.reject(new Error("late bootstrap failure"));
      await pending.promise.catch(() => undefined);
    });

    try {
      expect(consoleErrorSpy).not.toHaveBeenCalled();
    } finally {
      consoleErrorSpy.mockRestore();
    }
  });

  it("does not update the wrapper after a session recreation settles post-unmount", async () => {
    const consoleErrorSpy = jest
      .spyOn(console, "error")
      .mockImplementation(() => {});
    mockedCreate.mockResolvedValueOnce({
      session_token: "srv-token-1",
      greeting: "Hi",
      expires_at: "2999-01-01T00:00:00Z",
    });
    mockedAxios.post.mockRejectedValueOnce({
      response: { status: 404, data: { code: "session_unknown" } },
    });
    const recreated = deferred<{
      session_token: string;
      greeting: string;
      expires_at: string;
    }>();
    mockedCreate.mockReturnValueOnce(recreated.promise);

    const { unmount } = render(
      <AiWaiter
        businessId={3}
        businessName="Cafe"
        menuData={[]}
        onAddToCart={jest.fn()}
        tableCode="T9"
      />,
    );
    fireEvent.click(screen.getAllByRole("button")[0]);
    await waitFor(() => expect(mockedCreate).toHaveBeenCalledTimes(1));
    const input = await screen.findByPlaceholderText(
      /Ask me anything about the menu/i,
    );
    fireEvent.change(input, { target: { value: "hi" } });
    fireEvent.keyDown(input, { key: "Enter" });
    await waitFor(() => expect(mockedCreate).toHaveBeenCalledTimes(2));

    unmount();
    await act(async () => {
      recreated.resolve({
        session_token: "srv-token-2",
        greeting: "Hi again",
        expires_at: "2999-01-01T00:00:00Z",
      });
      await recreated.promise;
      await Promise.resolve();
    });
    try {
      expect(consoleErrorSpy).not.toHaveBeenCalled();
    } finally {
      consoleErrorSpy.mockRestore();
    }
  });
});
