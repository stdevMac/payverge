/**
 * Simple vs Craft editor mode (Season 1 — Craft / S1-Editor).
 *
 * Progressive disclosure only: same Scene IR and composer state; fewer controls
 * in simple mode. Preference is localStorage per business — no backend field.
 */

export type EditorMode = "simple" | "craft";

const STORAGE_PREFIX = "payverge:marketing:editorMode:";

export function editorModeStorageKey(businessId: string | number): string {
  return `${STORAGE_PREFIX}${businessId}`;
}

export function isEditorMode(value: unknown): value is EditorMode {
  return value === "simple" || value === "craft";
}

/** Default for first visit: simple (time-to-export for non-designers). */
export function parseEditorMode(raw: string | null | undefined): EditorMode {
  if (raw == null) return "simple";
  const trimmed = raw.trim().toLowerCase();
  return isEditorMode(trimmed) ? trimmed : "simple";
}

export function readEditorMode(businessId: string | number): EditorMode {
  if (typeof window === "undefined" || !window.localStorage) return "simple";
  try {
    return parseEditorMode(
      window.localStorage.getItem(editorModeStorageKey(businessId)),
    );
  } catch {
    return "simple";
  }
}

export function writeEditorMode(
  businessId: string | number,
  mode: EditorMode,
): void {
  if (typeof window === "undefined" || !window.localStorage) return;
  try {
    window.localStorage.setItem(editorModeStorageKey(businessId), mode);
  } catch {
    // Quota / private mode — preference is non-critical.
  }
}
