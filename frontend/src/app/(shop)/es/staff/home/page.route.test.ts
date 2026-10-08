describe("localized staff home App Router pages (#879)", () => {
  it("re-exports the unprefixed staff home page under /es/staff/home", async () => {
    const localized = await import("./page");
    const base = await import("../../../staff/home/page");
    expect(localized.default).toBe(base.default);
  });
});
