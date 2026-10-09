/** L4-22 — caption language default for the marketing post composer. */
export function defaultCaptionLanguage(opts: {
  profileLang?: string | null;
  businessLang?: string | null;
  operatorLocale: string;
}): string {
  const profile = opts.profileLang?.trim();
  if (profile) return profile;
  const biz = opts.businessLang?.trim();
  if (biz) return biz;
  return opts.operatorLocale.toLowerCase().startsWith("es") ? "es" : "en";
}
