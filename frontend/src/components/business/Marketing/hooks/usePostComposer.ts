import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  cleanupMarketingImage,
  generateMarketingCaption,
  generateMarketingImage,
  ImageDailyLimitError,
  PromptRejectedError,
  CampaignSuggestion,
  type MarketingCreativeProfile,
  type MarketingCreativeSnapshot,
  type MarketingCrop,
  type MarketingImageSource,
} from "@/api/marketing";
import type { Business } from "@/api/business";
import { intlLocaleFor } from "@/utils/intlLocale";
import { useSimpleLocale } from "@/i18n/SimpleTranslationProvider";
import { isCleanupEligiblePhotoUrl } from "../cleanupPhotoEligibility";
import { defaultCaptionLanguage } from "../defaultCaptionLanguage";
import { getApiErrorStatus } from "@/utils/apiError";
import {
  kitFormatsFromSnapshot,
  resolveKitFormats,
} from "../formats/campaignKit";
import {
  captionForClipboard,
  captionGenerationOptionsFor,
  captionProfileFor,
  DEFAULT_DESTINATION_ID,
  destinationFor,
  effectiveHashtagBehavior,
  isDestinationId,
  mustIncludePhrasesFromSlots,
  packFormatsForDestination,
  type DestinationId,
} from "../destinations/destinations";
import { FORMATS, type FormatId } from "../formats/formats";
import { heroGenAspectFor } from "../formats/heroGenAspect";
import { SlotKey, TemplateStyle } from "../templates/types";
import {
  DEFAULT_CROP,
  loadOptionalImage,
  type RenderPostInput,
} from "../templates/renderPost";
import { type KitId } from "../artDirection/kits";
import {
  CHOOSABLE_COMPOSITIONS,
  COMPOSITIONS,
  type CompositionId,
} from "../composition/compositions";
import {
  chooseComposition,
  contentSignalsFrom,
  isChoosableCompositionId,
  nextComposition,
  usableCompositions,
} from "../composition/chooser";
import { reservedBandFor } from "../composition/reservedBand";
import { photoSignalsFrom } from "../photo/analyze";
import { photoAnalysisFor } from "../photo/cache";
import {
  buildSlots,
  composeStarterCaption,
  defaultTemplateForPlay,
  downloadPost,
  downloadPostPack,
  copyCaption,
  shareOrDownloadPostPack,
} from "../postContent";
import {
  applyHashtagPreference,
  brandLogoUrl,
  ctaLabelForBrand,
  resolveBrandPalette,
  seedKitForCreative,
  type LogoLoadStatus,
} from "../brandLock";
import {
  MOTION_PRESET_FOR_KIT,
  isMotionPresetId,
  type MotionPresetId,
} from "../motion/presets";
import {
  acceptGeneratedMarketingImage,
  browserReadablePhotoUrl,
  loadGeneratedImageDraft,
  persistGeneratedImageDraft,
  preflightBrowserImage,
} from "../generatedImageAcceptance";

export type Tone = "warm" | "playful" | "elegant" | "punchy";
type CreativeImageSource = MarketingImageSource | "none";

interface PostCreative {
  aspect: FormatId;
  templateStyle: TemplateStyle;
  slots: Partial<Record<SlotKey, string>>;
  caption: string;
  photoUrl: string;
  imageSource: CreativeImageSource;
  crop: MarketingCrop;
  /**
   * The art direction. These live on the creative rather than in sibling
   * `useState` for the same reason everything else here does: only fields on
   * this structure get cloned into `workingCreative`, frozen into
   * `baselineCreative`, and therefore seen by reset and by the dirty check.
   * State kept outside it is invisible to both — an operator's kit change
   * would neither mark the editor dirty nor survive Discard.
   *
   * They are also NOT derived from `templateStyle`. The two are independent
   * axes once restored: re-deriving on every template change would silently
   * discard a kit the operator picked, which is the exact bug this task exists
   * to close. `templateStyle` only seeds them, once, at open.
   */
  kit: KitId;
  composition: CompositionId;
  /**
   * True while `composition` is a derived value the analysis may still improve.
   *
   * Deliberately ABSENT from `normalizedCreative`'s allowlist below, and so
   * from the dirty check: it is provenance, not appearance. Two creatives that
   * paint identically must compare equal whether or not one of them arrived at
   * its composition automatically, otherwise Reshuffling back to the original
   * layout would leave the editor permanently dirty.
   */
  compositionAuto: boolean;
}

/**
 * Fair-use daily-limit rejection payload.
 *
 * Same shape as Menu Builder's `DailyLimitInfo` in useMenuMutations.ts. Declared
 * locally rather than imported across feature boundaries (Marketing → MenuBuilder
 * would be an ugly cross-feature dependency for a two-field DTO).
 */
type DailyLimitInfo = {
  dailyLimit: number;
  resetsInSeconds: number;
};

export interface UsePostComposerArgs {
  business: Business;
  suggestion: CampaignSuggestion | null; // null = manual studio
  /** Exact activity creative used to seed a local-only Reuse editor session. */
  initialSnapshot?: MarketingCreativeSnapshot | null;
  creativeProfile?: MarketingCreativeProfile;
  t: (key: string, params?: Record<string, string | number>) => string;
}

export interface PostComposerState {
  initialCreative: Readonly<PostCreative>;
  workingCreative: PostCreative;
  dirty: boolean;
  aspect: FormatId;
  templateStyle: TemplateStyle;
  kit: KitId;
  composition: CompositionId;
  /** False when Reshuffle would return the same composition — disable the control. */
  canReshuffle: boolean;
  tone: Tone;
  language: string;
  slots: Partial<Record<SlotKey, string>>;
  caption: string;
  photoUrl: string;
  imageSource: CreativeImageSource;
  crop: MarketingCrop;
  isGeneratingPhoto: boolean;
  isGeneratingCaption: boolean;
  dailyLimitReached: DailyLimitInfo | null;
  promptRejected: boolean;
  error: string | null;
  // actions
  setAspect: (a: FormatId) => void;
  /**
   * Formats this concept exports as (hero always present, resolved order).
   * Part of the concept the Library reuses — not a transient UI selection.
   */
  kitFormats: FormatId[];
  toggleKitFormat: (id: FormatId) => void;
  setKitFormats: (formats: FormatId[]) => void;
  /**
   * Destination pack (IG Feed, TikTok, print…). Selecting one sets the hero
   * format + kit_formats pack. Never implies a schedule.
   */
  destinationId: DestinationId;
  setDestination: (id: DestinationId) => void;
  /**
   * No UI drives this any more — the editor's template picker was replaced by
   * the kit picker, because `renderInput` reads the kit and never the template.
   * `templateStyle` itself is still live (it is persisted on the handoff and
   * seeds the kit for pre-scene snapshots), so the setter stays rather than
   * freezing the field at its seed value.
   */
  setTemplate: (s: TemplateStyle) => void;
  setKit: (k: KitId) => void;
  reshuffle: () => void;
  setTone: (t: Tone) => void;
  setLanguage: (l: string) => void;
  setSlot: (key: SlotKey, value: string) => void;
  setCaption: (v: string) => void;
  setPhotoUrl: (u: string) => void;
  setPhoto: (u: string, source: CreativeImageSource) => void;
  setCrop: (crop: MarketingCrop) => void;
  resetCrop: () => void;
  resetCreative: () => void;
  regeneratePhoto: () => Promise<void>;
  cleanupPhoto: () => Promise<void>;
  regenerateCaption: () => Promise<void>;
  downloadImage: () => Promise<void>;
  downloadPack: () => Promise<boolean>;
  sharePack: () => Promise<"shared" | "downloaded" | null>;
  copyCaption: () => Promise<boolean>;
  palette: { primary: string; secondary: string; fontFamily?: string };
  renderInput: RenderPostInput;
  /**
   * Wave 3 motion. Defaults to the kit's preset until the operator overrides.
   * `mediaKind` becomes `video` only after a successful video export — choosing
   * a preset alone must not badge a still as a video in the Library.
   */
  motionPreset: MotionPresetId;
  setMotionPreset: (next: MotionPresetId) => void;
  mediaKind: "image" | "video";
  markMotionExported: () => void;
  /**
   * Business logo probe for the editor. Missing means no logo URL; failed means
   * the URL could not load (CORS / 404) and the canvas watermark will be absent.
   */
  logoStatus: LogoLoadStatus;
  logoUrl: string;
}

function inferredImageSource(
  suggestion: CampaignSuggestion | null,
): CreativeImageSource {
  if (!suggestion?.image_url) return "none";
  if (suggestion.image_source) return suggestion.image_source;
  if (suggestion.play === "offer") return "offer";
  if (suggestion.play === "combo_deal") return "bundle";
  return "menu";
}

function freezeCreative(creative: PostCreative): Readonly<PostCreative> {
  return Object.freeze({
    ...creative,
    slots: Object.freeze({ ...creative.slots }),
    crop: Object.freeze({ ...creative.crop }),
  });
}

function cloneCreative(creative: Readonly<PostCreative>): PostCreative {
  return {
    ...creative,
    slots: { ...creative.slots },
    crop: { ...creative.crop },
  };
}

function formatBounds({
  x,
  y,
  w,
  h,
}: {
  x: number;
  y: number;
  w: number;
  h: number;
}) {
  return `x=${x.toFixed(3)},y=${y.toFixed(3)},w=${w.toFixed(3)},h=${h.toFixed(3)}`;
}

/**
 * The geometry hint sent with an image generation.
 *
 * Reads the CHOSEN COMPOSITION, not `TEMPLATES[templateStyle]`. The template
 * layouts stopped describing where type lands the moment the scene system
 * shipped, so every photo generated since was composed around a reserved area
 * the renderer no longer uses.
 *
 * Only the character count of this string reaches the model (see
 * `buildMarketingHeroPrompt` in backend/internal/services/ai.go, which does
 * `utf8.RuneCountInString` and nothing else with it). The geometry is here for
 * the dedup key and for anyone reading a request log; the direction the model
 * actually follows travels in `reserved_band`.
 */
export function safeRegionForCreative(
  composition: CompositionId,
  aspect: FormatId,
): string {
  const def = COMPOSITIONS[composition];
  const format = FORMATS[aspect] ?? FORMATS["4:5"];
  return [
    `${composition} ${aspect} composition`,
    `reserve copy within content ${formatBounds(def.boundsFor(format))}`,
    `compose the subject within image ${formatBounds(def.imageAreaFor(format))}`,
    `reserved band ${reservedBandFor(def, format)}`,
  ].join("; ");
}

/**
 * Resolve the art direction a creative opens with.
 *
 * Stored ids win whenever this build recognises them — that, and only that, is
 * what makes an operator's choice survive save-and-reopen. Every other branch
 * is a fall back for a snapshot that cannot supply one: written before the
 * scene system existed, or carrying an id from a build this one does not know.
 *
 * `stored` is deliberately typed as the wire shape (`string | undefined`) so
 * the guards are unavoidable. Casting instead would send a server-side typo
 * straight into `KITS[...]` / `COMPOSITIONS[...]` and render a blank post.
 */
function seedArtDirection(input: {
  storedKit?: string;
  storedComposition?: string;
  templateStyle: TemplateStyle;
  aspect: FormatId;
  slots: Partial<Record<SlotKey, string>>;
  photoUrl: string;
  play: string;
  hasDiscount: boolean;
  /** creative_profile.visual_mood — biases kit when no stored kit. */
  visualMood?: string | null;
}): { kit: KitId; composition: CompositionId; compositionAuto: boolean } {
  // Brand lock: stored kit → mood bias → play template map. Face type is kit-
  // owned (see brandLock.ts / buildScene bandTypeFor); design_settings.font_family
  // is deliberately not consulted.
  const kit = seedKitForCreative({
    storedKit: input.storedKit,
    visualMood: input.visualMood,
    templateStyle: input.templateStyle,
  });
  // Choosable-only: the composer always carries a kit, so a stored legacy*
  // family must not round-trip into PostCreative (storage-side pair of the
  // render-time refuse in resolveArtDirection). Unknown / legacy ids re-choose.
  if (isChoosableCompositionId(input.storedComposition)) {
    // A stored id is a decision someone already made. The analysis may not
    // revisit it, however much better it thinks it could do.
    return {
      kit,
      composition: input.storedComposition,
      compositionAuto: false,
    };
  }
  return {
    kit,
    compositionAuto: true,
    composition: chooseComposition(
      contentSignalsFrom({
        play: input.play,
        slots: input.slots,
        hasPhoto: !!input.photoUrl.trim(),
        hasDiscount: input.hasDiscount,
      }),
      // No photo has been measured at seed time — the image has not loaded yet.
      // The analysis effect below upgrades this once it has.
      null,
      kit,
      FORMATS[input.aspect] ?? FORMATS["4:5"],
    ),
  };
}

/**
 * Whether this crop is the untouched default.
 *
 * Comparing against `DEFAULT_CROP` is legitimate HERE and not in the renderer:
 * this is the composer's own state, and "still exactly centred at zoom 1" is
 * the only signal available that the operator has not used the crop stage.
 * An operator who deliberately re-centres therefore gets the automatic focal
 * point back, which is the same thing Reset does and is the intended reading of
 * both.
 */
function isUntouchedCrop(crop: MarketingCrop): boolean {
  return (
    Math.abs(crop.x - DEFAULT_CROP.x) < 1e-6 &&
    Math.abs(crop.y - DEFAULT_CROP.y) < 1e-6 &&
    Math.abs(crop.zoom - DEFAULT_CROP.zoom) < 1e-6
  );
}

function normalizedCreative(creative: PostCreative) {
  const slots = Object.fromEntries(
    Object.entries(creative.slots)
      .sort(([left], [right]) => left.localeCompare(right))
      .map(([key, value]) => [key, value?.trim() ?? ""]),
  );
  const normalizeNumber = (value: number) => Math.round(value * 1000) / 1000;
  // An explicit allowlist, not a spread: a field added to `PostCreative` is
  // invisible to `creativesEqual` — and therefore to the dirty flag and the
  // unsaved-changes guard — until it is named here.
  return {
    aspect: creative.aspect,
    templateStyle: creative.templateStyle,
    kit: creative.kit,
    composition: creative.composition,
    slots,
    caption: creative.caption.trim(),
    photoUrl: creative.photoUrl.trim(),
    imageSource: creative.photoUrl.trim() ? creative.imageSource : "none",
    crop: {
      x: normalizeNumber(creative.crop.x),
      y: normalizeNumber(creative.crop.y),
      zoom: normalizeNumber(creative.crop.zoom),
    },
  };
}

function creativesEqual(left: PostCreative, right: PostCreative): boolean {
  return (
    JSON.stringify(normalizedCreative(left)) ===
    JSON.stringify(normalizedCreative(right))
  );
}

/** Order-insensitive equality for kit format selections. */
function formatListsEqual(
  left: readonly FormatId[],
  right: readonly FormatId[],
): boolean {
  if (left.length !== right.length) return false;
  const a = [...left].sort();
  const b = [...right].sort();
  return a.every((id, i) => id === b[i]);
}

export function usePostComposer({
  business,
  suggestion,
  initialSnapshot,
  creativeProfile,
  t,
}: UsePostComposerArgs): PostComposerState {
  const currency =
    business.display_currency || business.default_currency || "USD";
  const intlLocale = intlLocaleFor(business.default_language || "en");
  const visualMood = creativeProfile?.visual_mood || "";
  const ctaStyle = creativeProfile?.cta_style || "";
  const profileHashtagBehavior = creativeProfile?.hashtag_behavior || "";
  /** Seed destination for initial caption + kit_formats (before state exists). */
  const seedDestinationRaw = initialSnapshot?.destination_id;
  const seedDestinationId: DestinationId = isDestinationId(seedDestinationRaw)
    ? seedDestinationRaw
    : DEFAULT_DESTINATION_ID;
  const seedHashtagBehavior = effectiveHashtagBehavior(
    seedDestinationId,
    profileHashtagBehavior,
  );
  const [tone, setToneState] = useState<Tone>(
    () =>
      (creativeProfile?.default_tone as Tone | undefined) || ("warm" as Tone),
  );
  // L4-22: default caption language from operator locale when profile/business
  // omit one — never hard-fall to English on a Spanish dashboard.
  const { locale: operatorLocale } = useSimpleLocale();
  const [language, setLanguageState] = useState<string>(() =>
    defaultCaptionLanguage({
      profileLang: creativeProfile?.default_language,
      businessLang: business.default_language,
      operatorLocale: operatorLocale || "en",
    }),
  );
  /**
   * Whether this post has a discount the render can actually show.
   *
   * Deliberately NOT the Wave 1 plan's `Boolean(suggestion?.discount_type)`.
   * This mirrors `buildSlots` (postContent.ts), which fills the badge slot from
   * `formatDiscountLabel` and therefore emits badge text only for an `offer`
   * carrying BOTH a type and a value. `chooseComposition` answers `badgeHero`
   * on `hasDiscount && hasPhoto`, so the plan's predicate would hand a
   * badge-hero layout to a post with no badge text to put in it.
   *
   * The gap is reachable, not theoretical: the backend marks `discount_value`
   * `json:"discount_value,omitempty"` (services/marketing/suggestion.go), so a
   * zero-value offer arrives with `discount_type` alone. Do not "restore" the
   * plan's predicate — usePostComposer.test.ts pins both sides.
   */
  const hasDiscount =
    (suggestion?.play === "offer" || suggestion?.play === "happy_hour") &&
    !!suggestion.discount_value;
  /**
   * The play as a CONTENT signal, for the composition chooser.
   *
   * One default, shared by the seed below and the live rotation signals further
   * down, because both describe the same post — two defaults would let them
   * disagree about which play it is the moment Wave 2 teaches
   * `chooseComposition` to read `content.play` (chooser.ts).
   *
   * `""` rather than `marketingPlay`'s `"featured_dish"`: `buildSlots` already
   * builds a manual post's slots from `PLAY_SLOTS[""]`, so the signals derived
   * from those slots must name the play they were actually built for rather
   * than invent one the operator never chose. `marketingPlay` keeps its own
   * default for the opposite reason — it is the API's `play`, typed
   * `MarketingPlay`, which has no "none" member.
   */
  const compositionPlay = suggestion?.play ?? "";
  const [initialCreative] = useState<Readonly<PostCreative>>(() => {
    if (initialSnapshot) {
      // Reuse / Tweak: preserve stored kit/composition/content. Logo + colours
      // re-apply from the live business (palette below); kit is not re-biased.
      return freezeCreative({
        aspect: initialSnapshot.aspect,
        templateStyle: initialSnapshot.template,
        slots: { ...initialSnapshot.slots },
        caption: applyHashtagPreference(
          initialSnapshot.caption,
          seedHashtagBehavior,
        ),
        photoUrl: initialSnapshot.image_url,
        imageSource: initialSnapshot.image_source,
        crop: { ...initialSnapshot.crop },
        ...seedArtDirection({
          storedKit: initialSnapshot.kit,
          storedComposition: initialSnapshot.composition,
          templateStyle: initialSnapshot.template,
          aspect: initialSnapshot.aspect,
          slots: initialSnapshot.slots,
          photoUrl: initialSnapshot.image_url,
          play: compositionPlay,
          hasDiscount,
          visualMood,
        }),
      });
    }
    const slots = buildSlots({
      suggestion,
      business,
      currency,
      intlLocale,
      ctaLabel: ctaLabelForBrand(t, suggestion?.play, ctaStyle),
      badgeLabel:
        suggestion && suggestion.play !== "offer"
          ? t(`defaults.badge.${suggestion.play}`)
          : "",
      offLabel: t("discount.off"),
    });
    const caption = suggestion
      ? applyHashtagPreference(
          composeStarterCaption({
            suggestion,
            business,
            currency,
            intlLocale,
            template: t(`starterCaption.${suggestion.play}`),
            offLabel: t("discount.off"),
          }),
          seedHashtagBehavior,
        )
      : "";
    const persistedDraft = suggestion
      ? loadGeneratedImageDraft(String(business.id), suggestion.id)
      : null;
    const photoUrl = persistedDraft
      ? persistedDraft.url
      : (suggestion?.image_url ?? "");
    /**
     * The play's own default template — the same expression the feed card
     * renders through (`defaultTemplateForPlay` in PostCard.tsx) — so a fresh
     * offer opens Bold and a win-back opens Minimal instead of every play
     * opening on the editor's private idea of a default.
     *
     * Seeding `templateStyle` and the kit from ONE expression is the point.
     * `templateStyle` is what the drawer persists as `creative.template`
     * (PostEditorDrawer.tsx) and what seedKitForCreative falls back to when a
     * snapshot carries no kit and mood is unbiased. Image generation's reserved
     * geometry comes from the composition (`safeRegionForCreative` / `reserved_band`).
     *
     * visual_mood from MarketingSettings biases the kit when set (S1-Brand).
     */
    const template = defaultTemplateForPlay(suggestion?.play);
    return freezeCreative({
      aspect: "4:5",
      templateStyle: template,
      slots,
      caption,
      photoUrl,
      imageSource: persistedDraft
        ? "generated"
        : inferredImageSource(suggestion),
      crop: { ...DEFAULT_CROP },
      // No stored ids on a creative that has never been saved.
      ...seedArtDirection({
        templateStyle: template,
        aspect: "4:5",
        slots,
        photoUrl,
        play: compositionPlay,
        hasDiscount,
        visualMood,
      }),
    });
  });
  const [workingCreative, setWorkingCreative] = useState<PostCreative>(() => ({
    ...initialCreative,
    slots: { ...initialCreative.slots },
    crop: { ...initialCreative.crop },
  }));
  const [baselineCreative, setBaselineCreative] = useState<
    Readonly<PostCreative>
  >(() => freezeCreative(cloneCreative(initialCreative)));
  const workingCreativeRef = useRef(workingCreative);
  const baselineCreativeRef = useRef(baselineCreative);
  const creativeRevisionRef = useRef(0);
  const captionRequestRef = useRef(0);
  const imageRequestRef = useRef(0);
  const editorLockedRef = useRef(false);
  const toneRef = useRef(tone);
  const languageRef = useRef(language);
  const [isGeneratingPhoto, setIsGeneratingPhoto] = useState(false);
  const [isGeneratingCaption, setIsGeneratingCaption] = useState(false);
  const [dailyLimitReached, setDailyLimitReached] =
    useState<DailyLimitInfo | null>(null);
  const [promptRejected, setPromptRejected] = useState(false);
  const [error, setError] = useState<string | null>(null);
  /**
   * The formats this concept exports as. Defaults to the whole registry; the
   * hero is always present and is what `aspect` records.
   *
   * Kept in the composer rather than in the drawer because `reportHandoff`
   * builds the persisted snapshot from composer state, and the format list is
   * part of the concept the Library reuses — not a transient UI selection.
   *
   * NOT part of `PostCreative` (different lifetime from canvas fields), but
   * still participates in dirty/Discard: a format toggle that cannot dirty the
   * drawer is the same class of bug the kit field used to have.
   */
  const seedKitFormats = useMemo(
    () =>
      initialSnapshot
        ? kitFormatsFromSnapshot(initialSnapshot)
        : packFormatsForDestination(seedDestinationId),
    // Seed once from the opening snapshot; later edits use setState.
    // eslint-disable-next-line react-hooks/exhaustive-deps -- intentional seed
    [],
  );
  const [kitFormats, setKitFormatsState] = useState<FormatId[]>(() => [
    ...seedKitFormats,
  ]);
  const [baselineKitFormats] = useState<FormatId[]>(() => [...seedKitFormats]);
  const baselineKitFormatsRef = useRef(baselineKitFormats);
  const [destinationId, setDestinationIdState] =
    useState<DestinationId>(seedDestinationId);
  const [baselineDestinationId] = useState<DestinationId>(seedDestinationId);
  const baselineDestinationIdRef = useRef(baselineDestinationId);
  baselineDestinationIdRef.current = baselineDestinationId;

  /**
   * An explicit operator choice, or null while the preset is still following the
   * kit. Two states rather than one, because "the operator picked slowPan" and
   * "slowPan happens to be this kit's default" must behave differently the next
   * time the kit changes: the first sticks, the second follows.
   *
   * Restored from `initialSnapshot.motion_preset` when the id is recognised;
   * an unknown id falls through to the kit default rather than casting.
   */
  const seedMotionOverride = useRef<MotionPresetId | null>(
    isMotionPresetId(initialSnapshot?.motion_preset)
      ? initialSnapshot.motion_preset
      : null,
  ).current;
  const seedMediaKind: "image" | "video" =
    initialSnapshot?.media_kind === "video" ? "video" : "image";
  const [motionOverride, setMotionOverride] = useState<MotionPresetId | null>(
    seedMotionOverride,
  );
  const [baselineMotionOverride, setBaselineMotionOverride] =
    useState<MotionPresetId | null>(seedMotionOverride);
  const baselineMotionOverrideRef = useRef(baselineMotionOverride);
  const [mediaKind, setMediaKind] = useState<"image" | "video">(seedMediaKind);
  const [baselineMediaKind, setBaselineMediaKind] = useState<"image" | "video">(
    seedMediaKind,
  );
  const baselineMediaKindRef = useRef(baselineMediaKind);

  const {
    aspect,
    templateStyle,
    slots,
    caption,
    photoUrl,
    imageSource,
    crop,
    kit,
    composition,
  } = workingCreative;
  const dirty = useMemo(() => {
    if (!creativesEqual(baselineCreative as PostCreative, workingCreative)) {
      return true;
    }
    if (!formatListsEqual(kitFormats, baselineKitFormats)) return true;
    if (destinationId !== baselineDestinationId) return true;
    if (motionOverride !== baselineMotionOverride) return true;
    if (mediaKind !== baselineMediaKind) return true;
    return false;
  }, [
    baselineCreative,
    workingCreative,
    kitFormats,
    baselineKitFormats,
    destinationId,
    baselineDestinationId,
    motionOverride,
    baselineMotionOverride,
    mediaKind,
    baselineMediaKind,
  ]);
  /**
   * The play as an API argument, for caption and image generation. Its default
   * is NOT `compositionPlay`'s: `GenerateMarketingCaptionRequest.play` is typed
   * `MarketingPlay`, an enum with no "none" member, so a manual post has to
   * name a real play for the backend to pick a prompt at all. Keep the two
   * apart — this one is what the server is told, that one is what the layout is
   * derived from.
   */
  const marketingPlay = suggestion?.play ?? "featured_dish";

  const updateWorkingCreative = useCallback(
    (update: (current: PostCreative) => PostCreative, userInitiated = true) => {
      if (userInitiated && editorLockedRef.current) {
        return workingCreativeRef.current;
      }
      if (userInitiated) creativeRevisionRef.current += 1;
      const next = update(workingCreativeRef.current);
      workingCreativeRef.current = next;
      setWorkingCreative(next);
      return next;
    },
    [],
  );

  const replaceBaselineCreative = useCallback((creative: PostCreative) => {
    const next = freezeCreative(creative);
    baselineCreativeRef.current = next;
    setBaselineCreative(next);
  }, []);

  const setKitFormats = useCallback((formats: FormatId[]) => {
    setKitFormatsState([...formats]);
  }, []);

  const motionOverrideRef = useRef(motionOverride);
  motionOverrideRef.current = motionOverride;

  const requestCaption = useCallback(
    async (autonomousBootstrap: boolean) => {
      if (editorLockedRef.current) return;
      const requestCreative = workingCreativeRef.current;
      const itemName = (
        requestCreative.slots.dishName ||
        suggestion?.target_name ||
        ""
      ).trim();
      if (!itemName) {
        setError("caption_item_required");
        return;
      }
      const requestRevision = creativeRevisionRef.current;
      const requestID = captionRequestRef.current + 1;
      captionRequestRef.current = requestID;
      const requestTone = toneRef.current;
      const requestLanguage = languageRef.current;
      // Destination budget + effective hashtags at call time (story vs feed).
      const destId = destinationId;
      const captionOpts = captionGenerationOptionsFor(
        destId,
        profileHashtagBehavior,
      );
      const mustInclude = mustIncludePhrasesFromSlots({
        cta: requestCreative.slots.cta,
        handle: requestCreative.slots.handle,
      });
      setIsGeneratingCaption(true);
      setError(null);
      try {
        // House voice: tone/language from editor (seeded from creative_profile);
        // CTA/hashtags/banned phrases load server-side from settings; destination
        // max_chars + hashtag force-off + must_include come from this request.
        const text = await generateMarketingCaption(business.id, {
          item_name: itemName,
          play: marketingPlay,
          tone: requestTone,
          language: requestLanguage,
          angle: suggestion?.copy_angle ?? "",
          why_data: suggestion?.why_data ?? "",
          max_chars: captionOpts.max_chars,
          hashtag_behavior: captionOpts.hashtag_behavior,
          ...(mustInclude.length > 0 ? { must_include: mustInclude } : {}),
        });
        if (
          captionRequestRef.current !== requestID ||
          creativeRevisionRef.current !== requestRevision
        ) {
          return;
        }
        const nextWorking = updateWorkingCreative(
          (current) => ({
            ...current,
            // Server already applies profile/destination hashtags; re-apply on
            // the client so a stale cache / offline path cannot reintroduce
            // tags when hashtag_behavior is "none".
            caption: applyHashtagPreference(text, captionOpts.hashtag_behavior),
          }),
          false,
        );
        if (autonomousBootstrap) {
          replaceBaselineCreative({
            ...baselineCreativeRef.current,
            caption: nextWorking.caption,
            slots: { ...baselineCreativeRef.current.slots },
            crop: { ...baselineCreativeRef.current.crop },
          });
        }
      } catch (err) {
        if (captionRequestRef.current === requestID) {
          setError(
            err instanceof Error && err.message === "item_unavailable"
              ? "caption_unavailable"
              : "caption_failed",
          );
        }
      } finally {
        if (captionRequestRef.current === requestID) {
          setIsGeneratingCaption(false);
        }
      }
    },
    [
      business.id,
      suggestion,
      marketingPlay,
      destinationId,
      profileHashtagBehavior,
      updateWorkingCreative,
      replaceBaselineCreative,
    ],
  );

  const regenerateCaption = useCallback(async () => {
    if (editorLockedRef.current) return;
    creativeRevisionRef.current += 1;
    await requestCaption(false);
  }, [requestCaption]);
  const requestCaptionRef = useRef(requestCaption);
  requestCaptionRef.current = requestCaption;
  const regenerateCaptionRef = useRef(regenerateCaption);
  regenerateCaptionRef.current = regenerateCaption;
  const suggestionRef = useRef(suggestion);
  suggestionRef.current = suggestion;

  const regeneratePhoto = useCallback(async () => {
    if (editorLockedRef.current) return;
    editorLockedRef.current = true;
    creativeRevisionRef.current += 1;
    captionRequestRef.current += 1;
    setIsGeneratingCaption(false);
    const requestID = imageRequestRef.current + 1;
    imageRequestRef.current = requestID;
    const requestCreative = cloneCreative(workingCreativeRef.current);
    const requestName = (
      requestCreative.slots.dishName ||
      suggestion?.target_name ||
      business.name ||
      "dish"
    ).trim();
    setIsGeneratingPhoto(true);
    setError(null);
    setDailyLimitReached(null);
    setPromptRejected(false);
    try {
      // Align gen aspect with destination/format when native; re-crop otherwise
      // (wide/strip/5:7 → nearest of 1:1|4:5|9:16 — see heroGenAspectFor).
      const genAspect = heroGenAspectFor(requestCreative.aspect);
      const { url } = await generateMarketingImage(business.id, {
        name: requestName,
        description: suggestion?.target_description ?? "",
        play: marketingPlay,
        aspect_ratio: genAspect,
        template_style: requestCreative.templateStyle,
        visual_mood: creativeProfile?.visual_mood || undefined,
        safe_region: safeRegionForCreative(
          requestCreative.composition,
          requestCreative.aspect,
        ),
        reserved_band: reservedBandFor(
          COMPOSITIONS[requestCreative.composition],
          FORMATS[requestCreative.aspect] ?? FORMATS["4:5"],
        ),
      });
      if (imageRequestRef.current !== requestID) return;
      const accepted = await acceptGeneratedMarketingImage({
        providerUrl: url,
        businessId: String(business.id),
        suggestionId: suggestion?.id ?? `manual:${business.id}`,
        preflight: (candidate) =>
          preflightBrowserImage(candidate, (nextUrl) =>
            loadOptionalImage(nextUrl, "photo"),
          ),
        persist: persistGeneratedImageDraft,
      });
      if (imageRequestRef.current !== requestID) return;
      if (!accepted.ok) {
        setError(
          accepted.reason === "decode_failed"
            ? "image_decode_failed"
            : "image_failed",
        );
        return;
      }
      setDailyLimitReached(null);
      updateWorkingCreative(
        () => ({
          ...requestCreative,
          photoUrl: accepted.draft.url,
          imageSource: "generated",
          crop: { ...DEFAULT_CROP },
        }),
        false,
      );
    } catch (e) {
      if (imageRequestRef.current !== requestID) return;
      if (e instanceof ImageDailyLimitError) {
        setDailyLimitReached({
          dailyLimit: e.dailyLimit,
          resetsInSeconds: e.resetsInSeconds,
        });
        return;
      }
      setDailyLimitReached(null);
      if (e instanceof PromptRejectedError) setPromptRejected(true);
      else setError("image_failed");
    } finally {
      if (imageRequestRef.current === requestID) {
        editorLockedRef.current = false;
        setIsGeneratingPhoto(false);
      }
    }
  }, [
    business.id,
    marketingPlay,
    suggestion,
    creativeProfile?.visual_mood,
    business.name,
    updateWorkingCreative,
  ]);

  /**
   * Relight and declutter the CURRENT photo.
   *
   * Reuses `regeneratePhoto`'s locking exactly — one image job at a time,
   * caption requests cancelled, the editor locked while it runs — because the
   * failure mode it guards against is the same: an operator editing underneath
   * an in-flight generation.
   *
   * `imageSource` is deliberately NOT changed to "generated". The result is
   * still the operator's own dish; the backend prompt forbids substituting it.
   * Relabelling it would tell them their real food is synthetic, which is the
   * one claim this feature exists to be able to deny.
   */
  const cleanupPhoto = useCallback(async () => {
    if (editorLockedRef.current) return;
    const requestCreative = cloneCreative(workingCreativeRef.current);
    const sourceUrl = requestCreative.photoUrl.trim();
    if (!sourceUrl) {
      setError("no_photo");
      return;
    }
    // L4-20: demo Unsplash URLs fail SSRF allowlist 100% — refuse client-side.
    if (!isCleanupEligiblePhotoUrl(sourceUrl)) {
      setError("image_not_hosted");
      return;
    }
    editorLockedRef.current = true;
    creativeRevisionRef.current += 1;
    captionRequestRef.current += 1;
    setIsGeneratingCaption(false);
    const requestID = imageRequestRef.current + 1;
    imageRequestRef.current = requestID;
    setIsGeneratingPhoto(true);
    setError(null);
    setDailyLimitReached(null);
    setPromptRejected(false);
    try {
      // Same aspect fidelity as generate: cleanup is subject to the same
      // marketing:write daily fair-use limit and must request the native hero
      // frame so print/wide destinations do not send 5:7/wide/strip to the
      // backend whitelist.
      const cleanupAspect = heroGenAspectFor(requestCreative.aspect);
      const { url } = await cleanupMarketingImage(business.id, {
        image_url: sourceUrl,
        name: (
          requestCreative.slots.dishName ||
          suggestion?.target_name ||
          business.name ||
          "dish"
        ).trim(),
        description: suggestion?.target_description ?? "",
        aspect_ratio: cleanupAspect,
      });
      if (imageRequestRef.current !== requestID) return;
      setDailyLimitReached(null);
      updateWorkingCreative(
        () => ({
          ...requestCreative,
          photoUrl: url,
          // Source unchanged, and the crop is reset because the cleaned photo is
          // a new frame: the old focal point named pixels that have moved.
          crop: { ...DEFAULT_CROP },
        }),
        false,
      );
    } catch (e) {
      if (imageRequestRef.current !== requestID) return;
      if (e instanceof ImageDailyLimitError) {
        setDailyLimitReached({
          dailyLimit: e.dailyLimit,
          resetsInSeconds: e.resetsInSeconds,
        });
        return;
      }
      setDailyLimitReached(null);
      // L4-20: surface host-allowlist failures distinctly from generic retry.
      const status = getApiErrorStatus(e);
      if (status === 400) {
        setError("image_not_hosted");
      } else {
        setError("image_failed");
      }
    } finally {
      if (imageRequestRef.current === requestID) {
        editorLockedRef.current = false;
        setIsGeneratingPhoto(false);
      }
    }
  }, [business.id, business.name, suggestion, updateWorkingCreative]);

  const setTone = useCallback((value: Tone) => {
    if (editorLockedRef.current) return;
    creativeRevisionRef.current += 1;
    toneRef.current = value;
    setToneState(value);
  }, []);
  const setLanguage = useCallback((value: string) => {
    if (editorLockedRef.current) return;
    creativeRevisionRef.current += 1;
    languageRef.current = value;
    setLanguageState(value);
  }, []);
  const setCaption = useCallback(
    (value: string) => {
      updateWorkingCreative((current) => ({ ...current, caption: value }));
    },
    [updateWorkingCreative],
  );

  // Partner-quality caption on open — skip the template-only first impression.
  const captionBootstrapped = useRef(false);
  useEffect(() => {
    if (
      !suggestionRef.current ||
      initialSnapshot ||
      captionBootstrapped.current
    ) {
      return;
    }
    captionBootstrapped.current = true;
    void requestCaptionRef.current(true);
  }, [suggestion?.id, initialSnapshot]);

  // Regenerate the caption when tone/language change (but not on first mount).
  const mounted = useRef(false);
  useEffect(() => {
    if (!mounted.current) {
      mounted.current = true;
      return;
    }
    void regenerateCaptionRef.current();
  }, [tone, language]);

  const setSlot = useCallback(
    (key: SlotKey, value: string) => {
      updateWorkingCreative((current) => ({
        ...current,
        slots: { ...current.slots, [key]: value },
      }));
    },
    [updateWorkingCreative],
  );

  const setAspect = useCallback(
    (value: FormatId) => {
      updateWorkingCreative((current) => ({ ...current, aspect: value }));
    },
    [updateWorkingCreative],
  );

  /**
   * Pick a destination pack: hero format + kit_formats from the registry.
   * Does not touch scheduling — pack is formats + caption only.
   */
  const setDestination = useCallback(
    (id: DestinationId) => {
      if (editorLockedRef.current) return;
      const dest = destinationFor(id);
      const pack = packFormatsForDestination(dest);
      setDestinationIdState(dest.id);
      setKitFormatsState([...pack]);
      updateWorkingCreative((current) => ({
        ...current,
        aspect: dest.primaryFormat,
      }));
    },
    [updateWorkingCreative],
  );

  const toggleKitFormat = useCallback(
    (id: FormatId) => {
      // The hero is not optional: it is the format the operator approved on
      // screen and the one `aspect` records, so dropping it would store a
      // concept whose hero is absent from its own kit.
      if (id === aspect) return;
      setKitFormatsState((current) =>
        current.includes(id)
          ? current.filter((entry) => entry !== id)
          : [...current, id],
      );
    },
    [aspect],
  );

  const resolvedKitFormats = useMemo(
    () => resolveKitFormats(aspect, kitFormats),
    [aspect, kitFormats],
  );

  const setTemplate = useCallback(
    (value: TemplateStyle) => {
      updateWorkingCreative((current) => ({
        ...current,
        templateStyle: value,
      }));
    },
    [updateWorkingCreative],
  );
  const setKit = useCallback(
    (value: KitId) => {
      updateWorkingCreative((current) => ({ ...current, kit: value }));
    },
    [updateWorkingCreative],
  );

  const motionPreset: MotionPresetId =
    motionOverride ?? MOTION_PRESET_FOR_KIT[kit];
  const setMotionPreset = useCallback((next: MotionPresetId) => {
    setMotionOverride(next);
  }, []);
  /**
   * Called after a successful video export, not when the picker changes. The
   * snapshot records what the operator actually took away — an operator who
   * chose a preset, looked at the preview and then downloaded the still has a
   * still, and the Library must not badge it as a video.
   *
   * Also accepts the new media kind (and current motion override) as the
   * discard baseline: otherwise dirty flips true and the close dialog asks to
   * discard a successful export.
   */
  const markMotionExported = useCallback(() => {
    setMediaKind("video");
    baselineMediaKindRef.current = "video";
    setBaselineMediaKind("video");
    const override = motionOverrideRef.current;
    baselineMotionOverrideRef.current = override;
    setBaselineMotionOverride(override);
  }, []);

  // Rebuilt from the live working creative, not from the seed: removing the
  // photo has to withdraw the photo-only candidates immediately, or Reshuffle
  // keeps offering a rotation that can no longer render.
  const compositionSignals = useMemo(
    () =>
      contentSignalsFrom({
        play: compositionPlay,
        slots,
        hasPhoto: !!photoUrl.trim(),
        hasDiscount,
      }),
    [compositionPlay, slots, photoUrl, hasDiscount],
  );

  /**
   * Whether Reshuffle would actually change anything. With no photo exactly one
   * choosable family survives the filter, so `nextComposition` correctly
   * returns the current id and the control would be a visible no-op — the UI
   * must disable it rather than offer a button that does nothing. Derived from
   * the same predicate the rotation uses so the two cannot disagree.
   */
  const canReshuffle = useMemo(
    () =>
      usableCompositions(
        compositionSignals,
        CHOOSABLE_COMPOSITIONS,
        FORMATS[workingCreative.aspect] ?? FORMATS["4:5"],
      ).length > 1,
    [compositionSignals, workingCreative.aspect],
  );

  /**
   * Advance the rotation. Deliberately not gated on `canReshuffle`: when the
   * current composition is no longer usable — the operator removed the photo
   * under a photo-only family — `nextComposition` restarts at the first usable
   * candidate, and blocking the call would strand the creative there. A true
   * fixed point returns the same creative object so it never reads as an edit.
   */
  const reshuffle = useCallback(() => {
    updateWorkingCreative((current) => {
      const next = nextComposition(
        current.composition,
        compositionSignals,
        CHOOSABLE_COMPOSITIONS,
        FORMATS[current.aspect] ?? FORMATS["4:5"],
      );
      return next === current.composition
        ? current
        : { ...current, composition: next, compositionAuto: false };
    });
  }, [updateWorkingCreative, compositionSignals]);

  /**
   * Adopt the photo's analysis once it is measurable.
   *
   * Runs once per photo URL. It reuses `loadOptionalImage`, so it shares the
   * renderer's image cache entry and costs no extra network request, and
   * `photoAnalysisFor`, so it shares the renderer's analysis and can never
   * disagree with what the preview paints.
   *
   * Two guards keep it from stepping on the operator: the crop moves only while
   * it is untouched, and the composition moves only while `compositionAuto`.
   * A third keeps it from lying about state: the analysis is a MEASUREMENT, not
   * an edit, so wherever the baseline still holds the value being replaced the
   * baseline advances too. Without that the editor would open dirty and the
   * unsaved-changes guard would fire on a post nobody touched.
   */
  const analysedPhotoRef = useRef<string>("");
  useEffect(() => {
    const url = photoUrl.trim();
    if (!url || analysedPhotoRef.current === url) return;
    analysedPhotoRef.current = url;

    const controller = new AbortController();
    let cancelled = false;

    void (async () => {
      let image: HTMLImageElement | null = null;
      try {
        image = await loadOptionalImage(url, "photo", controller.signal);
      } catch {
        return; // AbortError: the photo changed, or the drawer closed.
      }
      if (cancelled || !image) return;

      const analysis = photoAnalysisFor(url, image);
      if (cancelled || !analysis) return;
      const signals = photoSignalsFrom(analysis);

      let changedCrop = false;
      let changedComposition = false;
      const applied = updateWorkingCreative((current) => {
        if (current.photoUrl.trim() !== url) return current;
        const crop = isUntouchedCrop(current.crop)
          ? { x: analysis.focal.x, y: analysis.focal.y, zoom: 1 }
          : current.crop;
        const composition = current.compositionAuto
          ? chooseComposition(
              contentSignalsFrom({
                play: marketingPlay,
                slots: current.slots,
                hasPhoto: true,
                hasDiscount,
              }),
              signals,
              current.kit,
              FORMATS[current.aspect] ?? FORMATS["4:5"],
            )
          : current.composition;
        changedCrop = crop !== current.crop;
        changedComposition = composition !== current.composition;
        if (!changedCrop && !changedComposition) return current;
        return { ...current, crop, composition };
      }, false);

      if (cancelled || (!changedCrop && !changedComposition)) return;

      const baseline = baselineCreativeRef.current;
      const baselineCrop =
        changedCrop && isUntouchedCrop(baseline.crop)
          ? { ...applied.crop }
          : { ...baseline.crop };
      const baselineComposition =
        changedComposition && baseline.compositionAuto
          ? applied.composition
          : baseline.composition;
      if (
        baselineCrop.x === baseline.crop.x &&
        baselineCrop.y === baseline.crop.y &&
        baselineCrop.zoom === baseline.crop.zoom &&
        baselineComposition === baseline.composition
      ) {
        return;
      }
      replaceBaselineCreative({
        ...baseline,
        slots: { ...baseline.slots },
        crop: baselineCrop,
        composition: baselineComposition,
      });
    })();

    return () => {
      cancelled = true;
      controller.abort();
    };
  }, [
    photoUrl,
    marketingPlay,
    hasDiscount,
    updateWorkingCreative,
    replaceBaselineCreative,
  ]);

  const setPhoto = useCallback(
    (url: string, source: CreativeImageSource) => {
      updateWorkingCreative((current) => ({
        ...current,
        photoUrl: url,
        imageSource: url ? source : "none",
      }));
    },
    [updateWorkingCreative],
  );
  const setPhotoUrl = useCallback(
    (url: string) => setPhoto(url, url ? "upload" : "none"),
    [setPhoto],
  );
  const setCrop = useCallback(
    (value: MarketingCrop) => {
      updateWorkingCreative((current) => ({
        ...current,
        crop: { ...value },
      }));
    },
    [updateWorkingCreative],
  );
  const resetCrop = useCallback(() => setCrop({ ...DEFAULT_CROP }), [setCrop]);
  const resetCreative = useCallback(() => {
    if (editorLockedRef.current) return;
    creativeRevisionRef.current += 1;
    captionRequestRef.current += 1;
    imageRequestRef.current += 1;
    const reset = cloneCreative(baselineCreativeRef.current);
    workingCreativeRef.current = reset;
    setWorkingCreative(reset);
    setIsGeneratingCaption(false);
    setIsGeneratingPhoto(false);
    setDailyLimitReached(null);
    setPromptRejected(false);
    setError(null);
    // Restore extras to the same baseline Discard uses for PostCreative — not
    // the kit-default / "image" constants that used to wipe an opened video.
    setKitFormatsState([...baselineKitFormatsRef.current]);
    setDestinationIdState(baselineDestinationIdRef.current);
    setMotionOverride(baselineMotionOverrideRef.current);
    setMediaKind(baselineMediaKindRef.current);
  }, []);

  const logoUrl = useMemo(() => brandLogoUrl(business), [business]);
  /**
   * Brand palette: live colours from design settings; wire font derived from
   * the active kit (faces stay kit-owned — see brandLock.ts). Reuse re-applies
   * colours while preserving stored kit/composition.
   */
  const palette = useMemo(
    () =>
      resolveBrandPalette(business, {
        kit,
        storedFontFamily: initialSnapshot?.font_family,
      }),
    [business, initialSnapshot?.font_family, kit],
  );

  const [logoStatus, setLogoStatus] = useState<LogoLoadStatus>(() =>
    logoUrl ? "loading" : "missing",
  );
  useEffect(() => {
    if (!logoUrl) {
      setLogoStatus("missing");
      return;
    }
    const controller = new AbortController();
    setLogoStatus("loading");
    void loadOptionalImage(logoUrl, "logo", controller.signal)
      .then((image) => {
        if (controller.signal.aborted) return;
        setLogoStatus(image ? "ok" : "failed");
      })
      .catch((error) => {
        if ((error as Error)?.name === "AbortError") return;
        if (!controller.signal.aborted) setLogoStatus("failed");
      });
    return () => controller.abort();
  }, [logoUrl]);

  /**
   * The one canvas input the preview, the download, the pack and the share
   * sheet all render from — which is why the art direction has to be attached
   * here and nowhere else. A kit set on any single call site would leave the
   * others rendering a different post than the operator is looking at.
   *
   * `template` is deliberately absent. `RenderPostInput.template` is the LEGACY
   * selector: `resolveArtDirection` in renderPost.ts prefers `kit` whenever it
   * is set and only falls back to the template, so leaving both on the object
   * would be a field that documents one path while the renderer takes the
   * other. Nothing reads it off this object — `paintedSlotKeys` in
   * PostEditorDrawer.tsx consults `template?.id` only on the branch where `kit`
   * is absent, which this input never takes. `templateStyle` itself stays on
   * the creative: it is what the handoff persists as `creative.template` and
   * what seeds the kit for a snapshot saved before kits existed. Image-gen
   * geometry is composed from `composition` via `safeRegionForCreative`.
   */
  const renderInput = useMemo<RenderPostInput>(
    () => ({
      kit,
      composition,
      aspect,
      photoUrl: photoUrl ? browserReadablePhotoUrl(photoUrl) : photoUrl,
      slots,
      palette,
      logoUrl,
      crop,
    }),
    [kit, composition, aspect, photoUrl, slots, palette, logoUrl, crop],
  );

  const downloadImage = useCallback(async () => {
    if (!photoUrl) {
      setError("no_photo");
      return;
    }
    setError(null);
    try {
      await downloadPost({
        renderInput,
        filename: `${(slots.dishName || "post").replace(/\s+/g, "-").toLowerCase()}-${aspect.replace(":", "x")}.png`,
      });
    } catch {
      setError("download_failed");
    }
  }, [photoUrl, aspect, slots.dishName, renderInput]);

  const downloadPack = useCallback(async () => {
    if (!photoUrl) {
      setError("no_photo");
      return false;
    }
    if (!caption.trim()) {
      setError("no_caption");
      return false;
    }
    setError(null);
    try {
      await downloadPostPack({
        renderInput,
        caption,
        filename: `${(slots.dishName || "post").replace(/\s+/g, "-").toLowerCase()}-${aspect.replace(":", "x")}.png`,
      });
      return true;
    } catch {
      setError("download_failed");
      return false;
    }
  }, [photoUrl, caption, aspect, slots.dishName, renderInput]);

  const sharePack = useCallback(async () => {
    if (!photoUrl) {
      setError("no_photo");
      return null;
    }
    if (!caption.trim()) {
      setError("no_caption");
      return null;
    }
    setError(null);
    try {
      return await shareOrDownloadPostPack({
        renderInput,
        caption,
        filename: `${(slots.dishName || "post").replace(/\s+/g, "-").toLowerCase()}-${aspect.replace(":", "x")}.png`,
      });
    } catch (error) {
      if (error instanceof DOMException && error.name === "AbortError") {
        return null;
      }
      setError("download_failed");
      return null;
    }
  }, [photoUrl, caption, aspect, slots.dishName, renderInput]);

  const copyCaptionToClipboard = useCallback(async () => {
    if (!caption.trim()) {
      setError("no_caption");
      return false;
    }
    try {
      // Soft overages stay intact; only hard platform ceiling clips on copy.
      const { text } = captionForClipboard(
        caption,
        captionProfileFor(destinationId),
      );
      await copyCaption(text);
      return true;
    } catch {
      setError("copy_failed");
      return false;
    }
  }, [caption, destinationId]);

  return {
    initialCreative,
    workingCreative,
    dirty,
    aspect,
    templateStyle,
    kit,
    composition,
    canReshuffle,
    tone,
    language,
    slots,
    caption,
    photoUrl,
    imageSource,
    crop,
    isGeneratingPhoto,
    isGeneratingCaption,
    dailyLimitReached,
    promptRejected,
    error,
    setAspect,
    kitFormats: resolvedKitFormats,
    toggleKitFormat,
    setKitFormats,
    destinationId,
    setDestination,
    setTemplate,
    setKit,
    reshuffle,
    setTone,
    setLanguage,
    setSlot,
    setCaption,
    setPhotoUrl,
    setPhoto,
    setCrop,
    resetCrop,
    cleanupPhoto,
    resetCreative,
    regeneratePhoto,
    regenerateCaption,
    downloadImage,
    downloadPack,
    sharePack,
    copyCaption: copyCaptionToClipboard,
    palette,
    renderInput,
    motionPreset,
    setMotionPreset,
    mediaKind,
    markMotionExported,
    logoStatus,
    logoUrl,
  };
}
