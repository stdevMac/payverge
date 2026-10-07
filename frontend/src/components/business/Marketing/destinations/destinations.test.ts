import { FORMATS, FORMAT_ORDER, isFormatId } from "../formats/formats";
import {
  CAPTION_GENERATION_MODEL_CEILING,
  captionBudgetLevel,
  captionForClipboard,
  captionGenerationOptionsFor,
  captionProfileFor,
  DEFAULT_DESTINATION_ID,
  DESTINATION_ORDER,
  DESTINATIONS,
  destinationFor,
  destinationPackContents,
  destinationPlatformLabelKey,
  effectiveHashtagBehavior,
  FORBIDDEN_DESTINATION_KEYS,
  generationMaxCharsFor,
  isDestinationId,
  mustIncludePhrasesFromSlots,
  packFormatsForDestination,
  simpleModeFormatsForDestination,
  type CaptionLengthProfile,
  type DestinationId,
} from "./destinations";

describe("destinations registry", () => {
  it("keeps DESTINATION_ORDER in sync with DESTINATIONS", () => {
    expect(new Set(DESTINATION_ORDER)).toEqual(
      new Set(Object.keys(DESTINATIONS)),
    );
    expect(DESTINATION_ORDER).toHaveLength(new Set(DESTINATION_ORDER).size);
  });

  it("references only valid FormatIds from FORMATS", () => {
    DESTINATION_ORDER.forEach((id) => {
      const dest = DESTINATIONS[id];
      expect(isFormatId(dest.primaryFormat)).toBe(true);
      expect(FORMATS[dest.primaryFormat]).toBeDefined();
      dest.formats.forEach((formatId) => {
        expect(isFormatId(formatId)).toBe(true);
        expect(FORMATS[formatId]).toBeDefined();
        expect(FORMAT_ORDER).toContain(formatId);
      });
      expect(dest.formats).toContain(dest.primaryFormat);
    });
  });

  it("has positive caption budgets (soft ≤ hard when hard > 0)", () => {
    DESTINATION_ORDER.forEach((id) => {
      const { softMaxChars, hardMaxChars } = DESTINATIONS[id].captionProfile;
      expect(softMaxChars).toBeGreaterThan(0);
      expect(hardMaxChars).toBeGreaterThan(0);
      expect(softMaxChars).toBeLessThanOrEqual(hardMaxChars);
    });
  });

  it("never carries schedule / queue fields", () => {
    DESTINATION_ORDER.forEach((id) => {
      const keys = Object.keys(DESTINATIONS[id]);
      FORBIDDEN_DESTINATION_KEYS.forEach((forbidden) => {
        expect(keys).not.toContain(forbidden);
      });
      // Stringify guard: no accidental scheduled_at in nested objects either.
      const json = JSON.stringify(DESTINATIONS[id]);
      FORBIDDEN_DESTINATION_KEYS.forEach((forbidden) => {
        expect(json).not.toContain(forbidden);
      });
    });
  });

  it("requires non-empty checklist keys and valid media kinds", () => {
    DESTINATION_ORDER.forEach((id) => {
      const dest = DESTINATIONS[id];
      expect(dest.checklistKeys.length).toBeGreaterThan(0);
      expect(dest.mediaKinds.length).toBeGreaterThan(0);
      dest.mediaKinds.forEach((kind) => {
        expect(["image", "video"]).toContain(kind);
      });
      if (dest.motionAllowed) {
        expect(dest.mediaKinds).toContain("video");
      }
      expect(["single", "kit"]).toContain(dest.exportBundle);
    });
  });

  it("maps each destination to expected primary formats", () => {
    const expected: Record<DestinationId, FormatIdLike> = {
      ig_feed: "4:5",
      ig_stories: "9:16",
      ig_reels: "9:16",
      tiktok: "9:16",
      whatsapp: "1:1",
      google_business: "1:1",
      print_tent: "5:7",
      print_window: "1:1",
      print_strip: "strip",
    };
    (Object.keys(expected) as DestinationId[]).forEach((id) => {
      expect(DESTINATIONS[id].primaryFormat).toBe(expected[id]);
    });
  });

  it("isDestinationId / destinationFor guard unknowns", () => {
    expect(isDestinationId("ig_feed")).toBe(true);
    expect(isDestinationId("billboard")).toBe(false);
    expect(isDestinationId(null)).toBe(false);
    expect(destinationFor("billboard").id).toBe(DEFAULT_DESTINATION_ID);
    expect(destinationFor(undefined).id).toBe(DEFAULT_DESTINATION_ID);
    expect(destinationFor("tiktok").id).toBe("tiktok");
  });
});

type FormatIdLike = string;

describe("destination pack contents", () => {
  it("puts primary format first and includes caption when provided", () => {
    const pack = destinationPackContents("ig_feed", {
      caption: "Tonight only",
    });
    expect(pack.destinationId).toBe("ig_feed");
    expect(pack.formats[0]).toBe("4:5");
    expect(pack.formats).toEqual(["4:5", "1:1"]);
    expect(pack.includeCaptionFile).toBe(true);
    expect(pack.exportBundle).toBe("kit");
    expect(pack.motionAllowed).toBe(false);
    expect(pack.checklistKeys).toContain("post_manual");
  });

  it("omits caption file when caption is blank", () => {
    const pack = destinationPackContents("ig_stories", { caption: "  " });
    expect(pack.includeCaptionFile).toBe(false);
    expect(pack.formats).toEqual(["9:16"]);
    expect(pack.exportBundle).toBe("single");
    expect(pack.motionAllowed).toBe(true);
  });

  it("print tent pack is 5:7 only with print checklist", () => {
    const pack = destinationPackContents("print_tent");
    expect(pack.formats).toEqual(["5:7"]);
    expect(pack.checklistKeys).toEqual(
      expect.arrayContaining(["print_file", "print_shop"]),
    );
    expect(pack.motionAllowed).toBe(false);
  });

  it("packFormatsForDestination reorders when primary is not first in formats", () => {
    // Registry already has primary first; still pin the helper contract.
    expect(packFormatsForDestination(DESTINATIONS.whatsapp)).toEqual([
      "1:1",
      "4:5",
    ]);
  });

  it("simpleModeFormatsForDestination returns at most two formats", () => {
    expect(simpleModeFormatsForDestination("ig_feed")).toEqual(["4:5", "1:1"]);
    expect(simpleModeFormatsForDestination("tiktok")).toEqual(["9:16"]);
  });
});

describe("caption budget helpers", () => {
  it("levels: ok → soft → hard", () => {
    const profile = captionProfileFor("ig_feed");
    expect(captionBudgetLevel(10, profile)).toBe("ok");
    expect(captionBudgetLevel(profile.softMaxChars + 1, profile)).toBe("soft");
    expect(captionBudgetLevel(profile.hardMaxChars + 1, profile)).toBe("hard");
  });

  it("forces hashtags none for Google and WhatsApp", () => {
    expect(effectiveHashtagBehavior("google_business", "standard")).toBe("none");
    expect(effectiveHashtagBehavior("whatsapp", "light")).toBe("none");
    expect(effectiveHashtagBehavior("ig_feed", "light")).toBe("light");
    expect(effectiveHashtagBehavior("ig_feed", "")).toBe("standard");
  });

  it("assigns length profiles by destination band (story/feed/whatsapp)", () => {
    const expected: Record<DestinationId, CaptionLengthProfile> = {
      ig_feed: "long",
      ig_stories: "short",
      ig_reels: "medium",
      tiktok: "medium",
      whatsapp: "medium",
      google_business: "medium",
      print_tent: "short",
      print_window: "short",
      print_strip: "short",
    };
    (Object.keys(expected) as DestinationId[]).forEach((id) => {
      expect(DESTINATIONS[id].captionProfile.length).toBe(expected[id]);
    });
  });

  it("generationMaxChars prefers soft budget under model ceiling", () => {
    expect(generationMaxCharsFor(captionProfileFor("ig_stories"))).toBe(80);
    expect(generationMaxCharsFor(captionProfileFor("print_tent"))).toBe(40);
    expect(generationMaxCharsFor(captionProfileFor("ig_feed"))).toBe(150);
    expect(generationMaxCharsFor(captionProfileFor("whatsapp"))).toBe(
      CAPTION_GENERATION_MODEL_CEILING,
    );
    expect(
      generationMaxCharsFor(captionProfileFor("ig_feed")),
    ).toBeLessThanOrEqual(CAPTION_GENERATION_MODEL_CEILING);
  });

  it("captionGenerationOptionsFor merges destination hashtag force-off", () => {
    expect(captionGenerationOptionsFor("google_business", "standard")).toEqual({
      max_chars: generationMaxCharsFor(captionProfileFor("google_business")),
      hashtag_behavior: "none",
      length: "medium",
      hashtagsForcedOff: true,
    });
    expect(captionGenerationOptionsFor("ig_feed", "light")).toEqual({
      max_chars: 150,
      hashtag_behavior: "light",
      length: "long",
      hashtagsForcedOff: false,
    });
  });

  it("captionForClipboard truncates only past hard max", () => {
    const profile = captionProfileFor("ig_stories");
    const short = captionForClipboard("hi", profile);
    expect(short).toEqual({ text: "hi", truncated: false });
    const long = "x".repeat(profile.hardMaxChars + 5);
    const clipped = captionForClipboard(long, profile);
    expect(clipped.truncated).toBe(true);
    expect(clipped.text.length).toBe(profile.hardMaxChars);
  });

  it("mustIncludePhrasesFromSlots bounds CTA + handle", () => {
    expect(mustIncludePhrasesFromSlots({})).toEqual([]);
    expect(
      mustIncludePhrasesFromSlots({ cta: "  BOOK NOW  ", handle: "@cafe" }),
    ).toEqual(["BOOK NOW", "@cafe"]);
    expect(
      mustIncludePhrasesFromSlots({
        cta: "A".repeat(50),
      })[0],
    ).toHaveLength(40);
  });
});

describe("platform chrome labels", () => {
  it("maps chrome to preview keys", () => {
    expect(destinationPlatformLabelKey("ig_feed")).toBe("preview.platform.feed");
    expect(destinationPlatformLabelKey("ig_stories")).toBe(
      "preview.platform.story",
    );
    expect(destinationPlatformLabelKey("ig_reels")).toBe(
      "preview.platform.reel",
    );
    expect(destinationPlatformLabelKey("print_tent")).toBe(
      "preview.platform.tent",
    );
  });
});
