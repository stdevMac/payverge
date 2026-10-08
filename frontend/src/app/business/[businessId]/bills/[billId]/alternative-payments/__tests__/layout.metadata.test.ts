import { metadata } from "../layout";

describe("alternative-payments route metadata (#377)", () => {
  it("uses an operator document title, not the marketing root title", () => {
    expect(metadata.title).toBe("Alternative payments — Payverge");
    expect(String(metadata.title)).not.toMatch(
      /AI-Powered Restaurant Management/i,
    );
  });

  it("keeps the authenticated money route out of search indexes", () => {
    expect(metadata.robots).toEqual({ index: false, follow: false });
  });
});
