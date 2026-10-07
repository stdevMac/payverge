/** @jest-environment jsdom */
import { renderHook, act, waitFor } from "@testing-library/react";
import { safeRegionForCreative, usePostComposer } from "./usePostComposer";
import * as api from "@/api/marketing";
import * as renderer from "../templates/renderPost";
import type {
  CampaignSuggestion,
  MarketingCreativeSnapshot,
} from "@/api/marketing";
import { KITS, kitForLegacyTemplate } from "../artDirection/kits";
import {
  CHOOSABLE_COMPOSITIONS,
  COMPOSITIONS,
} from "../composition/compositions";
import { chooseComposition, nextComposition } from "../composition/chooser";
import * as photoCache from "../photo/cache";
import type { PhotoAnalysis } from "../photo/types";

// Generated assets come from the deployment's CDN, which the same-origin media
// proxy only trusts when it is declared in MEDIA_ORIGINS.
const savedMediaOrigins = process.env.MEDIA_ORIGINS;
beforeAll(() => {
  process.env.MEDIA_ORIGINS = "https://media.example.test";
});
afterAll(() => {
  if (savedMediaOrigins === undefined) delete process.env.MEDIA_ORIGINS;
  else process.env.MEDIA_ORIGINS = savedMediaOrigins;
});

// Partial mock: keep real Error subclasses (ImageDailyLimitError) so instanceof
// and constructor parameter properties work. Only the network calls are stubbed.
jest.mock("@/api/marketing", () => {
  const actual =
    jest.requireActual<typeof import("@/api/marketing")>("@/api/marketing");
  return {
    ...actual,
    generateMarketingImage: jest.fn(),
    cleanupMarketingImage: jest.fn(),
    generateMarketingCaption: jest.fn(),
  };
});
jest.mock("../templates/renderPost", () => ({
  DEFAULT_CROP: { x: 0.5, y: 0.5, zoom: 1 },
  loadOptionalImage: jest.fn().mockResolvedValue({
    naturalWidth: 800,
    naturalHeight: 1000,
    decode: async () => undefined,
  }),
  renderPostToBlob: jest.fn(),
  renderPost: jest.fn(),
}));
jest.mock("../photo/cache", () => {
  const actual = jest.requireActual("../photo/cache");
  return {
    ...actual,
    photoAnalysisFor: jest.fn().mockReturnValue(null),
  };
});
/**
 * Delegating spies, never stubs: every chooser export keeps its real behaviour,
 * so the rest of this suite is unaffected. Recording the calls is the only way
 * to observe the `play` signal the hook feeds the chooser — no rule reads
 * `ContentSignals.play` yet (chooser.ts:9-14 stages it for Wave 2), so a
 * disagreement between the seed's default and the live rotation's default is
 * invisible from the hook's public surface until the wave that consults it.
 */
jest.mock("../composition/chooser", () => {
  const actual = jest.requireActual<typeof import("../composition/chooser")>(
    "../composition/chooser",
  );
  return {
    ...actual,
    chooseComposition: jest.fn(actual.chooseComposition),
    nextComposition: jest.fn(actual.nextComposition),
  };
});

const sugg: CampaignSuggestion = {
  id: "1:featured_dish:m1",
  play: "featured_dish",
  title: "Feature your star: Carbonara",
  why_data: "why",
  source: "menu_engineering",
  target_item_id: "m1",
  target_name: "Carbonara",
  copy_angle: "celebrate the best-seller",
  rank: 180,
};

const business = {
  id: 42,
  name: "Trattoria",
  social_media: "trattoria_roma",
  display_currency: "USD",
  default_language: "en",
  design_settings: {
    primary_color: "#1a6b6a",
    secondary_color: "#0f3d3c",
    font_family: "Serif",
  },
} as any;

const t = (k: string) =>
  k.startsWith("starterCaption.")
    ? "{discount} — {name} is on now. {handle}"
    : k;

describe("safeRegionForCreative", () => {
  it("describes the composition's geometry, not the template's", () => {
    // photoBottomStack at 4:5 solves to bounds x 0.07 y 0.50 w 0.86 h 0.43 and a
    // full-bleed image area. Neither number appears in TEMPLATES.editorial.
    expect(safeRegionForCreative("photoBottomStack", "4:5")).toBe(
      "photoBottomStack 4:5 composition; " +
        "reserve copy within content x=0.070,y=0.500,w=0.860,h=0.430; " +
        "compose the subject within image x=0.000,y=0.000,w=1.000,h=1.000; " +
        "reserved band bottom",
    );
  });

  it("reports the reserved band of a family whose type clears the photo", () => {
    expect(safeRegionForCreative("splitPanel", "4:5")).toContain(
      "reserved band none",
    );
  });
});

describe("usePostComposer", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (api.generateMarketingCaption as jest.Mock).mockResolvedValue("AI caption");
    (renderer.loadOptionalImage as jest.Mock).mockResolvedValue({
      naturalWidth: 800,
      naturalHeight: 1000,
      decode: async () => undefined,
    });
  });

  it("prefills slots from the suggestion + business brand", async () => {
    const { result } = renderHook(() =>
      usePostComposer({ business, suggestion: sugg, t }),
    );
    expect(result.current.slots.dishName).toBe("Carbonara");
    expect(result.current.slots.handle).toBe("@trattoria_roma");
    // Font is kit-derived (editorial display = serif), not design_settings.font_family.
    expect(result.current.kit).toBe("editorial");
    expect(result.current.palette.fontFamily).toBe("Serif");
    await waitFor(() => expect(result.current.caption).toBe("AI caption"));
  });

  it("biases the seed kit from creative_profile visual_mood", async () => {
    // Hang caption so bootstrap does not race sibling tests with act warnings.
    (api.generateMarketingCaption as jest.Mock).mockReturnValueOnce(
      new Promise(() => undefined),
    );
    const { result, unmount } = renderHook(() =>
      usePostComposer({
        business,
        suggestion: sugg,
        creativeProfile: {
          audience: "",
          voice: "",
          visual_mood: "moody",
          cta_style: "direct",
          hashtag_behavior: "none",
          avoid_phrases: [],
          default_language: "en",
          default_tone: "punchy",
        },
        t: (k) =>
          k === "defaults.ctaStyle.direct"
            ? "ORDER NOW"
            : k.startsWith("starterCaption.")
              ? "{name} {handle}"
              : k,
      }),
    );
    expect(result.current.kit).toBe("chalkboard");
    // Chalkboard display is serif → wire font Serif (still not design_settings).
    expect(result.current.palette.fontFamily).toBe("Serif");
    expect(result.current.slots.cta).toBe("ORDER NOW");
    expect(result.current.tone).toBe("punchy");
    expect(result.current.language).toBe("en");
    unmount();
  });

  it("preserves stored kit on Reuse and re-applies live brand colours", () => {
    const initialSnapshot: MarketingCreativeSnapshot = {
      image_url: "https://cdn/reused.jpg",
      image_source: "gallery",
      caption: "Keep me #food",
      aspect: "4:5",
      template: "bold",
      crop: { x: 0.5, y: 0.5, zoom: 1 },
      slots: { dishName: "Kept", cta: "OLD", handle: "@old" },
      kit: "ticket",
      composition: "photoBottomStack",
      font_family: "Serif",
    };
    const { result } = renderHook(() =>
      usePostComposer({
        business: {
          ...business,
          design_settings: {
            primary_color: "#abcdef",
            secondary_color: "#123456",
            font_family: "Inter",
          },
        },
        suggestion: sugg,
        initialSnapshot,
        creativeProfile: {
          audience: "",
          voice: "",
          visual_mood: "bright",
          cta_style: "",
          hashtag_behavior: "none",
          avoid_phrases: [],
          default_language: "",
          default_tone: "",
        },
        t,
      }),
    );
    // Stored kit wins over mood bias (bright → bold would otherwise apply).
    expect(result.current.kit).toBe("ticket");
    // Ticket display is sans → wire font Sans, not snapshot Serif or design Inter.
    expect(result.current.palette.fontFamily).toBe("Sans");
    expect(result.current.palette.primary).toBe("#abcdef");
    expect(result.current.slots.cta).toBe("OLD");
    // hashtag_behavior none strips tags from reused caption.
    expect(result.current.caption).toBe("Keep me");
    // Reuse does not bootstrap a new caption.
    expect(api.generateMarketingCaption).not.toHaveBeenCalled();
  });

  it("opens a reused activity snapshot as the exact clean local working state", () => {
    const initialSnapshot: MarketingCreativeSnapshot = {
      image_url: "https://cdn/reused.jpg",
      image_source: "gallery",
      caption: "Reused exact caption",
      aspect: "9:16",
      template: "minimal",
      crop: { x: 0.17, y: 0.81, zoom: 2.1 },
      slots: {
        dishName: "Reused dish",
        price: "$31",
        badge: "ARCHIVE",
        cta: "VISIT",
        handle: "@archive",
      },
      font_family: "Sans",
    };

    const { result } = renderHook(() =>
      usePostComposer({
        business,
        suggestion: sugg,
        initialSnapshot,
        t,
      }),
    );

    expect(result.current.workingCreative).toEqual({
      aspect: "9:16",
      templateStyle: "minimal",
      slots: initialSnapshot.slots,
      caption: "Reused exact caption",
      photoUrl: "https://cdn/reused.jpg",
      imageSource: "gallery",
      crop: { x: 0.17, y: 0.81, zoom: 2.1 },
      // This snapshot predates the scene system, so the art direction is
      // derived from its own template and content rather than restored.
      kit: "minimal",
      composition: "photoBottomStack",
      // No stored composition ⇒ chooser seeded it; analysis may still improve.
      compositionAuto: true,
    });
    expect(result.current.palette.fontFamily).toBe("Sans");
    expect(result.current.renderInput).toEqual(
      expect.objectContaining({
        aspect: "9:16",
        photoUrl: "https://cdn/reused.jpg",
        slots: initialSnapshot.slots,
        crop: initialSnapshot.crop,
        palette: expect.objectContaining({ fontFamily: "Sans" }),
      }),
    );
    expect(result.current.dirty).toBe(false);
    expect(api.generateMarketingCaption).not.toHaveBeenCalled();
  });

  it("seeds photo from a suggestion and bootstraps an AI caption", async () => {
    const suggestion = {
      id: "x",
      play: "offer",
      title: "T",
      why_data: "w",
      source: "offers",
      copy_angle: "a",
      rank: 55,
      image_url: "https://cdn/o.jpg",
      target_name: "Summer Sale",
      discount_type: "percentage",
      discount_value: 20,
    } as never;
    const { result } = renderHook(() =>
      usePostComposer({ business, suggestion, t }),
    );
    expect(result.current.photoUrl).toBe("https://cdn/o.jpg");
    expect(result.current.slots.dishName).toBe("Summer Sale");
    expect(result.current.dirty).toBe(false);
    expect(result.current.initialCreative.imageSource).toBe("offer");
    await waitFor(() => expect(result.current.caption).toBe("AI caption"));
    expect(result.current.dirty).toBe(false);
  });

  it("adopts an untouched deferred bootstrap caption as the clean baseline", async () => {
    let resolveCaption!: (caption: string) => void;
    (api.generateMarketingCaption as jest.Mock).mockReturnValueOnce(
      new Promise<string>((resolve) => {
        resolveCaption = resolve;
      }),
    );
    const { result } = renderHook(() =>
      usePostComposer({ business, suggestion: sugg, t }),
    );
    const starter = result.current.caption;

    expect(result.current.dirty).toBe(false);
    await act(async () => resolveCaption("Deferred AI caption"));

    expect(result.current.caption).toBe("Deferred AI caption");
    expect(result.current.initialCreative.caption).toBe(starter);
    expect(result.current.dirty).toBe(false);
  });

  it("never overwrites or cleans a user edit made before bootstrap resolves", async () => {
    let resolveCaption!: (caption: string) => void;
    (api.generateMarketingCaption as jest.Mock).mockReturnValueOnce(
      new Promise<string>((resolve) => {
        resolveCaption = resolve;
      }),
    );
    const { result } = renderHook(() =>
      usePostComposer({ business, suggestion: sugg, t }),
    );

    act(() => result.current.setCaption("My in-progress caption"));
    await act(async () => resolveCaption("Stale AI caption"));

    expect(result.current.caption).toBe("My in-progress caption");
    expect(result.current.dirty).toBe(true);
  });

  it("resets edits to the clean autonomous baseline without mutating the initial snapshot", async () => {
    const { result } = renderHook(() =>
      usePostComposer({ business, suggestion: sugg, t }),
    );
    const starter = result.current.initialCreative.caption;
    await waitFor(() => expect(result.current.caption).toBe("AI caption"));
    act(() => result.current.setCaption("Discard this caption"));
    expect(result.current.dirty).toBe(true);

    act(() => result.current.resetCreative());

    expect(result.current.caption).toBe("AI caption");
    expect(result.current.initialCreative.caption).toBe(starter);
    expect(result.current.dirty).toBe(false);
  });

  it.each([
    [
      "crop",
      (state: ReturnType<typeof usePostComposer>) =>
        state.setCrop({ x: 0.2, y: 0.7, zoom: 1.4 }),
    ],
    [
      "aspect",
      (state: ReturnType<typeof usePostComposer>) => state.setAspect("9:16"),
    ],
    [
      "template",
      (state: ReturnType<typeof usePostComposer>) => state.setTemplate("bold"),
    ],
    [
      "slots",
      (state: ReturnType<typeof usePostComposer>) =>
        state.setSlot("dishName", "New name"),
    ],
    [
      "caption",
      (state: ReturnType<typeof usePostComposer>) =>
        state.setCaption("New caption"),
    ],
    [
      "image",
      (state: ReturnType<typeof usePostComposer>) =>
        state.setPhoto("https://cdn/new.jpg", "gallery"),
    ],
  ])(
    "marks the normalized creative dirty when %s changes",
    async (_field, change) => {
      (api.generateMarketingCaption as jest.Mock).mockReturnValueOnce(
        new Promise(() => undefined),
      );
      const existing = {
        ...sugg,
        image_url: "https://cdn/original.jpg",
        image_source: "menu" as const,
      };
      const { result } = renderHook(() =>
        usePostComposer({
          business,
          suggestion: existing,
          t,
        }),
      );

      expect(result.current.dirty).toBe(false);
      expect(Object.isFrozen(result.current.initialCreative)).toBe(true);
      act(() => change(result.current));
      expect(result.current.dirty).toBe(true);
    },
  );

  it("normalizes inconsequential whitespace before comparing creative state", () => {
    const { result } = renderHook(() =>
      usePostComposer({ business, suggestion: null, t }),
    );

    act(() => result.current.setCaption("   "));
    expect(result.current.dirty).toBe(false);
  });

  it("setCaption updates the caption (manual edit)", async () => {
    const { result } = renderHook(() =>
      usePostComposer({ business, suggestion: sugg, t }),
    );
    await waitFor(() => expect(result.current.caption).toBe("AI caption"));
    act(() => result.current.setCaption("My own caption"));
    expect(result.current.caption).toBe("My own caption");
  });

  it("passes menu description to image generation when available", async () => {
    const withDesc = { ...sugg, target_description: "Wood-fired, silky sauce" };
    (api.generateMarketingImage as jest.Mock).mockResolvedValue({
      url: "https://s3/p.png",
    });
    const { result } = renderHook(() =>
      usePostComposer({ business, suggestion: withDesc, t }),
    );
    await act(async () => {
      await result.current.regeneratePhoto();
    });
    expect(api.generateMarketingImage).toHaveBeenCalledWith(
      42,
      expect.objectContaining({
        description: "Wood-fired, silky sauce",
      }),
    );
  });

  it("generates from the effective visual mood and selected layout bounds, aspect, and template", async () => {
    const multibyteHeadline = "🍝深夜のカルボナーラ".repeat(15);
    (api.generateMarketingCaption as jest.Mock).mockReturnValueOnce(
      new Promise(() => undefined),
    );
    (api.generateMarketingImage as jest.Mock).mockResolvedValue({
      url: "https://s3/current.png",
    });
    // Photo present so the seed picks a photo family (photoBottomStack), not
    // the no-photo poster. Slot edits do not re-choose composition.
    const withPhoto = { ...sugg, image_url: "https://cdn/carbonara.jpg" };
    const { result } = renderHook(() =>
      usePostComposer({
        business,
        suggestion: withPhoto,
        creativeProfile: {
          audience: "",
          voice: "",
          visual_mood: "moody",
          cta_style: "",
          hashtag_behavior: "",
          avoid_phrases: [],
          default_language: "en",
          default_tone: "",
        },
        t,
      }),
    );
    act(() => {
      result.current.setAspect("9:16");
      result.current.setTemplate("minimal");
      result.current.setSlot("dishName", multibyteHeadline);
    });

    await act(async () => result.current.regeneratePhoto());

    expect(api.generateMarketingImage).toHaveBeenCalledWith(
      42,
      expect.objectContaining({
        name: multibyteHeadline,
        aspect_ratio: "9:16",
        template_style: "minimal",
        visual_mood: "moody",
        // The composition, not the template, now drives the region — and the
        // band travels as its own closed-enum field because the backend only
        // ever consumed safe_region's LENGTH.
        safe_region: expect.stringMatching(
          /^photoBottomStack 9:16 composition; reserve copy within content /,
        ),
        reserved_band: "bottom",
      }),
    );
    expect(
      (api.generateMarketingImage as jest.Mock).mock.calls[0][1].safe_region,
    ).not.toContain(multibyteHeadline);
  });

  it("leaves an automatic visual mood for server-side business derivation", async () => {
    (api.generateMarketingCaption as jest.Mock).mockReturnValueOnce(
      new Promise(() => undefined),
    );
    (api.generateMarketingImage as jest.Mock).mockResolvedValue({
      url: "https://s3/current.png",
    });
    const { result } = renderHook(() =>
      usePostComposer({
        business: { ...business, business_type: "cafe" },
        suggestion: sugg,
        creativeProfile: {
          audience: "",
          voice: "",
          visual_mood: "",
          cta_style: "",
          hashtag_behavior: "",
          avoid_phrases: [],
          default_language: "en",
          default_tone: "",
        },
        t,
      }),
    );

    await act(async () => result.current.regeneratePhoto());

    expect(api.generateMarketingImage).toHaveBeenCalledWith(
      42,
      expect.objectContaining({ visual_mood: undefined }),
    );
  });

  it.each([
    ["photoBottomStack", "1:1"],
    ["badgeHero", "4:5"],
    ["splitPanel", "9:16"],
  ] as const)(
    "keeps the %s %s safe region within the backend 240-rune contract",
    (composition, aspect) => {
      const safeRegion = safeRegionForCreative(composition, aspect);

      expect(Array.from(safeRegion.trim()).length).toBeLessThanOrEqual(240);
    },
  );

  it("uses featured_dish for genuinely play-less manual image requests", async () => {
    (api.generateMarketingImage as jest.Mock).mockResolvedValue({
      url: "https://s3/manual.png",
    });
    const { result } = renderHook(() =>
      usePostComposer({ business, suggestion: null, t }),
    );

    await act(async () => {
      await result.current.regeneratePhoto();
    });

    expect(api.generateMarketingImage).toHaveBeenCalledWith(
      42,
      expect.objectContaining({ play: "featured_dish" }),
    );
  });

  it("requires an item before generating a manual caption", async () => {
    const { result } = renderHook(() =>
      usePostComposer({ business, suggestion: null, t }),
    );

    await act(async () => {
      await result.current.regenerateCaption();
    });

    expect(api.generateMarketingCaption).not.toHaveBeenCalled();
    expect(result.current.error).toBe("caption_item_required");
  });

  it.each([
    ["wide", "1:1"],
    ["strip", "1:1"],
    ["5:7", "4:5"],
    ["9:16", "9:16"],
  ] as const)(
    "generates AI hero at native aspect for format %s → %s",
    async (format, expectedAspect) => {
      (api.generateMarketingImage as jest.Mock).mockResolvedValue({
        url: "https://s3/hero.png",
      });
      const { result } = renderHook(() =>
        usePostComposer({ business, suggestion: sugg, t }),
      );
      act(() => result.current.setAspect(format));
      await act(async () => result.current.regeneratePhoto());
      expect(api.generateMarketingImage).toHaveBeenCalledWith(
        42,
        expect.objectContaining({ aspect_ratio: expectedAspect }),
      );
    },
  );

  it("regenerating the photo on success sets the url", async () => {
    (api.generateMarketingImage as jest.Mock).mockResolvedValue({
      url: "https://s3/p.png",
    });
    const { result } = renderHook(() =>
      usePostComposer({ business, suggestion: sugg, t }),
    );
    await act(async () => {
      await result.current.regeneratePhoto();
    });
    expect(api.generateMarketingImage).toHaveBeenCalledWith(
      42,
      expect.objectContaining({
        name: "Carbonara",
        play: "featured_dish",
      }),
    );
    expect(result.current.photoUrl).toBe("https://s3/p.png");
    expect(result.current.workingCreative.imageSource).toBe("generated");
    expect(result.current.workingCreative.crop).toEqual({
      x: 0.5,
      y: 0.5,
      zoom: 1,
    });
  });

  it("retains aspect but resets crop after successful generation", async () => {
    (api.generateMarketingImage as jest.Mock).mockResolvedValue({
      url: "https://s3/fresh.png",
    });
    const { result } = renderHook(() =>
      usePostComposer({ business, suggestion: sugg, t }),
    );
    act(() => {
      result.current.setAspect("1:1");
      result.current.setCrop({ x: 0.1, y: 0.9, zoom: 2 });
    });

    await act(async () => result.current.regeneratePhoto());

    expect(result.current.aspect).toBe("1:1");
    expect(result.current.renderInput.crop).toEqual({
      x: 0.5,
      y: 0.5,
      zoom: 1,
    });
    expect(result.current.workingCreative.imageSource).toBe("generated");
  });

  it("locks the captured creative against every mutator until paid generation settles", async () => {
    let resolveGeneration!: (result: { url: string }) => void;
    (api.generateMarketingImage as jest.Mock)
      .mockReturnValueOnce(
        new Promise((resolve) => {
          resolveGeneration = resolve;
        }),
      )
      .mockResolvedValue({ url: "https://s3/unexpected-second.png" });
    const { result } = renderHook(() =>
      usePostComposer({ business, suggestion: sugg, t }),
    );
    await waitFor(() => expect(result.current.caption).toBe("AI caption"));
    act(() => {
      result.current.setAspect("1:1");
      result.current.setTemplate("bold");
      result.current.setCrop({ x: 0.2, y: 0.7, zoom: 1.4 });
      result.current.setSlot("dishName", "Captured Carbonara");
      result.current.setCaption("Captured caption");
    });
    const captured = JSON.parse(
      JSON.stringify(result.current.workingCreative),
    ) as typeof result.current.workingCreative;

    let generation!: Promise<void>;
    act(() => {
      generation = result.current.regeneratePhoto();
    });
    await waitFor(() => expect(result.current.isGeneratingPhoto).toBe(true));
    const captionCalls = (api.generateMarketingCaption as jest.Mock).mock.calls
      .length;

    act(() => {
      result.current.setCrop({ x: 0.9, y: 0.1, zoom: 2.8 });
      result.current.resetCrop();
      result.current.setAspect("9:16");
      result.current.setTemplate("minimal");
      result.current.setSlot("dishName", "Diverged headline");
      result.current.setSlot("cta", "Diverged call to action");
      result.current.setCaption("Diverged caption");
      result.current.setPhoto("https://cdn/gallery.jpg", "gallery");
      result.current.setPhotoUrl("https://cdn/upload.jpg");
      result.current.setTone("playful");
      result.current.setLanguage("es");
      void result.current.regenerateCaption();
      void result.current.regeneratePhoto();
      result.current.resetCreative();
    });

    expect(result.current.workingCreative).toEqual(captured);
    expect(result.current.tone).toBe("warm");
    expect(result.current.language).toBe("en");
    expect(api.generateMarketingCaption).toHaveBeenCalledTimes(captionCalls);
    expect(api.generateMarketingImage).toHaveBeenCalledTimes(1);
    expect(result.current.isGeneratingPhoto).toBe(true);

    await act(async () => {
      resolveGeneration({ url: "https://s3/captured.png" });
      await generation;
    });

    expect(result.current.workingCreative).toEqual({
      ...captured,
      photoUrl: "https://s3/captured.png",
      imageSource: "generated",
      crop: { x: 0.5, y: 0.5, zoom: 1 },
    });
    expect(result.current.isGeneratingPhoto).toBe(false);

    act(() => result.current.setAspect("9:16"));
    expect(result.current.aspect).toBe("9:16");
  });

  it("surfaces the daily limit with its reset window", async () => {
    (api.generateMarketingImage as jest.Mock).mockRejectedValueOnce(
      new api.ImageDailyLimitError(500, 21600),
    );
    const { result } = renderHook(() =>
      usePostComposer({ business, suggestion: sugg, t }),
    );
    await act(async () => {
      await result.current.regeneratePhoto();
    });
    expect(result.current.dailyLimitReached).toEqual({
      dailyLimit: 500,
      resetsInSeconds: 21600,
    });
    expect(result.current.photoUrl).toBe("");
  });

  it("clears dailyLimitReached after a subsequent successful generation", async () => {
    (api.generateMarketingImage as jest.Mock)
      .mockRejectedValueOnce(new api.ImageDailyLimitError(500, 21600))
      .mockResolvedValueOnce({ url: "https://s3/after-limit.png" });
    const { result } = renderHook(() =>
      usePostComposer({ business, suggestion: sugg, t }),
    );
    await act(async () => {
      await result.current.regeneratePhoto();
    });
    expect(result.current.dailyLimitReached).toEqual({
      dailyLimit: 500,
      resetsInSeconds: 21600,
    });

    await act(async () => {
      await result.current.regeneratePhoto();
    });
    expect(result.current.dailyLimitReached).toBeNull();
    expect(result.current.photoUrl).toBe("https://s3/after-limit.png");
  });

  it("does not label the photo AI-generated when the returned asset cannot decode", async () => {
    (api.generateMarketingImage as jest.Mock).mockResolvedValue({
      url: "https://media.example.test/menu_items/ai_generated/broken.jpg",
    });
    (renderer.loadOptionalImage as jest.Mock).mockResolvedValue({
      naturalWidth: 800,
      naturalHeight: 1000,
      decode: async () => {
        throw new Error("corrupt");
      },
    });
    const { result } = renderHook(() =>
      usePostComposer({ business, suggestion: sugg, t }),
    );
    await act(async () => {
      await result.current.regeneratePhoto();
    });
    expect(result.current.workingCreative.imageSource).not.toBe("generated");
    expect(result.current.error).toBe("image_decode_failed");
    expect(result.current.photoUrl).not.toBe(
      "https://media.example.test/menu_items/ai_generated/broken.jpg",
    );
  });

  it("persists a readable generated asset and restores it on reopen", async () => {
    const providerUrl =
      "https://media.example.test/menu_items/ai_generated/date-night.jpg";
    (api.generateMarketingImage as jest.Mock).mockResolvedValue({
      url: providerUrl,
    });
    (renderer.loadOptionalImage as jest.Mock).mockImplementation(
      async (url: string) => {
        if (String(url).startsWith("/api/marketing-media")) {
          return {
            naturalWidth: 900,
            naturalHeight: 1125,
            decode: async () => undefined,
          };
        }
        return null;
      },
    );
    const suggestion = {
      ...sugg,
      id: "1:combo_deal:date-night",
      play: "combo_deal" as const,
      image_url: "https://images.unsplash.com/stale.jpg",
      image_source: "bundle" as const,
    };
    const { result, unmount } = renderHook(() =>
      usePostComposer({ business, suggestion, t }),
    );
    await act(async () => {
      await result.current.regeneratePhoto();
    });
    expect(result.current.workingCreative.imageSource).toBe("generated");
    expect(result.current.photoUrl.startsWith("/api/marketing-media")).toBe(
      true,
    );

    unmount();
    (api.generateMarketingCaption as jest.Mock).mockReturnValue(
      new Promise(() => undefined),
    );
    const reopened = renderHook(() =>
      usePostComposer({ business, suggestion, t }),
    );
    expect(reopened.result.current.photoUrl).toBe(result.current.photoUrl);
    expect(reopened.result.current.imageSource).toBe("generated");
    expect(api.generateMarketingImage).toHaveBeenCalledTimes(1);
    reopened.unmount();
  });

  it("does not set dailyLimitReached for a generic rate-limiter 429", async () => {
    (api.generateMarketingCaption as jest.Mock).mockReturnValueOnce(
      new Promise(() => undefined),
    );
    (api.generateMarketingImage as jest.Mock).mockRejectedValueOnce({
      response: { status: 429, data: { message: "too many requests" } },
    });
    const { result } = renderHook(() =>
      usePostComposer({ business, suggestion: sugg, t }),
    );
    await act(async () => {
      await result.current.regeneratePhoto();
    });
    expect(result.current.dailyLimitReached).toBeNull();
    expect(result.current.error).toBe("image_failed");
  });

  it("changing tone triggers a caption regeneration", async () => {
    (api.generateMarketingCaption as jest.Mock).mockResolvedValue(
      "New caption",
    );
    const { result } = renderHook(() =>
      usePostComposer({ business, suggestion: sugg, t }),
    );
    await waitFor(() =>
      expect(api.generateMarketingCaption).toHaveBeenCalled(),
    );
    await act(async () => {
      result.current.setTone("playful");
    });
    await waitFor(() =>
      expect(
        (api.generateMarketingCaption as jest.Mock).mock.calls.length,
      ).toBeGreaterThan(1),
    );
    const playfulCall = (
      api.generateMarketingCaption as jest.Mock
    ).mock.calls.find((call) => call[1]?.tone === "playful");
    expect(playfulCall?.[1]).toEqual(
      expect.objectContaining({ tone: "playful", item_name: "Carbonara" }),
    );
    await waitFor(() => expect(result.current.caption).toBe("New caption"));
  });

  it("regenerate caption sends destination max_chars and hashtag override", async () => {
    (api.generateMarketingCaption as jest.Mock).mockResolvedValue(
      "Story caption",
    );
    const { result } = renderHook(() =>
      usePostComposer({
        business,
        suggestion: sugg,
        creativeProfile: {
          audience: "locals",
          voice: "warm",
          visual_mood: "",
          cta_style: "soft",
          hashtag_behavior: "standard",
          avoid_phrases: ["best ever"],
          default_language: "es-AR",
          default_tone: "warm",
        },
        t,
      }),
    );
    act(() => {
      result.current.setDestination("ig_stories");
      result.current.setSlot("cta", "PROBALO");
      result.current.setSlot("handle", "@cafesur");
    });
    await act(async () => {
      await result.current.regenerateCaption();
    });
    const lastCall = (api.generateMarketingCaption as jest.Mock).mock.calls.at(
      -1,
    );
    expect(lastCall?.[1]).toEqual(
      expect.objectContaining({
        item_name: "Carbonara",
        max_chars: 80,
        hashtag_behavior: "standard",
        language: "es-AR",
        tone: "warm",
        must_include: ["PROBALO", "@cafesur"],
      }),
    );
  });

  it("google destination forces hashtag_behavior none on regenerate", async () => {
    (api.generateMarketingCaption as jest.Mock).mockResolvedValue("GMB copy");
    const { result } = renderHook(() =>
      usePostComposer({
        business,
        suggestion: sugg,
        creativeProfile: {
          audience: "",
          voice: "",
          visual_mood: "",
          cta_style: "",
          hashtag_behavior: "standard",
          avoid_phrases: [],
          default_language: "en",
          default_tone: "warm",
        },
        t,
      }),
    );
    act(() => result.current.setDestination("google_business"));
    await act(async () => {
      await result.current.regenerateCaption();
    });
    const lastCall = (api.generateMarketingCaption as jest.Mock).mock.calls.at(
      -1,
    );
    expect(lastCall?.[1]).toEqual(
      expect.objectContaining({
        hashtag_behavior: "none",
        max_chars: 280,
      }),
    );
  });

  it("download renders a blob and triggers a file download", async () => {
    (renderer.renderPostToBlob as jest.Mock).mockResolvedValue(
      new Blob(["x"], { type: "image/png" }),
    );
    const { result } = renderHook(() =>
      usePostComposer({ business, suggestion: sugg, t }),
    );
    act(() => result.current.setPhotoUrl("https://s3/p.png"));
    await act(async () => {
      await result.current.downloadImage();
    });
    expect(result.current.renderInput.crop).toEqual({
      x: 0.5,
      y: 0.5,
      zoom: 1,
    });
    expect(result.current.renderInput.palette.fontFamily).toBe("Serif");
    expect(renderer.renderPostToBlob).toHaveBeenCalledWith(
      result.current.renderInput,
    );
  });

  it("downloadPack returns true after a rendered blob download", async () => {
    (renderer.renderPostToBlob as jest.Mock).mockResolvedValue(
      new Blob(["png"], { type: "image/png" }),
    );
    Object.defineProperty(URL, "createObjectURL", {
      configurable: true,
      value: jest.fn(() => "blob:pack"),
    });
    Object.defineProperty(URL, "revokeObjectURL", {
      configurable: true,
      value: jest.fn(),
    });
    const click = jest
      .spyOn(HTMLAnchorElement.prototype, "click")
      .mockImplementation(() => undefined);
    const { result } = renderHook(() =>
      usePostComposer({ business, suggestion: sugg, t }),
    );
    act(() => {
      result.current.setPhotoUrl("https://s3/p.png");
      result.current.setCaption("Tonight only.");
    });
    let ok = false;
    await act(async () => {
      ok = await result.current.downloadPack();
    });
    expect(ok).toBe(true);
    expect(result.current.error).toBeNull();
    expect(click).toHaveBeenCalled();
    click.mockRestore();
  });

  it("downloadPack returns false and sets download_failed when render throws", async () => {
    (renderer.renderPostToBlob as jest.Mock).mockRejectedValue(
      new Error("photo_load_failed"),
    );
    const { result } = renderHook(() =>
      usePostComposer({ business, suggestion: sugg, t }),
    );
    act(() => {
      result.current.setPhotoUrl("https://s3/p.png");
      result.current.setCaption("Tonight only.");
    });
    let ok = true;
    await act(async () => {
      ok = await result.current.downloadPack();
    });
    expect(ok).toBe(false);
    expect(result.current.error).toBe("download_failed");
  });
});

describe("usePostComposer art direction", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (api.generateMarketingCaption as jest.Mock).mockResolvedValue("AI caption");
  });

  // `template: "minimal"` throughout: it makes the legacy derivation land on
  // "minimal", so any test asserting a restored kit fails loudly if the hook
  // quietly re-derives from the template instead of reading the stored kit.
  const snapshot = (
    over: Partial<MarketingCreativeSnapshot> = {},
  ): MarketingCreativeSnapshot => ({
    image_url: "https://cdn/reused.jpg",
    image_source: "gallery",
    caption: "Reused exact caption",
    aspect: "4:5",
    template: "minimal",
    crop: { x: 0.5, y: 0.5, zoom: 1 },
    slots: { dishName: "Reused dish", cta: "VISIT", handle: "@archive" },
    font_family: "Sans",
    ...over,
  });

  const open = (initialSnapshot?: MarketingCreativeSnapshot) =>
    renderHook(() =>
      usePostComposer({
        business,
        suggestion: sugg,
        initialSnapshot,
        t,
      }),
    );

  it("restores the exact kit and composition the operator saved", () => {
    const { result } = open(
      snapshot({ kit: "chalkboard", composition: "posterStack" }),
    );

    expect(result.current.kit).toBe("chalkboard");
    expect(result.current.composition).toBe("posterStack");
    // The restored choice is the clean baseline, not an unsaved edit.
    expect(result.current.initialCreative.kit).toBe("chalkboard");
    expect(result.current.initialCreative.composition).toBe("posterStack");
    expect(result.current.dirty).toBe(false);
  });

  it("derives the kit from the legacy template on a snapshot saved before kits existed", () => {
    const { result } = open(snapshot({ template: "bold" }));

    expect(result.current.kit).toBe("bold");
    expect(KITS[result.current.kit]).toBeDefined();
    expect(result.current.composition).toBe("photoBottomStack");
  });

  it("falls back to the legacy derivation when the stored ids are unknown to this build", () => {
    const { result } = open(
      snapshot({ kit: "neon-vaporwave", composition: "hologram" }),
    );

    // Not a crash, not a blank post: both land on something with a definition.
    expect(result.current.kit).toBe("minimal");
    expect(KITS[result.current.kit]).toBeDefined();
    expect(result.current.composition).toBe("photoBottomStack");
    expect(COMPOSITIONS[result.current.composition]).toBeDefined();
  });

  it("seeds a fresh creative from the default template and the content", async () => {
    const withPhoto = { ...sugg, image_url: "https://cdn/fresh.jpg" };
    const { result } = renderHook(() =>
      usePostComposer({
        business,
        suggestion: withPhoto,
        t,
      }),
    );

    expect(result.current.kit).toBe("editorial");
    expect(result.current.composition).toBe("photoBottomStack");
    await waitFor(() => expect(result.current.caption).toBe("AI caption"));
    // The bootstrapped caption must not smuggle in an art-direction change.
    expect(result.current.dirty).toBe(false);
  });

  it("chooses the poster composition for a post with no photo", () => {
    const { result } = open(snapshot({ image_url: "" }));

    expect(result.current.composition).toBe("posterStack");
  });

  it("treats a kit change as a dirty edit and restores it on reset", () => {
    const { result } = open(snapshot({ kit: "chalkboard" }));
    expect(result.current.dirty).toBe(false);

    act(() => result.current.setKit("ticket"));

    expect(result.current.kit).toBe("ticket");
    expect(result.current.workingCreative.kit).toBe("ticket");
    expect(result.current.dirty).toBe(true);

    act(() => result.current.resetCreative());

    expect(result.current.kit).toBe("chalkboard");
    expect(result.current.dirty).toBe(false);
  });

  it("treats a kit-format toggle as a dirty edit and restores formats on reset", () => {
    const { result } = open(snapshot({ aspect: "4:5" }));
    expect(result.current.dirty).toBe(false);
    const before = [...result.current.kitFormats];

    act(() => result.current.toggleKitFormat("wide"));

    expect(result.current.kitFormats).not.toEqual(before);
    expect(result.current.dirty).toBe(true);

    act(() => result.current.resetCreative());

    expect(result.current.kitFormats).toEqual(before);
    expect(result.current.dirty).toBe(false);
  });

  it("treats a motion-preset pick as a dirty edit and restores it on reset", () => {
    const { result } = open(snapshot({ kit: "chalkboard" }));
    const baseline = result.current.motionPreset;
    expect(result.current.dirty).toBe(false);

    act(() => result.current.setMotionPreset("slowPan"));

    expect(result.current.motionPreset).toBe("slowPan");
    expect(result.current.dirty).toBe(true);

    act(() => result.current.resetCreative());

    expect(result.current.motionPreset).toBe(baseline);
    expect(result.current.dirty).toBe(false);
  });

  it("restores mediaKind video baseline on discard after markMotionExported", () => {
    const { result } = open(
      snapshot({
        kit: "chalkboard",
        media_kind: "video",
        motion_preset: "slowPan",
      }),
    );
    expect(result.current.mediaKind).toBe("video");
    expect(result.current.dirty).toBe(false);

    act(() => result.current.setKit("ticket"));
    expect(result.current.dirty).toBe(true);

    act(() => result.current.resetCreative());

    expect(result.current.mediaKind).toBe("video");
    expect(result.current.motionPreset).toBe("slowPan");
    expect(result.current.dirty).toBe(false);
  });

  it("treats a reshuffle as a dirty edit and restores the composition on reset", () => {
    const { result } = open(snapshot({ composition: "photoBottomStack" }));

    act(() => result.current.reshuffle());

    expect(result.current.composition).not.toBe("photoBottomStack");
    expect(result.current.dirty).toBe(true);

    act(() => result.current.resetCreative());

    expect(result.current.composition).toBe("photoBottomStack");
    expect(result.current.dirty).toBe(false);
  });

  it("rotates through every photo composition and returns to where it started", () => {
    const { result } = open(snapshot({ composition: "photoBottomStack" }));
    const seen: string[] = [];

    for (let i = 0; i < CHOOSABLE_COMPOSITIONS.length; i += 1) {
      act(() => result.current.reshuffle());
      seen.push(result.current.composition);
    }

    expect(new Set(seen)).toEqual(new Set(CHOOSABLE_COMPOSITIONS));
    expect(result.current.composition).toBe("photoBottomStack");
  });

  it("reports reshuffle as unavailable with no photo, where the rotation is a fixed point", () => {
    const { result } = open(snapshot({ image_url: "" }));

    expect(result.current.canReshuffle).toBe(false);

    act(() => result.current.reshuffle());

    // The control would be a no-op, which is why the editor has to disable it.
    expect(result.current.composition).toBe("posterStack");
    expect(result.current.dirty).toBe(false);
  });

  it("reports reshuffle as available once a photo is present, and withdraws it when the photo goes", () => {
    const { result } = open(snapshot({ image_url: "" }));
    expect(result.current.canReshuffle).toBe(false);

    act(() => result.current.setPhotoUrl("https://cdn/new.jpg"));
    expect(result.current.canReshuffle).toBe(true);

    act(() => result.current.setPhotoUrl(""));
    expect(result.current.canReshuffle).toBe(false);
  });

  /**
   * `renderInput` is the single object the preview, the download, the pack and
   * the share sheet all render from, so it is the only place the art direction
   * can be attached without the four disagreeing. These pin that it carries the
   * kit path and NOT the legacy `template`, which `resolveArtDirection` would
   * otherwise be free to prefer.
   */
  it("renders through the kit and composition rather than the legacy template", () => {
    const { result } = open(
      snapshot({ kit: "chalkboard", composition: "posterStack" }),
    );

    expect(result.current.renderInput.kit).toBe("chalkboard");
    expect(result.current.renderInput.composition).toBe("posterStack");
    expect(result.current.renderInput.template).toBeUndefined();
  });

  it("carries a kit change and a reshuffle straight onto the render input", () => {
    const { result } = open(snapshot({ composition: "photoBottomStack" }));

    act(() => result.current.setKit("linen"));
    expect(result.current.renderInput.kit).toBe("linen");

    act(() => result.current.reshuffle());
    expect(result.current.renderInput.composition).toBe(
      result.current.composition,
    );
    expect(result.current.renderInput.composition).not.toBe("photoBottomStack");
  });

  /**
   * The template picker is gone from the editor, but `templateStyle` is not:
   * it is what the handoff persists as `creative.template` and what seeds the
   * kit for pre-scene snapshots. Nothing re-derives it from the kit, so a
   * restored snapshot keeps its own. Image-gen geometry reads composition.
   */
  it("keeps the stored template style alongside an independently chosen kit", () => {
    const { result } = open(snapshot({ template: "bold", kit: "ticket" }));

    expect(result.current.templateStyle).toBe("bold");
    expect(result.current.kit).toBe("ticket");

    act(() => result.current.setKit("linen"));

    expect(result.current.templateStyle).toBe("bold");
    expect(result.current.workingCreative.templateStyle).toBe("bold");
  });

  /**
   * Storage-side kit/legacy pairing guard (Wave 1 closeout extension).
   *
   * `resolveArtDirection` already refuses a legacy composition on the kit path
   * at render time, but `seedArtDirection` used to restore kit and composition
   * independently via `isCompositionId` (all nine ids). A stored
   * `{ kit: "ticket", composition: "legacyEditorial" }` therefore reopened as
   * that pair in PostCreative — and every downstream surface (dirty baseline,
   * handoff persist, renderInput) carried the toxic pairing until the renderer
   * silently re-chose. Refuse it on seed so the working creative never holds it.
   */
  it("refuses a stored legacy composition when a kit is restored (textured kit)", () => {
    const { result } = open(
      snapshot({ kit: "ticket", composition: "legacyEditorial" }),
    );

    expect(result.current.kit).toBe("ticket");
    // Not the legacy family: re-chosen for content (photo present → photoBottomStack).
    expect(result.current.composition).toBe("photoBottomStack");
    expect(result.current.composition.startsWith("legacy")).toBe(false);
    expect(result.current.initialCreative.composition).toBe("photoBottomStack");
    expect(result.current.renderInput.composition).toBe("photoBottomStack");
    expect(result.current.dirty).toBe(false);
  });

  it("refuses every legacy family for every stored kit, not only textured ones", () => {
    const kits = Object.keys(KITS) as Array<keyof typeof KITS>;
    const legacyIds = [
      "legacyEditorial",
      "legacyBold",
      "legacyMinimal",
    ] as const;

    kits.forEach((kit) => {
      legacyIds.forEach((composition) => {
        const { result } = open(snapshot({ kit, composition }));
        expect(result.current.kit).toBe(kit);
        expect(result.current.composition.startsWith("legacy")).toBe(false);
        expect(COMPOSITIONS[result.current.composition]).toBeDefined();
        expect(
          (CHOOSABLE_COMPOSITIONS as readonly string[]).includes(
            result.current.composition,
          ),
        ).toBe(true);
      });
    });
  });

  /** Kit-only creative: stored kit, no composition — chooser fills the layout. */
  it("still seeds a kit-only snapshot (no stored composition)", () => {
    const { result } = open(snapshot({ kit: "chalkboard" }));

    expect(result.current.kit).toBe("chalkboard");
    expect(result.current.composition).toBe("photoBottomStack");
    expect(result.current.dirty).toBe(false);
  });

  /**
   * Legacy-only creative: no stored kit, no stored composition — the pre-scene
   * snapshot path. Kit derives from template; composition from the chooser.
   * Must remain untouched by the pairing guard.
   */
  it("still seeds a legacy-only snapshot (no kit, no composition)", () => {
    const { result } = open(snapshot({ template: "bold" }));

    expect(result.current.kit).toBe("bold");
    expect(result.current.composition).toBe("photoBottomStack");
    expect(result.current.composition.startsWith("legacy")).toBe(false);
    expect(result.current.dirty).toBe(false);
  });

  it("still restores a choosable composition stored beside a kit", () => {
    const { result } = open(
      snapshot({ kit: "ticket", composition: "badgeHero" }),
    );

    expect(result.current.kit).toBe("ticket");
    expect(result.current.composition).toBe("badgeHero");
    expect(result.current.dirty).toBe(false);
  });
});

/**
 * The seed for a creative that has never been saved — the branch with no stored
 * kit, no stored composition and no snapshot to restore. Everything it opens
 * with is derived, so every derivation needs a pin.
 */
describe("usePostComposer fresh-creative seeding", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    // Never resolves: the bootstrap caption is irrelevant to seeding, and a
    // resolved one lands state outside act().
    (api.generateMarketingCaption as jest.Mock).mockReturnValue(
      new Promise(() => undefined),
    );
  });

  const fresh = (over: Partial<CampaignSuggestion>) =>
    renderHook(() =>
      usePostComposer({
        business,
        suggestion: { ...sugg, ...over } as CampaignSuggestion,
        t,
      }),
    );

  /**
   * The plays are chosen so the two candidate expressions DISAGREE. The suite's
   * own `sugg` fixture is `featured_dish`, where `defaultTemplateForPlay` and a
   * hard-coded `"editorial"` happen to return the same thing — which is exactly
   * why the pre-existing fresh-seed test could not see the defect. `move_item`
   * and `featured_dish` stay in the table as the controls that keep the fix
   * from degenerating into "always bold".
   */
  it.each([
    ["offer", "bold"],
    ["happy_hour", "bold"],
    ["combo_deal", "bold"],
    ["win_back", "minimal"],
    ["featured_dish", "editorial"],
    ["move_item", "editorial"],
  ] as const)("opens a fresh %s post in the %s kit", (play, kit) => {
    const { result } = fresh({ play, image_url: "https://cdn/fresh.jpg" });

    expect(result.current.kit).toBe(kit);
    expect(KITS[result.current.kit]).toBeDefined();
    // A derived seed is the clean baseline, never an unsaved edit.
    expect(result.current.dirty).toBe(false);
    expect(result.current.initialCreative.kit).toBe(kit);
  });

  /**
   * `templateStyle` follows the same expression rather than staying pinned to
   * `"editorial"`. It is what the drawer persists as `creative.template`
   * (PostEditorDrawer.tsx:322) and what seeds the kit when a snapshot carries
   * no kit — so a `template` that disagrees with the `kit` saved beside it
   * silently downgrades a Bold offer to Editorial on any reopen that has to
   * fall back. One expression for both makes that disagreement unrepresentable.
   */
  it.each([
    ["offer", "bold"],
    ["happy_hour", "bold"],
    ["combo_deal", "bold"],
    ["win_back", "minimal"],
    ["featured_dish", "editorial"],
  ] as const)(
    "seeds the %s template style and kit from one expression",
    (play, template) => {
      const { result } = fresh({ play });

      expect(result.current.templateStyle).toBe(template);
      expect(kitForLegacyTemplate(result.current.templateStyle)).toBe(
        result.current.kit,
      );
    },
  );

  /**
   * Image-gen geometry follows the composition (and `reserved_band`), while
   * `template_style` still travels for prompt flavour. An offer seeds Bold and
   * the default photoBottomStack layout — both must land on the request.
   */
  it("composes the image-generation safe region from the composition", async () => {
    (api.generateMarketingImage as jest.Mock).mockResolvedValue({
      url: "https://s3/offer.png",
    });
    const { result } = fresh({
      play: "offer",
      target_name: "Summer Sale",
      // Photo present → photoBottomStack (not no-photo posterStack).
      image_url: "https://cdn/sale.jpg",
    });

    await act(async () => result.current.regeneratePhoto());

    expect(api.generateMarketingImage).toHaveBeenCalledWith(
      42,
      expect.objectContaining({
        template_style: "bold",
        safe_region: expect.stringMatching(/^photoBottomStack 4:5/),
        reserved_band: "bottom",
      }),
    );
  });

  /**
   * `hasDiscount` deliberately diverges from the Wave 1 plan, which specified
   * `Boolean(suggestion?.discount_type)`. It mirrors `buildSlots`
   * (postContent.ts:275-288) instead, because `chooseComposition` answers
   * `badgeHero` on `hasDiscount && hasPhoto` and a badge hero with no badge
   * text is a layout with a hole in it.
   *
   * The gap is reachable, not theoretical: the backend's `DiscountValue` is
   * `json:"discount_value,omitempty"` (suggestion.go:53), so a zero-value offer
   * ships `discount_type` alone — and `formatDiscountLabel` then emits nothing.
   * Do not "restore" the plan's predicate.
   */
  it("keeps a valueless offer discount off the badge hero, because there is no badge to hero", () => {
    const { result } = fresh({
      play: "offer",
      image_url: "https://cdn/offer.jpg",
      discount_type: "percentage",
    });

    expect(result.current.slots.badge).toBe("");
    expect(result.current.composition).toBe("photoBottomStack");
  });

  it("routes a fully specified offer discount to the badge hero", () => {
    const { result } = fresh({
      play: "offer",
      image_url: "https://cdn/offer.jpg",
      discount_type: "percentage",
      discount_value: 20,
    });

    expect(result.current.slots.badge).toMatch(/^20% /);
    expect(result.current.composition).toBe("badgeHero");
  });

  /**
   * One default for the play signal. The seed and the live rotation describe
   * the same post, so they cannot answer "which play is this?" differently —
   * inert only while `chooseComposition` ignores `content.play`.
   *
   * `""` is the answer, not `"featured_dish"`: `buildSlots` already builds a
   * manual post's slots from `PLAY_SLOTS[""]` (templates.ts:495), so the
   * signals describing that content have to name the same play the content
   * came from. The API's `play` is a separate concern with its own pin ("uses
   * featured_dish for genuinely play-less manual image requests" above) — it is
   * typed `MarketingPlay` and cannot take `""`.
   */
  it("feeds the seed and the live rotation the same play signal for a manual post", () => {
    const { result } = renderHook(() =>
      usePostComposer({ business, suggestion: null, t }),
    );
    const seedSignals = (chooseComposition as jest.Mock).mock.calls[0][0];

    act(() => result.current.setPhotoUrl("https://cdn/manual.jpg"));
    act(() => result.current.reshuffle());
    const rotationSignals = (nextComposition as jest.Mock).mock.calls.at(
      -1,
    )![1];

    expect(seedSignals.play).toBe("");
    expect(rotationSignals.play).toBe(seedSignals.play);
  });
});

function stubAnalysis(over: Partial<PhotoAnalysis>): PhotoAnalysis {
  return {
    luma: { size: 1, values: [128] },
    edges: { size: 1, values: [0] },
    blurScore: 500,
    exposure: {
      histogram: new Array(16).fill(0),
      meanLuma: 128,
      shadowClipping: 0,
      highlightClipping: 0,
      channelMeans: { r: 128, g: 128, b: 128 },
    },
    subject: { x: 0, y: 0, w: 1, h: 1 },
    focal: { x: 0.5, y: 0.5 },
    negativeSpace: "none",
    busy: false,
    dominantColors: [],
    ...over,
  };
}

describe("usePostComposer photo analysis", () => {
  beforeEach(() => {
    (renderer.loadOptionalImage as jest.Mock).mockResolvedValue({
      naturalWidth: 1200,
      naturalHeight: 800,
    });
  });

  afterEach(() => {
    (renderer.loadOptionalImage as jest.Mock).mockResolvedValue(null);
    jest.spyOn(photoCache, "photoAnalysisFor").mockRestore();
  });

  it("seeds the crop from the subject and does not mark the editor dirty", async () => {
    jest
      .spyOn(photoCache, "photoAnalysisFor")
      .mockReturnValue(stubAnalysis({ focal: { x: 0.7, y: 0.35 } }));
    const { result } = renderHook(() =>
      usePostComposer({
        business,
        suggestion: { ...sugg, image_url: "https://cdn/dish-focal.png" },
        t,
      }),
    );
    await waitFor(() => expect(result.current.crop.x).toBeCloseTo(0.7, 6));
    expect(result.current.crop.y).toBeCloseTo(0.35, 6);
    expect(result.current.dirty).toBe(false);
  });

  it("upgrades an auto composition once the photo is measured", async () => {
    jest
      .spyOn(photoCache, "photoAnalysisFor")
      .mockReturnValue(stubAnalysis({ negativeSpace: "top" }));
    const { result } = renderHook(() =>
      usePostComposer({
        business,
        suggestion: { ...sugg, image_url: "https://cdn/dish-top.png" },
        t,
      }),
    );
    await waitFor(() =>
      expect(result.current.composition).toBe("photoTopStack"),
    );
    expect(result.current.dirty).toBe(false);
  });

  it("never overrides a composition the operator reshuffled to", async () => {
    jest
      .spyOn(photoCache, "photoAnalysisFor")
      .mockReturnValue(stubAnalysis({ negativeSpace: "top" }));
    const { result } = renderHook(() =>
      usePostComposer({
        business,
        suggestion: { ...sugg, image_url: "https://cdn/dish-reshuffle.png" },
        t,
      }),
    );
    act(() => result.current.reshuffle());
    const chosen = result.current.composition;
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 20));
    });
    expect(result.current.composition).toBe(chosen);
    expect(result.current.dirty).toBe(true);
  });

  it("never overrides a crop the operator dragged", async () => {
    jest
      .spyOn(photoCache, "photoAnalysisFor")
      .mockReturnValue(stubAnalysis({ focal: { x: 0.7, y: 0.35 } }));
    const { result } = renderHook(() =>
      usePostComposer({
        business,
        suggestion: { ...sugg, image_url: "https://cdn/dish-drag.png" },
        t,
      }),
    );
    act(() => result.current.setCrop({ x: 0.2, y: 0.9, zoom: 2 }));
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 20));
    });
    expect(result.current.crop).toEqual({ x: 0.2, y: 0.9, zoom: 2 });
  });

  it("never overrides a composition restored from a stored snapshot", async () => {
    jest
      .spyOn(photoCache, "photoAnalysisFor")
      .mockReturnValue(stubAnalysis({ negativeSpace: "top" }));
    const { result } = renderHook(() =>
      usePostComposer({
        business,
        suggestion: sugg,
        initialSnapshot: {
          caption: "c",
          image_url: "https://cdn/dish-snapshot.png",
          image_source: "menu",
          template: "editorial",
          aspect: "4:5",
          slots: { dishName: "Milanesa" },
          crop: { x: 0.5, y: 0.5, zoom: 1 },
          kit: "linen",
          composition: "cornerCard",
        },
        t,
      }),
    );
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 20));
    });
    expect(result.current.composition).toBe("cornerCard");
  });
});

describe("cleanupPhoto", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("cleans up the current photo and keeps it as the operator's own", async () => {
    (api.cleanupMarketingImage as jest.Mock).mockResolvedValue({
      url: "https://images.payverge.io/clean.png",
    });
    const { result } = renderHook(() =>
      usePostComposer({
        business,
        suggestion: {
          ...sugg,
          image_url: "https://images.payverge.io/raw.png",
        },
        t,
      }),
    );
    await act(async () => result.current.cleanupPhoto());
    expect(api.cleanupMarketingImage).toHaveBeenCalledWith(
      42,
      expect.objectContaining({
        image_url: "https://images.payverge.io/raw.png",
        aspect_ratio: "4:5",
      }),
    );
    expect(result.current.photoUrl).toBe(
      "https://images.payverge.io/clean.png",
    );
    // The result is still the operator's dish, so the provenance tag must NOT
    // flip to "generated" — that label is what tells them it is not their food.
    expect(result.current.imageSource).toBe("menu");
  });

  it.each([
    ["wide", "1:1"],
    ["strip", "1:1"],
    ["5:7", "4:5"],
  ] as const)(
    "coerces cleanup aspect for non-native format %s → %s (intentional re-crop)",
    async (format, expectedAspect) => {
      (api.cleanupMarketingImage as jest.Mock).mockResolvedValue({
        url: "https://images.payverge.io/clean.png",
      });
      const { result } = renderHook(() =>
        usePostComposer({
          business,
          suggestion: {
            ...sugg,
            image_url: "https://images.payverge.io/raw.png",
          },
          t,
        }),
      );
      act(() => result.current.setAspect(format));
      await act(async () => result.current.cleanupPhoto());
      expect(api.cleanupMarketingImage).toHaveBeenCalledWith(
        42,
        expect.objectContaining({ aspect_ratio: expectedAspect }),
      );
    },
  );

  it("surfaces the daily limit on cleanup without changing the photo", async () => {
    (api.cleanupMarketingImage as jest.Mock).mockRejectedValue(
      new api.ImageDailyLimitError(500, 7200),
    );
    const { result } = renderHook(() =>
      usePostComposer({
        business,
        suggestion: {
          ...sugg,
          image_url: "https://images.payverge.io/raw.png",
        },
        t,
      }),
    );
    await act(async () => result.current.cleanupPhoto());
    expect(result.current.dailyLimitReached).toEqual({
      dailyLimit: 500,
      resetsInSeconds: 7200,
    });
    expect(result.current.photoUrl).toBe("https://images.payverge.io/raw.png");
  });

  it("does nothing without a photo", async () => {
    const { result } = renderHook(() =>
      usePostComposer({ business, suggestion: null, t }),
    );
    await act(async () => result.current.cleanupPhoto());
    expect(api.cleanupMarketingImage).not.toHaveBeenCalled();
  });
});

describe("art-direction seed agrees with the feed card", () => {
  it("seeds the kit from the play, not from a constant", () => {
    const { result } = renderHook(() =>
      usePostComposer({
        business,
        suggestion: { ...sugg, play: "win_back" },
        t,
      }),
    );
    // defaultTemplateForPlay("win_back") is "minimal", and the kit ids are the
    // same three names, so a win-back opens minimal in the editor exactly as it
    // renders minimal in the feed.
    expect(result.current.templateStyle).toBe("minimal");
    expect(result.current.kit).toBe("minimal");
  });

  it("seeds a bold kit for an offer", () => {
    const { result } = renderHook(() =>
      usePostComposer({
        business,
        suggestion: {
          ...sugg,
          play: "offer",
          discount_value: 20,
        } as CampaignSuggestion,
        t,
      }),
    );
    expect(result.current.kit).toBe("bold");
  });

  it("still prefers a stored kit over the play default", () => {
    const { result } = renderHook(() =>
      usePostComposer({
        business,
        suggestion: { ...sugg, play: "win_back" },
        initialSnapshot: {
          caption: "c",
          image_url: "",
          image_source: "menu",
          template: "minimal",
          aspect: "4:5",
          slots: {},
          crop: { x: 0.5, y: 0.5, zoom: 1 },
          kit: "linen",
        },
        t,
      }),
    );
    expect(result.current.kit).toBe("linen");
  });

  describe("motion preset", () => {
    function renderComposer(
      opts: {
        storedKit?: string;
        storedMotionPreset?: string;
        storedMediaKind?: string;
      } = {},
    ) {
      const initialSnapshot: MarketingCreativeSnapshot | undefined =
        opts.storedKit || opts.storedMotionPreset || opts.storedMediaKind
          ? {
              caption: "c",
              image_url: "https://cdn/x.jpg",
              image_source: "gallery",
              template: "editorial",
              aspect: "4:5",
              slots: { dishName: "X" },
              crop: { x: 0.5, y: 0.5, zoom: 1 },
              ...(opts.storedKit ? { kit: opts.storedKit } : {}),
              ...(opts.storedMotionPreset
                ? { motion_preset: opts.storedMotionPreset }
                : {}),
              ...(opts.storedMediaKind
                ? { media_kind: opts.storedMediaKind }
                : {}),
            }
          : undefined;
      return renderHook(() =>
        usePostComposer({
          business,
          suggestion: sugg,
          initialSnapshot,
          t,
        }),
      );
    }

    it("defaults to the preset the kit prescribes", () => {
      const { result } = renderComposer({ storedKit: "chalkboard" });
      expect(result.current.motionPreset).toBe("grainDrift");
    });

    it("follows the kit when the operator changes it", () => {
      const { result } = renderComposer({ storedKit: "editorial" });
      act(() => result.current.setKit("ticket"));
      expect(result.current.motionPreset).toBe("ticketSlide");
    });

    it("keeps an explicit choice when the operator overrides the kit default", () => {
      const { result } = renderComposer({ storedKit: "editorial" });
      act(() => result.current.setMotionPreset("slowPan"));
      expect(result.current.motionPreset).toBe("slowPan");
      act(() => result.current.setKit("bold"));
      expect(result.current.motionPreset).toBe("slowPan");
    });

    it("restores a stored preset on reopen", () => {
      const { result } = renderComposer({
        storedKit: "editorial",
        storedMotionPreset: "badgePop",
      });
      expect(result.current.motionPreset).toBe("badgePop");
    });

    it("falls back to the kit default for an unrecognised stored preset", () => {
      const { result } = renderComposer({
        storedKit: "linen",
        storedMotionPreset: "zoomBlur",
      });
      expect(result.current.motionPreset).toBe("slowPan");
    });

    it("starts as a still and only becomes a video once one is exported", () => {
      const { result } = renderComposer({ storedKit: "editorial" });
      expect(result.current.mediaKind).toBe("image");
      act(() => result.current.markMotionExported());
      expect(result.current.mediaKind).toBe("video");
    });

    it("restores a stored video media kind on reopen", () => {
      const { result } = renderComposer({
        storedKit: "editorial",
        storedMediaKind: "video",
        storedMotionPreset: "pushIn",
      });
      expect(result.current.mediaKind).toBe("video");
    });
  });
});
