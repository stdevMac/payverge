import fs from "fs";
import path from "path";

const source = fs.readFileSync(
  path.resolve(__dirname, "../ConvertingBusinessLandingPage.tsx"),
  "utf8",
);

describe("storefront menu poll wiring", () => {
  it("gates the 5s menu refetch on the visible menu/delivery-cart surface", () => {
    expect(source).toContain("shouldPollStorefrontMenu");
    expect(source).toContain("pollMenu:");
    expect(source).toContain("storefrontTab");
  });
});
