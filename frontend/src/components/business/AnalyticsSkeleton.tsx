"use client";

import React from "react";
import { SkeletonKPI, SkeletonChart } from "@/components/ui/skeletons";
import { useSimpleLocale, getTranslation } from "@/i18n/SimpleTranslationProvider";

export function AnalyticsSkeleton() {
  const { locale } = useSimpleLocale();
  return (
    <div className="space-y-6 animate-pulse" role="status" aria-busy="true">
      <span className="sr-only">
        {getTranslation("common.loadingAnalytics", locale) as string}
      </span>
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
        <SkeletonKPI />
        <SkeletonKPI />
        <SkeletonKPI />
        <SkeletonKPI />
      </div>
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <SkeletonChart height="18rem" />
        <SkeletonChart height="18rem" />
      </div>
    </div>
  );
}
