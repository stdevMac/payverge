"use client";

import React from "react";
import { SkeletonCard, SkeletonLine } from "@/components/ui/skeletons";
import { useSimpleLocale, getTranslation } from "@/i18n/SimpleTranslationProvider";

export function KitchenSkeleton() {
  const { locale } = useSimpleLocale();
  return (
    <div className="animate-pulse space-y-6" role="status" aria-busy="true">
      <span className="sr-only">
        {getTranslation("common.loadingKitchen", locale) as string}
      </span>
      {/* Section heading + order grid only — the page title and subtitle stay
          mounted on the shell's PageHeader while orders load (S-9). */}
      <div className="space-y-2">
        <SkeletonLine width="10rem" height="1.25rem" />
        <SkeletonLine width="16rem" height="0.875rem" />
      </div>
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
        {Array.from({ length: 6 }, (_, i) => (
          <SkeletonCard key={i} lines={4} hasHeader />
        ))}
      </div>
    </div>
  );
}
