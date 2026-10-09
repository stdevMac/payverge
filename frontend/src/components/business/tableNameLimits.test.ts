import { TABLE_NAME_MAX_LENGTH } from "./tableNameLimits";

describe("L3-25 table name max length", () => {
  it("is a positive finite cap used by FE inputs and BE binding", () => {
    expect(TABLE_NAME_MAX_LENGTH).toBeGreaterThan(0);
    expect(TABLE_NAME_MAX_LENGTH).toBeLessThanOrEqual(255);
    expect(TABLE_NAME_MAX_LENGTH).toBe(64);
  });
});
