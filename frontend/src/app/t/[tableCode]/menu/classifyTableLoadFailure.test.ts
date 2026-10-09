import {
  classifyTableLoadFailure,
  isTransientTableMiss,
} from "./classifyTableLoadFailure";

describe("classifyTableLoadFailure", () => {
  it("treats HTTP 404 as not_found", () => {
    expect(classifyTableLoadFailure({ response: { status: 404 } })).toBe(
      "not_found",
    );
    expect(classifyTableLoadFailure({ status: 404 })).toBe("not_found");
  });

  it("treats 5xx and offline as network (not not_found)", () => {
    expect(classifyTableLoadFailure({ response: { status: 500 } })).toBe(
      "network",
    );
    expect(classifyTableLoadFailure({ response: { status: 503 } })).toBe(
      "network",
    );
    expect(classifyTableLoadFailure({ code: "ERR_NETWORK" })).toBe("network");
    expect(classifyTableLoadFailure({ message: "Failed to fetch" })).toBe(
      "network",
    );
  });

  it("does not label missing status as not_found", () => {
    expect(classifyTableLoadFailure(new Error("boom"))).toBe("network");
    expect(classifyTableLoadFailure(undefined)).toBe("network");
  });
});

describe("isTransientTableMiss (#682)", () => {
  it("flags a fulfilled 200 whose body carries no table", () => {
    // An edge/proxy hiccup answers 200 with a lost body. axios resolves it, so
    // it never rejects — the 404-only retry used to skip this shape entirely.
    expect(isTransientTableMiss({ status: "fulfilled", value: undefined })).toBe(
      true,
    );
    expect(isTransientTableMiss({ status: "fulfilled", value: "" })).toBe(true);
    expect(isTransientTableMiss({ status: "fulfilled", value: {} })).toBe(true);
  });

  it("accepts a fulfilled payload that carries a table", () => {
    expect(
      isTransientTableMiss({
        status: "fulfilled",
        value: { table: { table_code: "T1" } },
      }),
    ).toBe(false);
  });

  it("flags a 404 rejection — a live code can 404 on one edge hop", () => {
    expect(
      isTransientTableMiss({
        status: "rejected",
        reason: { response: { status: 404 } },
      }),
    ).toBe(true);
  });

  it("does not retry a network failure", () => {
    // Network already renders retryable copy; a second blocking hop only makes
    // the diner wait longer for the same message.
    expect(
      isTransientTableMiss({ status: "rejected", reason: { code: "ERR_NETWORK" } }),
    ).toBe(false);
  });
});
