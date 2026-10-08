/**
 * L4-5 — FE mirror of MaxAiNameLen (40). maxLength is enforced on the Input;
 * this pure check documents the bound the dashboard uses for isInvalid.
 */
import { AI_NAME_MAX_LEN, isAiNameTooLong } from "../aiNameValidation";

describe("AI name length bound (L4-5)", () => {
  it("allows names up to 40 characters", () => {
    expect(isAiNameTooLong("a".repeat(AI_NAME_MAX_LEN))).toBe(false);
    expect(isAiNameTooLong("  Sage  ")).toBe(false);
  });

  it("flags names over 40 characters", () => {
    expect(isAiNameTooLong("a".repeat(AI_NAME_MAX_LEN + 1))).toBe(true);
  });
});
