import type { AssistantResponse } from "@/types/assistant";

/** A suggested next step rendered under an assistant reply. */
export interface ChatAction {
  label: string;
  href: string;
  kind: "navigate" | "external" | "handoff";
  disabled?: boolean;
  disabled_reason?: string;
}

/** One message in a ChatShell transcript (ops assistant, AI waiter). */
export interface ChatMessage {
  id?: number;
  role: "user" | "assistant";
  content: string;
  response?: AssistantResponse;
  actions?: ChatAction[];
  steps?: string[];
  followUps?: string[];
  contractVersion?: "v1" | "v2";
}

/** The structured assistant reply the ops-assistant API returns per turn. */
export interface StructuredChatResponse {
  answer: string;
  steps: string[];
  actions: ChatAction[];
  follow_ups: string[];
}
