/**
 * L3-6: interpolate the translated-view banner.
 * Template uses {viewLanguage} (locale being viewed) and {editLanguage}
 * (canonical/default locale to switch to for edits). Also supports the legacy
 * dual `{language}` form where the first occurrence is the viewed language and
 * the second is the edit language.
 */
export function interpolateTranslatedViewNotice(
  template: string,
  viewLanguageName: string,
  editLanguageName: string,
): string {
  if (
    template.includes("{viewLanguage}") ||
    template.includes("{editLanguage}")
  ) {
    return template
      .replaceAll("{viewLanguage}", viewLanguageName)
      .replaceAll("{editLanguage}", editLanguageName);
  }
  let n = 0;
  return template.replace(/\{language\}/g, () => {
    n += 1;
    return n === 1 ? viewLanguageName : editLanguageName;
  });
}
