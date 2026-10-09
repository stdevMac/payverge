/**
 * Mirrors backend/internal/database.NormalizeLoyaltyTierName:
 * strings.TrimSpace followed by strings.ToLower's simple Unicode mapping.
 *
 * JavaScript's native trim and whole-string lowercase are intentionally not
 * used: ECMAScript omits Go's U+0085 whitespace, trims FEFF (which Go keeps),
 * applies contextual Greek sigma casing, and expands U+0130. This explicit
 * compatibility implementation keeps the UI from rejecting a name the API
 * accepts, or accepting a duplicate the API rejects.
 */
function isGoUnicodeSpace(codePoint: number): boolean {
  return (
    (codePoint >= 0x0009 && codePoint <= 0x000d) ||
    codePoint === 0x0020 ||
    codePoint === 0x0085 ||
    codePoint === 0x00a0 ||
    codePoint === 0x1680 ||
    (codePoint >= 0x2000 && codePoint <= 0x200a) ||
    codePoint === 0x2028 ||
    codePoint === 0x2029 ||
    codePoint === 0x202f ||
    codePoint === 0x205f ||
    codePoint === 0x3000
  );
}

function trimGoUnicodeSpace(value: string): string {
  const codePoints = Array.from(value);
  let start = 0;
  let end = codePoints.length;

  while (
    start < end &&
    isGoUnicodeSpace(codePoints[start].codePointAt(0) ?? 0)
  ) {
    start += 1;
  }
  while (
    end > start &&
    isGoUnicodeSpace(codePoints[end - 1].codePointAt(0) ?? 0)
  ) {
    end -= 1;
  }

  return codePoints.slice(start, end).join("");
}

function toGoSimpleLowercase(value: string): string {
  return Array.from(value)
    .map((codePoint) =>
      // Go's unicode.ToLower maps U+0130 to one rune; JS's default mapping
      // expands it to "i" + combining dot.
      codePoint === "İ" ? "i" : codePoint.toLowerCase(),
    )
    .join("");
}

export function normalizeLoyaltyTierName(value: string): string {
  return toGoSimpleLowercase(trimGoUnicodeSpace(value));
}
