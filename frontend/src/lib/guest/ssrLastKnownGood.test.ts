/** @jest-environment node */
import { SsrLastKnownGoodCache } from "./ssrLastKnownGood";

describe("SsrLastKnownGoodCache", () => {
  it("returns the value set for a slug", () => {
    const cache = new SsrLastKnownGoodCache<string>();
    cache.set("payverge-ai-pro-demo-lounge", "Payverge AI Pro Demo Lounge");
    expect(cache.get("payverge-ai-pro-demo-lounge")).toBe(
      "Payverge AI Pro Demo Lounge",
    );
  });

  it("returns undefined for an unknown slug", () => {
    const cache = new SsrLastKnownGoodCache<string>();
    expect(cache.get("missing-venue")).toBeUndefined();
  });

  it("delete drops a slug so later get is empty (404 / unpublish)", () => {
    const cache = new SsrLastKnownGoodCache<string>();
    cache.set("lounge", "live");
    cache.delete("lounge");
    expect(cache.get("lounge")).toBeUndefined();
  });

  it("clear drops every slug", () => {
    const cache = new SsrLastKnownGoodCache<string>();
    cache.set("a", "1");
    cache.set("b", "2");
    cache.clear();
    expect(cache.get("a")).toBeUndefined();
    expect(cache.get("b")).toBeUndefined();
  });

  it("keeps the value within ttl", () => {
    let now = 1_000;
    const cache = new SsrLastKnownGoodCache<string>({
      ttlMs: 30_000,
      now: () => now,
    });
    cache.set("lounge", "live");
    now = 30_999;
    expect(cache.get("lounge")).toBe("live");
  });

  it("expires after ttl", () => {
    let now = 1_000;
    const cache = new SsrLastKnownGoodCache<string>({
      ttlMs: 30_000,
      now: () => now,
    });
    cache.set("lounge", "live");
    now = 31_000;
    expect(cache.get("lounge")).toBeUndefined();
  });

  it("isolates values by slug", () => {
    const cache = new SsrLastKnownGoodCache<string>();
    cache.set("lounge", "live");
    cache.set("other", "other-live");
    cache.delete("lounge");
    expect(cache.get("lounge")).toBeUndefined();
    expect(cache.get("other")).toBe("other-live");
  });
});
