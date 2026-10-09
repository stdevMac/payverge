import { extractContext, fillUrl } from "../template.js";

describe("extractContext", () => {
  test("pulls businessId and origin from a dashboard URL", () => {
    const ctx = extractContext(
      "https://payverge.io/business/abc123/dashboard?tab=analytics",
    );
    expect(ctx.businessId).toBe("abc123");
    expect(ctx.origin).toBe("https://payverge.io");
  });

  test("pulls tableCode from a guest menu URL", () => {
    const ctx = extractContext("http://localhost:3000/t/TBL9/menu");
    expect(ctx.tableCode).toBe("TBL9");
    expect(ctx.businessId).toBeUndefined();
  });

  test("pulls customUrl from a storefront URL", () => {
    const ctx = extractContext("https://payverge.io/b/joes-diner");
    expect(ctx.customUrl).toBe("joes-diner");
  });
});

describe("fillUrl", () => {
  test("substitutes a single token", () => {
    expect(fillUrl("/business/{businessId}/dashboard?tab=menu", { businessId: "x1" }))
      .toBe("/business/x1/dashboard?tab=menu");
  });

  test("substitutes multiple distinct tokens", () => {
    expect(fillUrl("/t/{tableCode}/menu", { tableCode: "TBL9" }))
      .toBe("/t/TBL9/menu");
  });

  test("throws a named error when a token has no context value", () => {
    expect(() => fillUrl("/b/{customUrl}", {})).toThrow("missing context: customUrl");
  });

  test("leaves a path with no tokens untouched", () => {
    expect(fillUrl("/features", { businessId: "x1" })).toBe("/features");
  });
});
