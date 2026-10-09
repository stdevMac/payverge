/**
 * Guest deep links + QR helpers for Marketing (S2-D).
 *
 * Only public storefront routes (`/b/{slug}…`) are accepted. Signed private
 * asset URLs must never become QR payloads.
 *
 * Pure construction lives here; QR bitmap generation is async and optional.
 */

import { getSiteUrl } from "@/config/publicConfig";

type GuestURLKind = "business" | "menu" | "reservations";

/** Conservative slug charset matching backend normalizeGuestSlug. */
const SLUG_RE = /^[a-zA-Z0-9_-]{2,64}$/;

export interface BuildGuestLinkInput {
  customUrl?: string | null;
  pageEnabled?: boolean | null;
  play?: string | null;
  /** Override origin (tests / local). Empty → this deployment's PUBLIC_URL. */
  origin?: string | null;
}

export interface GuestLink {
  url: string;
  kind: GuestURLKind;
}

/**
 * Build a public guest deep link when the business has a live storefront.
 * Returns null when the URL cannot be constructed safely.
 */
export function buildGuestLink(input: BuildGuestLinkInput): GuestLink | null {
  const slug = (input.customUrl ?? "").trim();
  if (!input.pageEnabled || !SLUG_RE.test(slug)) return null;

  let origin = (input.origin ?? "").trim().replace(/\/+$/, "");
  if (!origin) origin = getSiteUrl();
  if (!origin.startsWith("https://") && !origin.startsWith("http://")) {
    return null;
  }

  const base = `${origin}/b/${encodeURIComponent(slug)}`;
  const play = (input.play ?? "").trim();
  switch (play) {
    case "featured_dish":
    case "move_item":
    case "combo_deal":
    case "offer":
    case "happy_hour":
      return { url: `${base}?tab=menu`, kind: "menu" };
    case "win_back":
      return { url: `${base}?tab=reservations`, kind: "reservations" };
    default:
      return { url: base, kind: "business" };
  }
}

/**
 * True when a URL is safe to encode into a QR / print footer.
 * Rejects empty, non-http(s), and any signed private asset patterns.
 */
export function isPublicGuestUrl(url: string | null | undefined): boolean {
  const u = (url ?? "").trim();
  if (!u) return false;
  if (!u.startsWith("https://") && !u.startsWith("http://")) return false;
  // Defense: never QR a signed S3/R2 object URL.
  if (/[?&]X-Amz-/i.test(u)) return false;
  if (/s3[.-]/i.test(u) && /Signature=/i.test(u)) return false;
  // Prefer storefront paths; still allow same-origin /b/ for local.
  try {
    const parsed = new URL(u);
    if (!parsed.pathname.startsWith("/b/")) return false;
    // Path must not escape the slug segment into arbitrary hosts via @ tricks —
    // URL parser already normalizes host; require a non-empty slug segment.
    const parts = parsed.pathname.split("/").filter(Boolean);
    if (parts.length < 2 || parts[0] !== "b") return false;
    if (!SLUG_RE.test(decodeURIComponent(parts[1]))) return false;
    return true;
  } catch {
    return false;
  }
}

/**
 * Prefer server-provided guest_url when public; else build from business fields.
 */
export function resolveGuestLink(args: {
  guestUrl?: string | null;
  guestUrlKind?: string | null;
  customUrl?: string | null;
  pageEnabled?: boolean | null;
  play?: string | null;
  origin?: string | null;
}): GuestLink | null {
  const provided = (args.guestUrl ?? "").trim();
  if (provided && isPublicGuestUrl(provided)) {
    const kind = normalizeKind(args.guestUrlKind);
    return { url: provided, kind };
  }
  return buildGuestLink({
    customUrl: args.customUrl,
    pageEnabled: args.pageEnabled,
    play: args.play,
    origin: args.origin,
  });
}

function normalizeKind(raw: string | null | undefined): GuestURLKind {
  switch ((raw ?? "").trim()) {
    case "menu":
      return "menu";
    case "reservations":
      return "reservations";
    default:
      return "business";
  }
}

/**
 * Generate a PNG data URL for a public guest URL. Returns null when the URL
 * is empty/unsafe or the QR library fails — callers must omit QR cleanly.
 */
export async function guestUrlToQrDataUrl(
  url: string | null | undefined,
  size = 256,
): Promise<string | null> {
  if (!isPublicGuestUrl(url)) return null;
  try {
    const QRCode = (await import("qrcode")).default;
    /* eslint-disable no-restricted-syntax -- qrcode expects hex, not Tailwind tokens */
    const dark = "#1c1917"; // ink-900 — print-safe dark module
    const light = "#ffffff";
    /* eslint-enable no-restricted-syntax */
    return await QRCode.toDataURL(url!.trim(), {
      errorCorrectionLevel: "M",
      margin: 1,
      width: size,
      color: { dark, light },
    });
  } catch {
    return null;
  }
}
