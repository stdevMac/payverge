import { resolvePluginImageSrc } from "./pluginImages";

describe("resolvePluginImageSrc", () => {
  it("uses bundled assets for known plugins when seeded image is missing", () => {
    expect(
      resolvePluginImageSrc({
        name: "stripe",
        image: "",
      }),
    ).toBe("/images/plugins/stripe-logo.png");

    expect(
      resolvePluginImageSrc({
        name: "usdc_payment",
        image: null,
      }),
    ).toBe("/images/plugins/usdc.png");
  });

  it("keeps explicit local or allowed remote images for unknown plugins", () => {
    expect(
      resolvePluginImageSrc({
        name: "custom_plugin",
        image: "/images/plugins/custom.png",
      }),
    ).toBe("/images/plugins/custom.png");

    expect(
      resolvePluginImageSrc({
        name: "custom_remote",
        image: "https://images.unsplash.com/photo-1",
      }),
    ).toBe("https://images.unsplash.com/photo-1");
  });

  it("does not override non-placeholder images for known plugins", () => {
    expect(
      resolvePluginImageSrc({
        name: "stripe",
        image: "https://images.unsplash.com/stripe-custom.png",
      }),
    ).toBe("https://images.unsplash.com/stripe-custom.png");
  });
});
