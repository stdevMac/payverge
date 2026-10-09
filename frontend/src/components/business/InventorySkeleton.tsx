"use client";

import React from "react";
import { SkeletonTable, SkeletonLine } from "@/components/ui/skeletons";
import { useSimpleLocale, getTranslation } from "@/i18n/SimpleTranslationProvider";

export function InventorySkeleton() {
  const { locale } = useSimpleLocale();
  return (
    <div className="space-y-6 animate-pulse" role="status" aria-busy="true">
      <span className="sr-only">
        {getTranslation("common.loadingInventory", locale) as string}
      </span>
      {/* Toolbar / filters — the title and refresh action stay live on the
          shell's PageHeader while this loads, so no header placeholder here. */}
      <div className="flex gap-2">
        <SkeletonLine width="10rem" height="2.25rem" className="rounded-lg" />
        <SkeletonLine width="10rem" height="2.25rem" className="rounded-lg" />
      </div>
      {/* Item table */}
      <SkeletonTable rows={8} columns={6} />
    </div>
  );
}
