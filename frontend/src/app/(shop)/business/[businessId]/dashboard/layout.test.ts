import { metadata } from "./layout";

describe("business dashboard route metadata", () => {
  it("always provides a non-empty document title for authenticated operator pages", () => {
    expect(metadata.title).toBe("Business dashboard — Payverge");
  });
});
