"use client";

import React from "react";
import { SkeletonLine } from "@/components/ui/skeletons";
import { useSimpleLocale, getTranslation } from "@/i18n/SimpleTranslationProvider";

export function CRMLoyaltySkeleton() {
  const { locale } = useSimpleLocale();
  return (
    <div
      className="space-y-6 max-w-2xl animate-pulse"
      role="status"
      aria-busy="true"
    >
      <span className="sr-only">
        {getTranslation("common.loadingLoyalty", locale) as string}
      </span>
      <div className="flex items-center gap-2">
        <SkeletonLine width="1.25rem" height="1.25rem" className="rounded" />
        <SkeletonLine width="12rem" height="0.875rem" />
      </div>
      <div className="space-y-1">
        <SkeletonLine width="16rem" height="0.75rem" />
        <SkeletonLine width="8rem" height="2.5rem" className="rounded-lg" />
      </div>
      <div className="space-y-2">
        <SkeletonLine width="10rem" height="1rem" />
        {Array.from({ length: 3 }, (_, i) => (
          <div key={i} className="flex items-center gap-2">
            <SkeletonLine width="100%" height="2.5rem" className="rounded-lg flex-1" />
            <SkeletonLine width="8rem" height="2.5rem" className="rounded-lg" />
            <SkeletonLine width="2.5rem" height="2.5rem" className="rounded" />
          </div>
        ))}
      </div>
      <SkeletonLine width="10rem" height="2.5rem" className="rounded-lg" />
    </div>
  );
}
