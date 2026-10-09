import { computeAllowedImageHosts, isAllowedImageUrl } from "../aiImageOrigins";

describe("isAllowedImageUrl with own /media uploads", () => {
  const hosts = computeAllowedImageHosts([], "");

  it("renders relative /media images the AI waiter keeps", () => {
    expect(
      isAllowedImageUrl("/media/businesses/1/menu_items/a.png", hosts),
    ).toBe(true);
  });

  it("still rejects foreign hosts and non-media relative paths", () => {
    expect(isAllowedImageUrl("https://evil.example/a.png", hosts)).toBe(false);
    expect(isAllowedImageUrl("/api/v1/inside/me", hosts)).toBe(false);
    expect(isAllowedImageUrl("//evil.example/media/a.png", hosts)).toBe(false);
  });
});
