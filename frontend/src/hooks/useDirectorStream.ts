import { useCallback, useRef, useState } from "react";

import type { DirectorProposedAction } from "@/api/directorConsole";

interface DirectorToolCallStarted {
  name: string;
  human_label?: string;
  args?: Record<string, unknown>;
}

interface DirectorToolCallCompleted {
  name: string;
  summary?: string;
  duration_ms?: number;
  success: boolean;
  error?: string;
}

export interface DirectorToolCallState extends DirectorToolCallStarted {
  /** True while the tool is running; flipped false on the matching completed event. */
  pending: boolean;
  /** Populated when the tool completes. */
  summary?: string;
  duration_ms?: number;
  success?: boolean;
  error?: string;
}

/**
 * Presentational shape consumed by `ToolTrace`. Producers (e.g. the Director
 * Console dashboard) flatten `DirectorToolCallState` into this view-model with
 * an explicit `state` discriminator and stable `id` per call.
 */
export interface ToolCallEvent {
  id: string;
  name: string;
  human_label: string;
  args: Record<string, unknown>;
  state: "running" | "done" | "error";
  summary?: string;
  duration_ms?: number;
  error?: string;
}

interface DirectorStreamComplete {
  message_id: number;
  thread_id: number;
  latency_ms?: number;
  model?: string;
  /** Staged write proposals (Phase 5 proposal card); server-truth DTOs. */
  proposed_actions?: DirectorProposedAction[] | null;
}

interface DirectorStreamError {
  message: string;
  code?: string;
}

interface UseDirectorStreamState {
  toolCalls: DirectorToolCallState[];
  streaming: boolean;
  complete: DirectorStreamComplete | null;
  error: DirectorStreamError | null;
  aborted: boolean;
}

interface StartArgs {
  url: string;
  body: Record<string, unknown>;
  headers?: Record<string, string>;
}

export interface UseDirectorStreamApi extends UseDirectorStreamState {
  start: (args: StartArgs) => Promise<void>;
  abort: () => void;
  reset: () => void;
}

const INITIAL_STATE: UseDirectorStreamState = {
  toolCalls: [],
  streaming: false,
  complete: null,
  error: null,
  aborted: false,
};

interface ParsedEvent {
  event: string;
  data: string;
}

/**
 * Parse a raw SSE block (separated by a blank line) into a {event, data} pair.
 * Lines starting with ":" are SSE comments / heartbeats and are skipped here.
 */
function parseSSEBlock(block: string): ParsedEvent | null {
  const lines = block.split(/\r?\n/);
  let event = "message";
  const dataLines: string[] = [];
  for (const line of lines) {
    if (!line) continue;
    if (line.startsWith(":")) continue; // heartbeat / comment
    if (line.startsWith("event:")) {
      event = line.slice(6).trim();
    } else if (line.startsWith("data:")) {
      dataLines.push(line.slice(5).trimStart());
    }
  }
  if (!dataLines.length) return null;
  return { event, data: dataLines.join("\n") };
}

/**
 * useDirectorStream — consumes the Director Console `/ask/stream` SSE endpoint
 * via a manual fetch + ReadableStream pipeline.
 *
 * We use fetch instead of EventSource because EventSource is GET-only and the
 * ask payload is delivered as a POST body. Cookies are forwarded for auth via
 * `credentials: "include"`.
 *
 * Exposes parsed tool-call lifecycle events, the final response.complete
 * payload, an error channel, and an abort handle.
 */
export function useDirectorStream(): UseDirectorStreamApi {
  const [state, setState] = useState<UseDirectorStreamState>(INITIAL_STATE);
  const controllerRef = useRef<AbortController | null>(null);

  const reset = useCallback(() => {
    setState(INITIAL_STATE);
  }, []);

  const abort = useCallback(() => {
    controllerRef.current?.abort();
    controllerRef.current = null;
    setState((prev) => ({ ...prev, streaming: false, aborted: true }));
  }, []);

  const handleEvent = useCallback((evt: ParsedEvent) => {
    let payload: unknown;
    try {
      payload = JSON.parse(evt.data);
    } catch {
      return;
    }

    if (evt.event === "tool.call.started") {
      const p = payload as DirectorToolCallStarted;
      setState((prev) => ({
        ...prev,
        toolCalls: [
          ...prev.toolCalls,
          {
            name: p.name,
            human_label: p.human_label,
            args: p.args,
            pending: true,
          },
        ],
      }));
      return;
    }

    if (evt.event === "tool.call.completed") {
      const p = payload as DirectorToolCallCompleted;
      setState((prev) => {
        // Match the most recent pending entry with the same name. If none is
        // pending, append a synthetic completed entry so the trace is still
        // visible to the user.
        const next = [...prev.toolCalls];
        for (let i = next.length - 1; i >= 0; i--) {
          if (next[i].name === p.name && next[i].pending) {
            next[i] = {
              ...next[i],
              pending: false,
              summary: p.summary,
              duration_ms: p.duration_ms,
              success: p.success,
              error: p.error,
            };
            return { ...prev, toolCalls: next };
          }
        }
        next.push({
          name: p.name,
          pending: false,
          summary: p.summary,
          duration_ms: p.duration_ms,
          success: p.success,
          error: p.error,
        });
        return { ...prev, toolCalls: next };
      });
      return;
    }

    if (evt.event === "response.complete") {
      const p = payload as DirectorStreamComplete;
      setState((prev) => ({
        ...prev,
        complete: p,
        streaming: false,
      }));
      return;
    }

    if (evt.event === "error") {
      const p = payload as DirectorStreamError;
      setState((prev) => ({
        ...prev,
        error: p,
        streaming: false,
      }));
      return;
    }
  }, []);

  const start = useCallback(
    async ({ url, body, headers }: StartArgs) => {
      // Reset on each start so consumers don't see stale state.
      setState({ ...INITIAL_STATE, streaming: true });

      const controller = new AbortController();
      controllerRef.current = controller;

      let response: Response;
      try {
        response = await fetch(url, {
          method: "POST",
          credentials: "include",
          headers: {
            "Content-Type": "application/json",
            Accept: "text/event-stream",
            ...(headers ?? {}),
          },
          body: JSON.stringify(body),
          signal: controller.signal,
        });
      } catch (err) {
        if ((err as { name?: string })?.name === "AbortError") {
          setState((prev) => ({ ...prev, streaming: false, aborted: true }));
          return;
        }
        setState((prev) => ({
          ...prev,
          streaming: false,
          error: { message: (err as Error)?.message ?? "fetch failed" },
        }));
        return;
      }

      if (!response.ok || !response.body) {
        setState((prev) => ({
          ...prev,
          streaming: false,
          error: {
            message: `stream HTTP ${response.status}`,
            code: String(response.status),
          },
        }));
        return;
      }

      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let buffer = "";

      try {
        // Read frames until the stream closes or is aborted.
        // SSE frames are delimited by a blank line ("\n\n").
        while (true) {
          const { done, value } = await reader.read();
          if (done) break;
          buffer += decoder.decode(value, { stream: true });
          let sepIdx = buffer.indexOf("\n\n");
          while (sepIdx !== -1) {
            const block = buffer.slice(0, sepIdx);
            buffer = buffer.slice(sepIdx + 2);
            const parsed = parseSSEBlock(block);
            if (parsed) handleEvent(parsed);
            sepIdx = buffer.indexOf("\n\n");
          }
        }
        // Flush any trailing partial block (rare; server usually closes after
        // response.complete which ends with \n\n).
        if (buffer.trim().length > 0) {
          const parsed = parseSSEBlock(buffer);
          if (parsed) handleEvent(parsed);
        }
      } catch (err) {
        if ((err as { name?: string })?.name === "AbortError") {
          setState((prev) => ({ ...prev, streaming: false, aborted: true }));
          return;
        }
        setState((prev) => ({
          ...prev,
          streaming: false,
          error: { message: (err as Error)?.message ?? "stream read failed" },
        }));
        return;
      } finally {
        controllerRef.current = null;
      }

      // If the loop exits without a response.complete or error event, mark
      // streaming as finished. Otherwise the event handlers have already
      // cleared the flag.
      setState((prev) => (prev.streaming ? { ...prev, streaming: false } : prev));
    },
    [handleEvent],
  );

  return {
    ...state,
    start,
    abort,
    reset,
  };
}
