import { isCatalogPluginOffered } from "./catalogPluginOffered";

describe("isCatalogPluginOffered", () => {
  it("offers an active catalog plugin that is not coming soon", () => {
    expect(isCatalogPluginOffered({ is_active: true, coming_soon: false })).toBe(
      true,
    );
  });

  it("does not offer a coming-soon plugin (cross-chain while guests cannot settle)", () => {
    expect(isCatalogPluginOffered({ is_active: true, coming_soon: true })).toBe(
      false,
    );
  });

  it("does not offer an inactive or missing plugin", () => {
    expect(isCatalogPluginOffered({ is_active: false, coming_soon: false })).toBe(
      false,
    );
    expect(isCatalogPluginOffered(undefined)).toBe(false);
    expect(isCatalogPluginOffered(null)).toBe(false);
  });
});
