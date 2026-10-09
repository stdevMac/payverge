/**
 * Print-template font registry and @font-face CSS builder.
 *
 * Pure, DOM-free module. Fonts load ONLY inside generated print HTML via
 * @font-face — never in the app bundle.
 */

export type PrintFontFace = {
  /** CSS font-weight (number, or range string e.g. "400 700" for variable faces). */
  weight: number | string;
  /** CSS font-style. */
  style: "normal" | "italic";
  /** Public URL path starting with `/fonts/...`. */
  path: string;
};

export type PrintFontFamilyName =
  | "DM Serif Display"
  | "DM Sans"
  | "EB Garamond"
  | "Cormorant Garamond"
  | "Oswald"
  | "Noto Naskh Arabic"
  | "Noto Sans Arabic"
  | "Noto Sans Hebrew"
  | "Noto Serif CJK JP"
  | "Noto Sans CJK JP"
  | "Noto Sans CJK KR"
  | "Noto Sans CJK SC"
  | "Noto Sans CJK TC";

export type PrintScriptFontFallback = {
  script: "arabic" | "hebrew" | "cjk-jp" | "cjk-kr" | "cjk-sc" | "cjk-tc";
  displayFamily: PrintFontFamilyName;
  bodyFamily: PrintFontFamilyName;
  /** Families that must be passed to buildFontFaceCss. */
  families: readonly PrintFontFamilyName[];
};

/**
 * Self-hosted faces available to print templates.
 * DM Serif Display / DM Sans reuse existing app public paths (not duplicated).
 * EB Garamond / Cormorant Garamond / Oswald live under /fonts/print/.
 */
export const PRINT_FONT_FAMILIES: Readonly<
  Record<PrintFontFamilyName, readonly PrintFontFace[]>
> = {
  "DM Serif Display": [
    {
      weight: 400,
      style: "normal",
      path: "/fonts/dm-serif-display/dm-serif-display-regular-latin.woff2",
    },
    {
      weight: 400,
      style: "normal",
      path: "/fonts/dm-serif-display/dm-serif-display-regular-latin-ext.woff2",
    },
    {
      weight: 400,
      style: "italic",
      path: "/fonts/dm-serif-display/dm-serif-display-italic-latin.woff2",
    },
    {
      weight: 400,
      style: "italic",
      path: "/fonts/dm-serif-display/dm-serif-display-italic-latin-ext.woff2",
    },
  ],
  "DM Sans": [
    {
      weight: "400 700",
      style: "normal",
      path: "/fonts/dm-sans/dm-sans-latin.woff2",
    },
    {
      weight: "400 700",
      style: "normal",
      path: "/fonts/dm-sans/dm-sans-latin-ext.woff2",
    },
  ],
  "EB Garamond": [
    {
      weight: 400,
      style: "normal",
      path: "/fonts/print/eb-garamond/EBGaramond-Regular.woff2",
    },
    {
      weight: 400,
      style: "italic",
      path: "/fonts/print/eb-garamond/EBGaramond-Italic.woff2",
    },
    {
      weight: 600,
      style: "normal",
      path: "/fonts/print/eb-garamond/EBGaramond-SemiBold.woff2",
    },
  ],
  "Cormorant Garamond": [
    {
      weight: 500,
      style: "normal",
      path: "/fonts/print/cormorant-garamond/CormorantGaramond-Medium.woff2",
    },
    {
      weight: 600,
      style: "normal",
      path: "/fonts/print/cormorant-garamond/CormorantGaramond-SemiBold.woff2",
    },
  ],
  Oswald: [
    {
      weight: 600,
      style: "normal",
      path: "/fonts/print/oswald/Oswald-SemiBold.woff2",
    },
  ],
  "Noto Naskh Arabic": [
    {
      weight: "400 700",
      style: "normal",
      path: "/fonts/print/noto-naskh-arabic/NotoNaskhArabic-Variable.woff2",
    },
  ],
  "Noto Sans Arabic": [
    {
      weight: "100 900",
      style: "normal",
      path: "/fonts/print/noto-sans-arabic/NotoSansArabic-Variable.woff2",
    },
  ],
  "Noto Sans Hebrew": [
    {
      weight: "100 900",
      style: "normal",
      path: "/fonts/print/noto-sans-hebrew/NotoSansHebrew-Variable.woff2",
    },
  ],
  "Noto Serif CJK JP": [
    {
      weight: "200 900",
      style: "normal",
      path: "/fonts/print/noto-serif-cjk-jp/NotoSerifCJKjp-Variable.woff2",
    },
  ],
  "Noto Sans CJK JP": [
    {
      weight: "100 900",
      style: "normal",
      path: "/fonts/print/noto-sans-cjk-jp/NotoSansCJKjp-Variable.woff2",
    },
  ],
  "Noto Sans CJK KR": [
    {
      weight: "100 900",
      style: "normal",
      path: "/fonts/print/noto-sans-cjk-kr/NotoSansKR-Variable.woff2",
    },
  ],
  "Noto Sans CJK SC": [
    {
      weight: "100 900",
      style: "normal",
      path: "/fonts/print/noto-sans-cjk-sc/NotoSansSC-Variable.woff2",
    },
  ],
  "Noto Sans CJK TC": [
    {
      weight: "100 900",
      style: "normal",
      path: "/fonts/print/noto-sans-cjk-tc/NotoSansTC-Variable.woff2",
    },
  ],
};

/** License files shipped beside the converted OFL variable webfonts. */
export const SCRIPT_PRINT_FONT_LICENSE_PATHS: Readonly<
  Record<
    | "Noto Naskh Arabic"
    | "Noto Sans Arabic"
    | "Noto Sans Hebrew"
    | "Noto Serif CJK JP"
    | "Noto Sans CJK JP"
    | "Noto Sans CJK KR"
    | "Noto Sans CJK SC"
    | "Noto Sans CJK TC",
    string
  >
> = {
  "Noto Naskh Arabic": "/fonts/print/noto-naskh-arabic/OFL.txt",
  "Noto Sans Arabic": "/fonts/print/noto-sans-arabic/OFL.txt",
  "Noto Sans Hebrew": "/fonts/print/noto-sans-hebrew/OFL.txt",
  "Noto Serif CJK JP": "/fonts/print/noto-serif-cjk-jp/OFL.txt",
  "Noto Sans CJK JP": "/fonts/print/noto-sans-cjk-jp/OFL.txt",
  "Noto Sans CJK KR": "/fonts/print/noto-sans-cjk-kr/OFL.txt",
  "Noto Sans CJK SC": "/fonts/print/noto-sans-cjk-sc/OFL.txt",
  "Noto Sans CJK TC": "/fonts/print/noto-sans-cjk-tc/OFL.txt",
};

const ARABIC_LANGUAGE_SUBTAGS = new Set([
  "ar",
  "ckb",
  "fa",
  "ps",
  "sd",
  "ug",
  "ur",
]);
const RTL_LANGUAGE_SUBTAGS = new Set([
  ...ARABIC_LANGUAGE_SUBTAGS,
  "dv",
  "he",
  "yi",
]);
const RTL_SCRIPT_SUBTAGS = new Set([
  "adlm",
  "arab",
  "hebr",
  "mand",
  "nkoo",
  "rohg",
  "syrc",
  "thaa",
]);
const TRADITIONAL_CHINESE_REGIONS = new Set(["hk", "mo", "tw"]);

const ARABIC_FALLBACK: PrintScriptFontFallback = {
  script: "arabic",
  displayFamily: "Noto Naskh Arabic",
  bodyFamily: "Noto Sans Arabic",
  families: ["Noto Naskh Arabic", "Noto Sans Arabic"],
};

const HEBREW_FALLBACK: PrintScriptFontFallback = {
  script: "hebrew",
  displayFamily: "Noto Sans Hebrew",
  bodyFamily: "Noto Sans Hebrew",
  families: ["Noto Sans Hebrew"],
};

const CJK_JP_FALLBACK: PrintScriptFontFallback = {
  script: "cjk-jp",
  displayFamily: "Noto Serif CJK JP",
  bodyFamily: "Noto Sans CJK JP",
  families: ["Noto Serif CJK JP", "Noto Sans CJK JP"],
};

const CJK_KR_FALLBACK: PrintScriptFontFallback = {
  script: "cjk-kr",
  displayFamily: "Noto Sans CJK KR",
  bodyFamily: "Noto Sans CJK KR",
  families: ["Noto Sans CJK KR"],
};

const CJK_SC_FALLBACK: PrintScriptFontFallback = {
  script: "cjk-sc",
  displayFamily: "Noto Sans CJK SC",
  bodyFamily: "Noto Sans CJK SC",
  families: ["Noto Sans CJK SC"],
};

const CJK_TC_FALLBACK: PrintScriptFontFallback = {
  script: "cjk-tc",
  displayFamily: "Noto Sans CJK TC",
  bodyFamily: "Noto Sans CJK TC",
  families: ["Noto Sans CJK TC"],
};

type ParsedPrintLocale = {
  language: string;
  script?: string;
  region?: string;
};

function parsePrintLocale(
  language: string | null | undefined,
): ParsedPrintLocale | null {
  const normalized = (language ?? "").trim().replaceAll("_", "-");
  if (!normalized) return null;

  try {
    const locale = new Intl.Locale(normalized);
    return {
      language: locale.language.toLowerCase(),
      script: locale.script?.toLowerCase(),
      region: locale.region?.toLowerCase(),
    };
  } catch {
    return null;
  }
}

function chineseFallback(region?: string): PrintScriptFontFallback {
  return region && TRADITIONAL_CHINESE_REGIONS.has(region)
    ? CJK_TC_FALLBACK
    : CJK_SC_FALLBACK;
}

/**
 * Resolve deterministic non-Latin print fallbacks from a BCP-47 language tag.
 *
 * Explicit script subtags take precedence, so `ar-Latn` does not receive an
 * Arabic face while `en-Arab` does. Japanese, Korean, Simplified Chinese, and
 * Traditional Chinese route to locale-appropriate faces. Generic `zh`
 * deterministically defaults to Simplified Chinese; zh-HK/MO/TW default to
 * Traditional Chinese when no explicit script is present.
 */
export function getPrintScriptFontFallback(
  language: string | null | undefined,
): PrintScriptFontFallback | null {
  const locale = parsePrintLocale(language);
  if (!locale) return null;

  if (locale.script) {
    if (locale.script === "arab") return ARABIC_FALLBACK;
    if (locale.script === "hebr") return HEBREW_FALLBACK;
    if (["hira", "jpan", "kana"].includes(locale.script)) {
      return CJK_JP_FALLBACK;
    }
    if (["hang", "kore"].includes(locale.script)) return CJK_KR_FALLBACK;
    if (locale.script === "hans") return CJK_SC_FALLBACK;
    if (locale.script === "hant") return CJK_TC_FALLBACK;
    if (locale.script === "hani") {
      if (locale.language === "ja") return CJK_JP_FALLBACK;
      if (locale.language === "ko") return CJK_KR_FALLBACK;
      return chineseFallback(locale.region);
    }
    return null;
  }

  if (ARABIC_LANGUAGE_SUBTAGS.has(locale.language)) return ARABIC_FALLBACK;
  if (locale.language === "he" || locale.language === "yi") {
    return HEBREW_FALLBACK;
  }
  if (locale.language === "ja") return CJK_JP_FALLBACK;
  if (locale.language === "ko") return CJK_KR_FALLBACK;
  if (locale.language === "zh") return chineseFallback(locale.region);
  return null;
}

/** Resolve HTML/CSS inline direction from the same BCP-47 interpretation. */
export function getPrintLanguageDirection(
  language: string | null | undefined,
): "ltr" | "rtl" {
  const locale = parsePrintLocale(language);
  if (!locale) return "ltr";
  if (locale.script) {
    return RTL_SCRIPT_SUBTAGS.has(locale.script) ? "rtl" : "ltr";
  }
  return RTL_LANGUAGE_SUBTAGS.has(locale.language) ? "rtl" : "ltr";
}

function validateFontOrigin(origin: string): void {
  if (origin === "") return;
  try {
    const parsed = new URL(origin);
    if (
      !["http:", "https:"].includes(parsed.protocol) ||
      parsed.origin !== origin ||
      parsed.username ||
      parsed.password
    ) {
      throw new Error("unsafe");
    }
  } catch {
    throw new Error("Invalid print font origin");
  }
}

/**
 * Build concatenated @font-face CSS for the given family names.
 *
 * @param families - Family names to include (deduped, order preserved).
 * @param origin - Absolute origin prefix (e.g. "https://pos.example.com" or "").
 *                 Combined as `${origin}${path}` — path always starts with `/`.
 * @throws if any family is not in PRINT_FONT_FAMILIES.
 */
export function buildFontFaceCss(
  families: readonly string[],
  origin: string,
): string {
  validateFontOrigin(origin);
  const seen = new Set<string>();
  const unique: string[] = [];
  for (const name of families) {
    if (seen.has(name)) continue;
    seen.add(name);
    unique.push(name);
  }

  const blocks: string[] = [];
  for (const name of unique) {
    if (!Object.prototype.hasOwnProperty.call(PRINT_FONT_FAMILIES, name)) {
      throw new Error(`Unknown print font family: ${name}`);
    }
    const faces = PRINT_FONT_FAMILIES[name as PrintFontFamilyName];
    for (const face of faces) {
      const url = `${origin}${face.path}`;
      blocks.push(
        [
          "@font-face {",
          `  font-family: "${name}";`,
          `  font-style: ${face.style};`,
          `  font-weight: ${face.weight};`,
          `  src: url("${url}") format("woff2");`,
          "  font-display: block;",
          "}",
        ].join("\n"),
      );
    }
  }
  return blocks.join("\n");
}
