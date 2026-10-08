import fs from "fs";
import path from "path";

const SRC = path.resolve(__dirname, "BusinessHeroSection.tsx");

describe("BusinessHeroSection PG-15.4 contrast floor", () => {
  const src = fs.readFileSync(SRC, "utf8");

  it("keeps an unconditional scrim contrast floor (not gated on image load)", () => {
    // Warm base still paints under banners.
    expect(src).toMatch(/bg-warm-100/);
    // Scrim must always contribute contrast — never opacity-0 waiting on load.
    expect(src).toMatch(/data-testid="hero-scrim"/);
    expect(src).toMatch(/opacity-100/);
    // The prior regression: scrim zeroed until firstBannerLoaded.
    expect(src).not.toMatch(/firstBannerLoaded \? "opacity-100" : "opacity-0"/);
    expect(src).not.toMatch(/firstBannerLoaded/);
  });

  it("marks failed/hung banner slides and never renders them", () => {
    // onError + zero natural size + load timeout → handleBannerError.
    expect(src).toMatch(/handleBannerError/);
    expect(src).toMatch(/onError=\{\(\) => handleBannerError\(idx\)\}/);
    expect(src).toMatch(/naturalWidth === 0/);
    expect(src).toMatch(/BANNER_LOAD_TIMEOUT_MS/);
    // Failed slides must not remain in the DOM as blank opacity layers.
    expect(src).toMatch(/if \(failedBanners\.has\(idx\)\) return null;/);
    // When every banner fails, fall back to the brand gradient (not blank).
    expect(src).toMatch(/loadableBanners\.length > 0/);
    expect(src).toMatch(/linear-gradient\(135deg/);
  });
});
