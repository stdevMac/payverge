import { axiosInstance } from "./tools/instance";

type MarketingAspectRatio = "1:1" | "4:5" | "9:16";
/**
 * Wire type for `creative_snapshot.aspect` and `kit_formats[]`.
 *
 * Deliberately a superset of `MarketingAspectRatio` rather than a replacement:
 * `MarketingTemplateStyle` and the legacy `TemplateDef` path still key on the
 * three-value union, and widening that would ripple into `templates/templates.ts`
 * for no gain. The backend whitelist in marketing_activity_handlers.go
 * (`marketingFormats`) is the authority; this mirrors it.
 */
type MarketingFormatId = MarketingAspectRatio | "wide" | "strip" | "5:7";
type MarketingTemplateStyle = "editorial" | "bold" | "minimal";
export type MarketingVisualMood =
  | "natural"
  | "bright"
  | "moody"
  | "editorial"
  | "rustic";
export type MarketingCTAStyle = "soft" | "direct" | "urgent";
type MarketingHashtagBehavior = "none" | "light" | "standard";
export type MarketingTone = "warm" | "playful" | "elegant" | "punchy";
export type MarketingRenderFontFamily = "Inter" | "Sans" | "Serif";
export type MarketingEmptyReason =
  | ""
  | "paused"
  | "no_data"
  | "no_enabled_plays"
  | "all_handled";
export type MarketingImageSource =
  | "menu"
  | "offer"
  | "bundle"
  | "gallery"
  | "upload"
  | "generated";

export const MARKETING_PLAYS = [
  "happy_hour",
  "featured_dish",
  "move_item",
  "win_back",
  "combo_deal",
  "offer",
] as const;

export type MarketingPlay = (typeof MARKETING_PLAYS)[number];

const MARKETING_PLAY_SET = new Set<string>(MARKETING_PLAYS);

export function isMarketingPlay(value: unknown): value is MarketingPlay {
  return typeof value === "string" && MARKETING_PLAY_SET.has(value);
}

/** Structured explainability atom from S2 ranking (why-this-post). */
export interface WhyFactor {
  /** i18n key suffix under marketingDashboard.why.factors.* */
  key: string;
  /** Already-formatted operator-facing value. */
  value: string;
  /** Optional contribution to rank (display-only). */
  weight?: number;
}

export interface CampaignSuggestion {
  id: string;
  play: MarketingPlay;
  /** Stable localization key (usually matches `play`). */
  play_key?: MarketingPlay;
  /** Happy-hour daypart label for title interpolation (e.g. "Tue 5–7pm"). */
  daypart_key?: string;
  title: string;
  why_data: string;
  /** S2 structured why-this-post factors (additive; prefer over parsing why_data). */
  why_factors?: WhyFactor[];
  /** Ranker version tag (e.g. "s2"); absent = pre-S2 payload. */
  ranking_version?: string;
  source: string;
  target_item_id?: string;
  target_name?: string;
  copy_angle: string;
  metrics?: Record<string, unknown>;
  rank: number;
  image_url?: string;
  image_source?: MarketingImageSource;
  discount_type?: "percentage" | "fixed";
  discount_value?: number;
  target_description?: string;
  /**
   * Optional public storefront deep link (S2-D). Absent when the business has
   * no live custom_url page. Never a signed private asset URL.
   */
  guest_url?: string;
  /** "business" | "menu" | "reservations" when guest_url is set. */
  guest_url_kind?: "business" | "menu" | "reservations" | string;
}

export interface GenerateMarketingImageRequest {
  name: string;
  description?: string;
  ingredients?: string;
  play?: MarketingPlay;
  aspect_ratio?: MarketingAspectRatio;
  template_style?: MarketingTemplateStyle;
  visual_mood?: MarketingVisualMood;
  safe_region?: string;
  /**
   * Which band of the frame the overlay type will occupy, derived from the
   * chosen composition. A closed enum, because the backend maps it to a prompt
   * sentence it owns — `safe_region` is untrusted free text and only ever
   * contributes its character count.
   */
  reserved_band?: "top" | "bottom" | "center" | "left" | "right" | "none";
}

export interface GenerateMarketingImageResponse {
  url: string;
  credit: string;
}

export interface GenerateMarketingCaptionRequest {
  item_name: string;
  play: MarketingPlay;
  tone?: MarketingTone;
  language?: string;
  angle?: string;
  why_data?: string;
  /**
   * Destination soft budget for AI generation (S1-Copy). Optional.
   * Backend clamps to [40, 280]; absent uses server default 280.
   */
  max_chars?: number;
  /**
   * Optional hashtag override when destination forces none (Google, WhatsApp)
   * or when the operator profile differs from the saved default for this request.
   */
  hashtag_behavior?: MarketingHashtagBehavior;
  /**
   * Optional short phrases to weave in when natural (CTA / handle). Max 3 × 40.
   */
  must_include?: string[];
}

export interface MarketingCreativeProfile {
  audience: string;
  voice: string;
  visual_mood: MarketingVisualMood | "";
  cta_style: MarketingCTAStyle | "";
  hashtag_behavior: MarketingHashtagBehavior | "";
  avoid_phrases: string[];
  default_language: string;
  default_tone: MarketingTone | "";
}

export interface MarketingCrop {
  x: number;
  y: number;
  zoom: number;
}

type MarketingCreativeSlots = Record<string, string>;

export interface MarketingCreativeSnapshot {
  caption: string;
  image_url: string;
  image_source: MarketingImageSource;
  template: MarketingTemplateStyle;
  aspect: MarketingFormatId;
  slots: MarketingCreativeSlots;
  crop: MarketingCrop;
  /** Renderer font captured by editor handoffs; legacy activity may omit it. */
  font_family?: MarketingRenderFontFamily;
  /**
   * Art-direction kit id (`KitId`), the layout composition (`CompositionId`)
   * and the photo treatment the operator settled on. Absent on every snapshot
   * written before the scene system, which is why all three are optional.
   *
   * Typed as bare `string` on purpose — this is the wire shape, and a backend
   * older or newer than this build can legitimately send an id this build has
   * no definition for. Consumers must narrow with `isKitId` / `isCompositionId`
   * and fall back, never cast: an unrecognised id reaching `KITS[...]` or
   * `COMPOSITIONS[...]` is an undefined lookup and a blank post.
   */
  kit?: string;
  composition?: string;
  treatment?: string;
  /**
   * Wave 3 motion. Absent on every snapshot written before video export, so
   * absent means a still image and the row renders exactly as it always did.
   *
   * `media_kind` is a closed backend concept and is whitelisted server-side;
   * `motion_preset` is frontend-owned and only length- and charset-bounded, the
   * same treatment `composition` gets. Both are typed as bare `string` here for
   * that reason — narrow `motion_preset` with `isMotionPresetId` and fall back,
   * never cast, because a backend older or newer than this build can send an id
   * this build has no definition for.
   *
   * DEPLOY GATE: emit only after the Phase A backend that accepts these fields
   * is live; against a pre-Wave-3 backend every video post is rejected with 400
   * `invalid_creative_snapshot` and the activity row is never written.
   */
  media_kind?: string;
  motion_preset?: string;
  /**
   * The coordinated format set this concept was exported as. Absent on every
   * snapshot written before Wave 4, which is what distinguishes a single-format
   * post from a campaign. `aspect` above remains the HERO format.
   *
   * Typed as bare `string[]` for the same reason `kit` is a bare string: this is
   * the wire shape, and a backend newer than this build can send an id this
   * build has no definition for. Narrow with `isFormatId` and drop what does not
   * resolve — never cast.
   */
  kit_formats?: string[];
  /**
   * Season 1 destination pack id (`DestinationId`). Absent on every snapshot
   * written before destination packs. Bare string on the wire — narrow with
   * `isDestinationId` and fall back; never cast. Backend bounds length/charset
   * only (no closed whitelist) so destinations can grow FE-first after deploy.
   *
   * DEPLOY GATE: emit only after the backend that accepts `destination_id` is
   * live; against a pre-S1 backend every post with this field is rejected with
   * 400 `invalid_creative_snapshot` if the field is unknown to older parsers —
   * current Go unmarshaller ignores unknown keys on read, but validation only
   * runs on known struct fields after the accepting deploy.
   */
  destination_id?: string;
  /**
   * S3-Loop: optional freeform channel the operator typed when confirming
   * mark_posted (e.g. "Instagram Stories"). Not a schedule field. Absent on
   * every pre-S3-Loop snapshot. Max 40 runes server-side.
   *
   * DEPLOY GATE: backend first (additive snapshot field).
   */
  posted_channel?: string;
  /**
   * S3-Reach: short promo / correlation code stamped on print & destination
   * pack QR payloads (e.g. PV-HH-A3F2). Not multi-touch ads attribution.
   * Charset-bounded server-side. DEPLOY GATE: backend first.
   */
  promo_code?: string;
  /**
   * S3-Reach: print-shop preset id (table_tent_5x7, window_clings_square, …).
   * Charset-bounded; not a closed whitelist. DEPLOY GATE: backend first.
   */
  print_preset?: string;
}

export function normalizeMarketingRenderFontFamily(
  value?: string,
): MarketingRenderFontFamily {
  const normalized = value?.trim().toLowerCase();
  if (normalized === "sans") return "Sans";
  if (normalized === "serif") return "Serif";
  return "Inter";
}

/** Thrown when the backend returns 429 image_daily_limit_reached. */
export class ImageDailyLimitError extends Error {
  constructor(
    public readonly dailyLimit: number,
    public readonly resetsInSeconds: number,
  ) {
    super("image_daily_limit_reached");
    this.name = "ImageDailyLimitError";
  }
}

/** Thrown when the backend guardrail rejects the prompt (422). */
export class PromptRejectedError extends Error {
  constructor() {
    super("prompt_rejected");
    this.name = "PromptRejectedError";
  }
}

interface ErrorShape {
  response?: {
    status?: number;
    data?: { code?: string; daily_limit?: number; resets_in_seconds?: number };
  };
}

/** Maps the backend's 429 fair-use rejection to a typed error, or null. */
function readDailyLimit(err: ErrorShape): ImageDailyLimitError | null {
  const data = err.response?.data;
  if (data?.code !== "image_daily_limit_reached") return null;
  return new ImageDailyLimitError(
    data.daily_limit ?? 0,
    data.resets_in_seconds ?? 0,
  );
}

export const getMarketingSuggestions = async (
  businessId: string | number,
): Promise<MarketingSuggestionsResponse> => {
  const res = await axiosInstance.get<MarketingSuggestionsResponse>(
    `/inside/businesses/${businessId}/marketing/suggestions`,
  );
  return res.data;
};

export const generateMarketingImage = async (
  businessId: string | number,
  req: GenerateMarketingImageRequest,
): Promise<GenerateMarketingImageResponse> => {
  try {
    const res = await axiosInstance.post<GenerateMarketingImageResponse>(
      `/inside/businesses/${businessId}/marketing/image`,
      req,
    );
    return res.data;
  } catch (e) {
    const err = e as ErrorShape;
    const limitErr = readDailyLimit(err);
    if (limitErr) throw limitErr;
    if (
      err.response?.status === 422 ||
      err.response?.data?.code === "prompt_rejected"
    ) {
      throw new PromptRejectedError();
    }
    throw e;
  }
};

export interface CleanupMarketingImageRequest {
  image_url: string;
  name: string;
  description?: string;
  aspect_ratio?: MarketingAspectRatio;
}

/**
 * Relight and declutter a photo the operator already owns.
 *
 * Deliberately has no prompt field. The backend runs an image-to-image pass
 * whose prompt forbids substituting the dish — "that is actually my food" is
 * the trust anchor and the reason this differs from generic AI imagery. A
 * prompt field would be a hole straight through that.
 *
 * Subject to the same daily fair-use limit as generation; raises
 * `ImageDailyLimitError` when the backend returns `image_daily_limit_reached`.
 */
export const cleanupMarketingImage = async (
  businessId: string | number,
  request: CleanupMarketingImageRequest,
): Promise<GenerateMarketingImageResponse> => {
  try {
    const response = await axiosInstance.post<GenerateMarketingImageResponse>(
      `/inside/businesses/${businessId}/marketing/image/cleanup`,
      request,
    );
    return response.data;
  } catch (error) {
    const limitErr = readDailyLimit(error as ErrorShape);
    if (limitErr) throw limitErr;
    throw error;
  }
};

const MARKETING_CAPTION_UNAVAILABLE_CODE = "item_unavailable";

export class MarketingCaptionUnavailableError extends Error {
  readonly code = MARKETING_CAPTION_UNAVAILABLE_CODE;
  constructor() {
    super(MARKETING_CAPTION_UNAVAILABLE_CODE);
    this.name = "MarketingCaptionUnavailableError";
  }
}

export const generateMarketingCaption = async (
  businessId: string | number,
  req: GenerateMarketingCaptionRequest,
  signal?: AbortSignal,
): Promise<string> => {
  const res = await axiosInstance.post<{ caption: string; code?: string }>(
    `/inside/businesses/${businessId}/marketing/caption`,
    req,
    ...(signal ? [{ signal }] : []),
  );
  if (res.data?.code === MARKETING_CAPTION_UNAVAILABLE_CODE) {
    throw new MarketingCaptionUnavailableError();
  }
  return res.data.caption;
};

/**
 * Activity lifecycle statuses (S2-E handoff).
 * - ready / approved: review handoff, no clock
 * - posted / dismissed: existing mark_posted + hide
 * Never scheduled / queued.
 */
export type MarketingActivityStatus =
  | "posted"
  | "dismissed"
  | "ready"
  | "approved";
export type MarketingActivityPlay = MarketingPlay | "unknown";

export interface MarketingActivity {
  id: number;
  business_id: number;
  suggestion_id: string;
  play: MarketingActivityPlay;
  /** Original server value when an older or future play is not recognized. */
  raw_play?: string;
  title: string;
  target_name: string;
  status: MarketingActivityStatus;
  image_url: string;
  caption: string;
  creative_snapshot: MarketingCreativeSnapshot | null;
  posted_at?: string;
  dismissed_at?: string;
  created_by: string;
  created_at: string;
  updated_at: string;
}

export interface MarketingActivityPage {
  activity: MarketingActivity[];
  total: number;
  page: number;
  per_page: number;
}

export interface MarketingActivityFilters {
  status?: MarketingActivityStatus | "";
  play?: MarketingPlay | "";
  from?: string; // RFC3339 or YYYY-MM-DD
  to?: string;
  handled_from?: string; // lifecycle timestamp, independent of created_at
  page?: number;
  per_page?: number;
}

export interface MarketingActivitySnapshot {
  id: string;
  play: MarketingPlay;
  title: string;
  target_name?: string;
  image_url?: string;
  caption?: string;
  creative_snapshot?: MarketingCreativeSnapshot | null;
}

export interface MarketingSettings {
  enabled: boolean;
  disabled_plays: MarketingPlay[];
  /** Always present in current responses; optional for legacy update callers. */
  creative_profile?: MarketingCreativeProfile;
}

export interface MarketingSuggestionsResponse {
  suggestions: CampaignSuggestion[];
  paused: boolean;
  empty_reason: MarketingEmptyReason;
  /** Dish names the ranker refused to campaign (86'd / recipe out). */
  inventory_blocked?: string[];
}

/** Suggestions plus the paused flag (settings.enabled === false). */
export const getMarketingSuggestionsWithState = async (
  businessId: string | number,
): Promise<MarketingSuggestionsResponse> => {
  return getMarketingSuggestions(businessId);
};

export const getMarketingActivity = async (
  businessId: string | number,
  filters: MarketingActivityFilters = {},
): Promise<MarketingActivityPage> => {
  const params: Record<string, string | number> = {};
  if (filters.status) params.status = filters.status;
  if (filters.play) params.play = filters.play;
  if (filters.from) params.from = filters.from;
  if (filters.to) params.to = filters.to;
  if (filters.handled_from) params.handled_from = filters.handled_from;
  if (filters.page) params.page = filters.page;
  if (filters.per_page) params.per_page = filters.per_page;
  type RawMarketingActivity = Omit<MarketingActivity, "play" | "raw_play"> & {
    play: unknown;
  };
  type RawMarketingActivityPage = Omit<MarketingActivityPage, "activity"> & {
    activity: RawMarketingActivity[];
  };
  const res = await axiosInstance.get<RawMarketingActivityPage>(
    `/inside/businesses/${businessId}/marketing/activity`,
    { params },
  );
  return {
    ...res.data,
    activity: res.data.activity.map((row): MarketingActivity => {
      if (isMarketingPlay(row.play)) return { ...row, play: row.play };
      return {
        ...row,
        play: "unknown",
        ...(typeof row.play === "string" ? { raw_play: row.play } : {}),
      };
    }),
  };
};

export type RecordMarketingActivityRequest =
  | {
      action: "post";
      suggestion: MarketingActivitySnapshot;
      creative_snapshot: MarketingCreativeSnapshot;
      /** S3-Loop freeform channel tag (optional; never a schedule). */
      posted_channel?: string;
    }
  | {
      /** S2-E: creator finished craft; waiting owner review. No due date. */
      action: "mark_ready";
      suggestion: MarketingActivitySnapshot;
      creative_snapshot: MarketingCreativeSnapshot;
    }
  | {
      /** S2-E: owner OK to post externally. Snapshot optional (COALESCE). */
      action: "approve";
      suggestion: MarketingActivitySnapshot;
      creative_snapshot?: MarketingCreativeSnapshot;
    }
  | {
      action: "dismiss";
      suggestion: MarketingActivitySnapshot;
      creative_snapshot?: never;
    };

export const recordMarketingActivity = async (
  businessId: string | number,
  body: RecordMarketingActivityRequest,
): Promise<MarketingActivity> => {
  const res = await axiosInstance.post<{ activity: MarketingActivity }>(
    `/inside/businesses/${businessId}/marketing/activity`,
    body,
  );
  return res.data.activity;
};

export const restoreMarketingActivity = async (
  businessId: string | number,
  suggestionId: string,
): Promise<boolean> => {
  const res = await axiosInstance.post<{ restored: boolean }>(
    `/inside/businesses/${businessId}/marketing/activity`,
    { action: "restore", suggestion_id: suggestionId },
  );
  return !!res.data.restored;
};

export const getMarketingSettings = async (
  businessId: string | number,
): Promise<MarketingSettings> => {
  const res = await axiosInstance.get<MarketingSettings>(
    `/inside/businesses/${businessId}/marketing/settings`,
  );
  return res.data;
};

export const updateMarketingSettings = async (
  businessId: string | number,
  settings: MarketingSettings,
): Promise<MarketingSettings> => {
  const res = await axiosInstance.put<MarketingSettings>(
    `/inside/businesses/${businessId}/marketing/settings`,
    settings,
  );
  return res.data;
};
