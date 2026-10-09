"use client";

import React from "react";
import { SkeletonTable, SkeletonLine } from "@/components/ui/skeletons";
import { useSimpleLocale, getTranslation } from "@/i18n/SimpleTranslationProvider";

export function TablesSkeleton() {
  const { locale } = useSimpleLocale();
  return (
    <div className="space-y-6 animate-pulse" role="status" aria-busy="true">
      <span className="sr-only">
        {getTranslation("common.loadingTables", locale) as string}
      </span>
      {/* Search + view toggle only — the shell's PageHeader stays live above
          this slot, so a second title/subtitle pair here would double up. */}
      <div className="flex gap-2">
        <SkeletonLine width="100%" height="2.5rem" className="rounded-lg max-w-md" />
        <SkeletonLine width="10rem" height="2.5rem" className="rounded-lg" />
      </div>
      <SkeletonTable rows={10} columns={7} />
    </div>
  );
}
