"use client";

import React from "react";
import { Tooltip } from "@nextui-org/react";
import { Check } from "lucide-react";
import { ALLERGENS, DIETARY_TAGS } from "../../constants/menu-tags";
import Image from "next/image";
import {
  getTranslation,
  useSimpleLocale,
} from "../../i18n/SimpleTranslationProvider";

interface TagSelectorProps {
  selectedTags: string[];
  onToggleTag: (tagId: string) => void;
  type: "allergens" | "dietary";
  label?: string;
  className?: string;
}

/**
 * Unified tag selector. Allergens and dietary tags both render as
 * labeled pills (icon + name) so operators can scan without hovering
 * for tooltips. Selected state uses tone-coded borders + tinted bg.
 *
 * Was previously two different visual styles: 40×40 icon-only buttons
 * for allergens vs labeled pills for dietary, plus a redundant
 * "selected" chip strip below. The strip is gone — selection is
 * already obvious from the highlighted state in the row above.
 */
export default function TagSelector({
  selectedTags,
  onToggleTag,
  type,
  label,
  className = "",
}: TagSelectorProps) {
  const { locale } = useSimpleLocale();
  const tags = type === "allergens" ? ALLERGENS : DIETARY_TAGS;

  const t = (key: string) => {
    const fullKey = `businessDashboard.dashboard.menuBuilder.items.${key}`;
    const result = getTranslation(fullKey, locale);
    return Array.isArray(result) ? result[0] || key : (result as string);
  };

  return (
    <div className={`space-y-2 ${className}`}>
      {label && (
        <label className="block text-xs font-medium text-ink-600">
          {label}
        </label>
      )}
      <div className="flex flex-wrap gap-1.5">
        {tags.map((tag) => {
          const isSelected = selectedTags.includes(tag.id);

          const baseClasses =
            "relative inline-flex h-8 items-center gap-1.5 rounded-full border pl-2 pr-3 text-xs font-medium shadow-sm transition-all duration-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand active:scale-[0.98]";

          if (type === "allergens") {
            const allergen = tag as (typeof ALLERGENS)[0];
            return (
              <Tooltip
                key={tag.id}
                content={t(allergen.name)}
                delay={300}
                closeDelay={0}
              >
                <button
                  type="button"
                  onClick={() => onToggleTag(tag.id)}
                  className={`${baseClasses} ${
                    isSelected
                      ? "border-amber-300 bg-amber-50 text-amber-900 shadow-amber-200/50"
                      : "border-warm-200 bg-white text-ink-600 shadow-warm-900/5 hover:border-brand/40 hover:bg-warm-50"
                  }`}
                  aria-pressed={isSelected}
                >
                  <span
                    className={`relative w-4 h-4 ${
                      !isSelected ? "opacity-60 grayscale" : ""
                    }`}
                  >
                    <Image
                      src={allergen.icon}
                      alt={t(allergen.name)}
                      width={16}
                      height={16}
                      className="w-full h-full object-contain"
                    />
                  </span>
                  <span>{t(allergen.name)}</span>
                  {isSelected && <Check className="w-3 h-3 text-amber-700" />}
                </button>
              </Tooltip>
            );
          }

          const dietary = tag as (typeof DIETARY_TAGS)[0];
          return (
            <Tooltip
              key={tag.id}
              content={t(dietary.description)}
              delay={300}
              closeDelay={0}
            >
              <button
                type="button"
                onClick={() => onToggleTag(tag.id)}
                className={`${baseClasses} ${
                  isSelected
                    ? "border-emerald-300 bg-emerald-50 text-emerald-900 shadow-emerald-200/50"
                    : "border-warm-200 bg-white text-ink-600 shadow-warm-900/5 hover:border-brand/40 hover:bg-warm-50"
                }`}
                aria-pressed={isSelected}
              >
                <span className="text-sm leading-none">{dietary.emoji}</span>
                <span>{t(dietary.name)}</span>
                {isSelected && <Check className="w-3 h-3 text-emerald-700" />}
              </button>
            </Tooltip>
          );
        })}
      </div>
    </div>
  );
}
