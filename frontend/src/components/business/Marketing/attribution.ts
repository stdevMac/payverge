/**
 * Lightweight print / destination-pack attribution (S3-Reach).
 *
 * Operators get a stable promo code + QR URL query tags on pack export so a
 * later first-party or manual correlation can match traffic or redemptions.
 *
 * LIMITATIONS (honest, not multi-touch ads):
 * - Guest storefront does NOT currently log or act on `pv_*` query params.
 * - Correlation is offline / future first-party analytics / operator-reported.
 * - Not Meta/Google conversion APIs, not multi-touch attribution graphs.
 * - Promo codes are deterministic from play + suggestion id — not secrets.
 * - Signed private asset URLs are never tagged (isPublicGuestUrl rejects them).
 *
 * Pure data + pure helpers. No React, no schedule fields.
 */

import { isPublicGuestUrl } from "./guestLinks";

/** Query keys stamped onto public guest deep links. */
export const ATTR_QUERY = {
  ref: "pv_ref",
  play: "pv_mkt",
  dest: "pv_dest",
} as const;

export interface AttributionTag {
  /** Human-readable correlation code, e.g. PV-HH-A3F2. */
  promoCode: string;
  play?: string;
  suggestionId?: string;
  destinationId?: string;
  /** Optional durable activity row id when known. */
  activityId?: number | string;
}

const PLAY_ABBREV: Record<string, string> = {
  happy_hour: "HH",
  featured_dish: "FD",
  move_item: "MV",
  win_back: "WB",
  combo_deal: "CD",
  offer: "OF",
};

/**
 * Stable FNV-1a style 16-bit hex from a string (deterministic, not crypto).
 */
function shortHash(input: string): string {
  let h = 0x811c9dc5;
  for (let i = 0; i < input.length; i += 1) {
    h ^= input.charCodeAt(i);
    h = Math.imul(h, 0x01000193);
  }
  // 4 hex digits — enough for operator-facing uniqueness within a venue.
  return (h >>> 0).toString(16).toUpperCase().padStart(8, "0").slice(0, 4);
}

/**
 * Build a short promo / correlation code for packs and print badges.
 * Empty play + empty suggestion → generic PV-GEN-0000.
 */
export function buildPromoCode(input: {
  play?: string | null;
  suggestionId?: string | null;
}): string {
  const play = (input.play ?? "").trim().toLowerCase();
  const sid = (input.suggestionId ?? "").trim();
  const abbrev =
    PLAY_ABBREV[play] ??
    (play
      ? play
          .replace(/[^a-z0-9]/g, "")
          .slice(0, 4)
          .toUpperCase() || "GEN"
      : "GEN");
  const hash = shortHash(`${play}|${sid || "none"}`);
  return `PV-${abbrev}-${hash}`;
}

/**
 * Attach attribution query params to a public guest URL.
 * Returns null when the base URL is empty/unsafe.
 */
export function attachAttributionQuery(
  url: string | null | undefined,
  tag: Pick<AttributionTag, "promoCode" | "play" | "destinationId">,
): string | null {
  const base = (url ?? "").trim();
  if (!isPublicGuestUrl(base)) return null;
  try {
    const parsed = new URL(base);
    const code = tag.promoCode.trim().toUpperCase();
    if (code) parsed.searchParams.set(ATTR_QUERY.ref, code);
    const play = (tag.play ?? "").trim().toLowerCase();
    if (play) parsed.searchParams.set(ATTR_QUERY.play, play);
    const dest = (tag.destinationId ?? "").trim().toLowerCase();
    if (dest) parsed.searchParams.set(ATTR_QUERY.dest, dest);
    return parsed.toString();
  } catch {
    return null;
  }
}

/**
 * Build an AttributionTag for pack export from suggestion + destination context.
 */
export function buildAttributionTag(input: {
  play?: string | null;
  suggestionId?: string | null;
  destinationId?: string | null;
  activityId?: number | string | null;
  /** Optional operator override; otherwise deterministic. */
  promoCode?: string | null;
}): AttributionTag {
  const promoCode = (input.promoCode ?? "").trim().toUpperCase()
    ? (input.promoCode ?? "").trim().toUpperCase()
    : buildPromoCode({
        play: input.play,
        suggestionId: input.suggestionId,
      });
  return {
    promoCode,
    play: (input.play ?? "").trim() || undefined,
    suggestionId: (input.suggestionId ?? "").trim() || undefined,
    destinationId: (input.destinationId ?? "").trim() || undefined,
    activityId:
      input.activityId === null || input.activityId === undefined
        ? undefined
        : input.activityId,
  };
}

/**
 * Plain-text pack member for operators and print shops.
 * Documents correlation limitations explicitly.
 */
export function attributionFileText(args: {
  tag: AttributionTag;
  attributedUrl: string | null;
  limitations?: string;
}): string {
  const lines: string[] = [
    "Payverge marketing attribution",
    "==============================",
    "",
    `Promo code: ${args.tag.promoCode}`,
  ];
  if (args.tag.play) lines.push(`Play: ${args.tag.play}`);
  if (args.tag.destinationId) {
    lines.push(`Destination: ${args.tag.destinationId}`);
  }
  if (args.tag.suggestionId) {
    lines.push(`Suggestion id: ${args.tag.suggestionId}`);
  }
  if (args.tag.activityId != null && args.tag.activityId !== "") {
    lines.push(`Activity id: ${String(args.tag.activityId)}`);
  }
  if (args.attributedUrl) {
    lines.push(`Attributed guest URL: ${args.attributedUrl}`);
  } else {
    lines.push("Attributed guest URL: (none — storefront page off or no slug)");
  }
  lines.push("");
  lines.push("Limitations");
  lines.push("-----------");
  lines.push(
    args.limitations?.trim() ||
      [
        "Guest pages do not currently record these query parameters automatically.",
        "Use the promo code for manual or future first-party correlation only.",
        "This is not multi-touch ads attribution and does not call Meta/Google APIs.",
        "Operators post when ready — Payverge does not own a publish clock.",
      ].join("\n"),
  );
  lines.push("");
  return lines.join("\n");
}

/** Extract promo code from a URL if present. */
export function promoCodeFromUrl(url: string | null | undefined): string | null {
  try {
    const u = new URL((url ?? "").trim());
    const code = u.searchParams.get(ATTR_QUERY.ref)?.trim().toUpperCase() ?? "";
    return code || null;
  } catch {
    return null;
  }
}
