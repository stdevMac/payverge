import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";

import {
  PRINT_FONT_FAMILIES,
  SCRIPT_PRINT_FONT_LICENSE_PATHS,
  buildFontFaceCss,
  getPrintLanguageDirection,
  getPrintScriptFontFallback,
  type PrintFontFamilyName,
} from "./fonts";

const KNOWN_FAMILIES = Object.keys(
  PRINT_FONT_FAMILIES,
) as PrintFontFamilyName[];

describe("PRINT_FONT_FAMILIES", () => {
  it("covers the Latin and deterministic script print families", () => {
    expect(KNOWN_FAMILIES).toEqual(
      expect.arrayContaining([
        "DM Serif Display",
        "DM Sans",
        "EB Garamond",
        "Cormorant Garamond",
        "Oswald",
        "Noto Naskh Arabic",
        "Noto Sans Arabic",
        "Noto Sans Hebrew",
        "Noto Serif CJK JP",
        "Noto Sans CJK JP",
        "Noto Sans CJK KR",
        "Noto Sans CJK SC",
        "Noto Sans CJK TC",
      ]),
    );
    expect(KNOWN_FAMILIES).toHaveLength(13);
  });

  it("points DM faces at existing public paths (not /fonts/print/)", () => {
    for (const face of PRINT_FONT_FAMILIES["DM Serif Display"]) {
      expect(face.path).toMatch(/^\/fonts\/dm-serif-display\//);
      expect(face.path).not.toMatch(/\/fonts\/print\//);
    }
    for (const face of PRINT_FONT_FAMILIES["DM Sans"]) {
      expect(face.path).toMatch(/^\/fonts\/dm-sans\//);
      expect(face.path).not.toMatch(/\/fonts\/print\//);
    }
  });

  it("points new families at /fonts/print/", () => {
    for (const face of PRINT_FONT_FAMILIES["EB Garamond"]) {
      expect(face.path).toMatch(/^\/fonts\/print\/eb-garamond\//);
      expect(face.path).toMatch(/\.woff2$/);
    }
    for (const face of PRINT_FONT_FAMILIES["Cormorant Garamond"]) {
      expect(face.path).toMatch(/^\/fonts\/print\/cormorant-garamond\//);
    }
    for (const face of PRINT_FONT_FAMILIES.Oswald) {
      expect(face.path).toMatch(/^\/fonts\/print\/oswald\//);
    }
  });

  it("includes EB Garamond regular, italic, and semibold", () => {
    const faces = PRINT_FONT_FAMILIES["EB Garamond"];
    expect(faces).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ weight: 400, style: "normal" }),
        expect.objectContaining({ weight: 400, style: "italic" }),
        expect.objectContaining({ weight: 600, style: "normal" }),
      ]),
    );
  });

  it("uses exact safe public paths for every script face", () => {
    const scriptFamilies: PrintFontFamilyName[] = [
      "Noto Naskh Arabic",
      "Noto Sans Arabic",
      "Noto Sans Hebrew",
      "Noto Serif CJK JP",
      "Noto Sans CJK JP",
      "Noto Sans CJK KR",
      "Noto Sans CJK SC",
      "Noto Sans CJK TC",
    ];

    for (const family of scriptFamilies) {
      for (const face of PRINT_FONT_FAMILIES[family]) {
        expect(face.path).toMatch(
          /^\/fonts\/print\/noto-[a-z-]+\/[A-Za-z]+-Variable\.woff2$/u,
        );
        expect(face.path).not.toMatch(/(?:\.\.|[?#]|\\)/u);
      }
    }
  });

  it("ships each OFL license beside a valid WOFF2 asset", () => {
    for (const [family, licensePath] of Object.entries(
      SCRIPT_PRINT_FONT_LICENSE_PATHS,
    )) {
      const publicRoot = join(process.cwd(), "public");
      const licenseFile = join(publicRoot, licensePath);
      expect(existsSync(licenseFile)).toBe(true);
      expect(readFileSync(licenseFile, "utf8")).toContain(
        "SIL OPEN FONT LICENSE Version 1.1",
      );

      for (const face of PRINT_FONT_FAMILIES[family as PrintFontFamilyName]) {
        const fontFile = join(publicRoot, face.path);
        expect(existsSync(fontFile)).toBe(true);
        expect(readFileSync(fontFile).subarray(0, 4).toString("ascii")).toBe(
          "wOF2",
        );
      }
    }
  });
});

describe("getPrintScriptFontFallback", () => {
  it.each(["ar", "ar-JO", "fa-IR", "ur", "ckb", "en-Arab-US"])(
    "routes %s to the Arabic display and body faces",
    (language) => {
      expect(getPrintScriptFontFallback(language)).toEqual({
        script: "arabic",
        displayFamily: "Noto Naskh Arabic",
        bodyFamily: "Noto Sans Arabic",
        families: ["Noto Naskh Arabic", "Noto Sans Arabic"],
      });
    },
  );

  it.each(["ja", "ja-JP", "ja-Hira", "en-Jpan", "en-Kana"])(
    "routes %s to the self-hosted CJK JP display and body faces",
    (language) => {
      expect(getPrintScriptFontFallback(language)).toEqual({
        script: "cjk-jp",
        displayFamily: "Noto Serif CJK JP",
        bodyFamily: "Noto Sans CJK JP",
        families: ["Noto Serif CJK JP", "Noto Sans CJK JP"],
      });
    },
  );

  it.each(["ko", "ko-KR", "ko-Kore", "en-Hang"])(
    "routes %s to the self-hosted CJK KR display and body faces",
    (language) => {
      expect(getPrintScriptFontFallback(language)).toEqual({
        script: "cjk-kr",
        displayFamily: "Noto Sans CJK KR",
        bodyFamily: "Noto Sans CJK KR",
        families: ["Noto Sans CJK KR"],
      });
    },
  );

  it.each(["zh", "zh-CN", "zh-SG", "zh-Hans", "en-Hans"])(
    "routes %s to the self-hosted Simplified Chinese faces",
    (language) => {
      expect(getPrintScriptFontFallback(language)).toEqual({
        script: "cjk-sc",
        displayFamily: "Noto Sans CJK SC",
        bodyFamily: "Noto Sans CJK SC",
        families: ["Noto Sans CJK SC"],
      });
    },
  );

  it.each(["zh-TW", "zh-HK", "zh-MO", "zh-Hant", "en-Hant"])(
    "routes %s to the self-hosted Traditional Chinese faces",
    (language) => {
      expect(getPrintScriptFontFallback(language)).toEqual({
        script: "cjk-tc",
        displayFamily: "Noto Sans CJK TC",
        bodyFamily: "Noto Sans CJK TC",
        families: ["Noto Sans CJK TC"],
      });
    },
  );

  it.each([
    ["ja-Hani", "cjk-jp"],
    ["ko-Hani", "cjk-kr"],
    ["zh-Hani-TW", "cjk-tc"],
    ["en-Hani", "cjk-sc"],
  ])("resolves generic Han tag %s deterministically", (language, script) => {
    expect(getPrintScriptFontFallback(language)?.script).toBe(script);
  });

  it.each([
    ["ja-Hans", "cjk-sc"],
    ["ko-Hant", "cjk-tc"],
    ["zh-Jpan", "cjk-jp"],
    ["ar-Latn", undefined],
  ])("lets explicit script override language in %s", (language, script) => {
    expect(getPrintScriptFontFallback(language)?.script).toBe(script);
  });

  it.each([undefined, null, "", "en", "es-AR", "ar-Latn", "not_a_locale"])(
    "does not misroute %s to an unrelated script face",
    (language) => {
      expect(getPrintScriptFontFallback(language)).toBeNull();
    },
  );

  it.each(["he", "he-IL", "he-Hebr", "yi-Hebr"])(
    "uses the self-hosted Hebrew fallback for %s",
    (language) => {
      const fallback = getPrintScriptFontFallback(language);
      expect(fallback).toMatchObject({
        script: "hebrew",
        displayFamily: "Noto Sans Hebrew",
        bodyFamily: "Noto Sans Hebrew",
      });
      expect(fallback?.families).toEqual(["Noto Sans Hebrew"]);
    },
  );

  it("provides the complete family list expected by buildFontFaceCss", () => {
    const fallback = getPrintScriptFontFallback("ja-JP");
    expect(fallback).not.toBeNull();
    const css = buildFontFaceCss(
      [...(fallback?.families ?? [])],
      "https://payverge.io",
    );
    expect(css).toContain('font-family: "Noto Serif CJK JP";');
    expect(css).toContain('font-family: "Noto Sans CJK JP";');
    expect(css).toContain(
      'url("https://payverge.io/fonts/print/noto-serif-cjk-jp/NotoSerifCJKjp-Variable.woff2")',
    );
  });
});

describe("getPrintLanguageDirection", () => {
  it.each(["ar", "fa-IR", "ur", "ckb", "he", "en-Arab", "en-Hebr"])(
    "resolves %s as RTL",
    (language) => {
      expect(getPrintLanguageDirection(language)).toBe("rtl");
    },
  );

  it.each([
    "en",
    "ja",
    "ko",
    "zh-Hant",
    "ar-Latn",
    "fa-Latn",
    "ur-Latn",
    "",
    "not_a_locale",
  ])("resolves %s as LTR", (language) => {
    expect(getPrintLanguageDirection(language)).toBe("ltr");
  });
});

describe("buildFontFaceCss", () => {
  it("builds @font-face rules for known families", () => {
    const css = buildFontFaceCss(["EB Garamond"], "https://example.com");
    expect(css).toContain("@font-face {");
    expect(css).toContain('font-family: "EB Garamond";');
    expect(css).toContain("font-style: normal;");
    expect(css).toContain("font-style: italic;");
    expect(css).toContain("font-weight: 400;");
    expect(css).toContain("font-weight: 600;");
    expect(css).toContain('format("woff2")');
    expect(css).toContain("font-display: block;");
  });

  it("composes absolute URLs from origin + path", () => {
    const css = buildFontFaceCss(["Oswald"], "https://payverge.io");
    expect(css).toContain(
      'src: url("https://payverge.io/fonts/print/oswald/Oswald-SemiBold.woff2") format("woff2");',
    );
  });

  it("supports empty origin (relative public paths)", () => {
    const css = buildFontFaceCss(["DM Sans"], "");
    expect(css).toContain(
      'src: url("/fonts/dm-sans/dm-sans-latin.woff2") format("woff2");',
    );
  });

  it("throws on unknown family", () => {
    expect(() => buildFontFaceCss(["Comic Sans"], "https://x")).toThrow(
      /Unknown print font family: Comic Sans/,
    );
  });

  it.each([
    "javascript:alert(1)",
    "https://example.com/",
    'https://example.com\");}body{color:red}/*',
    "https://user:secret@example.com",
  ])("rejects unsafe font origin %s", (origin) => {
    expect(() => buildFontFaceCss(["Oswald"], origin)).toThrow(
      /Invalid print font origin/,
    );
  });

  it("dedupes repeated families", () => {
    const once = buildFontFaceCss(["Oswald"], "https://x");
    const twice = buildFontFaceCss(["Oswald", "Oswald", "Oswald"], "https://x");
    expect(twice).toBe(once);
    expect((twice.match(/@font-face/g) || []).length).toBe(1);
  });

  it("emits rules for multiple distinct families", () => {
    const css = buildFontFaceCss(["DM Sans", "EB Garamond"], "https://host");
    expect(css).toContain('font-family: "DM Sans";');
    expect(css).toContain('font-family: "EB Garamond";');
    const count = (css.match(/@font-face/g) || []).length;
    expect(count).toBe(
      PRINT_FONT_FAMILIES["DM Sans"].length +
        PRINT_FONT_FAMILIES["EB Garamond"].length,
    );
  });

  it("output contains no undefined/NaN", () => {
    const css = buildFontFaceCss(KNOWN_FAMILIES, "https://example.com");
    expect(css).not.toMatch(/undefined/i);
    expect(css).not.toMatch(/NaN/);
    expect(css.length).toBeGreaterThan(0);
  });

  it("preserves first-seen order when deduping", () => {
    const css = buildFontFaceCss(
      ["Oswald", "EB Garamond", "Oswald"],
      "https://x",
    );
    const oswaldIdx = css.indexOf('font-family: "Oswald"');
    const ebIdx = css.indexOf('font-family: "EB Garamond"');
    expect(oswaldIdx).toBeGreaterThanOrEqual(0);
    expect(ebIdx).toBeGreaterThan(oswaldIdx);
  });
});
