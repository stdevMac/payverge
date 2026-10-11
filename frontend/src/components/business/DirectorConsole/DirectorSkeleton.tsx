"use client";

import React from "react";
import { SkeletonCard } from "@/components/ui/skeletons";
import { useSimpleLocale, getTranslation } from "@/i18n/SimpleTranslationProvider";

export function DirectorSkeleton() {
  const { locale } = useSimpleLocale();
  return (
    <div
      className="p-4 space-y-4 animate-pulse"
      role="status"
      aria-busy="true"
    >
      <span className="sr-only">
        {getTranslation("common.loadingDirector", locale) as string}
      </span>
      <SkeletonCard lines={6} hasHeader className="min-h-[12rem]" />
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <SkeletonCard lines={3} hasHeader />
        <SkeletonCard lines={3} hasHeader />
        <SkeletonCard lines={3} hasHeader />
      </div>
    </div>
  );
}
