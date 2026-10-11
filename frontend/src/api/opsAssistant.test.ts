/**
 * @jest-environment jsdom
 */
const store: Record<string, string> = {};

beforeEach(() => {
  for (const k of Object.keys(store)) delete store[k];
  Object.defineProperty(global, "sessionStorage", {
    value: {
      getItem: (k: string) => store[k] ?? null,
      setItem: (k: string, v: string) => {
        store[k] = v;
      },
      removeItem: (k: string) => {
        delete store[k];
      },
      clear: () => {
        for (const k of Object.keys(store)) delete store[k];
      },
    },
    configurable: true,
  });
});

import {
  askOpsAssistant,
  OpsHistoryContractMismatchError,
  parseOpsThreadMessages,
  readAndClearDirectorHandoff,
  writeDirectorHandoff,
} from "./opsAssistant";
import { axiosInstance } from "./tools/instance";

describe("Director handoff sessionStorage", () => {
  it("writes and reads one-shot handoff payload", () => {
    writeDirectorHandoff(42, "Why did revenue drop?");
    expect(readAndClearDirectorHandoff(42)).toBe("Why did revenue drop?");
    expect(sessionStorage.getItem("payverge_director_handoff_42")).toBeNull();
  });

  it("returns null when no handoff exists", () => {
    expect(readAndClearDirectorHandoff(99)).toBeNull();
  });
});

describe("Ops assistant contract provenance", () => {
  const responseV2 = {
    version: 2,
    response_id: "response-1",
    answer: { format: "plain_text", content: "Grounded answer" },
    sections: [],
    steps: [],
    actions: [],
    sources: [],
    entities: [],
    follow_ups: [],
    workflow: null,
    notices: [],
    status: "complete",
  };

  it("marks valid v2 and absent-only legacy adaptations explicitly", () => {
    expect(
      parseOpsThreadMessages({
        messages: [
          { id: 1, role: "assistant", content: "Legacy answer" },
          {
            id: 2,
            role: "assistant",
            content: "V2 projection",
            response_v2: responseV2,
          },
        ],
      }).messages.map(({ contract_version }) => contract_version),
    ).toEqual(["v1", "v2"]);
  });

  it("throws a bounded mismatch code for malformed present v2 without downgrade", () => {
    let caught: unknown;
    try {
      parseOpsThreadMessages({
        messages: [
          {
            id: 7,
            role: "assistant",
            content: "Legacy content must not be used",
            structured_response: {
              answer: "Legacy content must not be used",
              steps: [],
              actions: [],
              follow_ups: [],
            },
            response_v2: { version: 2, answer: { content: "broken" } },
          },
        ],
      });
    } catch (error) {
      caught = error;
    }

    expect(caught).toBeInstanceOf(OpsHistoryContractMismatchError);
    expect(caught).toMatchObject({
      code: "assistant_history_contract_mismatch",
      message: "assistant_history_contract_mismatch",
    });
    expect(JSON.stringify(caught)).not.toContain(
      "Legacy content must not be used",
    );
  });

  it("throws a bounded direct-response mismatch without retaining payload content", async () => {
    const post = jest.spyOn(axiosInstance, "post").mockResolvedValueOnce({
      data: {
        thread: { id: 4, title: "Thread" },
        assistant_message: { id: 8, content: "sentinel legacy content" },
        response: {
          answer: "sentinel legacy content",
          steps: [],
          actions: [],
          follow_ups: [],
        },
        response_v2: { version: 2, answer: { content: "sentinel broken" } },
        usage: { model: "test", latency_ms: 1 },
      },
    });

    let caught: unknown;
    try {
      await askOpsAssistant(7, { message: "hello" });
    } catch (error) {
      caught = error;
    }

    expect(caught).toMatchObject({
      code: "assistant_response_contract_mismatch",
      message: "assistant_response_contract_mismatch",
    });
    expect(JSON.stringify(caught)).not.toMatch(/sentinel|broken|legacy/);
    post.mockRestore();
  });
});
