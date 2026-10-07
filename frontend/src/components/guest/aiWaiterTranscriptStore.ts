import { parseAssistantResponse } from "@/types/assistant";
import type { AssistantResponse } from "@/types/assistant";
import { dinerProtocolChrome } from "./aiWaiterDinerTranscript";
import type { AiWaiterTransportMessage } from "./useAiWaiterTransport";

/**
 * A diner has no account, so the only place their Sage conversation can
 * survive a refresh is this browser. The server mints a brand-new conversation
 * on every page load (the bearer token lives in memory only), which is why a
 * refresh used to wipe the transcript.
 *
 * Nothing here is a credential: the session token is never written, only the
 * messages already rendered on screen. Everything read back is re-validated —
 * storage is attacker-writable on a shared tablet.
 */
const STORAGE_VERSION = 1;
/** One long dinner. After this the table has almost certainly turned over. */
const AI_WAITER_TRANSCRIPT_TTL_MS = 6 * 60 * 60 * 1000;
const MAX_STORED_MESSAGES = 40;
const MAX_STORED_CHARS = 192_000;
const MAX_MESSAGE_CONTENT = 12_000;

export interface StoredTranscript {
  messages: AiWaiterTransportMessage[];
  /**
   * The opening line of the conversation being resumed. Each new session saves
   * its own greeting server-side, so without this the restored transcript
   * would grow one duplicate hello per refresh.
   */
  greeting: string | null;
}

const EMPTY: StoredTranscript = { messages: [], greeting: null };

export const aiWaiterTranscriptKey = (scopeKey: string): string =>
  `pv_ai_waiter_chat_v${STORAGE_VERSION}:${scopeKey.replace(/\u0000/g, "|")}`;

const storage = (): Storage | null => {
  try {
    return typeof window === "undefined" ? null : window.localStorage;
  } catch {
    // Safari private mode and "block all cookies" throw on property access.
    return null;
  }
};

const validID = (value: unknown): value is number =>
  typeof value === "number" && Number.isSafeInteger(value) && value > 0;

/**
 * Identity for a restored turn (#724).
 *
 * A restored turn belongs to a conversation that no longer exists, so the ids
 * it was saved with are input, not identity — the DB sequence is free to hand
 * the SAME id to a row of the brand-new session, and localStorage is
 * attacker-writable besides. `mergeMessages` resolves an id collision by
 * splicing the existing row out, tombstoning the id and returning WITHOUT
 * pushing the incoming one, so a collision deletes the genuine server message
 * and poisons that id for the rest of the session.
 *
 * Restored turns therefore get ids from a namespace the server can never mint
 * into: negative for the numeric id (a primary key is always positive) and a
 * `restored:` prefix for the response id (server ids are `waiter-message-<pk>`
 * or a model-issued id). They stay distinct from each other so they remain
 * usable as React keys.
 */
const restoredID = (index: number): number => -(index + 1);
const restoredResponseID = (index: number): string => `restored:${index + 1}`;

const reviveMessage = (
  raw: unknown,
  index: number,
): AiWaiterTransportMessage[] => {
  if (typeof raw !== "object" || raw === null || Array.isArray(raw)) return [];
  const value = raw as Record<string, unknown>;
  if (value.role !== "user" && value.role !== "assistant") return [];
  if (typeof value.content !== "string") return [];
  if (value.content.length > MAX_MESSAGE_CONTENT) return [];
  const id = restoredID(index);
  const createdAt = validID(value.createdAt)
    ? { createdAt: value.createdAt }
    : {};

  if (value.role === "user") {
    return [
      {
        id,
        ...createdAt,
        role: "user",
        content: value.content,
        delivery: "client",
      },
    ];
  }
  let response: AssistantResponse;
  try {
    response = parseAssistantResponse(value.response);
  } catch {
    // A stored assistant turn without a well-formed envelope is unrenderable.
    return [];
  }
  // Reshape before the first paint: a raw blob still has Complete / sources /
  // "— available" chrome. Menu photos resolve on render; protocol must not
  // flash in the empty-menu window (#724).
  response = dinerProtocolChrome({
    ...response,
    response_id: restoredResponseID(index),
  });
  return [
    {
      id,
      ...createdAt,
      role: "assistant",
      content: response.answer.content,
      response,
      contractVersion: value.contractVersion === "v2" ? "v2" : "v1",
      // "client" keeps the restored turns ahead of any server batch in
      // mergeFullHistoryMessages instead of being discarded as stale history.
      delivery: "client",
    },
  ];
};

export const firstAssistantContent = (
  messages: readonly AiWaiterTransportMessage[],
): string | null => {
  for (const message of messages) {
    if (message.transient) continue;
    if (message.role === "assistant") return message.content.trim() || null;
  }
  return null;
};

export function readStoredTranscript(
  scopeKey: string,
  now: number = Date.now(),
): StoredTranscript {
  const store = storage();
  if (!store) return EMPTY;
  const key = aiWaiterTranscriptKey(scopeKey);
  let raw: string | null = null;
  try {
    raw = store.getItem(key);
  } catch {
    return EMPTY;
  }
  if (!raw) return EMPTY;

  const drop = () => {
    try {
      store.removeItem(key);
    } catch {
      /* nothing to do: the entry is already unusable */
    }
    return EMPTY;
  };

  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return drop();
  }
  if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
    return drop();
  }
  const payload = parsed as Record<string, unknown>;
  if (payload.v !== STORAGE_VERSION) return drop();
  const savedAt = payload.savedAt;
  if (typeof savedAt !== "number" || !Number.isFinite(savedAt)) return drop();
  // A clock that moved backwards must not resurrect a stale table's chat.
  if (savedAt > now || now - savedAt > AI_WAITER_TRANSCRIPT_TTL_MS) {
    return drop();
  }
  if (!Array.isArray(payload.messages)) return drop();

  const messages = payload.messages
    .slice(-MAX_STORED_MESSAGES)
    .flatMap((raw, index) => reviveMessage(raw, index));
  if (messages.length === 0) return drop();
  const stored =
    typeof payload.greeting === "string" && payload.greeting.trim()
      ? payload.greeting.trim()
      : null;
  return { messages, greeting: stored ?? firstAssistantContent(messages) };
}

export function writeStoredTranscript(
  scopeKey: string,
  messages: readonly AiWaiterTransportMessage[],
  greeting: string | null,
  now: number = Date.now(),
): void {
  const store = storage();
  if (!store) return;
  const key = aiWaiterTranscriptKey(scopeKey);
  // Client notices ("Sage started a new conversation", "Couldn't reach the
  // server") describe THIS session. Replaying one into the next restore would
  // stack another copy per refresh, so they never reach storage.
  const durable = messages.filter((message) => !message.transient);
  // Stop at the last answered turn: anything after it is a question still in
  // flight (or one that failed), and restoring it would show the diner a
  // question hanging with no reply and no way to retry.
  let lastAnswered = -1;
  for (let i = durable.length - 1; i >= 0; i -= 1) {
    if (durable[i].role === "assistant") {
      lastAnswered = i;
      break;
    }
  }
  let kept = durable.slice(0, lastAnswered + 1).slice(-MAX_STORED_MESSAGES);
  try {
    if (kept.length === 0) {
      store.removeItem(key);
      return;
    }
    const serialize = () =>
      JSON.stringify({
        v: STORAGE_VERSION,
        savedAt: now,
        greeting: greeting ?? firstAssistantContent(kept),
        // No `id`: see restoredID. A saved id survives its conversation and
        // would be read back as identity, so it is never written at all.
        messages: kept.map((message) => ({
          ...(message.createdAt === undefined
            ? {}
            : { createdAt: message.createdAt }),
          role: message.role,
          content: message.content,
          ...(message.response === undefined
            ? {}
            : { response: message.response }),
          ...(message.contractVersion === undefined
            ? {}
            : { contractVersion: message.contractVersion }),
        })),
      });
    let payload = serialize();
    while (payload.length > MAX_STORED_CHARS && kept.length > 1) {
      kept = kept.slice(1);
      payload = serialize();
    }
    if (payload.length > MAX_STORED_CHARS) {
      store.removeItem(key);
      return;
    }
    store.setItem(key, payload);
  } catch {
    // Quota or a storage-disabled browser: resuming the chat is a
    // convenience, never a reason to break the panel.
  }
}

export function clearStoredTranscript(scopeKey: string): void {
  const store = storage();
  if (!store) return;
  try {
    store.removeItem(aiWaiterTranscriptKey(scopeKey));
  } catch {
    /* see writeStoredTranscript */
  }
}
