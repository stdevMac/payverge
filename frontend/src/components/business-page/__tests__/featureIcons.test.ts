import {
  resolveFeatureIconKey,
  getFeatureIconComponent,
  FEATURE_ICON_KEYS,
} from "@/components/business-page/featureIcons";
import { Sparkles } from "lucide-react";

describe("featureIcons", () => {
  it("prefers the stored editor icon key over title heuristics", () => {
    // Pre-fix public page used title-only heuristics via
    // pickFeatureIcon(feature.title || feature.icon). A feature with
    // icon="credit-card" and title="Family dining" would resolve to "users"
    // (family) and ignore the stored key. Stored keys must win.
    expect(resolveFeatureIconKey("credit-card", "Family dining")).toBe(
      "credit-card",
    );
    expect(resolveFeatureIconKey("shield", "Quiet family room")).toBe(
      "shield",
    );
    expect(resolveFeatureIconKey("wifi", "Anything")).toBe("wifi");
    expect(resolveFeatureIconKey("award", "")).toBe("award");
  });

  it("falls back to title heuristics when icon is missing/unknown", () => {
    expect(resolveFeatureIconKey("", "Free WiFi everywhere")).toBe("wifi");
    expect(resolveFeatureIconKey("not-a-real-icon", "Parking lot")).toBe(
      "car",
    );
    expect(resolveFeatureIconKey(undefined, "Live music nightly")).toBe(
      "music",
    );
  });

  it("returns a distinct catalog component for every FEATURE_ICON_KEYS entry", () => {
    expect(FEATURE_ICON_KEYS.length).toBeGreaterThan(0);
    const unknownFallback = getFeatureIconComponent("__not-a-catalog-key__");
    expect(unknownFallback).toBe(Sparkles);
    for (const key of FEATURE_ICON_KEYS) {
      expect(key.length).toBeGreaterThan(0);
      const Comp = getFeatureIconComponent(key);
      expect(Comp).toBeTruthy();
      // Catalog keys must resolve to their mapped icon, not the Sparkles fallback.
      expect(Comp).not.toBe(Sparkles);
    }
  });

  it("ships a non-empty icon key set matching the editor options", () => {
    expect(FEATURE_ICON_KEYS).toEqual(
      expect.arrayContaining([
        "wifi",
        "car",
        "credit-card",
        "coffee",
        "music",
        "award",
      ]),
    );
  });
});
