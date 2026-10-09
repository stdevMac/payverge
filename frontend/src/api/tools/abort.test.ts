import { isAbortError } from "./abort";

describe("isAbortError", () => {
  it("recognizes axios cancel, AbortError, CanceledError, and ERR_CANCELED", () => {
    expect(
      isAbortError({ name: "CanceledError", code: "ERR_CANCELED" }),
    ).toBe(true);
    expect(isAbortError({ name: "AbortError" })).toBe(true);
    expect(isAbortError({ code: "ERR_CANCELED" })).toBe(true);
    expect(isAbortError(new Error("network"))).toBe(false);
  });
});
