import type { CampaignSuggestion, MarketingPlay } from "@/api/marketing";
import type { Business } from "@/api/business";
import { formatCurrency } from "@/api/currency";
import type { FormatId } from "./formats/formats";
import { renderPostToBlob } from "./templates/renderPost";
import type { RenderPostInput } from "./templates/renderPost";
import type { ExportVideoInput, ExportVideoResult } from "./motion/exportVideo";
import type { MotionPresetId } from "./motion/presets";
import { PLAY_SLOTS } from "./templates/templates";
import { SlotKey, TemplateStyle } from "./templates/types";

/** Default canvas layout per play — promos lean bold, win-back stays minimal. */
export function defaultTemplateForPlay(
  play?: MarketingPlay | string,
): TemplateStyle {
  switch (play) {
    case "offer":
    case "happy_hour":
    case "combo_deal":
      return "bold";
    case "win_back":
      return "minimal";
    default:
      return "editorial";
  }
}

export interface PlayActionTip {
  key: string;
  params?: Record<string, string | number>;
}

/** Optional operator hint derived from suggestion metrics (i18n key + params). */
export function playActionTip(
  suggestion: CampaignSuggestion,
  opts?: { currency?: string; intlLocale?: string; offLabel?: string },
): PlayActionTip | null {
  const m = suggestion.metrics;
  const currency = opts?.currency ?? "USD";
  const intlLocale = opts?.intlLocale ?? "en-US";
  const offLabel = opts?.offLabel ?? "OFF";
  if (!m) return null;
  switch (suggestion.play) {
    case "happy_hour": {
      const window = m.weakest_window;
      const offer = m.suggested_offer;
      const discountType = m.suggested_discount_type;
      const discountValue = m.suggested_discount_value;
      if (
        typeof offer === "string" &&
        offer.trim() &&
        typeof discountType === "string"
      ) {
        const discount = formatDiscountLabel(
          discountType,
          typeof discountValue === "number"
            ? discountValue
            : Number(discountValue),
          currency,
          intlLocale,
          offLabel,
        );
        if (discount) {
          return {
            key: "card.tips.happyHourOffer",
            params: {
              window: typeof window === "string" ? window : "",
              offer,
              discount,
            },
          };
        }
      }
      if (typeof window === "string" && window.trim()) {
        const pct = m.pct_below_mean;
        if (typeof pct === "number" && pct > 0) {
          return {
            key: "card.tips.happyHourWithPct",
            params: { window, pct: Math.round(pct * 100) },
          };
        }
        return { key: "card.tips.happyHour", params: { window } };
      }
      break;
    }
    case "win_back": {
      const count = m.marketable_lapsed;
      if (typeof count === "number" && count > 0) {
        return {
          key: count === 1 ? "card.tips.winBackOne" : "card.tips.winBack",
          params: { count },
        };
      }
      break;
    }
    case "move_item": {
      const qty = m.qty_sold;
      if (typeof qty === "number") {
        return {
          key: qty === 1 ? "card.tips.moveItemOne" : "card.tips.moveItem",
          params: { qty },
        };
      }
      break;
    }
    default:
      break;
  }
  return null;
}

export interface SocialLinks {
  instagram?: string;
  tiktok?: string;
}

/** Parse business.social_media JSON (or legacy string) into profile URLs. */
export function parseSocialLinks(business: Business): SocialLinks {
  const raw = business.social_media?.trim();
  if (!raw) return {};
  let links: Record<string, unknown> = {};
  if (raw.startsWith("{")) {
    try {
      links = JSON.parse(raw) as Record<string, unknown>;
    } catch {
      return {};
    }
  } else {
    return { instagram: raw };
  }
  const out: SocialLinks = {};
  const ig = links.instagram;
  if (typeof ig === "string" && ig.trim()) out.instagram = ig.trim();
  const tt = links.tiktok;
  if (typeof tt === "string" && tt.trim()) out.tiktok = tt.trim();
  return out;
}

/** Deep link into CRM segments with lapsed guests highlighted. */
export function crmLapsedSegmentsHref(businessId: string | number): string {
  return `/business/${businessId}/dashboard?tab=crm&sub=segments&focus=lapsed`;
}

/** Short outreach draft for win-back — paste into email/SMS or CRM notes. */
export function composeWinBackOutreach(args: {
  businessName: string;
  caption: string;
  guestCount?: number;
  template: string;
}): string {
  const count =
    args.guestCount != null && args.guestCount > 0
      ? String(args.guestCount)
      : "";
  return args.template
    .replace(/\{business\}/g, args.businessName)
    .replace(/\{caption\}/g, args.caption.trim())
    .replace(/\{count\}/g, count)
    .replace(/\s+/g, " ")
    .trim();
}

export function instagramProfileUrl(links: SocialLinks): string | null {
  const raw = links.instagram?.trim();
  if (!raw) return null;
  if (/^https?:\/\//i.test(raw)) return raw.replace(/\/+$/, "");
  const handle = raw.replace(/^@+/, "");
  return handle ? `https://www.instagram.com/${handle}/` : null;
}

/** Business logo for the canvas watermark. */
export function logoUrlFromBusiness(business: Business): string {
  return business.logo?.trim() || "";
}

/**
 * Safe filename stem for a downloaded post asset.
 *
 * `replace(":", "x")` is what makes "4:5" a legal filename on every platform; a
 * Wave 4 id like "wide" passes through untouched, and "5:7" gets the same
 * treatment as the legacy ids.
 */
export function buildPostFilename(
  targetName: string | undefined,
  format: FormatId,
  extension = "png",
): string {
  const stem = (targetName || "post").replace(/\s+/g, "-").toLowerCase();
  return `${stem}-${format.replace(":", "x")}.${extension}`;
}

// business.social_media is a JSON string of links, not a single URL — pick the
// most handle-like platform. Unknown keys (website, …) never become handles.
const HANDLE_PLATFORMS = ["instagram", "tiktok", "twitter", "x", "facebook"];

/** Strip a URL/`@`/trailing slash down to a single `@handle`, or "".
 *  Accepts a bare handle, a profile URL, or the business.social_media JSON blob. */
export function normalizeHandle(raw?: string): string {
  if (!raw) return "";
  const trimmed = raw.trim();
  if (trimmed.startsWith("{")) {
    let links: Record<string, unknown>;
    try {
      links = JSON.parse(trimmed);
    } catch {
      return "";
    }
    for (const platform of HANDLE_PLATFORMS) {
      const url = links?.[platform];
      if (typeof url === "string" && url.trim()) {
        const handle = normalizeHandle(url);
        if (handle) return handle;
      }
    }
    return "";
  }
  const cleaned = trimmed
    .replace(/^https?:\/\/[^/]+\//i, "")
    .replace(/^@+/, "")
    .replace(/\/+$/, "");
  return cleaned ? `@${cleaned}` : "";
}

/** Live happy-hour subject: attached offer name+discount, never a leftover dish. */
export function happyHourLiveOfferName(suggestion: CampaignSuggestion): string {
  if (suggestion.play !== "happy_hour") return "";
  const hasDiscount =
    !!suggestion.discount_type &&
    typeof suggestion.discount_value === "number" &&
    suggestion.discount_value > 0;
  if (!hasDiscount) return "";
  const fromMetrics =
    typeof suggestion.metrics?.suggested_offer === "string"
      ? suggestion.metrics.suggested_offer.trim()
      : "";
  return fromMetrics || suggestion.target_name?.trim() || "";
}

/** Drop happy-hour cards that would caption a dish with no redeemable deal. */
export function isHonestHappyHourSuggestion(
  suggestion: CampaignSuggestion,
): boolean {
  if (suggestion.play !== "happy_hour") return true;
  return happyHourLiveOfferName(suggestion) !== "";
}

export function formatDiscountLabel(
  type: string | undefined,
  value: number | undefined,
  currency: string,
  intlLocale: string,
  // Localized "OFF" suffix. Defaults to English so pure/legacy callers keep
  // working; UI callers pass the translated marketingDashboard.discount.off.
  offLabel = "OFF",
): string {
  if (!type || value == null) return "";
  if (type === "percentage") return `${value}% ${offLabel}`;
  return `${formatCurrency(value, currency, undefined, intlLocale)} ${offLabel}`;
}

interface SlotArgs {
  suggestion: CampaignSuggestion | null;
  business: Business;
  currency: string;
  intlLocale: string;
  ctaLabel: string;
  badgeLabel: string;
  offLabel?: string;
}

/** Prefill the template's editable slots from suggestion data (localized labels
 *  supplied by the caller). Keeps only the slots the play uses. */
export function buildSlots({
  suggestion,
  business,
  currency,
  intlLocale,
  ctaLabel,
  badgeLabel,
  offLabel = "OFF",
}: SlotArgs): Partial<Record<SlotKey, string>> {
  const play = suggestion?.play ?? "";
  const allowed = PLAY_SLOTS[play] ?? PLAY_SLOTS[""];
  const priceMetric = suggestion?.metrics?.price;
  // A non-positive price on a marketing asset is always wrong — omit the slot.
  const price =
    typeof priceMetric === "number" && priceMetric > 0
      ? formatCurrency(priceMetric, currency, undefined, intlLocale)
      : "";
  const discount =
    play === "offer" || play === "happy_hour"
      ? formatDiscountLabel(
          suggestion?.discount_type,
          suggestion?.discount_value,
          currency,
          intlLocale,
          offLabel,
        )
      : "";
  const dishName =
    play === "happy_hour" && suggestion
      ? happyHourLiveOfferName(suggestion) || suggestion.target_name || ""
      : (suggestion?.target_name ?? "");
  const base: Partial<Record<SlotKey, string>> = {
    dishName,
    price,
    badge:
      play === "offer" || (play === "happy_hour" && discount)
        ? discount
        : badgeLabel,
    cta: ctaLabel,
    handle: normalizeHandle(business.social_media),
  };
  const filtered: Partial<Record<SlotKey, string>> = {};
  (Object.keys(base) as SlotKey[]).forEach((k) => {
    if (allowed.includes(k)) filtered[k] = base[k];
  });
  return filtered;
}

interface CaptionArgs {
  suggestion: CampaignSuggestion;
  business: Business;
  currency: string;
  intlLocale: string;
  template: string; // localized template with {name} {discount} {handle}
  offLabel?: string;
}

/** Fill a localized caption template from suggestion data; collapse whitespace. */
export function composeStarterCaption({
  suggestion,
  business,
  currency,
  intlLocale,
  template,
  offLabel = "OFF",
}: CaptionArgs): string {
  const name =
    suggestion.play === "happy_hour"
      ? happyHourLiveOfferName(suggestion) || suggestion.target_name || ""
      : (suggestion.target_name ?? "");
  const handle = normalizeHandle(business.social_media);
  const discount =
    suggestion.play === "offer" || suggestion.play === "happy_hour"
      ? formatDiscountLabel(
          suggestion.discount_type,
          suggestion.discount_value,
          currency,
          intlLocale,
          offLabel,
        )
      : "";
  return template
    .replace(/\{name\}/g, name)
    .replace(/\{discount\}/g, discount)
    .replace(/\{handle\}/g, handle)
    .replace(/\s+/g, " ")
    .trim();
}

export type MarketingCaptionLocale = "en" | "es" | "es-AR";

/** Canonicalize the three operator locales accepted by marketing caption generation. */
export function canonicalMarketingCaptionLocale(
  locale?: string,
): MarketingCaptionLocale {
  const normalized = (locale || "en").trim().replace(/_/g, "-").toLowerCase();
  if (normalized === "es-ar" || normalized === "es-419") return "es-AR";
  if (normalized === "es" || normalized.startsWith("es-")) return "es";
  return "en";
}

function limitCaptionRunes(value: string, maxRunes = 280): string {
  const runes = Array.from(value);
  if (runes.length <= maxRunes) return value;
  return `${runes
    .slice(0, maxRunes - 1)
    .join("")
    .trimEnd()}…`;
}

/** Venue names allowed in public captions. Demo showrooms are omitted. */
export function publicMarketingVenueName(business: {
  name?: string;
}): string {
  const name = business.name?.trim() || "";
  if (!name) return "";
  const folded = name.toLowerCase();
  if (folded.includes("payverge") || folded.includes("demo lounge")) {
    return "";
  }
  return name;
}

/**
 * Factual caption shown synchronously before AI is available.
 *
 * It deliberately uses only the suggestion's target name and the business
 * name. No offer, timing, availability, popularity, or urgency is inferred.
 */
export function composeLocalizedFallbackCaption({
  suggestion,
  business,
  locale,
}: {
  suggestion: CampaignSuggestion;
  business: Business;
  locale?: string;
}): string {
  const language = canonicalMarketingCaptionLocale(locale);
  const target =
    suggestion.play === "happy_hour"
      ? happyHourLiveOfferName(suggestion)
      : suggestion.target_name?.trim() || "";
  const businessName = publicMarketingVenueName(business);

  if (language === "es-AR") {
    if (target && businessName)
      return limitCaptionRunes(`Conocé ${target} en ${businessName}.`);
    if (target) return limitCaptionRunes(`Conocé ${target}.`);
    if (businessName) return limitCaptionRunes(`Descubrí ${businessName}.`);
    return limitCaptionRunes("Descubrí una idea de tu restaurante.");
  }

  if (language === "es") {
    if (target && businessName)
      return limitCaptionRunes(`Conoce ${target} en ${businessName}.`);
    if (target) return limitCaptionRunes(`Conoce ${target}.`);
    if (businessName) return limitCaptionRunes(`Descubre ${businessName}.`);
    return limitCaptionRunes("Descubre una idea de tu restaurante.");
  }

  if (target && businessName)
    return limitCaptionRunes(`Discover ${target} at ${businessName}.`);
  if (target) return limitCaptionRunes(`Discover ${target}.`);
  if (businessName) return limitCaptionRunes(`Discover ${businessName}.`);
  return limitCaptionRunes("Discover an idea from your restaurant.");
}

interface DownloadArgs {
  renderInput: RenderPostInput;
  filename: string;
}

/**
 * How long a download object URL stays alive after the click.
 *
 * Revoking in the same turn as `a.click()` cancels the download in Chrome and
 * Safari when the blob is still in memory (ZIP kits, rendered PNGs). A minute
 * is long enough for the browser to take a snapshot of the blob and short
 * enough that a kit does not leak for the rest of the session.
 */
export const DOWNLOAD_OBJECT_URL_TTL_MS = 60_000;

function assertDownloadableBlob(blob: Blob): void {
  if (!(blob instanceof Blob) || blob.size <= 0) {
    throw new Error("empty_blob");
  }
}

/**
 * Trigger a browser download via a temporary anchor. Exported for tests and
 * for callers that already hold a blob (PNG, video, caption.txt, ZIP).
 *
 * Some mobile browsers ignore `download` and open the blob instead — that is
 * still a successful handoff (operator can long-press / save).
 */
export function downloadBlob(blob: Blob, filename: string): void {
  assertDownloadableBlob(blob);
  const safeName = filename.trim() || "download";
  const url = URL.createObjectURL(blob);
  try {
    const link = document.createElement("a");
    link.download = safeName;
    link.href = url;
    link.rel = "noopener";
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
  } catch (cause) {
    URL.revokeObjectURL(url);
    throw cause;
  }
  window.setTimeout(() => URL.revokeObjectURL(url), DOWNLOAD_OBJECT_URL_TTL_MS);
}

/**
 * Last-resort handoff when the download attribute is unusable (rare locked-down
 * WebViews). Opens the blob URL so the operator can save from the new tab.
 * Prefer `downloadBlob` first.
 */
function openBlobInNewTab(blob: Blob): void {
  const url = URL.createObjectURL(blob);
  const opened = window.open(url, "_blank", "noopener,noreferrer");
  if (!opened) {
    // Popup blocked — navigate current tab as a final fallback.
    window.location.assign(url);
  } else {
    window.setTimeout(() => URL.revokeObjectURL(url), 60_000);
  }
}

export type ShareOrDownloadResult = "shared" | "downloaded";

export interface ShareOrDownloadArgs {
  blob: Blob;
  filename: string;
  /** MIME for the File() constructor; defaults to blob.type or octet-stream. */
  mime?: string;
  /** Optional text included in the share sheet / clipboard on download. */
  text?: string;
  /**
   * When true (default) and share is unavailable or file-share is rejected,
   * also download a companion `{stem}-caption.txt` if `text` is non-empty.
   */
  companionCaptionFile?: boolean;
}

/**
 * Phone-first asset handoff (Season 1 — Craft / S1-D).
 *
 * Order:
 * 1. `navigator.share` + `canShare({ files })` with a File when available
 * 2. Anchor download (`downloadBlob`)
 * 3. Open blob URL in a new tab if download construction throws
 *
 * AbortError from a dismissed share sheet propagates so callers do not treat
 * cancel as success. Missing `navigator.share` is not an error — falls through.
 */
export async function shareOrDownload(
  args: ShareOrDownloadArgs,
): Promise<ShareOrDownloadResult> {
  const mime =
    args.mime ||
    (args.blob.type && args.blob.type.length > 0
      ? args.blob.type
      : "application/octet-stream");
  const text = args.text?.trim() ?? "";
  const withCaptionFile = args.companionCaptionFile !== false;

  if (
    typeof navigator !== "undefined" &&
    typeof navigator.share === "function"
  ) {
    try {
      const file = new File([args.blob], args.filename, { type: mime });
      const shareData: ShareData = text
        ? { files: [file], text }
        : { files: [file] };
      const can =
        typeof navigator.canShare !== "function" ||
        navigator.canShare(shareData);
      if (can) {
        await navigator.share(shareData);
        return "shared";
      }
    } catch (err) {
      // User dismissed the sheet — do not fall through to a surprise download.
      if (err instanceof DOMException && err.name === "AbortError") throw err;
      // NotAllowedError / TypeError / canShare false path: fall through.
    }
  }

  try {
    downloadBlob(args.blob, args.filename);
    if (withCaptionFile && text) {
      const base = args.filename.replace(/\.[^.]+$/i, "");
      downloadBlob(
        new Blob([text], { type: "text/plain;charset=utf-8" }),
        `${base}-caption.txt`,
      );
    }
    if (text) {
      try {
        await copyCaption(text);
      } catch {
        // Clipboard may be blocked after async share attempt; download still ok.
      }
    }
    return "downloaded";
  } catch {
    openBlobInNewTab(args.blob);
    return "downloaded";
  }
}

function downloadRenderedPostPack(
  blob: Blob,
  filename: string,
  caption: string,
): void {
  downloadBlob(blob, filename);
  const text = caption.trim();
  if (!text) return;
  const base = filename.replace(/\.png$/i, "");
  downloadBlob(
    new Blob([text], { type: "text/plain;charset=utf-8" }),
    `${base}-caption.txt`,
  );
}

/** Render the post to a PNG and trigger a browser download. */
export async function downloadPost({
  renderInput,
  filename,
}: DownloadArgs): Promise<void> {
  const blob = await renderPostToBlob(renderInput);
  downloadBlob(blob, filename);
}

/** Download the PNG plus a companion caption .txt file (same user gesture). */
export async function downloadPostPack({
  caption,
  ...args
}: DownloadArgs & { caption: string }): Promise<void> {
  const blob = await renderPostToBlob(args.renderInput);
  downloadRenderedPostPack(blob, args.filename, caption);
}

/**
 * Download the motion export: the MP4, its poster frame, and the caption.
 *
 * Three files, one user gesture, mirroring `downloadPostPack`. The poster ships
 * alongside because some scheduling tools want an explicit cover image, and
 * because an operator who decides the video is not right still walks away with
 * the still.
 *
 * `exportMotion` is injected so this stays testable: the real one reaches for
 * `VideoEncoder` and `OffscreenCanvas`, neither of which exists under jsdom.
 *
 * Motion export is entirely client-side — no model call and no daily fair-use
 * metering. Encoding runs in the browser via VideoEncoder / OffscreenCanvas.
 */
export async function downloadPostVideo({
  renderInput,
  filename,
  caption,
  preset,
  onProgress,
  signal,
  exportMotion,
}: DownloadArgs & {
  caption: string;
  preset: MotionPresetId;
  onProgress?: (fraction: number) => void;
  signal?: AbortSignal;
  /**
   * Defaults to the worker-preferring host. Loaded dynamically so Jest (CJS)
   * never has to parse the host's `import.meta.url` worker bootstrap.
   */
  exportMotion?: (input: ExportVideoInput) => Promise<ExportVideoResult>;
}): Promise<void> {
  const run =
    exportMotion ??
    (await import("./motion/exportVideoWorkerHost")).exportVideoPreferWorker;
  const { video, poster } = await run({
    renderInput,
    preset,
    ...(onProgress ? { onProgress } : {}),
    ...(signal ? { signal } : {}),
  });
  const base = filename.replace(/\.mp4$/i, "");
  downloadBlob(video, filename);
  downloadBlob(poster, `${base}-poster.png`);
  const text = caption.trim();
  if (text) {
    downloadBlob(
      new Blob([text], { type: "text/plain;charset=utf-8" }),
      `${base}-caption.txt`,
    );
  }
}

/** Copy caption text to the clipboard for pasting into social apps. */
export async function copyCaption(caption: string): Promise<void> {
  await navigator.clipboard.writeText(caption.trim());
}

/**
 * Render the post, then share or download via {@link shareOrDownload}.
 * Companion caption.txt ships on the download fallback path only (native share
 * already carries caption text when the OS supports it).
 */
export async function shareOrDownloadPostPack(
  args: DownloadArgs & { caption: string },
): Promise<ShareOrDownloadResult> {
  const blob = await renderPostToBlob(args.renderInput);
  return shareOrDownload({
    blob,
    filename: args.filename,
    mime: "image/png",
    text: args.caption,
    companionCaptionFile: true,
  });
}
