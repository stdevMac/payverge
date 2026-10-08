import type { MarketingAspectRatio } from "../templates/types";
import type { FormatId } from "./formats";

/**
 * AI hero generation + cleanup whitelist (backend `marketing_handlers.go` +
 * `services/ai.go` `nativeMarketingImageAspectRatio`).
 *
 * Intentionally **narrower** than campaign-kit `FormatId` / snapshot
 * `marketingFormats` (which also include wide | strip | 5:7). A hero photo is
 * generated **once** at a native provider aspect; destination packs re-crop
 * via `fitCover` into print/wide/strip canvases. Widening this list would
 * burn extra AI generations per kit format for little visual gain — native
 * aspects already cover the social destinations operators care about.
 *
 * | Canvas format | Gen aspect | Re-crop note |
 * |---|---|---|
 * | 1:1, 4:5, 9:16 | same | native — no re-crop beyond operator crop UI |
 * | 5:7 (print tent) | 4:5 | nearest portrait; fitCover into bleed box |
 * | wide (16:9) | 1:1 | square hero; horizontal cover crop |
 * | strip (4:1) | 1:1 | square hero; heavy horizontal cover crop |
 */
export const HERO_GEN_ASPECTS = ["1:1", "4:5", "9:16"] as const;

export type HeroGenAspect = (typeof HERO_GEN_ASPECTS)[number];

export function isHeroGenAspect(value: string): value is HeroGenAspect {
  return (HERO_GEN_ASPECTS as readonly string[]).includes(value);
}

/**
 * Map a post/export format (or unknown string) to the aspect the AI image
 * endpoints accept. Unknown / print / wide formats coerce; they never pass
 * through to the API as-is.
 */
export function heroGenAspectFor(
  format: FormatId | string | undefined | null,
): MarketingAspectRatio {
  const id = typeof format === "string" ? format.trim() : "";
  if (id === "1:1") return "1:1";
  if (id === "9:16") return "9:16";
  if (id === "4:5") return "4:5";
  // Print tent — portrait closer to feed than story.
  if (id === "5:7") return "4:5";
  // Landscape canvases share a square hero; packs re-crop.
  if (id === "wide" || id === "strip") return "1:1";
  // Destination default / unknown → feed portrait (product default).
  return "4:5";
}

/** True when the export canvas equals the AI gen frame (no intentional re-crop). */
export function heroGenAspectIsNative(
  format: FormatId | string | undefined | null,
): boolean {
  const id = typeof format === "string" ? format.trim() : "";
  return isHeroGenAspect(id);
}
