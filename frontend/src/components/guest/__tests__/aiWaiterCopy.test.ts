import { getStarterQuestions } from "../aiWaiterCopy";

describe("getStarterQuestions", () => {
  it("returns the localized array for a supported locale", async () => {
    const qs = await getStarterQuestions("es");
    expect(Array.isArray(qs)).toBe(true);
    expect(qs.length).toBeGreaterThanOrEqual(4);
    expect(qs[0]).toMatch(/hoy/i);
  });
  it("falls back to English for an unknown locale", async () => {
    const qs = await getStarterQuestions("xx-unknown");
    expect(qs[0]).toBe("What's good today?");
  });
  it("returns English for en", async () => {
    const qs = await getStarterQuestions("en");
    expect(qs).toContain("Do you have desserts?");
  });
  it("falls back to English for an empty language (no ./.json import)", async () => {
    const qs = await getStarterQuestions("");
    expect(qs).toContain("Do you have desserts?");
  });
});
