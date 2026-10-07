const cache: Record<string, Record<string, unknown>> = {};

async function loadBundle(language: string): Promise<Record<string, unknown>> {
  // Guard against an empty/undefined language: a blank interpolation would
  // resolve to `./.json` and throw "Cannot find module './.json'". Fall back
  // to the English bundle so a missing key never produces a broken import path.
  const lang = language || "en";
  if (cache[lang]) return cache[lang];
  try {
    const mod = await import(`../../i18n/guest-messages/${lang}.json`);
    cache[lang] = mod.default;
    return cache[lang];
  } catch (e) {
    if (lang === "en") {
      console.error("Failed to load starter questions bundle", e);
    }
    if (lang !== "en") return loadBundle("en");
    return {};
  }
}

const EN_FALLBACK: string[] = [
  "What's good today?",
  "Vegetarian options?",
  "Tell me about your best dish",
  "Do you have desserts?",
];

export async function getStarterQuestions(language: string): Promise<string[]> {
  const lang = language || "en";
  const bundle = await loadBundle(lang);
  const ai = bundle?.aiWaiter as { starterQuestions?: unknown } | undefined;
  const arr = ai?.starterQuestions;
  if (Array.isArray(arr) && arr.every((x) => typeof x === "string") && arr.length > 0) {
    return arr as string[];
  }
  if (lang !== "en") return getStarterQuestions("en");
  return EN_FALLBACK;
}
