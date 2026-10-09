import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import { axiosInstance } from "../../api";
import {
  AI_WAITER_CHAT_REQUEST_CONFIG,
  AI_WAITER_SESSION_HEADER,
  classifyChatError,
  parseAiWaiterChatResponse,
  type AiWaiterResponsePart,
} from "@/api/aiWaiter";
import {
  adaptLegacyResponse,
  parseAssistantResponse,
  type AssistantResponse,
} from "@/types/assistant";
import { useSSEMessages, type StreamMessage } from "@/hooks/useSSEMessages";
import {
  clearStoredTranscript,
  firstAssistantContent,
  readStoredTranscript,
  writeStoredTranscript,
} from "./aiWaiterTranscriptStore";

const MAX_UI_MESSAGES = 100;
const MAX_OUTBOUND_HISTORY = 40;
const MAX_MESSAGE_CONTENT = 12_000;
const POLL_INTERVAL_MS = 3_000;
const MAX_CONFLICT_TOMBSTONES = MAX_UI_MESSAGES * 2;

export interface AiWaiterTransportMessage {
  id?: number;
  role: "user" | "assistant";
  content: string;
  createdAt?: number;
  clientNonce?: string;
  response?: AssistantResponse;
  contractVersion?: "v1" | "v2";
  /** Internal delivery authority; direct POST projection wins over echoes. */
  delivery?: "optimistic" | "direct" | "history" | "poll" | "stream" | "client";
  /**
   * A client notice about the current session (a reset, a connection error).
   * It is part of no conversation, so it is never persisted and never replayed
   * into a restored transcript.
   */
  transient?: boolean;
}

type AiWaiterTransportError =
  | ReturnType<typeof classifyChatError>
  | "history_unavailable";

export type AiWaiterSendOutcome =
  | { type: "assistant"; message: AiWaiterTransportMessage }
  | { type: "human_ack"; text: string }
  | { type: "error"; error: AiWaiterTransportError }
  | { type: "ignored" }
  | { type: "stale" };

export interface UseAiWaiterTransportOptions {
  businessId: number;
  tableCode: string;
  mode: "ordering" | "concierge";
  language: string;
  isOpen: boolean;
  sessionToken: string | null;
  ensureSession: () => Promise<string>;
  billContext: string;
  projectAssistantResponse?: (
    response: AssistantResponse,
    context: {
      source: "v2" | "legacy";
      legacyParts: AiWaiterResponsePart[];
    },
  ) => AssistantResponse;
  onHistoryContractMismatch?: () => void;
  onRenderFallback?: () => void;
}

export interface UseAiWaiterTransportResult {
  messages: AiWaiterTransportMessage[];
  /**
   * The visible transcript was replayed from this browser, not resumed from
   * the server. The conversation behind it is brand new and the model has none
   * of these turns, so the panel owes the diner a notice saying so (#724).
   */
  resumedFromStorage: boolean;
  isLoading: boolean;
  isPolling: boolean;
  streamFellBack: boolean;
  lastError: AiWaiterTransportError | null;
  lastFailedInput: string | null;
  sendMessage: (content: string) => Promise<AiWaiterSendOutcome>;
  retryFailedMessage: () => Promise<AiWaiterSendOutcome>;
  replaceMessages: (messages: AiWaiterTransportMessage[]) => void;
  appendAssistantText: (
    content: string,
    status?: AssistantResponse["status"],
  ) => void;
  resetConversation: () => void;
}

type RawMessage = Record<string, unknown>;
type StreamFrame = StreamMessage & { response_v2?: unknown };

class BoundedTombstones<T> {
  private readonly values = new Set<T>();
  private readonly order: T[] = [];

  has(value: T): boolean {
    return this.values.has(value);
  }

  add(value: T): void {
    if (this.values.has(value)) return;
    this.values.add(value);
    this.order.push(value);
    if (this.order.length > MAX_CONFLICT_TOMBSTONES) {
      const expired = this.order.shift();
      if (expired !== undefined) this.values.delete(expired);
    }
  }
}

const hasOwn = (value: object, key: PropertyKey): boolean =>
  Object.prototype.hasOwnProperty.call(value, key);

const truncateCodePoints = (value: string, max: number): string =>
  Array.from(value).slice(0, max).join("");

const validID = (value: unknown): value is number =>
  typeof value === "number" && Number.isSafeInteger(value) && value > 0;

const validTimestamp = (value: unknown): value is number =>
  typeof value === "number" && Number.isSafeInteger(value) && value > 0;

const limitMessages = (
  messages: AiWaiterTransportMessage[],
): AiWaiterTransportMessage[] => messages.slice(-MAX_UI_MESSAGES);

const sortServerBatch = (
  messages: AiWaiterTransportMessage[],
): AiWaiterTransportMessage[] =>
  [...messages].sort((a, b) => {
    const byTime = (a.createdAt ?? 0) - (b.createdAt ?? 0);
    if (byTime !== 0) return byTime;
    return (a.id ?? 0) - (b.id ?? 0);
  });

const parseTransportMessage = (
  raw: unknown,
  delivery: "history" | "poll" | "stream",
  onContractMismatch?: (id: number) => void,
): AiWaiterTransportMessage | null => {
  if (typeof raw !== "object" || raw === null || Array.isArray(raw))
    return null;
  const value = raw as RawMessage;
  if (!validID(value.id)) return null;
  if (value.role !== "user" && value.role !== "assistant") return null;
  if (typeof value.content !== "string") return null;

  if (
    hasOwn(value, "created_at") &&
    hasOwn(value, "createdAt") &&
    value.created_at !== value.createdAt
  ) {
    return null;
  }
  const createdAt = value.created_at ?? value.createdAt;
  if (!validTimestamp(createdAt)) return null;
  if (
    hasOwn(value, "client_nonce") &&
    hasOwn(value, "clientNonce") &&
    value.client_nonce !== value.clientNonce
  ) {
    return null;
  }
  const clientNonce = value.client_nonce ?? value.clientNonce;
  if (clientNonce !== undefined && typeof clientNonce !== "string") return null;

  if (value.role === "user") {
    return {
      id: value.id,
      role: "user",
      content: truncateCodePoints(value.content, MAX_MESSAGE_CONTENT),
      createdAt,
      ...(typeof clientNonce === "string" ? { clientNonce } : {}),
      delivery,
    };
  }

  let response: AssistantResponse;
  let contractVersion: "v1" | "v2";
  if (hasOwn(value, "response_v2")) {
    try {
      response = parseAssistantResponse(value.response_v2);
    } catch {
      // A present structured payload is authoritative. Never downgrade corrupt
      // V2 to the legacy outer text.
      onContractMismatch?.(value.id);
      return null;
    }
    contractVersion = "v2";
  } else {
    if (value.content.trim().length === 0) return null;
    response = adaptLegacyResponse(`waiter-message-${value.id}`, {
      answer: value.content,
      steps: [],
      actions: [],
      follow_ups: [],
    });
    contractVersion = "v1";
  }

  return {
    id: value.id,
    role: "assistant",
    content: response.answer.content,
    createdAt,
    response,
    contractVersion,
    delivery,
  };
};

const normalizeDirectResponse = (raw: unknown): unknown => {
  if (typeof raw !== "object" || raw === null || Array.isArray(raw)) return raw;
  const value = raw as RawMessage;
  // During the compatibility window older servers omitted role on V1-only
  // responses. Present V2 remains strict and receives no such repair.
  if (!hasOwn(value, "response_v2") && value.role === undefined) {
    return { ...value, role: "model" };
  }
  return raw;
};

const messageSignature = (message: AiWaiterTransportMessage): string =>
  JSON.stringify({
    role: message.role,
    content: message.content,
    createdAt: message.createdAt,
    clientNonce: message.clientNonce,
    response: message.response,
  });

const mergeMessages = (
  previous: AiWaiterTransportMessage[],
  incoming: AiWaiterTransportMessage[],
  conflictingIDs: BoundedTombstones<number>,
  conflictingNonces: BoundedTombstones<string>,
  conflictingResponseIDs: BoundedTombstones<string>,
): AiWaiterTransportMessage[] => {
  const merged = [...previous];
  for (const message of incoming) {
    const responseID = message.response?.response_id;
    if (responseID) {
      if (conflictingResponseIDs.has(responseID)) continue;
      const responseConflictIndex = merged.findIndex(
        (candidate) =>
          candidate.response?.response_id === responseID &&
          candidate.id !== message.id,
      );
      if (responseConflictIndex >= 0) {
        const [removed] = merged.splice(responseConflictIndex, 1);
        if (removed.id !== undefined) conflictingIDs.add(removed.id);
        if (message.id !== undefined) conflictingIDs.add(message.id);
        conflictingResponseIDs.add(responseID);
        continue;
      }
    }
    if (message.id !== undefined) {
      if (conflictingIDs.has(message.id)) continue;
      const existingIndex = merged.findIndex(({ id }) => id === message.id);
      if (existingIndex >= 0) {
        const existing = merged[existingIndex];
        if (existing.delivery === "direct") continue;
        if (message.delivery === "direct") {
          merged[existingIndex] = message;
          continue;
        }
        if (messageSignature(existing) === messageSignature(message)) continue;
        merged.splice(existingIndex, 1);
        conflictingIDs.add(message.id);
        continue;
      }
    }

    if (message.clientNonce) {
      if (conflictingNonces.has(message.clientNonce)) continue;
      const existingIndex = merged.findIndex(
        ({ clientNonce }) => clientNonce === message.clientNonce,
      );
      if (existingIndex >= 0) {
        const existing = merged[existingIndex];
        if (
          existing.role === message.role &&
          existing.content === message.content
        ) {
          // Prefer the persisted identity over its optimistic copy.
          if (message.id !== undefined) merged[existingIndex] = message;
          continue;
        }
        merged.splice(existingIndex, 1);
        conflictingNonces.add(message.clientNonce);
        continue;
      }
    }

    // A persisted echo without a nonce still replaces its exact optimistic row.
    if (message.id !== undefined) {
      const optimisticIndex = merged.findIndex(
        (candidate) =>
          candidate.id === undefined &&
          candidate.role === message.role &&
          candidate.content === message.content,
      );
      if (optimisticIndex >= 0) merged.splice(optimisticIndex, 1);
    }
    merged.push(message);
  }
  return limitMessages(merged);
};

const mergeFullHistoryMessages = (
  previous: AiWaiterTransportMessage[],
  canonicalBatch: AiWaiterTransportMessage[],
  conflictingIDs: BoundedTombstones<number>,
  conflictingNonces: BoundedTombstones<string>,
  conflictingResponseIDs: BoundedTombstones<string>,
): AiWaiterTransportMessage[] => {
  if (canonicalBatch.length === 0) return previous;
  // Full history has no reliable request/response watermark and older rows can
  // repeat the current prompt. Keep local optimistic turns unless the normal
  // merge finds an exact nonempty nonce match; a bounded duplicate is safer
  // than erasing an unpersisted guest turn.
  //
  // Client notices (reset / new conversation) must stay ahead of the server
  // greeting. Putting the canonical batch first inverted "Starting a new
  // conversation." below the replacement hello.
  const prefix = previous.filter((message) => message.delivery === "client");
  const liveMessages = previous.filter(
    (message) =>
      message.delivery !== "history" && message.delivery !== "client",
  );
  return mergeMessages(
    [],
    [...prefix, ...canonicalBatch, ...liveMessages],
    conflictingIDs,
    conflictingNonces,
    conflictingResponseIDs,
  );
};

export function useAiWaiterTransport({
  businessId,
  tableCode,
  mode,
  language,
  isOpen,
  sessionToken,
  ensureSession,
  billContext,
  projectAssistantResponse,
  onHistoryContractMismatch,
  onRenderFallback,
}: UseAiWaiterTransportOptions): UseAiWaiterTransportResult {
  const scopeKey = `${businessId}\u0000${tableCode}\u0000${mode}\u0000${language || "en"}`;
  // The diner's own browser is the only durable home for this conversation:
  // the bearer token is memory-only, so a refresh always lands on a brand-new
  // server session (#724).
  //
  // Do NOT read localStorage in useState. Next.js still SSRs this client
  // hook; the initializer runs with no window, hydrates an empty snapshot,
  // and never re-runs. Opening Sage then writes the new session's greeting
  // over the stored two-turn dinner. Restore on the client in
  // useLayoutEffect, before the history fetch (a useEffect) can write.
  const [messageState, setMessageState] = useState<{
    scopeKey: string;
    messages: AiWaiterTransportMessage[];
  }>(() => ({ scopeKey, messages: [] }));
  const [resumedFromStorage, setResumedFromStorage] = useState(false);
  const [isLoading, setIsLoading] = useState(false);
  const [isPolling, setIsPolling] = useState(false);
  const [streamFellBack, setStreamFellBack] = useState(false);
  const [lastError, setLastError] = useState<AiWaiterTransportError | null>(
    null,
  );
  const [lastFailedInput, setLastFailedInput] = useState<string | null>(null);
  const messagesRef = useRef<AiWaiterTransportMessage[]>([]);
  // The greeting of a RESUMED conversation. Every new session persists its own
  // hello server-side, so the restored thread would otherwise gain a duplicate
  // hello on each refresh.
  const resumedGreetingRef = useRef<string | null>(null);
  const storedGreetingRef = useRef<string | null>(null);
  const storageHydratedRef = useRef(false);
  const serverTimestampRef = useRef(0);
  const historyInFlightRef = useRef<Promise<void> | null>(null);
  const deltaInFlightRef = useRef<Promise<void> | null>(null);
  const bootstrappedSessionRef = useRef<string | null>(null);
  const conflictingIDsRef = useRef(new BoundedTombstones<number>());
  const conflictingNoncesRef = useRef(new BoundedTombstones<string>());
  const conflictingResponseIDsRef = useRef(new BoundedTombstones<string>());
  const contractMismatchIDsRef = useRef(new BoundedTombstones<number>());
  const mountedRef = useRef(false);
  const generationRef = useRef(0);
  const scopeRef = useRef(scopeKey);
  const renderedScopeRef = useRef(scopeKey);
  const loadingRef = useRef(false);
  const openRef = useRef(isOpen);
  const failedTurnRef = useRef<{
    input: string;
    clientNonce: string;
  } | null>(null);
  const sessionTokenRef = useRef(sessionToken);
  openRef.current = isOpen;
  renderedScopeRef.current = scopeKey;
  sessionTokenRef.current = sessionToken;

  const updateMessages = useCallback(
    (
      next:
        | AiWaiterTransportMessage[]
        | ((
            previous: AiWaiterTransportMessage[],
          ) => AiWaiterTransportMessage[]),
    ) => {
      if (!mountedRef.current) return;
      const updated = limitMessages(
        typeof next === "function" ? next(messagesRef.current) : next,
      );
      messagesRef.current = updated;
      setMessageState({ scopeKey: scopeRef.current, messages: updated });
      storedGreetingRef.current =
        storedGreetingRef.current ?? firstAssistantContent(updated);
      // A write before the client re-read would persist the empty SSR
      // snapshot (or a greeting-only history fetch) and destroy the dinner.
      if (!storageHydratedRef.current) return;
      writeStoredTranscript(
        scopeRef.current,
        updated,
        storedGreetingRef.current,
      );
    },
    [],
  );

  useLayoutEffect(() => {
    mountedRef.current = true;
    const scoped = readStoredTranscript(scopeRef.current);
    resumedGreetingRef.current = scoped.greeting;
    storedGreetingRef.current = scoped.greeting;
    messagesRef.current = scoped.messages;
    setMessageState({
      scopeKey: scopeRef.current,
      messages: scoped.messages,
    });
    setResumedFromStorage(scoped.messages.length > 0);
    storageHydratedRef.current = true;
    return () => {
      mountedRef.current = false;
      storageHydratedRef.current = false;
      generationRef.current += 1;
      historyInFlightRef.current = null;
      deltaInFlightRef.current = null;
    };
  }, []);

  useEffect(() => {
    if (scopeRef.current === scopeKey) return;
    scopeRef.current = scopeKey;
    generationRef.current += 1;
    historyInFlightRef.current = null;
    deltaInFlightRef.current = null;
    bootstrappedSessionRef.current = null;
    conflictingIDsRef.current = new BoundedTombstones();
    conflictingNoncesRef.current = new BoundedTombstones();
    conflictingResponseIDsRef.current = new BoundedTombstones();
    contractMismatchIDsRef.current = new BoundedTombstones();
    const scoped = readStoredTranscript(scopeKey);
    resumedGreetingRef.current = scoped.greeting;
    storedGreetingRef.current = scoped.greeting;
    messagesRef.current = scoped.messages;
    storageHydratedRef.current = true;
    serverTimestampRef.current = 0;
    loadingRef.current = false;
    setMessageState({ scopeKey, messages: scoped.messages });
    setResumedFromStorage(scoped.messages.length > 0);
    setIsLoading(false);
    setIsPolling(false);
    setStreamFellBack(false);
    setLastError(null);
    setLastFailedInput(null);
  }, [scopeKey]);

  const isCurrent = useCallback(
    (generation: number, scope: string): boolean =>
      mountedRef.current &&
      generationRef.current === generation &&
      scopeRef.current === scope &&
      renderedScopeRef.current === scope,
    [],
  );

  const fetchMessagesImpl = useCallback(
    async (isDelta: boolean): Promise<void> => {
      const generation = generationRef.current;
      const scope = scopeRef.current;
      if (mountedRef.current) setIsPolling(true);
      try {
        const token = sessionTokenRef.current;
        const response = await axiosInstance.get(
          `/ai-waiter/${businessId}/messages`,
          {
            params: {
              since: isDelta
                ? Math.max(0, serverTimestampRef.current - 1)
                : undefined,
              mode,
              language: language || "en",
              table_code: tableCode || "",
              limit: MAX_UI_MESSAGES,
            },
            ...(token
              ? { headers: { [AI_WAITER_SESSION_HEADER]: token } }
              : {}),
            _useCache: false,
            // History failures surface in-thread ("history_unavailable"),
            // never as the global connection toast (#596).
            _skipErrorToast: true,
          },
        );
        if (!isCurrent(generation, scope) || !Array.isArray(response.data))
          return;
        const parsed = response.data.flatMap((raw: unknown) => {
          const message = parseTransportMessage(
            raw,
            isDelta ? "poll" : "history",
            (id) => {
              if (contractMismatchIDsRef.current.has(id)) return;
              contractMismatchIDsRef.current.add(id);
              onHistoryContractMismatch?.();
              onRenderFallback?.();
            },
          );
          return message === null ? [] : [message];
        });
        for (const message of parsed) {
          serverTimestampRef.current = Math.max(
            serverTimestampRef.current,
            message.createdAt ?? 0,
          );
        }
        // A resumed transcript already opens with this hello; the fresh
        // session saved an identical one, and appending it would stack one
        // more greeting under the conversation on every refresh. The
        // watermark above still advances past it.
        const resumedGreeting = resumedGreetingRef.current;
        const canonicalBatch = sortServerBatch(
          resumedGreeting === null
            ? parsed
            : parsed.filter(
                (message) =>
                  message.role !== "assistant" ||
                  message.content.trim() !== resumedGreeting,
              ),
        );
        updateMessages((previous) =>
          isDelta
            ? mergeMessages(
                previous,
                canonicalBatch,
                conflictingIDsRef.current,
                conflictingNoncesRef.current,
                conflictingResponseIDsRef.current,
              )
            : mergeFullHistoryMessages(
                previous,
                canonicalBatch,
                conflictingIDsRef.current,
                conflictingNoncesRef.current,
                conflictingResponseIDsRef.current,
              ),
        );
        if (!isDelta) setLastError(null);
      } catch (error) {
        if (isCurrent(generation, scope)) {
          if (!isDelta) setLastError("history_unavailable");
        }
      } finally {
        if (isCurrent(generation, scope)) setIsPolling(false);
      }
    },
    [
      businessId,
      isCurrent,
      language,
      mode,
      onHistoryContractMismatch,
      onRenderFallback,
      tableCode,
      updateMessages,
    ],
  );

  const fetchMessages = useCallback(
    async (isDelta: boolean): Promise<void> => {
      if (isDelta) {
        if (deltaInFlightRef.current) return deltaInFlightRef.current;
        const pending = fetchMessagesImpl(true).finally(() => {
          if (deltaInFlightRef.current === pending) {
            deltaInFlightRef.current = null;
          }
        });
        deltaInFlightRef.current = pending;
        return pending;
      }
      if (historyInFlightRef.current) return historyInFlightRef.current;
      const pending = fetchMessagesImpl(false).finally(() => {
        if (historyInFlightRef.current === pending) {
          historyInFlightRef.current = null;
        }
      });
      historyInFlightRef.current = pending;
      return pending;
    },
    [fetchMessagesImpl],
  );

  useEffect(() => {
    if (!isOpen) return;
    const generation = generationRef.current;
    const scope = scopeRef.current;
    void (async () => {
      try {
        const token = await ensureSession();
        if (!isCurrent(generation, scope)) return;
        sessionTokenRef.current = token;
        const bootstrapKey = `${scope}\u0000${token}`;
        if (bootstrappedSessionRef.current === bootstrapKey) return;
        bootstrappedSessionRef.current = bootstrapKey;
        await fetchMessages(false);
      } catch {
        // The session hook and the next explicit send own typed recovery. Never
        // log the raw rejection because Axios config can contain the bearer.
      }
    })();
  }, [ensureSession, fetchMessages, isCurrent, isOpen, sessionToken]);

  const appendStreamMessage = useCallback(
    (raw: StreamMessage) => {
      if (
        scopeRef.current !== scopeKey ||
        renderedScopeRef.current !== scopeKey ||
        !openRef.current
      ) {
        return;
      }
      const frame = raw as StreamFrame;
      if (frame.role !== "assistant") return;
      const message = parseTransportMessage(frame, "stream", (id) => {
        if (contractMismatchIDsRef.current.has(id)) return;
        contractMismatchIDsRef.current.add(id);
        onHistoryContractMismatch?.();
        onRenderFallback?.();
      });
      if (message === null) return;
      serverTimestampRef.current = Math.max(
        serverTimestampRef.current,
        message.createdAt ?? 0,
      );
      updateMessages((previous) =>
        mergeMessages(
          previous,
          [message],
          conflictingIDsRef.current,
          conflictingNoncesRef.current,
          conflictingResponseIDsRef.current,
        ),
      );
    },
    [onHistoryContractMismatch, onRenderFallback, scopeKey, updateMessages],
  );

  useSSEMessages({
    businessId,
    sessionToken: sessionToken ?? "",
    enabled: isOpen && !!sessionToken && !streamFellBack,
    onMessage: appendStreamMessage,
    onFallback: () => {
      if (
        scopeRef.current === scopeKey &&
        renderedScopeRef.current === scopeKey &&
        openRef.current
      ) {
        setStreamFellBack(true);
      }
    },
  });

  useEffect(() => {
    if (!isOpen) {
      bootstrappedSessionRef.current = null;
      setStreamFellBack(false);
    }
  }, [isOpen]);

  useEffect(() => {
    if (!isOpen || !streamFellBack) return;
    const interval = window.setInterval(() => {
      void fetchMessages(true);
    }, POLL_INTERVAL_MS);
    return () => window.clearInterval(interval);
  }, [fetchMessages, isOpen, streamFellBack]);

  const sendMessage = useCallback(
    async (
      content: string,
      retryNonce?: string,
    ): Promise<AiWaiterSendOutcome> => {
      const text = typeof content === "string" ? content : "";
      if (!text.trim() || loadingRef.current) return { type: "ignored" };

      const generation = generationRef.current;
      const scope = scopeRef.current;
      const clientNonce =
        retryNonce ?? `n-${Date.now()}-${Math.random().toString(36).slice(2)}`;
      const userMessage: AiWaiterTransportMessage = {
        role: "user",
        content: text,
        createdAt: Math.floor(Date.now() / 1000),
        clientNonce,
        delivery: "optimistic",
      };
      const outbound = [...messagesRef.current, userMessage].slice(
        -MAX_OUTBOUND_HISTORY,
      );
      updateMessages([...messagesRef.current, userMessage]);
      setLastError(null);
      setLastFailedInput(null);
      failedTurnRef.current = null;
      loadingRef.current = true;
      setIsLoading(true);

      try {
        const token = sessionToken || (await ensureSession());
        if (!isCurrent(generation, scope)) return { type: "stale" };
        const response = await axiosInstance.post(
          `/ai-waiter/${businessId}`,
          {
            history: outbound.map(({ role, content: messageContent }) => ({
              role,
              content: messageContent,
            })),
            language: language || "en",
            session_token: token,
            table_code: tableCode || "",
            bill_context: billContext,
            mode,
            client_nonce: clientNonce,
          },
          // #596 — a waiter turn (model budget + snapshot + finalize) can
          // exceed the global 30s axios timeout; that abort has no `response`
          // and used to toast a fake "Couldn't reach the server" while the
          // guest's connection was fine. Long timeout + skip the global toast:
          // failures surface only as the in-thread Sage error copy.
          AI_WAITER_CHAT_REQUEST_CONFIG,
        );
        if (!isCurrent(generation, scope)) return { type: "stale" };

        const raw = response.data as RawMessage;
        if (raw?.code === "over_budget") {
          setLastError("over_budget");
          return { type: "error", error: "over_budget" };
        }

        let parsed;
        try {
          parsed = parseAiWaiterChatResponse(normalizeDirectResponse(raw));
        } catch (error) {
          if (hasOwn(raw, "response_v2")) onRenderFallback?.();
          throw error;
        }
        if (raw?.human_ack === true) {
          return {
            type: "human_ack",
            text: parsed.response_v2.answer.content.trim(),
          };
        }
        if (
          parsed.id !== undefined &&
          conflictingIDsRef.current.has(parsed.id)
        ) {
          setLastError("generic");
          return { type: "error", error: "generic" };
        }
        if (
          conflictingResponseIDsRef.current.has(parsed.response_v2.response_id)
        ) {
          setLastError("generic");
          return { type: "error", error: "generic" };
        }
        const projectedRaw = projectAssistantResponse
          ? projectAssistantResponse(parsed.response_v2, {
              source: hasOwn(raw, "response_v2") ? "v2" : "legacy",
              legacyParts: parsed.parts,
            })
          : parsed.response_v2;
        let projected: AssistantResponse;
        try {
          projected = parseAssistantResponse(projectedRaw);
        } catch (error) {
          if (hasOwn(raw, "response_v2")) onRenderFallback?.();
          throw error;
        }
        if (!projected.answer.content.trim()) {
          throw new Error("invalid_ai_waiter_projected_response");
        }
        const assistantMessage: AiWaiterTransportMessage = {
          ...(parsed.id === undefined ? {} : { id: parsed.id }),
          role: "assistant",
          content: projected.answer.content,
          createdAt: Math.floor(Date.now() / 1000),
          response: projected,
          contractVersion: hasOwn(raw, "response_v2") ? "v2" : "v1",
          delivery: "direct",
        };
        updateMessages((previous) =>
          mergeMessages(
            previous,
            [assistantMessage],
            conflictingIDsRef.current,
            conflictingNoncesRef.current,
            conflictingResponseIDsRef.current,
          ),
        );
        return { type: "assistant", message: assistantMessage };
      } catch (error) {
        if (!isCurrent(generation, scope)) return { type: "stale" };
        const kind = classifyChatError(error);
        setLastError(kind);
        if (kind === "generic") {
          setLastFailedInput(text);
          failedTurnRef.current = { input: text, clientNonce };
        }
        return { type: "error", error: kind };
      } finally {
        if (isCurrent(generation, scope)) {
          loadingRef.current = false;
          setIsLoading(false);
        }
      }
    },
    [
      billContext,
      businessId,
      ensureSession,
      isCurrent,
      language,
      mode,
      onRenderFallback,
      projectAssistantResponse,
      sessionToken,
      tableCode,
      updateMessages,
    ],
  );

  const retryFailedMessage =
    useCallback(async (): Promise<AiWaiterSendOutcome> => {
      const failedTurn = failedTurnRef.current;
      if (
        !lastFailedInput ||
        !failedTurn ||
        failedTurn.input !== lastFailedInput
      ) {
        return { type: "ignored" };
      }
      updateMessages((previous) => {
        const index = [...previous]
          .map((message, position) => ({ message, position }))
          .reverse()
          .find(
            ({ message }) =>
              message.id === undefined &&
              message.role === "user" &&
              message.content === lastFailedInput,
          )?.position;
        if (index === undefined) return previous;
        return [...previous.slice(0, index), ...previous.slice(index + 1)];
      });
      return sendMessage(lastFailedInput, failedTurn.clientNonce);
    }, [lastFailedInput, sendMessage, updateMessages]);

  const replaceMessages = useCallback(
    (next: AiWaiterTransportMessage[]) => updateMessages(next),
    [updateMessages],
  );

  const appendAssistantText = useCallback(
    (content: string, status?: AssistantResponse["status"]) => {
      if (!content.trim()) return;
      const response = adaptLegacyResponse(`waiter-client-${Date.now()}`, {
        answer: content,
        steps: [],
        actions: [],
        follow_ups: [],
        status: status ?? "complete",
      });
      updateMessages((previous) => [
        ...previous,
        {
          role: "assistant",
          content: response.answer.content,
          createdAt: Math.floor(Date.now() / 1000),
          response,
          contractVersion: "v1",
          delivery: "client",
          // A notice about this session only — never stored, never replayed.
          transient: true,
        },
      ]);
    },
    [updateMessages],
  );

  const resetConversation = useCallback(() => {
    generationRef.current += 1;
    historyInFlightRef.current = null;
    deltaInFlightRef.current = null;
    bootstrappedSessionRef.current = null;
    conflictingIDsRef.current = new BoundedTombstones();
    conflictingNoncesRef.current = new BoundedTombstones();
    conflictingResponseIDsRef.current = new BoundedTombstones();
    contractMismatchIDsRef.current = new BoundedTombstones();
    messagesRef.current = [];
    serverTimestampRef.current = 0;
    loadingRef.current = false;
    failedTurnRef.current = null;
    // "New chat" is the diner asking to forget, on this device too.
    clearStoredTranscript(scopeRef.current);
    storageHydratedRef.current = true;
    resumedGreetingRef.current = null;
    storedGreetingRef.current = null;
    setMessageState({ scopeKey: scopeRef.current, messages: [] });
    setResumedFromStorage(false);
    setIsLoading(false);
    setIsPolling(false);
    setStreamFellBack(false);
    setLastError(null);
    setLastFailedInput(null);
  }, []);

  const scopeIsCurrent = messageState.scopeKey === scopeKey;

  return {
    messages: scopeIsCurrent ? messageState.messages : [],
    resumedFromStorage: scopeIsCurrent ? resumedFromStorage : false,
    isLoading: scopeIsCurrent ? isLoading : false,
    isPolling: scopeIsCurrent ? isPolling : false,
    streamFellBack: scopeIsCurrent ? streamFellBack : false,
    lastError: scopeIsCurrent ? lastError : null,
    lastFailedInput: scopeIsCurrent ? lastFailedInput : null,
    sendMessage,
    retryFailedMessage,
    replaceMessages,
    appendAssistantText,
    resetConversation,
  };
}
