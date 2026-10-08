import { isSafeExternalTrackingUrl } from "./externalUrl";

describe("isSafeExternalTrackingUrl", () => {
  it.each([
    ["https://partner.example/track/123", true],
    ["http://partner.example/track/123", true],
    ["javascript:alert(1)", false],
    ["data:text/html,hi", false],
    ["ftp://partner.example/x", false],
    ["not a url", false],
    ["", false],
  ])("%s → %s", (value, expected) => {
    expect(isSafeExternalTrackingUrl(value)).toBe(expected);
  });
});
