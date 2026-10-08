import {
  reviewProvidersFromEnabledPlugins,
  isReviewPluginName,
} from "@/lib/reviewProviders";

describe("reviewProvidersFromEnabledPlugins", () => {
  it("always offers Google as a first-party provider", () => {
    const providers = reviewProvidersFromEnabledPlugins([]);
    expect(providers.map((p) => p.id)).toEqual(["google"]);
  });

  it("includes Trustpilot when the trustpilot plugin is enabled", () => {
    const providers = reviewProvidersFromEnabledPlugins([
      "telegram",
      "trustpilot",
      "stripe",
    ]);
    expect(providers.map((p) => p.id)).toEqual(["google", "trustpilot"]);
  });

  it("does not offer Trustpilot when the plugin is not enabled", () => {
    const providers = reviewProvidersFromEnabledPlugins(["stripe", "telegram"]);
    expect(providers.map((p) => p.id)).toEqual(["google"]);
    expect(providers.some((p) => p.id === "trustpilot")).toBe(false);
  });

  it("is case-insensitive on plugin names", () => {
    const providers = reviewProvidersFromEnabledPlugins(["TrustPilot"]);
    expect(providers.map((p) => p.id)).toContain("trustpilot");
  });
});

describe("isReviewPluginName", () => {
  it("recognizes trustpilot", () => {
    expect(isReviewPluginName("trustpilot")).toBe(true);
    expect(isReviewPluginName("stripe")).toBe(false);
  });
});
