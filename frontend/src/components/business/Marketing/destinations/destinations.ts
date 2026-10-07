/**
 * Destination packs (Season 1 — Craft / S1-Destinations).
 *
 * A destination is NOT a publisher and never owns a clock. It tells the
 * operator which formats, caption budget, media kinds, and export bundle to
 * use so they can post elsewhere when ready. Pack = files + caption clipboard
 * now. mark_posted + the existing 30-day cooldown remain the only lifecycle
 * clocks.
 *
 * Pure data + pure helpers. No React, no canvas, no schedule fields.
 */

import {
  FORMATS,
  isFormatId,
  type FormatId,
} from "../formats/formats";

export type DestinationId =
  | "ig_feed"
  | "ig_stories"
  | "ig_reels"
  | "tiktok"
  | "whatsapp"
  | "google_business"
  | "print_tent"
  | "print_window"
  | "print_strip";

type DestinationMediaKind = "image" | "video";

/** How the pack leaves the browser. */
type DestinationExportBundle = "single" | "kit";

/**
 * Caption length band for UI + AI generation (story vs feed vs WhatsApp).
 *
 * - short  — Stories, print taglines (tight overlay / paste budgets)
 * - medium — Reels, TikTok, WhatsApp, Google (one-breath paste)
 * - long   — IG Feed (platform allows long; AI still targets soft recommend)
 */
export type CaptionLengthProfile = "short" | "medium" | "long";

/**
 * Caption guidance for one destination.
 *
 * `softMaxChars` is the operator-facing recommend length (counter soft-warn).
 * `hardMaxChars` is the platform hard ceiling used for copy/truncate hints.
 * `hashtags` forces a behaviour when the destination hates tags (Google),
 * otherwise the creative_profile hashtag_behavior is inherited.
 * `length` is the house length band shown in the editor and sent as max_chars.
 */
export interface CaptionProfile {
  softMaxChars: number;
  hardMaxChars: number;
  hashtags: "none" | "inherit";
  length: CaptionLengthProfile;
}

/** Model-side ceiling for free caption generation (matches backend default). */
export const CAPTION_GENERATION_MODEL_CEILING = 280;

/**
 * Max chars to request from AI for this destination profile.
 * Prefer soft recommend when tight; never above hard or the model ceiling.
 */
export function generationMaxCharsFor(profile: CaptionProfile): number {
  const hard =
    profile.hardMaxChars > 0
      ? profile.hardMaxChars
      : CAPTION_GENERATION_MODEL_CEILING;
  const soft =
    profile.softMaxChars > 0
      ? profile.softMaxChars
      : CAPTION_GENERATION_MODEL_CEILING;
  // Target soft when soft is the useful budget (stories/print); otherwise the
  // smaller of soft and model ceiling so feed AI stays scannable.
  const target = Math.min(soft, CAPTION_GENERATION_MODEL_CEILING);
  return Math.max(40, Math.min(hard, target, CAPTION_GENERATION_MODEL_CEILING));
}

/**
 * Platform chrome hint for `PlatformPreviewFrame` and safe-zone UI.
 * Distinct from FormatId: the same 9:16 canvas is a "story" chrome for IG
 * Stories and a "reel" chrome for Reels/TikTok.
 */
type PlatformChrome =
  | "feed"
  | "story"
  | "reel"
  | "square"
  | "print"
  | "strip"
  | "none";

export interface DestinationDef {
  id: DestinationId;
  /** `destinations.<id>.name` */
  labelKey: string;
  /** `destinations.<id>.hint` — short guidance under the chip. */
  hintKey: string;
  /** `destinations.<id>.export` — CTA for the pack download. */
  exportKey: string;
  /** Hero format applied when the operator picks this destination. */
  primaryFormat: FormatId;
  /** Full pack format set (hero first by convention; helpers re-order if not). */
  formats: readonly FormatId[];
  captionProfile: CaptionProfile;
  mediaKinds: readonly DestinationMediaKind[];
  /** Whether motion export is recommended/allowed for this destination. */
  motionAllowed: boolean;
  exportBundle: DestinationExportBundle;
  platformChrome: PlatformChrome;
  /**
   * Export checklist step keys under `destinations.checklist.*`.
   * Pure presentation; never includes schedule/due language.
   */
  checklistKeys: readonly string[];
}

const CHECKLIST_IMAGE = ["crop", "caption", "download", "post_manual"] as const;
const CHECKLIST_MOTION = [
  "crop",
  "caption",
  "download",
  "motion",
  "post_manual",
] as const;
const CHECKLIST_PRINT = ["crop", "print_file", "print_shop"] as const;

/**
 * Closed registry. Ids are stable snake_case wire values for
 * `creative_snapshot.destination_id` (charset-bounded on the backend).
 */
export const DESTINATIONS: Record<DestinationId, DestinationDef> = {
  ig_feed: {
    id: "ig_feed",
    labelKey: "destinations.ig_feed.name",
    hintKey: "destinations.ig_feed.hint",
    exportKey: "destinations.ig_feed.export",
    primaryFormat: "4:5",
    formats: ["4:5", "1:1"],
    captionProfile: {
      softMaxChars: 150,
      hardMaxChars: 2200,
      hashtags: "inherit",
      length: "long",
    },
    mediaKinds: ["image"],
    motionAllowed: false,
    exportBundle: "kit",
    platformChrome: "feed",
    checklistKeys: CHECKLIST_IMAGE,
  },
  ig_stories: {
    id: "ig_stories",
    labelKey: "destinations.ig_stories.name",
    hintKey: "destinations.ig_stories.hint",
    exportKey: "destinations.ig_stories.export",
    primaryFormat: "9:16",
    formats: ["9:16"],
    captionProfile: {
      softMaxChars: 80,
      hardMaxChars: 100,
      hashtags: "inherit",
      length: "short",
    },
    mediaKinds: ["image", "video"],
    motionAllowed: true,
    exportBundle: "single",
    platformChrome: "story",
    checklistKeys: CHECKLIST_MOTION,
  },
  ig_reels: {
    id: "ig_reels",
    labelKey: "destinations.ig_reels.name",
    hintKey: "destinations.ig_reels.hint",
    exportKey: "destinations.ig_reels.export",
    primaryFormat: "9:16",
    formats: ["9:16"],
    captionProfile: {
      softMaxChars: 150,
      hardMaxChars: 2200,
      hashtags: "inherit",
      length: "medium",
    },
    mediaKinds: ["video", "image"],
    motionAllowed: true,
    exportBundle: "single",
    platformChrome: "reel",
    checklistKeys: CHECKLIST_MOTION,
  },
  tiktok: {
    id: "tiktok",
    labelKey: "destinations.tiktok.name",
    hintKey: "destinations.tiktok.hint",
    exportKey: "destinations.tiktok.export",
    primaryFormat: "9:16",
    formats: ["9:16"],
    captionProfile: {
      softMaxChars: 150,
      hardMaxChars: 2200,
      hashtags: "inherit",
      length: "medium",
    },
    mediaKinds: ["video", "image"],
    motionAllowed: true,
    exportBundle: "single",
    platformChrome: "reel",
    checklistKeys: CHECKLIST_MOTION,
  },
  whatsapp: {
    id: "whatsapp",
    labelKey: "destinations.whatsapp.name",
    hintKey: "destinations.whatsapp.hint",
    exportKey: "destinations.whatsapp.export",
    primaryFormat: "1:1",
    formats: ["1:1", "4:5"],
    captionProfile: {
      softMaxChars: 300,
      hardMaxChars: 1000,
      hashtags: "none",
      length: "medium",
    },
    mediaKinds: ["image"],
    motionAllowed: false,
    exportBundle: "single",
    platformChrome: "square",
    checklistKeys: CHECKLIST_IMAGE,
  },
  google_business: {
    id: "google_business",
    labelKey: "destinations.google_business.name",
    hintKey: "destinations.google_business.hint",
    exportKey: "destinations.google_business.export",
    primaryFormat: "1:1",
    formats: ["1:1", "4:5"],
    captionProfile: {
      softMaxChars: 300,
      hardMaxChars: 1500,
      hashtags: "none",
      length: "medium",
    },
    mediaKinds: ["image"],
    motionAllowed: false,
    exportBundle: "single",
    platformChrome: "square",
    checklistKeys: CHECKLIST_IMAGE,
  },
  print_tent: {
    id: "print_tent",
    labelKey: "destinations.print_tent.name",
    hintKey: "destinations.print_tent.hint",
    exportKey: "destinations.print_tent.export",
    primaryFormat: "5:7",
    formats: ["5:7"],
    captionProfile: {
      softMaxChars: 40,
      hardMaxChars: 80,
      hashtags: "none",
      length: "short",
    },
    mediaKinds: ["image"],
    motionAllowed: false,
    exportBundle: "single",
    platformChrome: "print",
    checklistKeys: CHECKLIST_PRINT,
  },
  print_window: {
    id: "print_window",
    labelKey: "destinations.print_window.name",
    hintKey: "destinations.print_window.hint",
    exportKey: "destinations.print_window.export",
    primaryFormat: "1:1",
    formats: ["1:1"],
    captionProfile: {
      softMaxChars: 40,
      hardMaxChars: 80,
      hashtags: "none",
      length: "short",
    },
    mediaKinds: ["image"],
    motionAllowed: false,
    exportBundle: "single",
    platformChrome: "print",
    checklistKeys: CHECKLIST_PRINT,
  },
  print_strip: {
    id: "print_strip",
    labelKey: "destinations.print_strip.name",
    hintKey: "destinations.print_strip.hint",
    exportKey: "destinations.print_strip.export",
    primaryFormat: "strip",
    formats: ["strip"],
    captionProfile: {
      softMaxChars: 40,
      hardMaxChars: 80,
      hashtags: "none",
      length: "short",
    },
    mediaKinds: ["image"],
    motionAllowed: false,
    exportBundle: "single",
    platformChrome: "strip",
    checklistKeys: CHECKLIST_IMAGE,
  },
};

/** Display / picker order. Feed first (default hero). */
export const DESTINATION_ORDER: readonly DestinationId[] = [
  "ig_feed",
  "ig_stories",
  "ig_reels",
  "tiktok",
  "whatsapp",
  "google_business",
  "print_tent",
  "print_window",
  "print_strip",
];

export const DEFAULT_DESTINATION_ID: DestinationId = "ig_feed";

export function isDestinationId(value: unknown): value is DestinationId {
  return (
    typeof value === "string" &&
    (DESTINATION_ORDER as readonly string[]).includes(value)
  );
}

export function destinationFor(
  id: string | null | undefined,
): DestinationDef {
  return DESTINATIONS[
    isDestinationId(id) ? id : DEFAULT_DESTINATION_ID
  ];
}

/**
 * Pack formats for a destination: primary first, then remaining pack members.
 * Unknown FormatIds are impossible by construction (registry is closed against
 * FORMATS); still filter so a future edit cannot leak.
 */
export function packFormatsForDestination(
  dest: DestinationDef | DestinationId,
): FormatId[] {
  const def = typeof dest === "string" ? destinationFor(dest) : dest;
  const primary = isFormatId(def.primaryFormat)
    ? def.primaryFormat
    : ("4:5" as FormatId);
  const out: FormatId[] = [primary];
  const seen = new Set<FormatId>([primary]);
  for (const id of def.formats) {
    if (!isFormatId(id) || seen.has(id)) continue;
    // Drop formats that vanished from the FORMATS registry (defensive).
    if (!FORMATS[id]) continue;
    seen.add(id);
    out.push(id);
  }
  return out;
}

/**
 * Formats offered in simple mode: primary + first alternate (if any).
 * Craft mode can still expose the full pack or full campaign-kit registry.
 */
export function simpleModeFormatsForDestination(
  dest: DestinationDef | DestinationId,
): FormatId[] {
  return packFormatsForDestination(dest).slice(0, 2);
}

export function captionProfileFor(
  dest: DestinationDef | DestinationId,
): CaptionProfile {
  const def = typeof dest === "string" ? destinationFor(dest) : dest;
  return def.captionProfile;
}

/**
 * Soft/hard caption budget status for live counters.
 * Prefer warn over hard clamp — operators own the final paste.
 */
export type CaptionBudgetLevel = "ok" | "soft" | "hard";

export function captionBudgetLevel(
  length: number,
  profile: CaptionProfile,
): CaptionBudgetLevel {
  if (profile.hardMaxChars > 0 && length > profile.hardMaxChars) return "hard";
  if (profile.softMaxChars > 0 && length > profile.softMaxChars) return "soft";
  return "ok";
}

/**
 * Truncate on copy only when past the hard platform ceiling.
 * Soft overages stay intact — warn in UI, do not silently clip.
 */
export function captionForClipboard(
  caption: string,
  profile: CaptionProfile,
): { text: string; truncated: boolean } {
  const text = caption.trim();
  if (profile.hardMaxChars <= 0) return { text, truncated: false };
  const runes = Array.from(text);
  if (runes.length <= profile.hardMaxChars) return { text, truncated: false };
  return {
    text: runes.slice(0, profile.hardMaxChars).join("").trimEnd(),
    truncated: true,
  };
}

/**
 * Effective hashtag behaviour for the destination, merging profile preference.
 */
export function effectiveHashtagBehavior(
  dest: DestinationDef | DestinationId,
  profileBehavior: "none" | "light" | "standard" | "" | undefined,
): "none" | "light" | "standard" {
  const def = typeof dest === "string" ? destinationFor(dest) : dest;
  if (def.captionProfile.hashtags === "none") return "none";
  if (profileBehavior === "none" || profileBehavior === "light" || profileBehavior === "standard") {
    return profileBehavior;
  }
  return "standard";
}

/**
 * Pure builder for caption API options from destination + house voice settings.
 * Used by regenerate in the composer and by unit tests (no React).
 */
export interface CaptionGenerationOptions {
  max_chars: number;
  hashtag_behavior: "none" | "light" | "standard";
  length: CaptionLengthProfile;
  /** Destination-forced hashtag off (Google, WhatsApp, print). */
  hashtagsForcedOff: boolean;
}

export function captionGenerationOptionsFor(
  dest: DestinationDef | DestinationId,
  profileBehavior?: "none" | "light" | "standard" | "" | undefined,
): CaptionGenerationOptions {
  const profile = captionProfileFor(dest);
  return {
    max_chars: generationMaxCharsFor(profile),
    hashtag_behavior: effectiveHashtagBehavior(dest, profileBehavior),
    length: profile.length,
    hashtagsForcedOff: profile.hashtags === "none",
  };
}

/**
 * Cheap must-include phrases for regenerate: operator CTA slot + handle when set.
 * Bounded for the wire contract (backend max 3 × 40 runes).
 */
export function mustIncludePhrasesFromSlots(slots: {
  cta?: string;
  handle?: string;
}): string[] {
  const out: string[] = [];
  const cta = slots.cta?.trim();
  if (cta) out.push(cta.slice(0, 40));
  const handle = slots.handle?.trim();
  if (handle && !out.includes(handle)) out.push(handle.slice(0, 40));
  return out.slice(0, 3);
}

/**
 * Platform preview label key for a destination's chrome (falls back to format).
 */
export function destinationPlatformLabelKey(
  dest: DestinationDef | DestinationId,
): string {
  const def = typeof dest === "string" ? destinationFor(dest) : dest;
  switch (def.platformChrome) {
    case "story":
      return "preview.platform.story";
    case "reel":
      return "preview.platform.reel";
    case "square":
      return "preview.platform.square";
    case "print":
      return "preview.platform.tent";
    case "strip":
      return "preview.platform.strip";
    case "feed":
      return "preview.platform.feed";
    default:
      return "preview.platform.feed";
  }
}

/**
 * Contents of a destination export pack (what the operator gets).
 * Pure description for tests and UI — actual bytes are built by
 * `useCampaignKitExport` / single download.
 *
 * Explicitly does NOT include schedule, due date, or queue fields.
 */
export interface DestinationPackContents {
  destinationId: DestinationId;
  formats: FormatId[];
  includeCaptionFile: boolean;
  exportBundle: DestinationExportBundle;
  motionAllowed: boolean;
  checklistKeys: readonly string[];
}

export function destinationPackContents(
  dest: DestinationDef | DestinationId,
  opts?: { caption?: string },
): DestinationPackContents {
  const def = typeof dest === "string" ? destinationFor(dest) : dest;
  const caption = (opts?.caption ?? "").trim();
  return {
    destinationId: def.id,
    formats: packFormatsForDestination(def),
    includeCaptionFile: caption.length > 0,
    exportBundle: def.exportBundle,
    motionAllowed: def.motionAllowed,
    checklistKeys: def.checklistKeys,
  };
}

/** Keys that must never appear on a DestinationDef (schedule guard). */
export const FORBIDDEN_DESTINATION_KEYS = [
  "scheduled_at",
  "schedule",
  "due_at",
  "dueDate",
  "cadence",
  "queue",
  "best_time",
  "publish_at",
] as const;
