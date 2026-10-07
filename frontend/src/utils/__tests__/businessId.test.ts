import { getNumericBusinessId, isStringBusinessId } from "../businessId";

describe("getNumericBusinessId", () => {
  it("returns the numeric id when the business object carries a numeric id", () => {
    expect(getNumericBusinessId({ id: 4 }, "mara-ai-pro-demo")).toBe(4);
  });

  it("parses a numeric-string businessId param when the object id is absent", () => {
    expect(getNumericBusinessId(null, "4")).toBe(4);
  });

  it("falls through to 0 for a slug param when the object id is not numeric", () => {
    // This is the trap behind the manager 'access denied' bug: a staff
    // dashboard built its mock business with a STRINGIFIED id, so this branch
    // returned 0 on slug URLs and downstream tier/staff calls hit business 0.
    expect(getNumericBusinessId({ id: "4" }, "mara-ai-pro-demo")).toBe(0);
  });

  it("resolves a staff business (numeric id) even on a slug-based URL", () => {
    // Regression guard: the dashboard now builds staffBusiness with the
    // numeric business_id, so a staff user on /business/<slug>/dashboard
    // resolves to the real id instead of 0.
    const staffBusiness = { id: 4, business_id: "mara-ai-pro-demo" };
    expect(getNumericBusinessId(staffBusiness, "mara-ai-pro-demo")).toBe(4);
  });
});

describe("isStringBusinessId", () => {
  it("treats numeric values and numeric strings as non-slug ids", () => {
    expect(isStringBusinessId(4)).toBe(false);
    expect(isStringBusinessId("4")).toBe(false);
  });

  it("treats non-numeric slugs as string ids", () => {
    expect(isStringBusinessId("mara-ai-pro-demo")).toBe(true);
  });
});
