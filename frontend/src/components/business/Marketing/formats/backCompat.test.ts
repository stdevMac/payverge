import type { MarketingCreativeSnapshot } from "@/api/marketing";
import { creativeForActivity } from "../MarketingLibrary";
import { COMPOSITIONS } from "../composition/compositions";
import { LEGACY_COMPOSITION_FOR_TEMPLATE } from "../composition/compositions";
import { kitFormatsFromSnapshot, kitRenderPlan } from "./campaignKit";
import { FORMATS } from "./formats";

/**
 * The regression net for the whole wave.
 *
 * Every row in `marketing_activities.creative_snapshot` was written by an older
 * build. Three generations exist on disk today:
 *
 *  1. PRE-SNAPSHOT rows, which have no `creative_snapshot` at all and are
 *     reconstructed by `creativeForActivity`;
 *  2. WAVE 0 rows, which carry `template` and no `kit`;
 *  3. WAVE 1 rows, which carry `kit` and `composition` and no `kit_formats`.
 *
 * None of them may break, and none of them may be silently promoted into a
 * six-format campaign the operator never approved.
 */

const WAVE0: MarketingCreativeSnapshot = {
  caption: "Milanesa night",
  image_url: "https://cdn.example.com/a.jpg",
  image_source: "menu",
  template: "editorial",
  aspect: "4:5",
  slots: {
    dishName: "Milanesa",
    price: "$12.00",
    badge: "CHEF'S PICK",
    cta: "Order",
    handle: "@casa",
  },
  crop: { x: 0.5, y: 0.5, zoom: 1 },
  font_family: "Inter",
};

const WAVE1: MarketingCreativeSnapshot = {
  ...WAVE0,
  kit: "chalkboard",
  composition: "posterStack",
  treatment: "grain",
};

describe("stored snapshots keep rendering", () => {
  it("reads a pre-snapshot row as a single 4:5 post", () => {
    const creative = creativeForActivity({
      id: 1,
      caption: "hi",
      image_url: "https://cdn.example.com/a.jpg",
      title: "Milanesa",
      target_name: "Milanesa",
    } as never);
    expect(creative.aspect).toBe("4:5");
    expect(kitFormatsFromSnapshot(creative)).toEqual(["4:5"]);
  });

  it.each([
    ["wave 0", WAVE0],
    ["wave 1", WAVE1],
  ])("reads a %s row as one format, not a campaign", (_label, snapshot) => {
    expect(snapshot.kit_formats).toBeUndefined();
    expect(kitFormatsFromSnapshot(snapshot)).toEqual([snapshot.aspect]);
  });

  it("keeps every legacy template mapped to a legacy composition", () => {
    (["editorial", "bold", "minimal"] as const).forEach((template) => {
      const id = LEGACY_COMPOSITION_FOR_TEMPLATE[template];
      expect(COMPOSITIONS[id]).toBeDefined();
      // And those families still refuse the Wave 4 canvases, so a reused legacy
      // row can never be laid out on a format its template never had.
      expect(COMPOSITIONS[id].supportsFormat(FORMATS.wide)).toBe(false);
      expect(COMPOSITIONS[id].supportsFormat(FORMATS["4:5"])).toBe(true);
    });
  });

  it("renders every legacy family at every legacy format without throwing", () => {
    (["editorial", "bold", "minimal"] as const).forEach((template) => {
      const composition = COMPOSITIONS[LEGACY_COMPOSITION_FOR_TEMPLATE[template]];
      (["1:1", "4:5", "9:16"] as const).forEach((formatId) => {
        const format = FORMATS[formatId];
        expect(() => composition.boundsFor(format)).not.toThrow();
        expect(() => composition.imageAreaFor(format)).not.toThrow();
        expect(() => composition.logoAnchorFor(format)).not.toThrow();
        expect(composition.bandsFor(format).length).toBeGreaterThan(0);
      });
    });
  });

  // A legacy concept exported as a kit gets only the formats its composition can
  // honour — not a silently-wrong wide banner.
  it("plans a legacy concept's kit down to the formats it can render", () => {
    const plan = kitRenderPlan(
      {
        kit: "bold",
        composition: "legacyBold",
        aspect: "4:5",
        photoUrl: "https://cdn.example.com/a.jpg",
        slots: WAVE0.slots,
        palette: { primary: "#1a6b6a", secondary: "#0f3d3c" },
      },
      ["4:5", "9:16", "wide", "5:7"],
    );
    expect(plan.map((entry) => entry.format.id)).toEqual(["4:5", "9:16"]);
  });
});
