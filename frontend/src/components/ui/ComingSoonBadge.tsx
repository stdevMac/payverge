"use client";

import { useSimpleLocale, getTranslation } from "@/i18n/SimpleTranslationProvider";

interface ComingSoonBadgeProps {
  className?: string;
}

export function ComingSoonBadge({ className = "" }: ComingSoonBadgeProps) {
  const { locale } = useSimpleLocale();
  const label = getTranslation("common.comingSoonBadge", locale) as string;
  return (
    <span
      className={
        "inline-flex items-center gap-1 rounded-full border border-brand/40 bg-brand/10 " +
        "px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wider text-brand-700 " +
        className
      }
      aria-label={label}
    >
      <span aria-hidden className="h-1.5 w-1.5 rounded-full bg-brand" />
      {label}
    </span>
  );
}
