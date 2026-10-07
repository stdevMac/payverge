import {
  AI_WAITER_CHAT_REQUEST_CONFIG,
  AI_WAITER_CHAT_TIMEOUT_MS,
  createAiWaiterSession,
  createAiWaiterTestSession,
  sendAiWaiterTestChat,
  isSessionUnknownError,
  classifyChatError,
  parseAiWaiterChatResponse,
} from "./aiWaiter";
import { axiosInstance } from "./tools/instance";

jest.mock("./tools/instance", () => ({
  axiosInstance: { post: jest.fn(), get: jest.fn() },
}));
const mocked = axiosInstance as jest.Mocked<typeof axiosInstance>;

describe("aiWaiter api", () => {
  beforeEach(() => jest.clearAllMocks());

  it("createAiWaiterSession posts table_code/mode/language and returns token+greeting+expiry", async () => {
    mocked.post.mockResolvedValueOnce({
      data: {
        session_token: "abc123",
        greeting: "Hi, I'm an AI",
        expires_at: "2026-06-06T12:00:00Z",
      },
    });
    const res = await createAiWaiterSession(5, {
      table_code: "T1",
      mode: "ordering",
      language: "es",
    });
    expect(mocked.post).toHaveBeenCalledWith(
      "/ai-waiter/5/session",
      { table_code: "T1", mode: "ordering", language: "es" },
      { withCredentials: true, _skipErrorToast: true },
    );
    expect(res.session_token).toBe("abc123");
    expect(res.greeting).toContain("AI");
    expect(res.expires_at).toBe("2026-06-06T12:00:00Z");
  });

  it("createAiWaiterSession can replace the previous conversation token", async () => {
    mocked.post.mockResolvedValueOnce({
      data: {
        session_token: "fresh",
        greeting: "Hola",
        expires_at: "2026-06-06T12:00:00Z",
      },
    });
    await createAiWaiterSession(5, {
      table_code: "T1",
      mode: "ordering",
      language: "es-AR",
      replace_session_token: "old-token",
    });
    expect(mocked.post).toHaveBeenCalledWith(
      "/ai-waiter/5/session",
      {
        table_code: "T1",
        mode: "ordering",
        language: "es-AR",
        replace_session_token: "old-token",
      },
      { withCredentials: true, _skipErrorToast: true },
    );
  });

  it("createAiWaiterTestSession uses operator inside path without withCredentials guest cookie", async () => {
    mocked.post.mockResolvedValueOnce({
      data: {
        session_token: "optest-abc",
        greeting: "Sandbox hi",
        expires_at: "2026-06-06T12:00:00Z",
      },
    });
    const res = await createAiWaiterTestSession(9, { language: "en" });
    expect(mocked.post).toHaveBeenCalledWith(
      "/inside/businesses/9/ai/test-chat/session",
      { language: "en" },
      { _skipErrorToast: true },
    );
    // No withCredentials: the operator sandbox must never touch the guest
    // pv_ai_waiter_session cookie.
    const call = mocked.post.mock.calls[0];
    expect(call[2]).not.toHaveProperty("withCredentials");
    expect(res.session_token).toBe("optest-abc");
  });

  it("sendAiWaiterTestChat posts to operator sandbox chat endpoint", async () => {
    mocked.post.mockResolvedValueOnce({
      data: { role: "model", parts: [{ text: "ok" }] },
    });
    await sendAiWaiterTestChat(9, {
      history: [{ role: "user", content: "hi" }],
      language: "en",
      session_token: "optest-abc",
    });
    expect(mocked.post).toHaveBeenCalledWith(
      "/inside/businesses/9/ai/test-chat",
      {
        history: [{ role: "user", content: "hi" }],
        language: "en",
        session_token: "optest-abc",
        mode: "concierge",
      },
      AI_WAITER_CHAT_REQUEST_CONFIG,
    );
  });

  // #596 — a waiter turn can exceed the global 30s axios timeout (model budget
  // ~20s + snapshot + finalize). Waiter chat requests must carry a much longer
  // timeout and must NOT let the interceptor toast the global "Couldn't reach
  // the server" copy — failures belong in-thread (errConnect).
  it("waiter chat config uses a long timeout and skips the global error toast", () => {
    expect(AI_WAITER_CHAT_TIMEOUT_MS).toBeGreaterThanOrEqual(60000);
    expect(AI_WAITER_CHAT_REQUEST_CONFIG.timeout).toBe(AI_WAITER_CHAT_TIMEOUT_MS);
    expect(AI_WAITER_CHAT_REQUEST_CONFIG._skipErrorToast).toBe(true);
  });

  it("isSessionUnknownError detects the canonical AUTH_SESSION_UNKNOWN shape (and legacy alias)", () => {
    expect(
      isSessionUnknownError({
        response: { status: 404, data: { code: "AUTH_SESSION_UNKNOWN" } },
      }),
    ).toBe(true);
    // Legacy backend code still triggers re-creation during the deploy window.
    expect(
      isSessionUnknownError({
        response: { status: 404, data: { code: "session_unknown" } },
      }),
    ).toBe(true);
    expect(
      isSessionUnknownError({
        response: { status: 500, data: { code: "AUTH_SESSION_UNKNOWN" } },
      }),
    ).toBe(false);
    expect(
      isSessionUnknownError({
        response: { status: 404, data: { code: "other" } },
      }),
    ).toBe(false);
    expect(isSessionUnknownError(new Error("network"))).toBe(false);
  });

  it("classifyChatError maps status/codes to friendly buckets", () => {
    expect(classifyChatError({ response: { status: 429 } })).toBe(
      "rate_limited",
    );
    expect(classifyChatError({ response: { status: 413 } })).toBe("too_long");
    expect(
      classifyChatError({
        response: { status: 200, data: { code: "over_budget" } },
      }),
    ).toBe("over_budget");
    expect(
      classifyChatError({
        response: { status: 404, data: { code: "AUTH_SESSION_UNKNOWN" } },
      }),
    ).toBe("session_unknown");
    expect(
      classifyChatError({
        response: { status: 404, data: { code: "session_unknown" } },
      }),
    ).toBe("session_unknown");
    // A 24h-expired session (401/"session_expired") must be recoverable, not a
    // permanent "trouble connecting" dead end — treat it as session_unknown so
    // the client recreates the session.
    expect(
      classifyChatError({
        response: { status: 401, data: { code: "session_expired" } },
      }),
    ).toBe("session_unknown");
    // A bare 401 without the expired code stays generic (don't wipe sessions on
    // unrelated auth failures).
    expect(classifyChatError({ response: { status: 401 } })).toBe("generic");
    // Budget exhaustion rides a 429 but must map to over_budget, not the
    // generic rate-limit "wait a moment" copy.
    expect(
      classifyChatError({
        response: { status: 429, data: { code: "over_budget" } },
      }),
    ).toBe("over_budget");
    expect(classifyChatError({ response: { status: 500 } })).toBe("generic");
  });

  it("passes only a structurally valid absent-V2 cart call to the legacy projector", () => {
    const parsed = parseAiWaiterChatResponse({
      id: 17,
      role: "model",
      parts: [
        {
          functionCall: {
            name: "add_to_cart",
            args: { item_name: "Bowl", quantity: 2 },
          },
        },
      ],
    });

    expect(parsed.response_v2.answer.content).toBe("");
    expect(parsed.parts).toHaveLength(1);
    expect(() =>
      parseAiWaiterChatResponse({
        role: "model",
        parts: [{ functionCall: { name: "unknown", args: {} } }],
      }),
    ).toThrow("invalid_ai_waiter_legacy_response");
    expect(() =>
      parseAiWaiterChatResponse({
        role: "model",
        parts: [{ functionCall: { name: "add_to_cart", args: null } }],
      }),
    ).toThrow("invalid_ai_waiter_legacy_response");
  });

  it("throws a bounded direct-response mismatch for malformed present V2", () => {
    let caught: unknown;
    try {
      parseAiWaiterChatResponse({
        id: 22,
        role: "model",
        parts: [{ text: "sentinel legacy downgrade" }],
        response_v2: { version: 2, answer: { content: "sentinel broken" } },
      });
    } catch (error) {
      caught = error;
    }

    expect(caught).toMatchObject({
      code: "assistant_response_contract_mismatch",
      message: "assistant_response_contract_mismatch",
    });
    expect(JSON.stringify(caught)).not.toMatch(/sentinel|broken|22/);
  });
});
