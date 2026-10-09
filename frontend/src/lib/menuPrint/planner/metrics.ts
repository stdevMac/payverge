export const PT_TO_MM = 25.4 / 72;

export interface TextBoxInput {
  text: string;
  widthMm: number;
  fontPt: number;
  floorPt?: number;
  lineHeight: number;
  glyphWidthFactor?: number;
}

export interface TextBoxEstimate {
  lines: number;
  heightMm: number;
}

const roundPublic = (value: number): number => Number(value.toFixed(2));

function wrappedLineCount(text: string, charsPerLine: number): number {
  const normalized = text.trim();
  if (!normalized) return 1;
  const words = normalized.split(/\s+/);
  let lines = 1;
  let used = 0;
  for (const word of words) {
    const length = [...word].length;
    if (length > charsPerLine) {
      if (used > 0) {
        lines += 1;
        used = 0;
      }
      const fullLines = Math.floor(length / charsPerLine);
      const remainder = length % charsPerLine;
      lines += remainder === 0 ? fullLines - 1 : fullLines;
      used = remainder === 0 ? charsPerLine : remainder;
      continue;
    }
    const next = used === 0 ? length : used + 1 + length;
    if (next > charsPerLine) {
      lines += 1;
      used = length;
    } else {
      used = next;
    }
  }
  return lines;
}

export function estimateTextBox(input: TextBoxInput): TextBoxEstimate {
  const validatePositive = (value: number, name: string): void => {
    if (!Number.isFinite(value) || value <= 0) {
      throw new Error(`${name} must be finite and positive`);
    }
  };
  validatePositive(input.widthMm, "widthMm");
  validatePositive(input.fontPt, "fontPt");
  validatePositive(input.lineHeight, "lineHeight");
  if (input.floorPt !== undefined) validatePositive(input.floorPt, "floorPt");
  const glyphWidthFactor = input.glyphWidthFactor ?? 0.52;
  validatePositive(glyphWidthFactor, "glyphWidthFactor");
  if (input.floorPt !== undefined && input.fontPt < input.floorPt) {
    throw new Error("below family type floor");
  }
  const charsPerLine = Math.max(
    1,
    Math.floor(input.widthMm / (input.fontPt * PT_TO_MM * glyphWidthFactor)),
  );
  const lines = wrappedLineCount(input.text, charsPerLine);
  return {
    lines,
    heightMm: roundPublic(lines * input.fontPt * PT_TO_MM * input.lineHeight),
  };
}
