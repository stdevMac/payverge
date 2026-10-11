"use client";

import React from "react";
import { SkeletonCard } from "@/components/ui/skeletons";
import { useSimpleLocale, getTranslation } from "@/i18n/SimpleTranslationProvider";

export function CounterSkeleton() {
  const { locale } = useSimpleLocale();
  return (
    <div className="space-y-6 animate-pulse" role="status" aria-busy="true">
      <span className="sr-only">
        {getTranslation("common.loadingCounter", locale) as string}
      </span>
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <SkeletonCard lines={6} hasHeader className="min-h-[24rem]" />
        <SkeletonCard lines={8} hasHeader className="min-h-[24rem]" />
      </div>
    </div>
  );
}
