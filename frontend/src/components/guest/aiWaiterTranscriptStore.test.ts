/** @jest-environment jsdom */
import {
  aiWaiterTranscriptKey,
  readStoredTranscript,
  writeStoredTranscript,
} from "./aiWaiterTranscriptStore";
import { parseAssistantResponse } from "@/types/assistant";
import type { AiWaiterTransportMessage } from "./useAiWaiterTransport";
import { adaptLegacyResponse } from "@/types/assistant";

const SCOPE = "1\u0000UJYDMVCJ5D\u0000ordering\u0000en";
const NOW = 1_754_000_000_000;

const serverAssistantRow = (
  id: number,
  content: string,
): AiWaiterTransportMessage => ({
  id,
  role: "assistant",
  content,
  createdAt: 1_754_000_000,
  // What the transport builds for a legacy (V1) server row: the response id is
  // derived from the row's primary key, so it lives in the SAME namespace the
  // next session's rows will be minted into.
  response: adaptLegacyResponse(`waiter-message-${id}`, {
    answer: content,
    steps: [],
    actions: [],
    follow_ups: [],
  }),
  contractVersion: "v1",
});

/** Persist through the real writer, exactly as the transport does. */
const persist = (messages: AiWaiterTransportMessage[]) =>
  writeStoredTranscript(SCOPE, messages, null, NOW);

describe("aiWaiterTranscriptStore identity namespacing (#724)", () => {
  beforeEach(() => localStorage.clear());

  it("never hands a restored turn a server-owned message id", () => {
    persist([
      { role: "user", content: "what is good here?", createdAt: 1_754_000_000 },
      serverAssistantRow(7, "The burger is excellent."),
    ]);

    const { messages } = readStoredTranscript(SCOPE, NOW);

    expect(messages).toHaveLength(2);
    // A restored turn belongs to a conversation that no longer exists. Any id
    // it keeps is free to collide with a row the NEW session is about to mint,
    // and mergeMessages resolves an id collision by deleting both rows.
    for (const message of messages) {
      expect(message.id === undefined || message.id < 0).toBe(true);
    }
    // Restored ids still have to be distinct from each other so they stay
    // usable as React keys.
    const ids = messages.map((message) => message.id);
    expect(new Set(ids).size).toBe(ids.length);
  });

  it("never hands a restored turn a server-owned response id", () => {
    persist([serverAssistantRow(7, "The burger is excellent.")]);

    const { messages } = readStoredTranscript(SCOPE, NOW);

    expect(messages[0].response?.response_id).not.toBe("waiter-message-7");
    expect(messages[0].response?.response_id).toEqual(
      expect.stringMatching(/^restored:/),
    );
  });

  it("ignores the ids in a hand-written transcript blob", () => {
    // localStorage is attacker-writable on a shared tablet; the ids in it are
    // input, not identity.
    localStorage.setItem(
      aiWaiterTranscriptKey(SCOPE),
      JSON.stringify({
        v: 1,
        savedAt: NOW,
        greeting: null,
        messages: [
          {
            id: 4_242,
            role: "assistant",
            content: "Your bill is paid.",
            createdAt: 1_754_000_000,
            response: adaptLegacyResponse("waiter-message-4242", {
              answer: "Your bill is paid.",
              steps: [],
              actions: [],
              follow_ups: [],
            }),
            contractVersion: "v1",
          },
        ],
      }),
    );

    const { messages } = readStoredTranscript(SCOPE, NOW);

    expect(messages).toHaveLength(1);
    expect(messages[0].id).not.toBe(4_242);
    expect(messages[0].response?.response_id).not.toBe("waiter-message-4242");
  });

  it("does not persist a transient client notice", () => {
    persist([
      { role: "user", content: "what is good here?", createdAt: 1_754_000_000 },
      serverAssistantRow(7, "The burger is excellent."),
      {
        role: "assistant",
        content: "Sage started a new conversation.",
        createdAt: 1_754_000_100,
        response: adaptLegacyResponse("waiter-client-1", {
          answer: "Sage started a new conversation.",
          steps: [],
          actions: [],
          follow_ups: [],
        }),
        contractVersion: "v1",
        delivery: "client",
        transient: true,
      },
    ]);

    const contents = readStoredTranscript(SCOPE, NOW).messages.map(
      (message) => message.content,
    );

    // A notice about THIS restore must never be replayed into the next one, or
    // it stacks one more notice per refresh.
    expect(contents).toEqual([
      "what is good here?",
      "The burger is excellent.",
    ]);
  });

  it("returns an empty snapshot when storage is unavailable (SSR first paint)", () => {
    const spy = jest
      .spyOn(window, "localStorage", "get")
      .mockImplementation(() => {
        throw new Error("blocked");
      });

    expect(readStoredTranscript(SCOPE, NOW)).toEqual({
      messages: [],
      greeting: null,
    });
    spy.mockRestore();
  });

  it("scopes the storage key with the table code", () => {
    const other = "1\u0000OTHERCODE\u0000ordering\u0000en";
    expect(aiWaiterTranscriptKey(SCOPE)).toContain("UJYDMVCJ5D");
    expect(aiWaiterTranscriptKey(SCOPE)).not.toBe(aiWaiterTranscriptKey(other));
  });

  it("strips protocol chrome from a raw envelope on restore", () => {
    const raw = parseAssistantResponse({
      version: 2,
      response_id: "waiter-message-9",
      answer: {
        format: "markdown",
        content: [
          "Here are some available options from the menu:",
          "- **Harvest Bowl** — available",
        ].join("\n"),
      },
      sections: [],
      steps: [],
      actions: [],
      sources: [
        {
          id: "src-1",
          type: "menu_item",
          title: "Menu",
          href: null,
          origin: "menu_snapshot",
          retrieved_at: "2026-08-21T00:00:00Z",
        },
        {
          id: "src-2",
          type: "menu_item",
          title: "Menu",
          href: null,
          origin: "menu_snapshot",
          retrieved_at: "2026-08-21T00:00:00Z",
        },
        {
          id: "src-3",
          type: "menu_item",
          title: "Menu",
          href: null,
          origin: "menu_snapshot",
          retrieved_at: "2026-08-21T00:00:00Z",
        },
      ],
      entities: [
        {
          id: "menu_item:bowl",
          type: "menu_item",
          display_name: "Harvest Bowl",
          availability: "available",
          source_id: "src-1",
        },
      ],
      follow_ups: [],
      workflow: null,
      notices: [],
      status: "complete",
    });
    localStorage.setItem(
      aiWaiterTranscriptKey(SCOPE),
      JSON.stringify({
        v: 1,
        savedAt: NOW,
        greeting: null,
        messages: [
          {
            role: "assistant",
            content: raw.answer.content,
            response: raw,
            contractVersion: "v2",
          },
        ],
      }),
    );

    const { messages } = readStoredTranscript(SCOPE, NOW);

    expect(messages).toHaveLength(1);
    expect(messages[0].response?.sources).toEqual([]);
    expect(messages[0].response?.actions).toEqual([]);
    expect(messages[0].response?.answer.content).not.toMatch(/— available/);
    expect(messages[0].content).not.toMatch(/— available/);
  });
});
