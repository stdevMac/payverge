/**
 * Why a marketing card Share/Download action is disabled.
 * Pure helper so tests and UI share one source of truth.
 */

type PreviewRenderState = "rendering" | "ready" | "failed" | string;

export type ExportBlockedReason =
  | "needs_photo"
  | "photo_broken"
  | "photo_too_dark"
  | "photo_weak"
  | "preview_rendering"
  | "caption_loading"
  | "caption_failed"
  | "needs_caption";

/** Photo-library verdicts from readinessFor — consulted before "Ready to post". */
export type PhotoQualityGate =
  | "ok"
  | "too_dark"
  | "weak"
  | "reshoot"
  | "unknown";

export interface ExportReadinessInput {
  hasPhoto: boolean;
  previewState: PreviewRenderState;
  displayCaption: string;
  captionGenerating?: boolean;
  captionError?: boolean;
  /** When set, blocks Ready-to-post for photos the library already grades poorly. */
  photoQuality?: PhotoQualityGate;
}

/** Null when export is allowed; otherwise the primary blocker key. */
export function exportBlockedReason(
  input: ExportReadinessInput,
): ExportBlockedReason | null {
  if (!input.hasPhoto) return "needs_photo";
  if (input.previewState === "failed") return "photo_broken";
  if (input.previewState !== "ready") return "preview_rendering";
  // Library grades "Usable · Too dark" as weak+underexposed — still not Ready.
  if (input.photoQuality === "too_dark") return "photo_too_dark";
  if (input.photoQuality === "reshoot") return "photo_weak";
  if (input.captionGenerating) return "caption_loading";
  if (input.captionError) return "caption_failed";
  if (!input.displayCaption.trim()) return "needs_caption";
  return null;
}

/** i18n key under marketingDashboard for a blocked export reason. */
export function exportBlockedReasonKey(
  reason: ExportBlockedReason,
): `card.blocked.${ExportBlockedReason}` {
  return `card.blocked.${reason}`;
}

export function isExportReady(input: ExportReadinessInput): boolean {
  return exportBlockedReason(input) === null;
}
