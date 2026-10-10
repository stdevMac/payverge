import { axiosInstance } from "./tools/instance";
import type {
  ChatAction,
  ChatMessage,
  StructuredChatResponse,
} from "@/components/chat/types";
import {
  adaptLegacyResponse,
  parseAssistantResponse,
  type AssistantResponse,
} from "@/types/assistant";
import { randomUUID } from "@/lib/randomUUID";

export interface OpsAskResponse {
  thread: { id: number; title: string };
  assistant_message: { id: number; content: string };
  response: StructuredChatResponse;
  response_v2: AssistantResponse;
  usage: { model: string; latency_ms: number };
  contract_version: "v1" | "v2";
}

export interface OpsHistoryMessage {
  id: number;
  role: "user" | "assistant";
  content: string;
  structured_response?: StructuredChatResponse;
  response_v2?: AssistantResponse;
  contract_version?: "v1" | "v2";
}

export class OpsHistoryContractMismatchError extends Error {
  readonly code = "assistant_history_contract_mismatch" as const;

  constructor() {
    super("assistant_history_contract_mismatch");
    this.name = "OpsHistoryContractMismatchError";
  }
}

export class OpsResponseContractMismatchError extends Error {
  readonly code = "assistant_response_contract_mismatch" as const;

  constructor() {
    super("assistant_response_contract_mismatch");
    this.name = "OpsResponseContractMismatchError";
  }
}

const hasOwn = (value: object, key: PropertyKey): boolean =>
  Object.prototype.hasOwnProperty.call(value, key);

function isChatAction(value: unknown): value is ChatAction {
  if (typeof value !== "object" || value === null) return false;
  const action = value as Partial<ChatAction>;
  return (
    typeof action.label === "string" &&
    typeof action.href === "string" &&
    (action.kind === "navigate" ||
      action.kind === "external" ||
      action.kind === "handoff") &&
    (action.disabled === undefined || typeof action.disabled === "boolean") &&
    (action.disabled_reason === undefined ||
      typeof action.disabled_reason === "string")
  );
}

function isStructuredResponse(value: unknown): value is StructuredChatResponse {
  if (typeof value !== "object" || value === null) return false;
  const response = value as Partial<StructuredChatResponse>;
  return (
    typeof response.answer === "string" &&
    Array.isArray(response.steps) &&
    response.steps.every((step) => typeof step === "string") &&
    Array.isArray(response.actions) &&
    response.actions.every(isChatAction) &&
    Array.isArray(response.follow_ups) &&
    response.follow_ups.every((followUp) => typeof followUp === "string")
  );
}

function projectV2ForCompatibility(
  response: AssistantResponse,
): StructuredChatResponse {
  const actions = response.actions.flatMap<ChatAction>((action) => {
    let kind: ChatAction["kind"];
    if (action.type === "navigate") kind = "navigate";
    else if (action.type === "external_link") kind = "external";
    else if (action.type === "director_handoff") kind = "handoff";
    else return [];

    return [
      {
        label: action.label,
        href: action.target.href,
        kind,
        disabled: action.state !== "ready" || action.confirmation !== "none",
        disabled_reason: action.disabled_reason ?? undefined,
      },
    ];
  });
  return {
    answer: response.answer.content,
    steps: response.steps,
    actions,
    follow_ups: response.follow_ups.map(({ prompt }) => prompt),
  };
}

export async function askOpsAssistant(
  businessId: number,
  body: {
    message: string;
    thread_id?: number;
    locale?: string;
    active_tab?: string;
    client_request_id?: string;
  },
): Promise<OpsAskResponse> {
  const { data } = await axiosInstance.post<{
    thread: OpsAskResponse["thread"];
    assistant_message: OpsAskResponse["assistant_message"];
    response?: unknown;
    response_v2?: unknown;
    usage: OpsAskResponse["usage"];
  }>(`/inside/businesses/${businessId}/assistant/ask`, body);
  if (hasOwn(data, "response_v2")) {
    let responseV2: AssistantResponse;
    try {
      responseV2 = parseAssistantResponse(data.response_v2);
    } catch {
      throw new OpsResponseContractMismatchError();
    }
    return {
      ...data,
      response: isStructuredResponse(data.response)
        ? data.response
        : projectV2ForCompatibility(responseV2),
      response_v2: responseV2,
      contract_version: "v2",
    };
  }
  if (!isStructuredResponse(data.response)) {
    throw new Error("invalid_ops_response");
  }
  return {
    ...data,
    response: data.response,
    response_v2: adaptLegacyResponse(
      `ops-legacy-${data.assistant_message.id}`,
      data.response,
    ),
    contract_version: "v1",
  };
}

type OpsThreadMessagesPayload = {
  messages?: Array<{
    id?: unknown;
    role?: unknown;
    content?: unknown;
    structured_response?: unknown;
    response_v2?: unknown;
  }>;
};

export function parseOpsThreadMessages(data: OpsThreadMessagesPayload): {
  messages: OpsHistoryMessage[];
} {
  if (!Array.isArray(data.messages)) throw new Error("invalid_ops_history");
  return {
    messages: data.messages.map<OpsHistoryMessage>((message) => {
      if (
        typeof message.id !== "number" ||
        (message.role !== "user" && message.role !== "assistant") ||
        typeof message.content !== "string"
      ) {
        throw new Error("invalid_ops_history_message");
      }
      if (message.role !== "assistant") {
        return { id: message.id, role: message.role, content: message.content };
      }

      let responseV2: AssistantResponse;
      let contractVersion: "v1" | "v2";
      if (hasOwn(message, "response_v2")) {
        try {
          responseV2 = parseAssistantResponse(message.response_v2);
        } catch {
          throw new OpsHistoryContractMismatchError();
        }
        contractVersion = "v2";
      } else {
        const legacy = isStructuredResponse(message.structured_response)
          ? message.structured_response
          : { answer: message.content, steps: [], actions: [], follow_ups: [] };
        responseV2 = adaptLegacyResponse(`ops-history-${message.id}`, legacy);
        contractVersion = "v1";
      }
      return {
        id: message.id,
        role: message.role,
        content: responseV2.answer.content,
        structured_response: isStructuredResponse(message.structured_response)
          ? message.structured_response
          : projectV2ForCompatibility(responseV2),
        response_v2: responseV2,
        contract_version: contractVersion,
      };
    }),
  };
}

export async function listOpsThreadMessages(
  businessId: number,
  threadId: number,
) {
  const { data } = await axiosInstance.get<OpsThreadMessagesPayload>(
    `/inside/businesses/${businessId}/assistant/threads/${threadId}/messages`,
  );
  return parseOpsThreadMessages(data);
}

export async function submitOpsFeedback(
  businessId: number,
  messageId: number,
  feedback: "up" | "down",
) {
  const { data } = await axiosInstance.post(
    `/inside/businesses/${businessId}/assistant/messages/${messageId}/feedback`,
    { feedback },
  );
  return data;
}

/** Stable tab-scoped session key for resumable Ops conversations. */
function opsSessionKey(businessId: number) {
  return `payverge_ops_session_${businessId}`;
}

export function readOpsSession(
  businessId: number,
): { threadId?: number } | null {
  if (typeof window === "undefined") return null;
  try {
    const raw = sessionStorage.getItem(opsSessionKey(businessId));
    return raw ? (JSON.parse(raw) as { threadId?: number }) : null;
  } catch {
    return null;
  }
}

export function writeOpsSession(
  businessId: number,
  threadId: number | undefined,
) {
  if (typeof window === "undefined") return;
  if (!threadId) {
    sessionStorage.removeItem(opsSessionKey(businessId));
    return;
  }
  sessionStorage.setItem(
    opsSessionKey(businessId),
    JSON.stringify({ threadId }),
  );
}

export function newClientRequestId(): string {
  return randomUUID();
}

function directorHandoffKey(businessId: number) {
  return `payverge_director_handoff_${businessId}`;
}

export function writeDirectorHandoff(businessId: number, prompt: string) {
  if (typeof window === "undefined") return;
  sessionStorage.setItem(
    directorHandoffKey(businessId),
    JSON.stringify({
      prompt,
      source: "ops_assistant",
      at: new Date().toISOString(),
    }),
  );
}

export function readAndClearDirectorHandoff(businessId: number): string | null {
  if (typeof window === "undefined") return null;
  const key = directorHandoffKey(businessId);
  const raw = sessionStorage.getItem(key);
  sessionStorage.removeItem(key);
  if (!raw) return null;
  try {
    return (JSON.parse(raw) as { prompt?: string }).prompt ?? null;
  } catch {
    return null;
  }
}

export type { ChatMessage };
