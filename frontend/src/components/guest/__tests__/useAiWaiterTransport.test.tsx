/** @jest-environment jsdom */
import React from "react";
import { renderToString } from "react-dom/server";
import { hydrateRoot, type Root } from "react-dom/client";
import { act, renderHook, waitFor } from "@testing-library/react";
import { axiosInstance } from "../../../api";
import { AI_WAITER_CHAT_REQUEST_CONFIG } from "@/api/aiWaiter";
import {
  adaptLegacyResponse,
  type AssistantResponse,
} from "@/types/assistant";
import { useSSEMessages } from "@/hooks/useSSEMessages";
import {
  useAiWaiterTransport,
  type UseAiWaiterTransportOptions,
} from "@/components/guest/useAiWaiterTransport";
import {
  readStoredTranscript,
  writeStoredTranscript,
} from "@/components/guest/aiWaiterTranscriptStore";

jest.mock("../../../api", () => ({
  axiosInstance: { get: jest.fn(), post: jest.fn() },
}));
jest.mock("@/hooks/useSSEMessages", () => ({ useSSEMessages: jest.fn() }));

const mockedAxios = axiosInstance as jest.Mocked<typeof axiosInstance>;
// axios >= 1.20 types request params generically, so a mocked call's
// config.params infers as `{}`; read them through a plain record.
const paramsOf = (config?: { params?: unknown }) =>
  config?.params as Record<string, unknown> | undefined;
const mockedSSE = useSSEMessages as jest.MockedFunction<typeof useSSEMessages>;

const v2 = (
  responseId: string,
  answer = "Grounded answer",
): AssistantResponse => ({
  version: 2,
  response_id: responseId,
  answer: { format: "plain_text", content: answer },
  sections: [],
  steps: [],
  actions: [],
  sources: [],
  entities: [],
  follow_ups: [],
  workflow: null,
  notices: [],
  status: "complete",
});

const historyAssistant = (id: number, response: AssistantResponse) => ({
  id,
  role: "assistant",
  content: "untrusted outer content",
  created_at: id,
  response_v2: response,
});

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

const baseOptions = (
  overrides: Partial<UseAiWaiterTransportOptions> = {},
): UseAiWaiterTransportOptions => ({
  businessId: 7,
  tableCode: "T1",
  mode: "ordering",
  language: "en",
  isOpen: true,
  sessionToken: "token-1",
  ensureSession: jest.fn().mockResolvedValue("token-1"),
  billContext: "",
  ...overrides,
});

describe("useAiWaiterTransport", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    jest.useRealTimers();
    Object.defineProperty(globalThis, "EventSource", {
      configurable: true,
      writable: true,
      value: class TestEventSource {},
    });
    mockedAxios.get.mockResolvedValue({ data: [] });
    localStorage.clear();
  });

  it("restores at most 100 messages, normalizes created_at, and preserves strict assistant V2 response IDs", async () => {
    const rows = Array.from({ length: 105 }, (_, index) =>
      historyAssistant(index + 1, v2(`history-${index + 1}`)),
    );
    mockedAxios.get.mockResolvedValueOnce({ data: rows });
    const project = jest.fn((response: AssistantResponse) => response);
    const options = baseOptions({ projectAssistantResponse: project });
    const { result } = renderHook(() => useAiWaiterTransport(options));

    await waitFor(() => expect(result.current.messages).toHaveLength(100));
    expect(result.current.messages[0]).toMatchObject({
      id: 6,
      createdAt: 6,
      response: { response_id: "history-6" },
      content: "Grounded answer",
    });
    expect(result.current.messages[99].response?.response_id).toBe(
      "history-105",
    );
    expect(project).not.toHaveBeenCalled();
  });

  it("orders same-second persisted rows by canonical message ID", async () => {
    mockedAxios.get.mockResolvedValueOnce({
      data: [
        { ...historyAssistant(3, v2("same-3", "third")), created_at: 100 },
        { ...historyAssistant(1, v2("same-1", "first")), created_at: 100 },
        { ...historyAssistant(2, v2("same-2", "second")), created_at: 100 },
      ],
    });
    const options = baseOptions();
    const { result } = renderHook(() => useAiWaiterTransport(options));
    await waitFor(() => expect(result.current.messages).toHaveLength(3));
    expect(result.current.messages.map(({ id }) => id)).toEqual([1, 2, 3]);
  });

  it("ignores user response_v2, drops invalid present assistant V2, and adapts only absent legacy assistant rows", async () => {
    const onHistoryContractMismatch = jest.fn();
    const onRenderFallback = jest.fn();
    mockedAxios.get.mockResolvedValueOnce({
      data: [
        {
          id: 1,
          role: "user",
          content: "Guest text",
          created_at: 1,
          response_v2: v2("must-not-attach"),
        },
        {
          id: 2,
          role: "assistant",
          content: "Unsafe downgrade",
          created_at: 2,
          response_v2: { ...v2("invalid"), version: 1 },
        },
        { id: 3, role: "assistant", content: "Legacy safe", created_at: 3 },
      ],
    });
    const options = baseOptions({
      onHistoryContractMismatch,
      onRenderFallback,
    });
    const { result } = renderHook(() => useAiWaiterTransport(options));

    await waitFor(() => expect(result.current.messages).toHaveLength(2));
    expect(result.current.messages[0]).toMatchObject({
      id: 1,
      role: "user",
      content: "Guest text",
    });
    expect(result.current.messages[0].response).toBeUndefined();
    expect(result.current.messages.some(({ id }) => id === 2)).toBe(false);
    expect(result.current.messages[1].response?.answer.content).toBe(
      "Legacy safe",
    );
    expect(result.current.messages[1].contractVersion).toBe("v1");
    expect(onHistoryContractMismatch).toHaveBeenCalledTimes(1);
    expect(onHistoryContractMismatch).toHaveBeenCalledWith();
    expect(onRenderFallback).toHaveBeenCalledTimes(1);
    expect(onRenderFallback).toHaveBeenCalledWith();
  });

  it("dedupes bounded history mismatch transitions across a reopen", async () => {
    const malformed = {
      id: 9,
      role: "assistant",
      content: "Unsafe downgrade",
      created_at: 9,
      response_v2: { ...v2("invalid"), version: 1 },
    };
    mockedAxios.get.mockResolvedValue({ data: [malformed, malformed] });
    const onHistoryContractMismatch = jest.fn();
    const onRenderFallback = jest.fn();
    const options = baseOptions({
      onHistoryContractMismatch,
      onRenderFallback,
    });
    const { rerender } = renderHook(
      ({ isOpen }: { isOpen: boolean }) =>
        useAiWaiterTransport({ ...options, isOpen }),
      { initialProps: { isOpen: true } },
    );

    await waitFor(() =>
      expect(onHistoryContractMismatch).toHaveBeenCalledTimes(1),
    );
    rerender({ isOpen: false });
    rerender({ isOpen: true });
    await waitFor(() => expect(mockedAxios.get).toHaveBeenCalledTimes(2));
    expect(onHistoryContractMismatch).toHaveBeenCalledTimes(1);
    expect(onRenderFallback).toHaveBeenCalledTimes(1);
  });

  it("dedupes SSE by persisted ID, preserves response_v2, and never projects restored/live actions", async () => {
    const project = jest.fn((response: AssistantResponse) => response);
    const options = baseOptions({ projectAssistantResponse: project });
    const { result } = renderHook(() => useAiWaiterTransport(options));
    await waitFor(() => expect(mockedSSE).toHaveBeenCalled());
    const frame = {
      id: 41,
      role: "assistant" as const,
      content: "outer",
      createdAt: 41,
      response_v2: v2("stream-41"),
    };

    act(() => {
      mockedSSE.mock.calls.at(-1)?.[0].onMessage(frame);
      mockedSSE.mock.calls.at(-1)?.[0].onMessage(frame);
    });

    expect(result.current.messages).toHaveLength(1);
    expect(result.current.messages[0]).toMatchObject({
      id: 41,
      content: "Grounded answer",
      response: { response_id: "stream-41" },
      contractVersion: "v2",
    });
    expect(project).not.toHaveBeenCalled();
  });

  it("falls back from SSE to 3-second delta polling and stops while closed", async () => {
    jest.useFakeTimers();
    const options = baseOptions();
    const { rerender } = renderHook(
      ({ isOpen }: { isOpen: boolean }) =>
        useAiWaiterTransport({ ...options, isOpen }),
      { initialProps: { isOpen: true } },
    );
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(mockedAxios.get).toHaveBeenCalledTimes(1);
    mockedAxios.get.mockClear();

    act(() => mockedSSE.mock.calls.at(-1)?.[0].onFallback());
    await act(async () => {
      jest.advanceTimersByTime(3_000);
      await Promise.resolve();
    });
    expect(mockedAxios.get).toHaveBeenCalledWith(
      "/ai-waiter/7/messages",
      expect.objectContaining({
        params: expect.objectContaining({ since: 0 }),
      }),
    );

    mockedAxios.get.mockClear();
    rerender({ isOpen: false });
    act(() => jest.advanceTimersByTime(6_000));
    expect(mockedAxios.get).not.toHaveBeenCalled();
  });

  it("keeps fetch-based SSE active when native EventSource is unavailable", async () => {
    jest.useFakeTimers();
    Object.defineProperty(globalThis, "EventSource", {
      configurable: true,
      writable: true,
      value: undefined,
    });
    const options = baseOptions();
    const { result } = renderHook(() => useAiWaiterTransport(options));
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    mockedAxios.get.mockClear();

    await act(async () => {
      jest.advanceTimersByTime(3_000);
      await Promise.resolve();
    });
    expect(mockedAxios.get).not.toHaveBeenCalled();
    expect(result.current.streamFellBack).toBe(false);
  });

  it("reuses the session across close/reopen but performs a fresh bounded history read", async () => {
    const ensureSession = jest.fn().mockResolvedValue("token-1");
    const options = baseOptions({ ensureSession });
    const { rerender } = renderHook(
      ({ isOpen }: { isOpen: boolean }) =>
        useAiWaiterTransport({ ...options, isOpen }),
      { initialProps: { isOpen: true } },
    );
    await waitFor(() => expect(mockedAxios.get).toHaveBeenCalledTimes(1));

    rerender({ isOpen: false });
    rerender({ isOpen: true });
    await waitFor(() => expect(mockedAxios.get).toHaveBeenCalledTimes(2));
    expect(ensureSession).toHaveBeenCalledTimes(2);
  });

  it("posts only the newest 40 history messages including the optimistic current prompt", async () => {
    mockedAxios.get.mockResolvedValueOnce({
      data: Array.from({ length: 100 }, (_, index) => ({
        id: index + 1,
        role: index % 2 === 0 ? "user" : "assistant",
        content: `message-${index + 1}`,
        created_at: index + 1,
        ...(index % 2 === 1
          ? { response_v2: v2(`history-${index + 1}`, `message-${index + 1}`) }
          : {}),
      })),
    });
    mockedAxios.post.mockResolvedValueOnce({
      data: {
        id: 101,
        role: "model",
        parts: [],
        response_v2: v2("direct-101"),
      },
    });
    const options = baseOptions();
    const { result } = renderHook(() => useAiWaiterTransport(options));
    await waitFor(() => expect(result.current.messages).toHaveLength(100));

    await act(
      async () => void (await result.current.sendMessage("current prompt")),
    );
    const payload = mockedAxios.post.mock.calls[0][1] as {
      history: Array<{ role: string; content: string }>;
    };
    expect(payload.history).toHaveLength(40);
    expect(payload.history[0].content).toBe("message-62");
    expect(payload.history[39]).toMatchObject({
      role: "user",
      content: "current prompt",
    });
    expect(result.current.messages.at(-1)?.response?.response_id).toBe(
      "direct-101",
    );
    expect(result.current.messages.at(-1)?.id).toBe(101);
  });

  // #596 — a waiter turn can exceed the global 30s axios timeout, and the
  // interceptor's connection toast must never fire for waiter transport.
  it("sends chat turns with the long-timeout, toast-suppressed waiter config", async () => {
    mockedAxios.get.mockResolvedValueOnce({ data: [] });
    mockedAxios.post.mockResolvedValueOnce({
      data: { id: 12, role: "model", parts: [], response_v2: v2("cfg-12") },
    });
    const options = baseOptions();
    const { result } = renderHook(() => useAiWaiterTransport(options));
    await waitFor(() => expect(mockedAxios.get).toHaveBeenCalledTimes(1));
    // History reads also opt out of the global toast (failures surface as the
    // in-thread history_unavailable notice instead).
    expect(mockedAxios.get.mock.calls[0][1]).toMatchObject({
      _skipErrorToast: true,
    });

    await act(async () => void (await result.current.sendMessage("hello")));
    const [url, , config] = mockedAxios.post.mock.calls[0];
    expect(url).toBe("/ai-waiter/7");
    expect(config).toBe(AI_WAITER_CHAT_REQUEST_CONFIG);
    expect(config).toMatchObject({ timeout: 90000, _skipErrorToast: true });
  });

  it("keeps failed input retryable and clears it after a successful retry", async () => {
    mockedAxios.post
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValueOnce({
        data: { id: 4, role: "model", parts: [], response_v2: v2("retry-ok") },
      });
    const options = baseOptions();
    const { result } = renderHook(() => useAiWaiterTransport(options));
    await waitFor(() => expect(mockedAxios.get).toHaveBeenCalled());

    await act(
      async () => void (await result.current.sendMessage("please retry")),
    );
    expect(result.current.lastError).toBe("generic");
    expect(result.current.lastFailedInput).toBe("please retry");

    await act(async () => void (await result.current.retryFailedMessage()));
    expect(result.current.lastFailedInput).toBeNull();
    expect(result.current.messages.at(-1)?.response?.response_id).toBe(
      "retry-ok",
    );
    expect(
      result.current.messages.filter(
        ({ role, content }) => role === "user" && content === "please retry",
      ),
    ).toHaveLength(1);
    const retryPayload = mockedAxios.post.mock.calls[1][1] as {
      history: Array<{ role: string; content: string }>;
      client_nonce: string;
    };
    expect(
      retryPayload.history.filter(
        ({ role, content }) => role === "user" && content === "please retry",
      ),
    ).toHaveLength(1);
    expect(retryPayload.client_nonce).toBe(
      (mockedAxios.post.mock.calls[0][1] as { client_nonce: string })
        .client_nonce,
    );
  });

  it("does not let a late full-history read erase an authoritative projected direct response", async () => {
    const history = deferred<{ data: unknown[] }>();
    mockedAxios.get.mockReturnValueOnce(history.promise);
    mockedAxios.post.mockResolvedValueOnce({
      data: { id: 70, role: "model", parts: [], response_v2: v2("direct-70") },
    });
    const project = jest.fn((response: AssistantResponse) => ({
      ...response,
      answer: { ...response.answer, content: "direct projected" },
    }));
    const options = baseOptions({ projectAssistantResponse: project });
    const { result } = renderHook(() => useAiWaiterTransport(options));
    await waitFor(() => expect(mockedAxios.get).toHaveBeenCalledTimes(1));

    await act(async () => void (await result.current.sendMessage("hello")));
    expect(result.current.messages.find(({ id }) => id === 70)?.content).toBe(
      "direct projected",
    );
    await act(async () =>
      history.resolve({ data: [historyAssistant(70, v2("direct-70"))] }),
    );
    expect(result.current.messages.filter(({ id }) => id === 70)).toHaveLength(
      1,
    );
    expect(result.current.messages.find(({ id }) => id === 70)?.content).toBe(
      "direct projected",
    );
    expect(project).toHaveBeenCalledTimes(1);
  });

  it("keeps a current direct turn after a late full 100-row history bootstrap", async () => {
    const history = deferred<{ data: unknown[] }>();
    mockedAxios.get.mockReturnValueOnce(history.promise);
    mockedAxios.post.mockResolvedValueOnce({
      data: {
        id: 201,
        role: "model",
        parts: [],
        response_v2: v2("direct-201", "Current answer"),
      },
    });
    const options = baseOptions();
    const { result } = renderHook(() => useAiWaiterTransport(options));
    await waitFor(() => expect(mockedAxios.get).toHaveBeenCalledTimes(1));

    await act(
      async () => void (await result.current.sendMessage("Current prompt")),
    );
    await act(async () =>
      history.resolve({
        data: Array.from({ length: 100 }, (_, index) =>
          historyAssistant(index + 1, v2(`older-${index + 1}`)),
        ),
      }),
    );

    expect(result.current.messages).toHaveLength(100);
    expect(result.current.messages.at(-2)?.content).toBe("Current prompt");
    expect(result.current.messages.at(-1)?.content).toBe("Current answer");
  });

  it("reconciles a full-history user row only by an exact nonempty client nonce", async () => {
    const history = deferred<{ data: unknown[] }>();
    mockedAxios.get
      .mockResolvedValueOnce({ data: [] })
      .mockReturnValueOnce(history.promise);
    const options = baseOptions();
    const { result, rerender } = renderHook(
      ({ isOpen }: { isOpen: boolean }) =>
        useAiWaiterTransport({ ...options, isOpen }),
      { initialProps: { isOpen: true } },
    );
    await waitFor(() => expect(mockedAxios.get).toHaveBeenCalledTimes(1));

    act(() =>
      result.current.replaceMessages([
        {
          role: "user",
          content: "Current prompt",
          clientNonce: "preexisting-nonce",
          delivery: "optimistic",
        },
      ]),
    );
    rerender({ isOpen: false });
    rerender({ isOpen: true });
    await waitFor(() => expect(mockedAxios.get).toHaveBeenCalledTimes(2));
    await act(async () =>
      history.resolve({
        data: [
          {
            id: 201,
            role: "user",
            content: "Current prompt",
            created_at: 201,
            client_nonce: "preexisting-nonce",
          },
        ],
      }),
    );

    expect(
      result.current.messages.filter(
        ({ role, content }) => role === "user" && content === "Current prompt",
      ),
    ).toHaveLength(1);
    expect(result.current.messages.find(({ id }) => id === 201)?.delivery).toBe(
      "history",
    );
  });

  it("preserves a preexisting repeated optimistic prompt without an exact nonce match", async () => {
    const history = deferred<{ data: unknown[] }>();
    mockedAxios.get
      .mockResolvedValueOnce({ data: [] })
      .mockReturnValueOnce(history.promise);
    const options = baseOptions();
    const { result, rerender } = renderHook(
      ({ isOpen }: { isOpen: boolean }) =>
        useAiWaiterTransport({ ...options, isOpen }),
      { initialProps: { isOpen: true } },
    );
    await waitFor(() => expect(mockedAxios.get).toHaveBeenCalledTimes(1));

    act(() =>
      result.current.replaceMessages([
        {
          role: "user",
          content: "status?",
          clientNonce: "unmatched-nonce",
          delivery: "optimistic",
        },
      ]),
    );
    rerender({ isOpen: false });
    rerender({ isOpen: true });
    await waitFor(() => expect(mockedAxios.get).toHaveBeenCalledTimes(2));
    await act(async () =>
      history.resolve({
        data: [
          {
            id: 200,
            role: "user",
            content: "status?",
            created_at: 200,
          },
        ],
      }),
    );

    expect(
      result.current.messages.filter(
        ({ role, content }) => role === "user" && content === "status?",
      ),
    ).toHaveLength(2);
  });

  it("preserves a repeated optimistic prompt created after a full-history request starts", async () => {
    const history = deferred<{ data: unknown[] }>();
    mockedAxios.get.mockReturnValueOnce(history.promise);
    mockedAxios.post.mockResolvedValueOnce({
      data: {
        id: 202,
        role: "model",
        parts: [],
        response_v2: v2("direct-202", "Current answer"),
      },
    });
    const options = baseOptions();
    const { result } = renderHook(() => useAiWaiterTransport(options));
    await waitFor(() => expect(mockedAxios.get).toHaveBeenCalledTimes(1));

    await act(async () => void (await result.current.sendMessage("status?")));
    await act(async () =>
      history.resolve({
        data: [
          {
            id: 201,
            role: "user",
            content: "status?",
            created_at: 201,
          },
        ],
      }),
    );

    expect(
      result.current.messages.filter(
        ({ role, content }) => role === "user" && content === "status?",
      ),
    ).toHaveLength(2);
    expect(
      result.current.messages.some(
        ({ role, content, id }) =>
          role === "user" && content === "status?" && id === undefined,
      ),
    ).toBe(true);
    expect(result.current.messages.at(-1)?.content).toBe("Current answer");
  });

  it("preserves the same-scope transcript when a reopen full-history response is empty", async () => {
    mockedAxios.get
      .mockResolvedValueOnce({
        data: [historyAssistant(1, v2("history-1", "Existing answer"))],
      })
      .mockResolvedValueOnce({ data: [] });
    const options = baseOptions();
    const { result, rerender } = renderHook(
      ({ isOpen }: { isOpen: boolean }) =>
        useAiWaiterTransport({ ...options, isOpen }),
      { initialProps: { isOpen: true } },
    );
    await waitFor(() => expect(result.current.messages).toHaveLength(1));

    rerender({ isOpen: false });
    rerender({ isOpen: true });
    await waitFor(() => expect(mockedAxios.get).toHaveBeenCalledTimes(2));

    expect(result.current.messages.map(({ content }) => content)).toEqual([
      "Existing answer",
    ]);
  });

  it("authenticates history with the in-memory session header, not a query token", async () => {
    const options = baseOptions({
      sessionToken: "tab-token-1",
      ensureSession: jest.fn().mockResolvedValue("tab-token-1"),
    });
    renderHook(() => useAiWaiterTransport(options));
    await waitFor(() => expect(mockedAxios.get).toHaveBeenCalledTimes(1));
    expect(mockedAxios.get).toHaveBeenCalledWith(
      "/ai-waiter/7/messages",
      expect.objectContaining({
        params: expect.not.objectContaining({
          session_token: expect.anything(),
        }),
        headers: expect.objectContaining({
          "X-AI-Waiter-Session": "tab-token-1",
        }),
      }),
    );
    expect(
      paramsOf(mockedAxios.get.mock.calls[0][1])?.session_token,
    ).toBeUndefined();
  });

  it("keeps a client new-conversation notice ahead of the replacement greeting", async () => {
    mockedAxios.get.mockResolvedValueOnce({ data: [] });
    const ensureSession = jest
      .fn()
      .mockResolvedValueOnce("token-1")
      .mockResolvedValue("token-2");
    const { result, rerender } = renderHook(
      ({ sessionToken }: { sessionToken: string }) =>
        useAiWaiterTransport(baseOptions({ sessionToken, ensureSession })),
      { initialProps: { sessionToken: "token-1" } },
    );
    await waitFor(() => expect(mockedAxios.get).toHaveBeenCalledTimes(1));

    mockedAxios.get.mockResolvedValueOnce({
      data: [historyAssistant(2, v2("greet-2", "Hello again"))],
    });
    act(() => {
      result.current.resetConversation();
      result.current.appendAssistantText("Starting a new conversation.");
    });
    expect(result.current.messages[0]?.response?.status).toBe("complete");
    rerender({ sessionToken: "token-2" });
    await waitFor(() =>
      expect(result.current.messages.map(({ content }) => content)).toEqual([
        "Starting a new conversation.",
        "Hello again",
      ]),
    );
  });

  it("marks kitchen-connect copy as degraded instead of complete", async () => {
    mockedAxios.get.mockResolvedValueOnce({ data: [] });
    const { result } = renderHook(() => useAiWaiterTransport(baseOptions()));
    await waitFor(() => expect(mockedAxios.get).toHaveBeenCalledTimes(1));
    act(() => {
      result.current.appendAssistantText(
        "problemas para conectar con la cocina",
        "degraded",
      );
    });
    expect(result.current.messages[0]?.response?.status).toBe("degraded");
    expect(result.current.messages[0]?.content).toContain("conectar con la cocina");
  });

  it("resetConversation invalidates old requests and clears transport state before a new-session notice", async () => {
    const history = deferred<{ data: unknown[] }>();
    mockedAxios.get.mockReturnValueOnce(history.promise);
    const options = baseOptions();
    const { result } = renderHook(() => useAiWaiterTransport(options));
    await waitFor(() => expect(mockedAxios.get).toHaveBeenCalledTimes(1));

    act(() => {
      result.current.resetConversation();
      result.current.appendAssistantText("New conversation");
    });
    await act(async () =>
      history.resolve({ data: [historyAssistant(90, v2("expired-session"))] }),
    );
    expect(result.current.messages.map(({ content }) => content)).toEqual([
      "New conversation",
    ]);
  });

  it("clears history_unavailable after a later successful reopen", async () => {
    mockedAxios.get
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValueOnce({ data: [] });
    const consoleError = jest
      .spyOn(console, "error")
      .mockImplementation(() => {});
    const options = baseOptions();
    const { result, rerender } = renderHook(
      ({ isOpen }: { isOpen: boolean }) =>
        useAiWaiterTransport({ ...options, isOpen }),
      { initialProps: { isOpen: true } },
    );
    await waitFor(() =>
      expect(result.current.lastError).toBe("history_unavailable"),
    );
    rerender({ isOpen: false });
    rerender({ isOpen: true });
    await waitFor(() => expect(mockedAxios.get).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(result.current.lastError).toBeNull());
    consoleError.mockRestore();
  });

  it("discards late history and send results after scope change or unmount", async () => {
    const history = deferred<{ data: unknown[] }>();
    const send = deferred<{ data: unknown }>();
    mockedAxios.get.mockReturnValueOnce(history.promise);
    mockedAxios.post.mockReturnValueOnce(send.promise);
    const project = jest.fn((response: AssistantResponse) => response);
    const options = baseOptions({ projectAssistantResponse: project });
    const { result, rerender, unmount } = renderHook(
      ({ tableCode }: { tableCode: string }) =>
        useAiWaiterTransport({ ...options, tableCode }),
      { initialProps: { tableCode: "T1" } },
    );

    await waitFor(() => expect(mockedAxios.get).toHaveBeenCalledTimes(1));

    let sendPromise!: ReturnType<typeof result.current.sendMessage>;
    act(() => {
      sendPromise = result.current.sendMessage("old scope prompt");
    });
    rerender({ tableCode: "T2" });
    await act(async () => {
      history.resolve({ data: [historyAssistant(9, v2("old-history"))] });
      send.resolve({
        data: { id: 10, role: "model", parts: [], response_v2: v2("old-send") },
      });
      await sendPromise;
    });
    expect(result.current.messages).toEqual([]);
    expect(project).not.toHaveBeenCalled();

    const late = deferred<{ data: unknown }>();
    mockedAxios.post.mockReturnValueOnce(late.promise);
    let unmountedSend!: ReturnType<typeof result.current.sendMessage>;
    act(() => {
      unmountedSend = result.current.sendMessage("unmounted prompt");
    });
    unmount();
    await act(async () => {
      late.resolve({
        data: {
          id: 11,
          role: "model",
          parts: [],
          response_v2: v2("late-send"),
        },
      });
      await unmountedSend;
    });
    expect(project).not.toHaveBeenCalled();
  });

  it("retries SSE after close/reopen instead of remaining in polling fallback", async () => {
    const options = baseOptions();
    const { rerender } = renderHook(
      ({ isOpen }: { isOpen: boolean }) =>
        useAiWaiterTransport({ ...options, isOpen }),
      { initialProps: { isOpen: true } },
    );
    await waitFor(() => expect(mockedSSE).toHaveBeenCalled());
    act(() => mockedSSE.mock.calls.at(-1)?.[0].onFallback());
    await waitFor(() =>
      expect(mockedSSE.mock.calls.at(-1)?.[0].enabled).toBe(false),
    );

    rerender({ isOpen: false });
    rerender({ isOpen: true });
    await waitFor(() =>
      expect(mockedSSE.mock.calls.at(-1)?.[0].enabled).toBe(true),
    );
  });

  it("ignores a queued fallback callback after close", async () => {
    const options = baseOptions();
    const { result, rerender } = renderHook(
      ({ isOpen }: { isOpen: boolean }) =>
        useAiWaiterTransport({ ...options, isOpen }),
      { initialProps: { isOpen: true } },
    );
    await waitFor(() => expect(mockedSSE).toHaveBeenCalled());
    const oldConnection = mockedSSE.mock.calls.at(-1)?.[0];
    rerender({ isOpen: false });
    act(() => oldConnection?.onFallback());
    expect(result.current.streamFellBack).toBe(false);
  });

  it("never exposes the previous table transcript during the next scope render", async () => {
    mockedAxios.get.mockResolvedValueOnce({
      data: [historyAssistant(1, v2("table-one"))],
    });
    const observed: string[][] = [];
    const options = baseOptions();
    const { result, rerender } = renderHook(
      ({ tableCode }: { tableCode: string }) => {
        const value = useAiWaiterTransport({ ...options, tableCode });
        observed.push(value.messages.map(({ content }) => content));
        return value;
      },
      { initialProps: { tableCode: "T1" } },
    );
    await waitFor(() => expect(result.current.messages).toHaveLength(1));
    rerender({ tableCode: "T2" });
    expect(observed.at(-1)).toEqual([]);
    expect(result.current.messages).toEqual([]);
    await waitFor(() => expect(mockedAxios.get).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(result.current.isPolling).toBe(false));
  });

  it("overlaps the whole-second poll cursor, skips overlapping polls, and dedupes the overlap", async () => {
    jest.useFakeTimers();
    const options = baseOptions();
    const { result } = renderHook(() => useAiWaiterTransport(options));
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    act(() => {
      mockedSSE.mock.calls.at(-1)?.[0].onMessage({
        id: 41,
        role: "assistant",
        content: "first",
        createdAt: 100,
        response_v2: v2("stream-41", "first"),
      } as never);
      mockedSSE.mock.calls.at(-1)?.[0].onFallback();
    });
    mockedAxios.get.mockClear();
    const delta = deferred<{ data: unknown[] }>();
    mockedAxios.get.mockReturnValueOnce(delta.promise);

    await act(async () => {
      jest.advanceTimersByTime(6_000);
      await Promise.resolve();
    });
    expect(mockedAxios.get).toHaveBeenCalledTimes(1);
    expect(paramsOf(mockedAxios.get.mock.calls[0][1])?.since).toBe(99);

    await act(async () =>
      delta.resolve({
        data: [
          historyAssistant(41, v2("stream-41", "first")),
          historyAssistant(42, v2("poll-42", "same second")),
        ].map((row) => ({ ...row, created_at: 100 })),
      }),
    );
    expect(result.current.messages.map(({ id }) => id)).toEqual([41, 42]);
    expect(result.current.messages.find(({ id }) => id === 41)?.delivery).toBe(
      "stream",
    );
    expect(result.current.messages.find(({ id }) => id === 42)?.delivery).toBe(
      "poll",
    );
  });

  it("derives the poll cursor only from server frames, including a deduped direct echo", async () => {
    jest.useFakeTimers();
    jest.setSystemTime(new Date(10_000_000 * 1000));
    mockedAxios.get
      .mockResolvedValueOnce({
        data: [{ ...historyAssistant(40, v2("history-40")), created_at: 100 }],
      })
      .mockResolvedValue({ data: [] });
    mockedAxios.post.mockResolvedValueOnce({
      data: { id: 50, role: "model", parts: [], response_v2: v2("direct-50") },
    });
    const options = baseOptions();
    const { result } = renderHook(() => useAiWaiterTransport(options));
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    await act(async () => void (await result.current.sendMessage("hello")));

    act(() => mockedSSE.mock.calls.at(-1)?.[0].onFallback());
    await act(async () => {
      jest.advanceTimersByTime(3_000);
      await Promise.resolve();
    });
    const firstDelta = mockedAxios.get.mock.calls.at(-1)?.[1];
    expect(paramsOf(firstDelta)?.since).toBe(99);

    act(() =>
      mockedSSE.mock.calls.at(-1)?.[0].onMessage({
        id: 50,
        role: "assistant",
        content: "Grounded answer",
        createdAt: 150,
        response_v2: v2("direct-50"),
      } as never),
    );
    mockedAxios.get.mockClear();
    await act(async () => {
      jest.advanceTimersByTime(3_000);
      await Promise.resolve();
    });
    expect(paramsOf(mockedAxios.get.mock.calls[0][1])?.since).toBe(149);
    expect(result.current.messages.filter(({ id }) => id === 50)).toHaveLength(
      1,
    );
  });

  it("keeps the current turn when the guest clock is behind a full 100-row history", async () => {
    jest.useFakeTimers();
    jest.setSystemTime(new Date(1_000));
    mockedAxios.get.mockResolvedValueOnce({
      data: Array.from({ length: 100 }, (_, index) => ({
        ...historyAssistant(index + 1, v2(`history-${index + 1}`)),
        created_at: 1_000 + index,
      })),
    });
    mockedAxios.post.mockResolvedValueOnce({
      data: {
        id: 101,
        role: "model",
        parts: [],
        response_v2: v2("direct-101"),
      },
    });
    const options = baseOptions();
    const { result } = renderHook(() => useAiWaiterTransport(options));
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    await act(
      async () => void (await result.current.sendMessage("current prompt")),
    );

    expect(result.current.messages).toHaveLength(100);
    expect(
      result.current.messages.some(
        ({ role, content }) => role === "user" && content === "current prompt",
      ),
    ).toBe(true);
    expect(result.current.messages.at(-1)?.response?.response_id).toBe(
      "direct-101",
    );
  });

  it("keeps a later staff SSE message after a clock-ahead current turn", async () => {
    jest.useFakeTimers();
    jest.setSystemTime(new Date(10_000_000 * 1000));
    mockedAxios.post.mockResolvedValueOnce({
      data: { id: 50, role: "model", parts: [], response_v2: v2("direct-50") },
    });
    const options = baseOptions();
    const { result } = renderHook(() => useAiWaiterTransport(options));
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    await act(
      async () => void (await result.current.sendMessage("current prompt")),
    );
    act(() =>
      mockedSSE.mock.calls.at(-1)?.[0].onMessage({
        id: 51,
        role: "assistant",
        content: "Staff reply",
        createdAt: 200,
        response_v2: v2("staff-51", "Staff reply"),
      } as never),
    );
    expect(result.current.messages.at(-1)?.content).toBe("Staff reply");
  });

  it.each([
    ["forward", ["first", "second"]],
    ["reverse", ["second", "first"]],
  ])(
    "fails closed on conflicting duplicate IDs and nonces (%s)",
    async (_name, order) => {
      const rows = order.flatMap((content, index) => [
        historyAssistant(7, v2(`duplicate-${content}`, content)),
        {
          id: 20 + index,
          role: "user",
          content,
          created_at: 20 + index,
          client_nonce: "same-nonce",
        },
      ]);
      mockedAxios.get.mockResolvedValueOnce({ data: rows });
      const options = baseOptions();
      const { result } = renderHook(() => useAiWaiterTransport(options));

      await waitFor(() => expect(mockedAxios.get).toHaveBeenCalled());
      await waitFor(() => expect(result.current.isPolling).toBe(false));
      expect(result.current.messages.some(({ id }) => id === 7)).toBe(false);
      expect(
        result.current.messages.some(
          ({ clientNonce }) => clientNonce === "same-nonce",
        ),
      ).toBe(false);
    },
  );

  it("bounds conflict tombstones while retaining the active conflict window", async () => {
    const conflicts = Array.from({ length: 205 }, (_, offset) => {
      const id = offset + 1;
      return [
        historyAssistant(id, v2(`conflict-${id}-a`, "A")),
        historyAssistant(id, v2(`conflict-${id}-b`, "B")),
      ];
    }).flat();
    mockedAxios.get.mockResolvedValueOnce({ data: conflicts });
    const options = baseOptions();
    const { result } = renderHook(() => useAiWaiterTransport(options));
    await waitFor(() => expect(mockedAxios.get).toHaveBeenCalled());
    await waitFor(() => expect(result.current.isPolling).toBe(false));
    expect(result.current.messages).toEqual([]);

    act(() =>
      mockedSSE.mock.calls.at(-1)?.[0].onMessage({
        id: 1,
        role: "assistant",
        content: "outside active tombstone window",
        createdAt: 500,
        response_v2: v2("after-cap-1", "outside active tombstone window"),
      } as never),
    );
    act(() =>
      mockedSSE.mock.calls.at(-1)?.[0].onMessage({
        id: 205,
        role: "assistant",
        content: "recent conflict remains blocked",
        createdAt: 501,
        response_v2: v2("after-cap-205", "recent conflict remains blocked"),
      } as never),
    );
    expect(result.current.messages.map(({ id }) => id)).toEqual([1]);
  });

  it("rejects conflicting timestamp/nonce aliases and malformed assistant SSE V2", async () => {
    mockedAxios.get.mockResolvedValueOnce({
      data: [
        {
          ...historyAssistant(1, v2("alias-time")),
          created_at: 1,
          createdAt: 2,
        },
        {
          id: 2,
          role: "user",
          content: "nonce aliases",
          created_at: 2,
          client_nonce: "one",
          clientNonce: "two",
        },
      ],
    });
    const options = baseOptions();
    const { result } = renderHook(() => useAiWaiterTransport(options));
    await waitFor(() => expect(mockedAxios.get).toHaveBeenCalled());
    await waitFor(() => expect(result.current.isPolling).toBe(false));
    expect(result.current.messages).toEqual([]);

    act(() =>
      mockedSSE.mock.calls.at(-1)?.[0].onMessage({
        id: 3,
        role: "assistant",
        content: "legacy must not win",
        createdAt: 3,
        response_v2: { ...v2("bad-stream"), unknown: true },
      } as never),
    );
    expect(result.current.messages).toEqual([]);

    act(() =>
      mockedSSE.mock.calls.at(-1)?.[0].onMessage({
        id: 4,
        role: "user",
        content: "user must not carry V2",
        createdAt: 4,
        response_v2: v2("user-v2"),
      } as never),
    );
    expect(result.current.messages).toEqual([]);
  });

  it("rejects non-finite, fractional, nonpositive, and unsafe persisted identities and timestamps", async () => {
    const invalidNumbers = [
      Number.NaN,
      Number.POSITIVE_INFINITY,
      Number.NEGATIVE_INFINITY,
      1.5,
      0,
      -1,
      Number.MAX_SAFE_INTEGER + 1,
    ];
    mockedAxios.get.mockResolvedValueOnce({
      data: [
        ...invalidNumbers.map((id) => ({
          id,
          role: "assistant",
          content: "bad id",
          created_at: 1,
          response_v2: v2(`bad-id-${String(id)}`),
        })),
        ...invalidNumbers.map((created_at, index) => ({
          id: index + 1,
          role: "assistant",
          content: "bad timestamp",
          created_at,
          response_v2: v2(`bad-time-${index}`),
        })),
      ],
    });
    const options = baseOptions();
    const { result } = renderHook(() => useAiWaiterTransport(options));
    await waitFor(() => expect(mockedAxios.get).toHaveBeenCalled());
    await waitFor(() => expect(result.current.isPolling).toBe(false));
    expect(result.current.messages).toEqual([]);

    for (const [index, invalid] of invalidNumbers.entries()) {
      act(() =>
        mockedSSE.mock.calls.at(-1)?.[0].onMessage({
          id: invalid,
          role: "assistant",
          content: "bad stream id",
          createdAt: 10,
          response_v2: v2(`stream-id-${index}`),
        } as never),
      );
      act(() =>
        mockedSSE.mock.calls.at(-1)?.[0].onMessage({
          id: 100 + index,
          role: "assistant",
          content: "bad stream timestamp",
          createdAt: invalid,
          response_v2: v2(`stream-time-${index}`),
        } as never),
      );
    }
    expect(result.current.messages).toEqual([]);
  });

  it("ignores queued SSE from the previous scope and refetches locale-sensitive history without changing bearer", async () => {
    const ensureSession = jest.fn().mockResolvedValue("same-token");
    const options = baseOptions({ ensureSession, sessionToken: "same-token" });
    const { result, rerender } = renderHook(
      ({ tableCode, language }: { tableCode: string; language: string }) =>
        useAiWaiterTransport({ ...options, tableCode, language }),
      { initialProps: { tableCode: "T1", language: "en" } },
    );
    await waitFor(() => expect(mockedAxios.get).toHaveBeenCalledTimes(1));
    const oldStream = mockedSSE.mock.calls.at(-1)?.[0];

    rerender({ tableCode: "T2", language: "en" });
    await waitFor(() => expect(mockedAxios.get).toHaveBeenCalledTimes(2));
    act(() =>
      oldStream?.onMessage({
        id: 9,
        role: "assistant",
        content: "old scope",
        createdAt: 9,
        response_v2: v2("old-scope"),
      } as never),
    );
    expect(result.current.messages).toEqual([]);

    rerender({ tableCode: "T2", language: "es" });
    await waitFor(() => expect(mockedAxios.get).toHaveBeenCalledTimes(3));
    expect(ensureSession).toHaveBeenCalledTimes(3);
    expect(mockedSSE.mock.calls.at(-1)?.[0].sessionToken).toBe("same-token");
  });

  it("ignores an older locale history response that resolves after the current locale", async () => {
    const english = deferred<{ data: unknown[] }>();
    const spanish = deferred<{ data: unknown[] }>();
    mockedAxios.get
      .mockReturnValueOnce(english.promise)
      .mockReturnValueOnce(spanish.promise);
    const ensureSession = jest.fn().mockResolvedValue("same-token");
    const options = baseOptions({ ensureSession, sessionToken: "same-token" });
    const { result, rerender } = renderHook(
      ({ language }: { language: string }) =>
        useAiWaiterTransport({ ...options, language }),
      { initialProps: { language: "en" } },
    );
    await waitFor(() => expect(mockedAxios.get).toHaveBeenCalledTimes(1));
    rerender({ language: "es" });
    await waitFor(() => expect(mockedAxios.get).toHaveBeenCalledTimes(2));

    await act(async () =>
      spanish.resolve({ data: [historyAssistant(2, v2("es-2", "Español"))] }),
    );
    expect(result.current.messages.map(({ content }) => content)).toEqual([
      "Español",
    ]);
    await act(async () =>
      english.resolve({ data: [historyAssistant(1, v2("en-1", "English"))] }),
    );
    expect(result.current.messages.map(({ content }) => content)).toEqual([
      "Español",
    ]);
    expect(ensureSession).toHaveBeenCalledTimes(2);
  });

  it.each(["direct-first", "stream-first"])(
    "projects a direct response exactly once and keeps it authoritative over its SSE echo (%s)",
    async (order) => {
      const direct = deferred<{ data: unknown }>();
      mockedAxios.post.mockReturnValueOnce(direct.promise);
      const project = jest.fn((response: AssistantResponse) => ({
        ...response,
        answer: {
          ...response.answer,
          content: `${response.answer.content} projected`,
        },
      }));
      const options = baseOptions({ projectAssistantResponse: project });
      const { result } = renderHook(() => useAiWaiterTransport(options));
      await waitFor(() => expect(mockedAxios.get).toHaveBeenCalled());

      let send!: ReturnType<typeof result.current.sendMessage>;
      act(() => {
        send = result.current.sendMessage("add it");
      });
      await waitFor(() => expect(mockedAxios.post).toHaveBeenCalledTimes(1));
      const echo = {
        id: 50,
        role: "assistant" as const,
        content: "Grounded answer",
        createdAt: 50,
        response_v2: v2("direct-50"),
      };
      if (order === "stream-first") {
        act(() => mockedSSE.mock.calls.at(-1)?.[0].onMessage(echo));
      }
      await act(async () =>
        direct.resolve({
          data: {
            id: 50,
            role: "model",
            parts: [],
            response_v2: v2("direct-50"),
          },
        }),
      );
      await send;
      if (order === "direct-first") {
        act(() => mockedSSE.mock.calls.at(-1)?.[0].onMessage(echo));
      }

      expect(project).toHaveBeenCalledTimes(1);
      expect(
        result.current.messages.filter(({ id }) => id === 50),
      ).toHaveLength(1);
      expect(result.current.messages.find(({ id }) => id === 50)?.content).toBe(
        "Grounded answer projected",
      );
    },
  );

  it("does not project or execute a direct response whose persisted ID was poisoned by conflicting transport frames", async () => {
    mockedAxios.get.mockResolvedValueOnce({
      data: [
        historyAssistant(80, v2("conflict-a", "A")),
        historyAssistant(80, v2("conflict-b", "B")),
      ],
    });
    mockedAxios.post.mockResolvedValueOnce({
      data: { id: 80, role: "model", parts: [], response_v2: v2("direct-80") },
    });
    const project = jest.fn((response: AssistantResponse) => response);
    const options = baseOptions({ projectAssistantResponse: project });
    const { result } = renderHook(() => useAiWaiterTransport(options));
    await waitFor(() => expect(mockedAxios.get).toHaveBeenCalled());
    await waitFor(() => expect(result.current.isPolling).toBe(false));

    await act(async () => void (await result.current.sendMessage("add it")));
    expect(project).not.toHaveBeenCalled();
    expect(result.current.messages.some(({ id }) => id === 80)).toBe(false);
  });

  it("does not log bearer-bearing Axios errors and does not downgrade invalid direct V2", async () => {
    const sentinel = "SENTINEL-SESSION-TOKEN";
    const consoleError = jest
      .spyOn(console, "error")
      .mockImplementation(() => {});
    const consoleWarn = jest
      .spyOn(console, "warn")
      .mockImplementation(() => {});
    mockedAxios.get.mockRejectedValueOnce({ config: { data: sentinel } });
    mockedAxios.post.mockRejectedValueOnce({ config: { data: sentinel } });
    const onRenderFallback = jest.fn();
    const options = baseOptions({ onRenderFallback });
    const { result } = renderHook(() => useAiWaiterTransport(options));
    await waitFor(() =>
      expect(result.current.lastError).toBe("history_unavailable"),
    );
    await act(async () => void (await result.current.sendMessage("hello")));
    expect(onRenderFallback).not.toHaveBeenCalled();

    const serializedLogs = JSON.stringify([
      ...consoleError.mock.calls,
      ...consoleWarn.mock.calls,
    ]);
    expect(serializedLogs).not.toContain(sentinel);
    consoleError.mockRestore();
    consoleWarn.mockRestore();

    mockedAxios.post.mockResolvedValueOnce({
      data: {
        id: 60,
        role: "model",
        parts: [{ text: "unsafe legacy" }],
        response_v2: { ...v2("bad-direct"), version: 1 },
      },
    });
    await act(async () => void (await result.current.sendMessage("again")));
    expect(onRenderFallback).toHaveBeenCalledTimes(1);
    expect(
      result.current.messages.some(
        ({ content }) => content === "unsafe legacy",
      ),
    ).toBe(false);
  });

  it("does not attribute a throwing projector or cart mutator to render fallback", async () => {
    mockedAxios.post.mockResolvedValueOnce({
      data: {
        id: 61,
        role: "model",
        parts: [],
        response_v2: v2("valid-before-projector"),
      },
    });
    const onRenderFallback = jest.fn();
    const projectAssistantResponse = jest.fn(() => {
      throw new Error("sentinel cart mutator failure");
    });
    const options = baseOptions({
      onRenderFallback,
      projectAssistantResponse,
    });
    const { result } = renderHook(() => useAiWaiterTransport(options));
    await waitFor(() => expect(mockedAxios.get).toHaveBeenCalled());

    await act(async () => void (await result.current.sendMessage("add it")));

    expect(projectAssistantResponse).toHaveBeenCalledTimes(1);
    expect(onRenderFallback).not.toHaveBeenCalled();
    expect(result.current.lastError).toBe("generic");
  });

  it("uses authoritative V2 copy for a human acknowledgement and ignores mismatching legacy parts", async () => {
    mockedAxios.post.mockResolvedValueOnce({
      data: {
        id: 91,
        role: "model",
        human_ack: true,
        parts: [{ text: "unsafe legacy acknowledgement" }],
        response_v2: v2("human-ack-91", "A staff member is helping you."),
      },
    });
    const options = baseOptions();
    const { result } = renderHook(() => useAiWaiterTransport(options));
    await waitFor(() => expect(mockedAxios.get).toHaveBeenCalled());

    let outcome:
      | Awaited<ReturnType<typeof result.current.sendMessage>>
      | undefined;
    await act(async () => {
      outcome = await result.current.sendMessage("help");
    });
    expect(outcome).toEqual({
      type: "human_ack",
      text: "A staff member is helping you.",
    });
    expect(JSON.stringify(outcome)).not.toContain(
      "unsafe legacy acknowledgement",
    );

    mockedAxios.post.mockResolvedValueOnce({
      data: {
        id: 92,
        role: "model",
        human_ack: true,
        parts: [{ text: "second unsafe legacy acknowledgement" }],
        response_v2: v2("human-ack-92", "   "),
      },
    });
    await act(async () => {
      outcome = await result.current.sendMessage("still helping?");
    });
    expect(outcome).toEqual({ type: "human_ack", text: "" });
    expect(JSON.stringify(outcome)).not.toContain("second unsafe");

    mockedAxios.post.mockResolvedValueOnce({
      data: {
        id: 93,
        human_ack: true,
        parts: [{ text: "x".repeat(13_000) }],
      },
    });
    await act(async () => {
      outcome = await result.current.sendMessage("legacy ack");
    });
    expect(outcome?.type).toBe("human_ack");
    if (outcome?.type === "human_ack") {
      expect(Array.from(outcome.text)).toHaveLength(12_000);
    }

    mockedAxios.post.mockResolvedValueOnce({
      data: {
        id: 94,
        role: "user",
        human_ack: true,
        parts: [{ text: "malformed unsafe acknowledgement" }],
      },
    });
    await act(async () => {
      outcome = await result.current.sendMessage("bad ack");
    });
    expect(outcome).toEqual({ type: "error", error: "generic" });
    expect(JSON.stringify(result.current.messages)).not.toContain(
      "malformed unsafe acknowledgement",
    );
  });

  it.each([
    ["unsafe id", { id: 0, parts: [{ text: "unsafe id ack" }] }],
    ["unsafe part", { id: 95, parts: [{ text: 7 }] }],
  ])(
    "fails closed on a malformed absent-V2 human acknowledgement (%s)",
    async (_case, raw) => {
      mockedAxios.post.mockResolvedValueOnce({
        data: { ...raw, role: "model", human_ack: true },
      });
      const options = baseOptions();
      const { result } = renderHook(() => useAiWaiterTransport(options));
      await waitFor(() => expect(mockedAxios.get).toHaveBeenCalled());

      let outcome:
        | Awaited<ReturnType<typeof result.current.sendMessage>>
        | undefined;
      await act(async () => {
        outcome = await result.current.sendMessage("help");
      });
      expect(outcome).toEqual({ type: "error", error: "generic" });
      expect(JSON.stringify(result.current.messages)).not.toContain("unsafe");
    },
  );

  const TRANSPORT_SCOPE = "7\u0000T1\u0000ordering\u0000en";
  const seedAssistant = (content: string, when: number) => ({
    role: "assistant" as const,
    content,
    createdAt: when,
    response: adaptLegacyResponse(`seed-${when}`, {
      answer: content,
      steps: [],
      actions: [],
      follow_ups: [],
    }),
    contractVersion: "v1" as const,
  });

  const seedTwoTurns = () => {
    writeStoredTranscript(
      TRANSPORT_SCOPE,
      [
        seedAssistant("Hi! I'm Sage — ask me anything about the menu.", 100),
        { role: "user", content: "what is good here?", createdAt: 110 },
        seedAssistant("The burger is excellent.", 120),
        { role: "user", content: "and a dessert?", createdAt: 130 },
        seedAssistant("The flan is wonderful.", 140),
      ],
      "Hi! I'm Sage — ask me anything about the menu.",
    );
  };

  const TWO_TURN_CONTENTS = [
    "what is good here?",
    "The burger is excellent.",
    "and a dessert?",
    "The flan is wonderful.",
  ];

  /**
   * Next.js SSR: `typeof window` is still defined in jsdom, but localStorage
   * is not. The store treats a throwing accessor as "no window".
   */
  const withNoWindowStorage = <T,>(run: () => T): T => {
    const descriptor = Object.getOwnPropertyDescriptor(
      window,
      "localStorage",
    );
    if (!descriptor) throw new Error("jsdom localStorage descriptor missing");
    const error = console.error;
    jest.spyOn(console, "error").mockImplementation((...args) => {
      if (
        String(args[0] ?? "").includes(
          "useLayoutEffect does nothing on the server",
        )
      ) {
        return;
      }
      error.apply(console, args);
    });
    Object.defineProperty(window, "localStorage", {
      configurable: true,
      get() {
        throw new Error("ssr-no-window");
      },
    });
    try {
      return run();
    } finally {
      Object.defineProperty(window, "localStorage", descriptor);
      (console.error as jest.Mock).mockRestore();
    }
  };

  it("restores two turns after a no-window first read, then a greeting must not clobber (#724)", async () => {
    seedTwoTurns();
    const pending = deferred<{ data: unknown }>();
    mockedAxios.get.mockReturnValueOnce(pending.promise as never);

    const paints: string[] = [];
    const TranscriptProbe = ({ isOpen }: { isOpen: boolean }) => {
      const { messages } = useAiWaiterTransport(baseOptions({ isOpen }));
      paints.push(messages.map((message) => message.content).join("||"));
      return (
        <div data-testid="transcript">
          {messages.map((message) => message.content).join("||")}
        </div>
      );
    };

    // SSR first paint: storage is unreachable, so a useState initializer
    // that reads here hydrates empty and never re-runs.
    const html = withNoWindowStorage(() =>
      renderToString(<TranscriptProbe isOpen />),
    );
    expect(html).not.toContain("The burger is excellent.");
    expect(html).not.toContain("The flan is wonderful.");
    expect(paints[0] ?? "").not.toContain("The burger is excellent.");

    const container = document.createElement("div");
    container.innerHTML = html;
    document.body.appendChild(container);
    let root: Root | undefined;
    try {
      paints.length = 0;
      // Client hydrate WITH localStorage. A lazy useState initializer
      // would restore on this first call and paints[0] would contain
      // the dinner. Restore must be a later layout-effect paint.
      await act(async () => {
        root = hydrateRoot(container, <TranscriptProbe isOpen />);
      });
      expect(paints[0] ?? "").not.toContain("The burger is excellent.");
      expect(paints[0] ?? "").not.toContain("The flan is wonderful.");
      expect(
        paints.some((paint) => paint.includes("The burger is excellent.")),
      ).toBe(true);
      expect(container.textContent).toContain("The burger is excellent.");
      expect(container.textContent).toContain("The flan is wonderful.");

      pending.resolve({
        data: [
          historyAssistant(
            1,
            v2("greet", "Hi! I'm Sage — ask me anything about the menu."),
          ),
        ],
      });
      await waitFor(() => expect(mockedAxios.get).toHaveBeenCalled());
      await waitFor(() =>
        expect(container.textContent).toContain("The burger is excellent."),
      );
      expect(container.textContent).toContain("The flan is wonderful.");
      expect(
        readStoredTranscript(TRANSPORT_SCOPE).messages.map(
          (message) => message.content,
        ),
      ).toEqual(expect.arrayContaining(TWO_TURN_CONTENTS));
    } finally {
      await act(async () => {
        root?.unmount();
      });
      container.remove();
    }
  });
});
