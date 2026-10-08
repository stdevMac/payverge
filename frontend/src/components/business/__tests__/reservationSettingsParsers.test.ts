/** @jest-environment node */
import {
  parseIntegerInput,
  parseFloatInput,
} from "../reservationSettingsParsers";

describe("reservationSettingsParsers", () => {
  it("returns the fallback for an empty field (callers pass the CURRENT value so a cleared field no longer snaps to an unrelated default — audit L6 #20)", () => {
    // Simulating "clear the box while it holds 4" with the new call-site
    // contract (fallback = current value): the value is preserved, not reset
    // to a hardcoded 1.
    expect(parseIntegerInput("", 4)).toBe(4);
    expect(parseFloatInput("", 12.5)).toBe(12.5);
  });

  it("returns the fallback for non-numeric / partial input rather than NaN", () => {
    expect(parseIntegerInput("abc", 7)).toBe(7);
    expect(parseFloatInput("--", 3)).toBe(3);
  });

  it("parses valid integers and floats", () => {
    expect(parseIntegerInput("12", 0)).toBe(12);
    expect(parseFloatInput("12.50", 0)).toBe(12.5);
  });

  it("trims surrounding whitespace before deciding emptiness", () => {
    expect(parseIntegerInput("   ", 9)).toBe(9);
  });
});
