"use client";

/**
 * Shared decimal text input — string state, parse/clamp/step only on blur.
 *
 * Root cause of L3-17 / L5-13 / L5-33: holding numeric React state and
 * rendering `value={String(n)}` while parsing on every keystroke destroys the
 * decimal separator the instant it is typed (`"12."` → 12 → `"12"`).
 *
 * Rules (brief §3.0):
 * 1. Display state is the raw string — never String(number) round-trip on change.
 * 2. Parse on blur/submit only (tryParseLocaleDecimal → null for garbage).
 * 3. Clamp and step-snap only on blur.
 * 4. Parent owns error copy; this component renders the error slot.
 */

import React, { useCallback } from "react";
import { Input, type InputProps } from "@nextui-org/react";
import {
  tryParseLocaleDecimal,
  type ParseLocaleDecimalOptions,
} from "@/lib/parseLocaleDecimal";

export type DecimalBlurOptions = {
  min?: number;
  max?: number;
  step?: number;
  maxFractionDigits?: number;
  /** When true, rewrite the display string to a canonical form after blur. Default true. */
  normalizeDisplay?: boolean;
};

export type DecimalBlurResult =
  | { ok: true; value: number; text: string }
  | { ok: false; value: null; text: string };

/**
 * Pure blur-boundary processing. Safe to unit-test without mounting Input.
 * - Invalid / empty → ok:false, text left as typed (no silent 0).
 * - Valid → clamp to [min,max], step-snap from base=min??0, optional reformat.
 */
export function applyDecimalBlur(
  raw: string,
  opts: DecimalBlurOptions = {},
): DecimalBlurResult {
  const parseOpts: ParseLocaleDecimalOptions | undefined =
    opts.maxFractionDigits !== undefined
      ? { maxFractionDigits: opts.maxFractionDigits }
      : undefined;
  const parsed = tryParseLocaleDecimal(raw, parseOpts);
  if (parsed === null) {
    return { ok: false, value: null, text: raw };
  }

  let next = parsed;
  if (opts.min !== undefined) next = Math.max(opts.min, next);
  if (opts.max !== undefined) next = Math.min(opts.max, next);
  if (opts.step !== undefined && opts.step > 0) {
    const base = opts.min ?? 0;
    next = base + Math.round((next - base) / opts.step) * opts.step;
    if (opts.min !== undefined) next = Math.max(opts.min, next);
    if (opts.max !== undefined) next = Math.min(opts.max, next);
  }

  // Keep fractional precision stable for money-ish defaults.
  const places =
    opts.maxFractionDigits !== undefined
      ? Math.max(0, Math.floor(opts.maxFractionDigits))
      : 2;
  if (places >= 0) {
    const f = 10 ** places;
    next = Math.round(next * f) / f;
  }

  const normalize = opts.normalizeDisplay !== false;
  // Prefer the separator the user typed when rewriting (comma vs dot).
  const usedComma = raw.includes(",") && !raw.includes(".");
  let text: string;
  if (!normalize) {
    text = raw;
  } else if (Number.isInteger(next) && places === 0) {
    text = String(next);
  } else if (places === 0) {
    text = String(Math.round(next));
  } else {
    // Strip trailing zeros after the decimal for cleaner UX (12.50 → "12.5"
    // would lose cents intention for money — keep fixed places when money).
    const fixed = next.toFixed(places);
    const trimmed = fixed.replace(/\.?0+$/, "");
    // For money (default places=2) keep at least one digit after decimal if
    // original had a separator; otherwise use trimmed integer when whole.
    if (raw.includes(".") || raw.includes(",")) {
      text = usedComma ? fixed.replace(".", ",") : fixed;
    } else {
      text = usedComma ? trimmed.replace(".", ",") : trimmed;
    }
  }

  return { ok: true, value: next, text };
}

export type DecimalInputProps = Omit<
  InputProps,
  "type" | "value" | "onValueChange" | "onChange" | "inputMode"
> & {
  /** Controlled raw text. Must be a string — never feed String(number) from numeric state mid-keystroke. */
  value: string;
  /** Fires on every keystroke with the raw string only (no parse). */
  onValueChange: (text: string) => void;
  /**
   * Fires on blur after parse/clamp/step.
   * `null` means empty or unparseable (use for validation UX).
   */
  onParsedChange?: (value: number | null) => void;
  min?: number;
  max?: number;
  step?: number;
  maxFractionDigits?: number;
  /** When false, blur does not rewrite the displayed string. Default true. */
  normalizeOnBlur?: boolean;
  /**
   * inputMode: "decimal" (default) for money/qty with fractions;
   * "numeric" for integer-ish fields (break minutes).
   */
  inputMode?: "decimal" | "numeric" | "text";
};

/**
 * NextUI Input wrapper that never round-trips through a number on change.
 */
export function DecimalInput({
  value,
  onValueChange,
  onParsedChange,
  min,
  max,
  step,
  maxFractionDigits,
  normalizeOnBlur = true,
  inputMode = "decimal",
  onBlur,
  ...rest
}: DecimalInputProps) {
  const handleBlur = useCallback(
    (e: React.FocusEvent<HTMLInputElement>) => {
      const result = applyDecimalBlur(value, {
        min,
        max,
        step,
        maxFractionDigits,
        normalizeDisplay: normalizeOnBlur,
      });
      if (result.ok) {
        if (normalizeOnBlur && result.text !== value) {
          onValueChange(result.text);
        }
        onParsedChange?.(result.value);
      } else {
        onParsedChange?.(null);
      }
      onBlur?.(e);
    },
    [
      value,
      min,
      max,
      step,
      maxFractionDigits,
      normalizeOnBlur,
      onValueChange,
      onParsedChange,
      onBlur,
    ],
  );

  return (
    <Input
      {...rest}
      type="text"
      inputMode={inputMode}
      value={value}
      onValueChange={onValueChange}
      onBlur={handleBlur}
    />
  );
}
