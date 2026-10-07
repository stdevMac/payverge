import { getPublicConfig } from "@/config/publicConfig";
import { useEffect, useRef } from "react";

export interface StreamMessage {
  id: number;
  role: "user" | "assistant";
  content: string;
  createdAt: number;
}

export interface UseSSEMessagesArgs {
  businessId: number;
  sessionToken: string;
  enabled: boolean;
  onMessage: (msg: StreamMessage) => void;
  onFallback: () => void;
}

const MAX_BACKOFF_MS = 30_000;
const FALLBACK_AFTER_ERRORS = 2;
const MAX_EVENT_BUFFER = 128_000;
const STABLE_STREAM_MS = 10_000;

type ParsedEvent = { event: string; data: string };

const nextEventBlock = (
  buffer: string,
): { block: string; rest: string } | null => {
  const match = /\r\n\r\n|\n\n|\r\r/.exec(buffer);
  if (!match || match.index === undefined) return null;
  return {
    block: buffer.slice(0, match.index),
    rest: buffer.slice(match.index + match[0].length),
  };
};

const parseEventBlock = (block: string): ParsedEvent => {
  let event = "message";
  const data: string[] = [];
  for (const line of block.split(/\r\n|\r|\n/)) {
    if (line === "" || line.startsWith(":")) continue;
    const separator = line.indexOf(":");
    const field = separator < 0 ? line : line.slice(0, separator);
    let value = separator < 0 ? "" : line.slice(separator + 1);
    if (value.startsWith(" ")) value = value.slice(1);
    if (field === "event") event = value;
    if (field === "data") data.push(value);
  }
  return { event, data: data.join("\n") };
};

export function useSSEMessages({
  businessId,
  sessionToken,
  enabled,
  onMessage,
  onFallback,
}: UseSSEMessagesArgs): void {
  const onMessageRef = useRef(onMessage);
  const onFallbackRef = useRef(onFallback);
  onMessageRef.current = onMessage;
  onFallbackRef.current = onFallback;

  useEffect(() => {
    if (!enabled || !sessionToken || !businessId) return;

    let controller: AbortController | null = null;
    let reader: ReadableStreamDefaultReader<Uint8Array> | null = null;
    let retries = 0;
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
    let stableTimer: ReturnType<typeof setTimeout> | null = null;
    let stopped = false;
    let fellBack = false;

    const base = getPublicConfig().apiUrl;
    const url = `${base}/ai-waiter/${businessId}/stream`;

    const clearTimers = () => {
      if (reconnectTimer) clearTimeout(reconnectTimer);
      if (stableTimer) clearTimeout(stableTimer);
      reconnectTimer = null;
      stableTimer = null;
    };

    const cancelReader = () => {
      const active = reader;
      reader = null;
      if (active) void active.cancel().catch(() => undefined);
    };

    const abortActive = () => {
      controller?.abort();
      controller = null;
      cancelReader();
    };

    const fallback = () => {
      stopped = true;
      clearTimers();
      abortActive();
      if (!fellBack) {
        fellBack = true;
        onFallbackRef.current();
      }
    };

    const dispatch = (block: string) => {
      const parsed = parseEventBlock(block);
      if (parsed.event !== "message.created" || parsed.data === "") return;
      try {
        const message = JSON.parse(parsed.data) as StreamMessage;
        if (stopped) return;
        retries = 0;
        onMessageRef.current(message);
      } catch {
        // Malformed frames are isolated; a later valid event stays readable.
      }
    };

    const readStream = async (signal: AbortSignal) => {
      if (typeof fetch !== "function") {
        throw new Error("fetch unavailable");
      }
      const response = await fetch(url, {
        method: "GET",
        headers: {
          Accept: "text/event-stream",
          Authorization: `Bearer ${sessionToken}`,
        },
        credentials: "omit",
        redirect: "error",
        cache: "no-store",
        signal,
      });
      if (!response.ok) {
        await response.body?.cancel().catch(() => undefined);
        throw new Error("stream rejected");
      }
      if (!response.body) throw new Error("stream body unavailable");

      reader = response.body.getReader();
      stableTimer = setTimeout(() => {
        stableTimer = null;
        retries = 0;
      }, STABLE_STREAM_MS);
      const decoder = new TextDecoder();
      let buffer = "";
      while (!stopped && !signal.aborted) {
        const { done, value } = await reader.read();
        if (stopped || signal.aborted) return;
        if (done) {
          buffer += decoder.decode();
          throw new Error("stream ended");
        }
        buffer += decoder.decode(value, { stream: true });
        for (;;) {
          const next = nextEventBlock(buffer);
          if (!next) break;
          if (next.block.length > MAX_EVENT_BUFFER) {
            throw new Error("stream event too large");
          }
          buffer = next.rest;
          dispatch(next.block);
        }
        if (buffer.length > MAX_EVENT_BUFFER) {
          throw new Error("stream event too large");
        }
      }
    };

    const scheduleReconnect = () => {
      if (stopped) return;
      retries += 1;
      if (retries >= FALLBACK_AFTER_ERRORS) {
        fallback();
        return;
      }
      const ceiling = Math.min(1000 * 2 ** (retries - 1), MAX_BACKOFF_MS);
      const delay = ceiling / 2 + Math.random() * (ceiling / 2);
      reconnectTimer = setTimeout(() => {
        reconnectTimer = null;
        connect();
      }, delay);
    };

    const connect = () => {
      if (stopped) return;
      abortActive();
      if (stableTimer) clearTimeout(stableTimer);
      stableTimer = null;
      const nextController = new AbortController();
      controller = nextController;
      void readStream(nextController.signal).catch(() => {
        if (stopped || nextController.signal.aborted) return;
        cancelReader();
        controller = null;
        if (stableTimer) clearTimeout(stableTimer);
        stableTimer = null;
        scheduleReconnect();
      });
    };

    connect();

    return () => {
      stopped = true;
      clearTimers();
      abortActive();
    };
  }, [businessId, sessionToken, enabled]);
}
