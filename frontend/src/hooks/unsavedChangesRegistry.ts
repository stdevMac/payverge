/**
 * unsavedChangesRegistry — a tiny, framework-agnostic module singleton that
 * tracks which forms currently hold unsaved edits, keyed by a stable string id.
 *
 * WHY a plain module singleton (not React context / a store): the dashboard's
 * tab-navigation handler needs to synchronously ask "is anything dirty?" the
 * instant a tab is clicked — outside any component render — and show a
 * ConfirmationModal before switching. Coupling this to React internals would
 * make that consult brittle. A module-level Map is race-free in the browser's
 * single-threaded model: every setDirty / hasUnsavedChanges call runs to
 * completion before the next, so there is no torn read.
 *
 * The API is intentionally minimal and STABLE — other components import
 * `hasUnsavedChanges()` (and `clearAll()` on unmount / navigation) and must not
 * depend on anything beyond these three functions.
 */

// id -> dirty. Only ids with dirty === true are retained; a clean form deletes
// its entry so the Map never grows unbounded across mount/unmount cycles.
const dirtyById = new Map<string, boolean>();

/**
 * Record (or clear) the dirty state for a given form id. Passing `false`
 * removes the id from the registry entirely, so `hasUnsavedChanges()` reflects
 * only forms that are currently dirty.
 */
export function setDirty(id: string, dirty: boolean): void {
  if (dirty) {
    dirtyById.set(id, true);
  } else {
    dirtyById.delete(id);
  }
}

/**
 * Returns true if ANY registered form currently has unsaved changes. Callers
 * (e.g. the dashboard tab handler) use this to decide whether to prompt before
 * navigating away.
 */
export function hasUnsavedChanges(): boolean {
  for (const value of dirtyById.values()) {
    if (value) return true;
  }
  return false;
}

/**
 * Clears every tracked dirty state. Call after a confirmed discard or a full
 * navigation so stale ids from an unmounted tree can't block later prompts.
 */
export function clearAll(): void {
  dirtyById.clear();
}
