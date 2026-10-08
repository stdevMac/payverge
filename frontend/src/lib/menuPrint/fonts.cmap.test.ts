import { existsSync } from "node:fs";
import { join } from "node:path";

import { openSync, type Font } from "fontkit";

import {
  cjkRestaurantMenu,
  rtlRestaurantMenu,
} from "./__fixtures__/restaurantMenus";
import { PRINT_FONT_FAMILIES, type PrintFontFamilyName } from "./fonts";

function collectNonAsciiCharacters(value: unknown): string {
  const strings: string[] = [];
  const visit = (candidate: unknown): void => {
    if (typeof candidate === "string") {
      strings.push(candidate);
      return;
    }
    if (Array.isArray(candidate)) {
      candidate.forEach(visit);
      return;
    }
    if (candidate && typeof candidate === "object") {
      Object.values(candidate).forEach(visit);
    }
  };
  visit(value);
  return [...new Set(strings.join("").split(""))]
    .filter((character) => (character.codePointAt(0) ?? 0) > 0x7f)
    .join("");
}

const ARABIC_FIXTURE_PROBE = collectNonAsciiCharacters(rtlRestaurantMenu);
const JAPANESE_FIXTURE_PROBE = collectNonAsciiCharacters(cjkRestaurantMenu);

const CMAP_CASES: ReadonlyArray<{
  family: PrintFontFamilyName;
  internalFamily: RegExp;
  probe: string;
  weightRange: readonly [number, number];
}> = [
  {
    family: "Noto Naskh Arabic",
    internalFamily: /^Noto Naskh Arabic/u,
    probe: `${ARABIC_FIXTURE_PROBE}منوی رستوران پ چ ژ گ ی ٹ ڈ ڑ ں ھ ے`,
    weightRange: [400, 700],
  },
  {
    family: "Noto Sans Arabic",
    internalFamily: /^Noto Sans Arabic/u,
    probe: `${ARABIC_FIXTURE_PROBE}منوی رستوران پ چ ژ گ ی ٹ ڈ ڑ ں ھ ے`,
    weightRange: [100, 900],
  },
  {
    family: "Noto Sans Hebrew",
    internalFamily: /^Noto Sans Hebrew/u,
    probe: "תפריט מסעדה מנות ראשונות עיקריות קינוחים ₪",
    weightRange: [100, 900],
  },
  {
    family: "Noto Serif CJK JP",
    internalFamily: /^Noto Serif JP/u,
    probe: `${JAPANESE_FIXTURE_PROBE}日本語メニュー`,
    weightRange: [200, 900],
  },
  {
    family: "Noto Sans CJK JP",
    internalFamily: /^Noto Sans JP/u,
    probe: `${JAPANESE_FIXTURE_PROBE}日本語メニュー`,
    weightRange: [100, 900],
  },
  {
    family: "Noto Sans CJK KR",
    internalFamily: /^Noto Sans KR/u,
    probe: "한글메뉴불고기비빔밥식당",
    weightRange: [100, 900],
  },
  {
    family: "Noto Sans CJK SC",
    internalFamily: /^Noto Sans SC/u,
    probe: "菜单单龙马面餐厅招牌菜",
    weightRange: [100, 900],
  },
  {
    family: "Noto Sans CJK TC",
    internalFamily: /^Noto Sans TC/u,
    probe: "菜單單龍馬麵餐廳招牌菜",
    weightRange: [100, 900],
  },
];

describe("script print font cmap coverage", () => {
  it.each(CMAP_CASES)(
    "$family covers its fixture and locale-specific probes",
    ({ family, internalFamily, probe, weightRange }) => {
      const faces = PRINT_FONT_FAMILIES[family];
      expect(faces).toHaveLength(1);
      const fontFile = join(process.cwd(), "public", faces[0].path);
      expect(existsSync(fontFile)).toBe(true);

      const font = openSync(fontFile) as Font;
      expect(font.type).toBe("WOFF2");
      expect(font.familyName).toMatch(internalFamily);
      expect(font.variationAxes.wght).toMatchObject({
        min: weightRange[0],
        max: weightRange[1],
      });

      const missing = [...new Set(probe.split(""))].filter(
        (character) =>
          !font.hasGlyphForCodePoint(character.codePointAt(0) ?? 0),
      );
      expect(missing).toEqual([]);
    },
  );
});
