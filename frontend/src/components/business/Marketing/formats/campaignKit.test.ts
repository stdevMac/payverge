import type { RenderPostInput } from "../templates/renderPost";
import {
  DEFAULT_KIT_FORMATS,
  kitFormatsFromSnapshot,
  kitRenderPlan,
  resolveKitFormats,
} from "./campaignKit";
import { FORMAT_ORDER, FORMATS } from "./formats";

const BASE: RenderPostInput = {
  kit: "editorial",
  composition: "photoBottomStack",
  aspect: "4:5",
  photoUrl: "https://cdn.example.com/dish.jpg",
  logoUrl: "https://cdn.example.com/logo.png",
  slots: {
    badge: "CHEF'S PICK",
    dishName: "Milanesa napolitana",
    price: "$12.00",
    cta: "Order tonight",
    handle: "@casasur",
  },
  palette: { primary: "#1a6b6a", secondary: "#0f3d3c" },
  crop: { x: 0.5, y: 0.4, zoom: 1 },
};

describe("campaign kit derivation", () => {
  it("defaults to the whole registry, hero first", () => {
    expect(resolveKitFormats("4:5", undefined)).toEqual([
      "4:5",
      ...FORMAT_ORDER.filter((id) => id !== "4:5"),
    ]);
    expect(resolveKitFormats("9:16", undefined)[0]).toBe("9:16");
  });

  // The determinism claim the snapshot contract rests on: the Library stores the
  // hero plus a list, and the full set is recomputed from them. If this were not
  // a pure function of its inputs, reuse would need one row per format.
  it("is a pure function of its inputs", () => {
    for (let run = 0; run < 5; run += 1) {
      expect(resolveKitFormats("1:1", ["1:1", "5:7"])).toEqual(["1:1", "5:7"]);
    }
  });

  it("always puts the hero first, wherever the stored list had it", () => {
    expect(resolveKitFormats("9:16", ["4:5", "9:16", "5:7"])).toEqual([
      "9:16",
      "4:5",
      "5:7",
    ]);
  });

  it("adds the hero when a stored list omits it", () => {
    expect(resolveKitFormats("strip", ["4:5", "1:1"])).toEqual([
      "strip",
      "4:5",
      "1:1",
    ]);
  });

  it("dedupes and drops ids this build cannot render", () => {
    expect(resolveKitFormats("4:5", ["4:5", "4:5", "billboard", "wide"])).toEqual(
      ["4:5", "wide"],
    );
  });

  it("falls back to the default hero for an unknown hero id", () => {
    expect(resolveKitFormats("hologram", ["wide"])).toEqual(["4:5", "wide"]);
  });

  // Absent means legacy: a single-format post, not a campaign.
  it("reads a legacy snapshot as one format", () => {
    expect(
      kitFormatsFromSnapshot({ aspect: "4:5", kit_formats: undefined }),
    ).toEqual(["4:5"]);
    expect(kitFormatsFromSnapshot({ aspect: "9:16" })).toEqual(["9:16"]);
  });

  it("reads a campaign snapshot as its stored set", () => {
    expect(
      kitFormatsFromSnapshot({
        aspect: "4:5",
        kit_formats: ["4:5", "9:16", "5:7"],
      }),
    ).toEqual(["4:5", "9:16", "5:7"]);
  });

  it("exports the default set as data callers can display", () => {
    expect(DEFAULT_KIT_FORMATS).toEqual(FORMAT_ORDER);
  });
});

describe("kit render plan", () => {
  it("produces one entry per format, varying only the format", () => {
    const plan = kitRenderPlan(BASE, ["4:5", "9:16", "5:7"]);
    expect(plan).toHaveLength(3);
    plan.forEach((entry, index) => {
      expect(entry.format.id).toBe(["4:5", "9:16", "5:7"][index]);
      expect(entry.input.aspect).toBe(entry.format.id);
      // Everything else is the concept, untouched. This is what "one art
      // direction across the set" means mechanically.
      expect(entry.input.kit).toBe(BASE.kit);
      expect(entry.input.composition).toBe(BASE.composition);
      expect(entry.input.photoUrl).toBe(BASE.photoUrl);
      expect(entry.input.slots).toEqual(BASE.slots);
      expect(entry.input.crop).toEqual(BASE.crop);
      expect(entry.input.palette).toEqual(BASE.palette);
    });
  });

  it("marks the print entries so callers route them to the print walker", () => {
    const plan = kitRenderPlan(BASE, FORMAT_ORDER);
    const print = plan.filter((entry) => entry.format.medium === "print");
    expect(print.map((entry) => entry.format.id)).toEqual(["5:7"]);
  });

  it("names each output file after the target and the format", () => {
    const plan = kitRenderPlan(BASE, ["4:5", "5:7"], "Milanesa Napolitana");
    expect(plan[0].filename).toBe("milanesa-napolitana-4x5.png");
    expect(plan[1].filename).toBe("milanesa-napolitana-5x7.html");
  });

  it("drops a composition that refuses a format rather than rendering it wrong", () => {
    const plan = kitRenderPlan(
      { ...BASE, composition: "legacyBold", kit: "bold" },
      FORMAT_ORDER,
    );
    plan.forEach((entry) => {
      expect(FORMATS[entry.format.id].legacyAspect).not.toBeNull();
    });
  });
});
