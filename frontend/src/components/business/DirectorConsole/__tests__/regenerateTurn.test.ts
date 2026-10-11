import {
  isTrailingAssistant,
  messagesAfterRegenerateStart,
  shouldShowPendingUserBubble,
} from "../regenerateTurn";

describe("regenerateTurn (L4-15)", () => {
  it("removes the regenerated assistant message from the local transcript", () => {
    const msgs = [
      { id: 1, role: "user" as const },
      { id: 2, role: "assistant" as const },
      { id: 3, role: "user" as const },
      { id: 4, role: "assistant" as const },
    ];
    expect(messagesAfterRegenerateStart(msgs, 4).map((m) => m.id)).toEqual([
      1, 2, 3,
    ]);
  });

  it("hides the pending user bubble when regenerating", () => {
    expect(shouldShowPendingUserBubble({ regenerate: true })).toBe(false);
    expect(shouldShowPendingUserBubble({ regenerate: false })).toBe(true);
    expect(shouldShowPendingUserBubble({})).toBe(true);
  });

  describe("isTrailingAssistant — only the last turn may be regenerated", () => {
    const msgs = [
      { id: 1, role: "user" as const },
      { id: 2, role: "assistant" as const },
      { id: 3, role: "user" as const },
      { id: 4, role: "assistant" as const },
    ];

    it("accepts only the final assistant message", () => {
      expect(isTrailingAssistant(msgs, 4)).toBe(true);
    });

    it("rejects a non-trailing assistant message (would destroy an unrelated answer)", () => {
      expect(isTrailingAssistant(msgs, 2)).toBe(false);
    });

    it("rejects when the last message is a user turn", () => {
      const pending = [...msgs, { id: 5, role: "user" as const }];
      expect(isTrailingAssistant(pending, 4)).toBe(false);
    });

    it("rejects unknown ids and empty transcripts", () => {
      expect(isTrailingAssistant(msgs, 99)).toBe(false);
      expect(isTrailingAssistant([], 1)).toBe(false);
    });
  });
});
