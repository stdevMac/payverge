import {
  parseFloatInput,
  parseIntegerInput,
} from "@/components/business/reservationSettingsParsers";

describe("ReservationSettings numeric parsers", () => {
  it("preserves zero values instead of replacing them with fallbacks", () => {
    expect(parseIntegerInput("0", 15)).toBe(0);
    expect(parseFloatInput("0", 25)).toBe(0);
    expect(parseFloatInput("0.0", 25)).toBe(0);
  });

  it("falls back only when the input is blank or invalid", () => {
    expect(parseIntegerInput("", 15)).toBe(15);
    expect(parseIntegerInput("not-a-number", 15)).toBe(15);
    expect(parseFloatInput("", 12.5)).toBe(12.5);
    expect(parseFloatInput("bad", 12.5)).toBe(12.5);
  });
});
