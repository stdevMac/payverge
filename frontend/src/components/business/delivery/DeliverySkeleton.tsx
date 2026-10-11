"use client";

import React from "react";
import { SkeletonCard } from "@/components/ui/skeletons";
import { useSimpleLocale, getTranslation } from "@/i18n/SimpleTranslationProvider";

export function DeliverySkeleton() {
  const { locale } = useSimpleLocale();
  return (
    <div className="space-y-6 animate-pulse" role="status" aria-busy="true">
      <span className="sr-only">
        {getTranslation("common.loadingDelivery", locale) as string}
      </span>
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {Array.from({ length: 6 }, (_, i) => (
          <SkeletonCard key={i} lines={3} hasHeader />
        ))}
      </div>
    </div>
  );
}
