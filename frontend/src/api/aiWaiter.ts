import { axiosInstance } from "./tools/instance";
import {
  adaptLegacyResponse,
  parseAssistantResponse,
  type AssistantResponse,
} from "@/types/assistant";

export interface AiWaiterResponsePart {
  text?: string;
  Text?: string;
  function_call?: unknown;
  functionCall?: unknown;
  FunctionCall?: unknown;
}

export interface AiWaiterChatResponse {
  id?: number;
  role: "model";
  parts: AiWaiterResponsePart[];
  response_v2: AssistantResponse;
  human_ack?: boolean;
  is_paused?: boolean;
}

class AiWaiterResponseContractMismatchError extends Error {
  readonly code = "assistant_response_contract_mismatch" as const;

  constructor() {
    super("assistant_response_contract_mismatch");
    this.name = "AiWaiterResponseContractMismatchError";
  }
}

let legacyResponseSequence = 0;

const hasOwn = (value: object, key: PropertyKey): boolean =>
  Object.prototype.hasOwnProperty.call(value, key);

const parsePart = (raw: unknown): AiWaiterResponsePart => {
  if (typeof raw !== "object" || raw === null || Array.isArray(raw)) {
    throw new Error("invalid_ai_waiter_response_part");
  }
  const part = raw as Record<string, unknown>;
  for (const key of ["text", "Text"] as const) {
    if (part[key] !== undefined && typeof part[key] !== "string") {
      throw new Error("invalid_ai_waiter_response_part");
    }
  }
  return part as AiWaiterResponsePart;
};

const hasValidLegacyCartCall = (parts: AiWaiterResponsePart[]): boolean =>
  parts.some((part) => {
    const rawCall =
      part.function_call ?? part.functionCall ?? part.FunctionCall;
    if (
      typeof rawCall !== "object" ||
      rawCall === null ||
      Array.isArray(rawCall)
    ) {
      return false;
    }
    const call = rawCall as Record<string, unknown>;
    return (
      call.name === "add_to_cart" &&
      typeof call.args === "object" &&
      call.args !== null &&
      !Array.isArray(call.args)
    );
  });

/** Parse the direct waiter response, treating a present V2 payload as authoritative. */
export function parseAiWaiterChatResponse(raw: unknown): AiWaiterChatResponse {
  if (typeof raw !== "object" || raw === null || Array.isArray(raw)) {
    throw new Error("invalid_ai_waiter_response");
  }
  const data = raw as Record<string, unknown>;
  if (data.role !== "model" || !Array.isArray(data.parts)) {
    throw new Error("invalid_ai_waiter_response");
  }
  if (
    data.id !== undefined &&
    (typeof data.id !== "number" ||
      !Number.isSafeInteger(data.id) ||
      data.id <= 0)
  ) {
    throw new Error("invalid_ai_waiter_response_id");
  }
  if (data.human_ack !== undefined && typeof data.human_ack !== "boolean") {
    throw new Error("invalid_ai_waiter_response");
  }
  if (data.is_paused !== undefined && typeof data.is_paused !== "boolean") {
    throw new Error("invalid_ai_waiter_response");
  }

  const parts = data.parts.map(parsePart);
  let responseV2: AssistantResponse;
  if (hasOwn(data, "response_v2")) {
    // Presence is authoritative. Invalid V2 is never downgraded to model-owned
    // legacy text or calls.
    try {
      responseV2 = parseAssistantResponse(data.response_v2);
    } catch {
      throw new AiWaiterResponseContractMismatchError();
    }
  } else {
    const answer = parts.map((part) => part.text ?? part.Text ?? "").join("");
    if (answer.trim().length === 0 && !hasValidLegacyCartCall(parts)) {
      throw new Error("invalid_ai_waiter_legacy_response");
    }
    legacyResponseSequence += 1;
    responseV2 = adaptLegacyResponse(
      `waiter-legacy-${typeof data.id === "number" ? data.id : legacyResponseSequence}`,
      { answer, steps: [], actions: [], follow_ups: [] },
    );
  }

  return {
    ...(typeof data.id === "number" ? { id: data.id } : {}),
    role: "model",
    parts,
    response_v2: responseV2,
    ...(typeof data.human_ack === "boolean"
      ? { human_ack: data.human_ack }
      : {}),
    ...(typeof data.is_paused === "boolean"
      ? { is_paused: data.is_paused }
      : {}),
  };
}

export interface AiWaiterSession {
  session_token: string;
  greeting: string;
  expires_at: string;
}

export const AI_WAITER_SESSION_HEADER = "X-AI-Waiter-Session";

// #596 — waiter transport must be honest about failures:
//
// A waiter turn can legitimately take longer than the global 30s axios
// timeout (model budget ~20s + snapshot build + deterministic finalize), at
// which point axios aborts with no `response` and the global interceptor
// toasted "Couldn't reach the server. Check your connection and try again."
// — a fake network error while the guest's connection was fine.
//
// So waiter requests (a) get a timeout comfortably above the backend worst
// case and (b) set `_skipErrorToast` so transport/5xx failures surface only
// as the in-thread Sage error copy (errConnect / classifyChatError), never as
// the global connection toast.
export const AI_WAITER_CHAT_TIMEOUT_MS = 90000;

/** Shared axios config for waiter chat turns (long-running model calls). */
export const AI_WAITER_CHAT_REQUEST_CONFIG = {
  timeout: AI_WAITER_CHAT_TIMEOUT_MS,
  _skipErrorToast: true,
} as const;

export interface CreateSessionPayload {
  table_code: string;
  mode: "ordering" | "concierge";
  language: string;
  replace_session_token?: string;
}

export async function createAiWaiterSession(
  businessId: number,
  payload: CreateSessionPayload,
): Promise<AiWaiterSession> {
  const res = await axiosInstance.post<AiWaiterSession>(
    `/ai-waiter/${businessId}/session`,
    payload,
    // withCredentials lets the browser store the HttpOnly session cookie the
    // backend sets here; the SSE stream then authenticates from that cookie.
    // Session-create failures surface in the Sage thread, not the global toast.
    { withCredentials: true, _skipErrorToast: true },
  );
  return res.data;
}

/** Operator dashboard sandbox — never touches guest pv_ai_waiter_session. */
export async function createAiWaiterTestSession(
  businessId: number,
  payload: { language: string },
): Promise<AiWaiterSession> {
  const res = await axiosInstance.post<AiWaiterSession>(
    `/inside/businesses/${businessId}/ai/test-chat/session`,
    payload,
    { _skipErrorToast: true },
  );
  return res.data;
}

export async function sendAiWaiterTestChat(
  businessId: number,
  payload: {
    history: Array<{ role: string; content: string }>;
    language: string;
    session_token: string;
  },
): Promise<unknown> {
  const res = await axiosInstance.post(
    `/inside/businesses/${businessId}/ai/test-chat`,
    {
      ...payload,
      // Server forces concierge + the reserved sandbox table code. Do not send
      // an empty table_code — that 400s as "table required" on older ordering
      // defaults if the sandbox overwrite is missed.
      mode: "concierge",
    },
    AI_WAITER_CHAT_REQUEST_CONFIG,
  );
  return res.data;
}

export function isSessionUnknownError(err: unknown): boolean {
  const e = err as { response?: { status?: number; data?: { code?: string } } };
  // Backend H1 renamed the legacy "session_unknown" envelope code to the
  // canonical AUTH_SESSION_UNKNOWN (matches apiErrors.json). Accept both during
  // the deploy window so a stale backend still triggers session re-creation.
  const code = e?.response?.data?.code;
  return (
    e?.response?.status === 404 &&
    (code === "AUTH_SESSION_UNKNOWN" || code === "session_unknown")
  );
}

export function classifyChatError(
  err: unknown,
): "rate_limited" | "too_long" | "over_budget" | "session_unknown" | "generic" {
  const e = err as { response?: { status?: number; data?: { code?: string } } };
  const status = e?.response?.status;
  const code = e?.response?.data?.code;
  if (isSessionUnknownError(err)) return "session_unknown";
  // A 24h-expired session returns 401/"session_expired" (ai_waiter_handler.go).
  // The client recovery is identical to an unknown session — recreate it — so
  // fold it into "session_unknown" rather than surfacing a dead-end "trouble
  // connecting" error the guest can never escape.
  if (status === 401 && code === "session_expired") return "session_unknown";
  // The daily-budget backstop returns 429 + code "over_budget"
  // (ai_waiter_handler.go). Check the code BEFORE the bare 429 → rate_limited
  // rule, otherwise a budget-exhausted guest is told to "wait a moment and try
  // again" when retrying can't help until the budget window resets.
  if (code === "over_budget") return "over_budget";
  if (status === 429) return "rate_limited";
  if (status === 413 || code === "too_long") return "too_long";
  return "generic";
}
