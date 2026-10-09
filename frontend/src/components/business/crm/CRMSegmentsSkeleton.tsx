"use client";

import React from "react";
import { SkeletonCard } from "@/components/ui/skeletons";
import { useSimpleLocale, getTranslation } from "@/i18n/SimpleTranslationProvider";

export function CRMSegmentsSkeleton() {
  const { locale } = useSimpleLocale();
  return (
    <div className="animate-pulse" role="status" aria-busy="true">
      <span className="sr-only">
        {getTranslation("common.loadingSegments", locale) as string}
      </span>
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
        <SkeletonCard lines={2} hasHeader />
        <SkeletonCard lines={2} hasHeader />
        <SkeletonCard lines={2} hasHeader />
        <SkeletonCard lines={2} hasHeader />
      </div>
    </div>
  );
}
