/**
 * Slash-palette helpers (L4-12).
 * Pure functions so dismiss/filter behavior is unit-tested without DOM.
 */

/** Token after the leading slash used as the filter query (and dismiss key). */
export function slashQueryFromValue(value: string): string | null {
  const trimmed = value.trimStart();
  if (!trimmed.startsWith("/")) return null;
  // Everything after "/" up to first whitespace is the filter token.
  const after = trimmed.slice(1);
  const space = after.search(/\s/);
  return space === -1 ? after : after.slice(0, space);
}

/** True when value still starts with a slash command draft. */
export function isSlashDraft(value: string): boolean {
  return slashQueryFromValue(value) !== null;
}

/** Filter palette items by substring after "/". Empty query → all items. */
export function filterSlashItems(items: string[], query: string): string[] {
  const q = query.trim().toLowerCase();
  if (!q) return items;
  return items.filter((item) => item.toLowerCase().includes(q));
}

/**
 * After selecting a palette item, replace the leading "/token" with the item
 * text (insert, do not auto-send). Trailing draft after the token is dropped.
 */
export function insertSlashSelection(value: string, item: string): string {
  const leadingWs = value.match(/^\s*/)?.[0] ?? "";
  return `${leadingWs}${item}`;
}
