/**
 * L3-41: `translatedEditBlocked` is a TEMPLATE ("Switch to {language} to
 * edit"). MenuBuilder/index.tsx interpolated it for the toast, but the card
 * tooltips rendered it raw, so operators saw the literal `{language}` in the
 * `title` attribute.
 *
 * The placeholder is always resolved — never leaked. When the language name is
 * unknown (empty), the placeholder is dropped and the leftover double space is
 * collapsed, which reads better than shipping braces to the DOM.
 */
export function interpolateEditBlockedTooltip(
  template: string,
  editLanguageName?: string,
): string {
  const name = (editLanguageName ?? "").trim();
  const filled = template.replaceAll("{language}", name);
  return name ? filled : filled.replace(/\s{2,}/g, " ").trim();
}
