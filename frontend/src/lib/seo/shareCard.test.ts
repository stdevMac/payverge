/**
 * @jest-environment node
 */
import { shareCardModel } from "./shareCard";
import { DEFAULT_SITE_DESCRIPTION, OG_IMAGE_URL } from "./openGraphImages";

jest.mock("next/og", () => ({ ImageResponse: jest.fn() }));

describe("shareCardModel", () => {
  it("carries the instance name and neutral copy, never the upstream product", () => {
    const model = shareCardModel({ name: "Casa Lola", brandColor: "#aa3355" });
    expect(model.name).toBe("Casa Lola");
    expect(model.initial).toBe("C");
    expect(model.brand).toBe("#aa3355");
    expect(model.description).toBe(DEFAULT_SITE_DESCRIPTION);
    expect(JSON.stringify(model)).not.toMatch(/payverge/i);
  });

  it("falls back to the default brand color for an invalid value", () => {
    expect(shareCardModel({ name: "X", brandColor: "red" }).brand).toBe("#1a6b6a");
  });

  it("is served from a generated route, not a static branded file", () => {
    expect(OG_IMAGE_URL).toBe("/share-card.png");
  });
});
