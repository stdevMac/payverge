import { cachedPhotoAnalysis } from "./photo/cache";
import { readinessFor } from "./photo/readiness";
import type { PhotoQualityGate } from "./exportReadiness";

/**
 * Map the photo-library readiness score into an export gate.
 * Underexposed ("Too dark") always fails Ready-to-post, even when the library
 * verdict is still "Usable" (weak).
 */
export function photoQualityGateForUrl(url: string | undefined | null): PhotoQualityGate {
  const trimmed = url?.trim() ?? "";
  if (!trimmed) return "unknown";
  const analysis = cachedPhotoAnalysis(trimmed);
  if (analysis === undefined) {
    // Cache miss — preview may still be rendering; don't block yet.
    return "unknown";
  }
  if (analysis === null) return "reshoot";
  const readiness = readinessFor(analysis);
  if (readiness.reasons.includes("underexposed")) return "too_dark";
  if (readiness.verdict === "reshoot") return "reshoot";
  if (readiness.verdict === "weak") return "ok";
  return "ok";
}
