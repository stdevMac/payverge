"use client";

import React from "react";

import {
  MENU_OUTPUT_FORMATS,
  PAPER_DIMENSIONS_MM,
  type MenuOutputFormat,
  type PaperFormat,
  type PrintMenuSection,
  type RegisteredMenuDesignFamily,
} from "@/lib/menuPrint/types";

const FORMAT_KEY_PARTS: Record<MenuOutputFormat, string> = {
  "single-sheet": "singleSheet",
  "two-page-spread": "twoPageSpread",
  "folded-booklet": "foldedBooklet",
  "takeaway-trifold": "takeawayTrifold",
  "drinks-card": "drinksCard",
  "counter-menu": "counterMenu",
};

const PAPER_KEY_PARTS: Record<PaperFormat, string> = {
  letter: "letter",
  a4: "a4",
  "half-letter": "halfLetter",
  a5: "a5",
};

const PAPER_CHOICES = Object.keys(PAPER_DIMENSIONS_MM) as PaperFormat[];

interface PrintLanguageChoice {
  value: string;
  label?: string;
}

export interface PrintShapeControlsProps {
  family: RegisteredMenuDesignFamily;
  outputFormat: MenuOutputFormat;
  paperFormat: PaperFormat;
  language: string;
  categories: readonly PrintMenuSection[];
  selectedCategoryIds: readonly string[] | "all";
  pageEstimates: Partial<Record<MenuOutputFormat, number>>;
  languageChoices?: readonly PrintLanguageChoice[];
  tString: (key: string) => string;
  onFormatChange: (format: MenuOutputFormat) => void;
  onPaperChange: (paper: PaperFormat) => void;
  onLanguageChange: (language: string) => void;
  onCategoryChange: (categoryIds: string[] | "all") => void;
}

function choiceClasses(active: boolean, disabled = false): string {
  if (disabled) {
    return "cursor-not-allowed border-warm-200 bg-warm-100 text-ink-500";
  }
  return active
    ? "border-brand bg-brand-50 text-brand-800 ring-1 ring-brand/20"
    : "border-warm-200 bg-white text-ink-800 hover:border-warm-300 hover:bg-warm-50";
}

export function PrintShapeControls({
  family,
  outputFormat,
  paperFormat,
  language,
  categories,
  selectedCategoryIds,
  pageEstimates,
  languageChoices,
  tString,
  onFormatChange,
  onPaperChange,
  onLanguageChange,
  onCategoryChange,
}: PrintShapeControlsProps) {
  const supportedFormats = new Set(family.supportedFormats);
  const hasUnavailableFormats = MENU_OUTPUT_FORMATS.some(
    (format) => !supportedFormats.has(format),
  );
  const languages = languageChoices?.length
    ? [...languageChoices]
    : [{ value: "en" }, { value: "es" }, { value: "es-ar" }];
  if (!languages.some(({ value }) => value === language)) {
    languages.unshift({ value: language });
  }
  const selectedIds =
    selectedCategoryIds === "all"
      ? categories.map(({ id }) => id)
      : [...selectedCategoryIds];

  const toggleCategory = (categoryId: string) => {
    if (selectedIds.length === 1 && selectedIds[0] === categoryId) return;
    const nextIds = selectedIds.includes(categoryId)
      ? selectedIds.filter((id) => id !== categoryId)
      : categories
          .map(({ id }) => id)
          .filter((id) => selectedIds.includes(id) || id === categoryId);
    onCategoryChange(nextIds.length === categories.length ? "all" : nextIds);
  };

  return (
    <div className="space-y-8 text-ink-950">
      <fieldset className="space-y-3">
        <legend className="text-heading-sm text-ink-950">
          {tString("print.format.label")}
        </legend>
        <p className="max-w-2xl text-sm leading-relaxed text-ink-600">
          {tString("print.format.help")}
        </p>
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {MENU_OUTPUT_FORMATS.map((format) => {
            const supported = supportedFormats.has(format);
            const selected = outputFormat === format;
            const estimate = pageEstimates[format];
            return (
              <button
                key={format}
                data-testid="print-format-card"
                type="button"
                aria-label={tString(`print.format.${FORMAT_KEY_PARTS[format]}`)}
                aria-pressed={selected}
                aria-describedby={
                  supported ? undefined : "print-format-compatibility"
                }
                disabled={!supported}
                onClick={() => onFormatChange(format)}
                className={[
                  "flex min-h-28 flex-col items-start justify-between rounded-xl border p-4 text-left",
                  "transition-[border-color,background-color,transform] active:translate-y-px",
                  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-2",
                  choiceClasses(selected, !supported),
                ].join(" ")}
              >
                <span className="text-sm font-semibold">
                  {tString(`print.format.${FORMAT_KEY_PARTS[format]}`)}
                </span>
                {supported && typeof estimate === "number" ? (
                  <span className="text-xs font-medium text-ink-600">
                    {estimate} {tString("print.pageEstimate.label")}
                  </span>
                ) : (
                  <span className="text-xs text-ink-500">
                    {supported
                      ? tString("print.pageEstimate.pending")
                      : tString("print.compatibility.unavailable")}
                  </span>
                )}
              </button>
            );
          })}
        </div>
        {hasUnavailableFormats ? (
          <p
            id="print-format-compatibility"
            className="rounded-lg border border-warm-200 bg-warm-50 px-3 py-2 text-sm text-ink-700"
          >
            {tString("print.compatibility.formatUnavailable")}
          </p>
        ) : null}
      </fieldset>

      <fieldset className="space-y-3">
        <legend className="text-heading-sm text-ink-950">
          {tString("print.paper.label")}
        </legend>
        <div className="flex flex-wrap gap-2">
          {PAPER_CHOICES.map((paper) => (
            <button
              key={paper}
              type="button"
              aria-label={tString(`print.paper.${PAPER_KEY_PARTS[paper]}`)}
              aria-pressed={paperFormat === paper}
              onClick={() => onPaperChange(paper)}
              className={[
                "min-h-10 rounded-full border px-4 py-2 text-sm font-medium transition-colors",
                "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-2",
                choiceClasses(paperFormat === paper),
              ].join(" ")}
            >
              {tString(`print.paper.${PAPER_KEY_PARTS[paper]}`)}
            </button>
          ))}
        </div>
      </fieldset>

      <fieldset className="space-y-3">
        <legend className="text-heading-sm text-ink-950">
          {tString("print.language.label")}
        </legend>
        <div className="flex flex-wrap gap-2">
          {languages.map(({ value, label }) => (
            <button
              key={value}
              type="button"
              aria-label={label ?? tString(`print.language.${value}`)}
              aria-pressed={language === value}
              onClick={() => onLanguageChange(value)}
              className={[
                "min-h-10 rounded-full border px-4 py-2 text-sm font-medium transition-colors",
                "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-2",
                choiceClasses(language === value),
              ].join(" ")}
            >
              {label ?? tString(`print.language.${value}`)}
            </button>
          ))}
        </div>
      </fieldset>

      <fieldset className="space-y-3">
        <legend className="text-heading-sm text-ink-950">
          {tString("print.categories.label")}
        </legend>
        <p className="text-sm text-ink-600">
          {tString("print.categories.help")}
        </p>
        <div className="grid gap-2 sm:grid-cols-2">
          {categories.map((category) => {
            const selected = selectedIds.includes(category.id);
            const isLastSelected = selected && selectedIds.length === 1;
            return (
              <label
                key={category.id}
                className={[
                  "flex min-h-12 items-center justify-between gap-4 rounded-lg border border-warm-200 px-4 py-3 text-sm font-medium",
                  isLastSelected
                    ? "cursor-not-allowed bg-warm-50 text-ink-500"
                    : "cursor-pointer bg-white text-ink-800 hover:bg-warm-50",
                ].join(" ")}
              >
                <span>{category.name}</span>
                <input
                  type="checkbox"
                  checked={selected}
                  disabled={isLastSelected}
                  onChange={() => toggleCategory(category.id)}
                  className="h-4 w-4 rounded border-warm-300 text-brand focus:ring-brand/40"
                />
              </label>
            );
          })}
        </div>
      </fieldset>
    </div>
  );
}
