import {
  isValidEmail,
  isValidPhone,
  isNonEmptyTrimmed,
  isWhitespaceOnly,
  isIntInRange,
  isNonNegativeNumber,
} from "./fieldValidation";

describe("isValidEmail", () => {
  it.each([
    ["a@b.co", true],
    ["user.name+tag@example.com", true],
    ["a@", false],
    ["@x", false],
    ["not-an-email", false],
    ["", false],
    ["  ", false],
    ["a@b", false], // needs a dot-TLD segment per our regex
  ])("%p → %p", (raw, expected) => {
    expect(isValidEmail(raw)).toBe(expected);
  });

  it("rejects includes(\"@\")-only weak checks (L5-16)", () => {
    expect("a@".includes("@")).toBe(true);
    expect(isValidEmail("a@")).toBe(false);
  });
});

describe("isValidPhone", () => {
  it.each([
    ["+54 11 5555-1234", true],
    ["1155551234", true],
    ["(11) 5555-1234", true],
    ["abc", false],
    ["123", false],
    ["", false],
    ["++++++", false],
    ["letters1234567", false],
  ])("%p → %p", (raw, expected) => {
    expect(isValidPhone(raw)).toBe(expected);
  });
});

describe("isNonEmptyTrimmed / isWhitespaceOnly", () => {
  it("detects whitespace-only names (L5-38)", () => {
    expect(isWhitespaceOnly("   ")).toBe(true);
    expect(isNonEmptyTrimmed("   ")).toBe(false);
    expect(isNonEmptyTrimmed("Tomatoes")).toBe(true);
    expect(isWhitespaceOnly("")).toBe(false);
  });
});

describe("isIntInRange", () => {
  it("rejects 0 when min is 1 (L2-33)", () => {
    expect(isIntInRange(0, 1, 20)).toBe(false);
    expect(isIntInRange(1, 1, 20)).toBe(true);
    expect(isIntInRange(20, 1, 20)).toBe(true);
    expect(isIntInRange(21, 1, 20)).toBe(false);
    expect(isIntInRange("3.5", 1, 20)).toBe(false);
  });
});

describe("isNonNegativeNumber", () => {
  it("rejects negatives (L5-27)", () => {
    expect(isNonNegativeNumber(-1)).toBe(false);
    expect(isNonNegativeNumber(0)).toBe(true);
    expect(isNonNegativeNumber("5")).toBe(true);
  });
});
