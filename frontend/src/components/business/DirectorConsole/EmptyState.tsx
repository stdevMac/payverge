"use client";

import React from "react";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";
import SageMark from "./SageMark";

interface EmptyStateProps {
  suggestions: string[];
  onPick: (suggestion: string) => void;
}

/**
 * Empty state for a fresh Director's Console thread.
 *
 * Replaces the older dashed-border placeholder. The framing is an
 * invitation: an icon, an open-ended prompt, and clickable suggestion
 * chips that kick off the first message.
 */
export default function EmptyState({ suggestions, onPick }: EmptyStateProps) {
  const { locale } = useSimpleLocale();
  const t = (key: string): string => {
    const result = getTranslation(`directorConsole.emptyState.${key}`, locale);
    return Array.isArray(result) ? result[0] || key : (result as string);
  };

  return (
    <div className="flex flex-col items-center text-center py-12">
      <SageMark size="lg" variant="soft" className="mb-4" />
      <h3 className="font-title text-xl text-ink-900 mb-2">
        {t("title")}
      </h3>
      <p className="text-ink-600 mb-4">{t("description")}</p>
      <div className="flex flex-wrap gap-2 justify-center max-w-xl">
        {suggestions.map((suggestion) => (
          <button
            key={suggestion}
            type="button"
            onClick={() => onPick(suggestion)}
            className="px-3 py-1.5 rounded-full border border-warm-200 text-sm text-ink-800 hover:bg-warm-50"
          >
            {suggestion}
          </button>
        ))}
      </div>
    </div>
  );
}
