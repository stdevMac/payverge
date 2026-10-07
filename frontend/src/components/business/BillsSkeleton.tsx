"use client";

import React from "react";
import { SkeletonTable, SkeletonLine } from "@/components/ui/skeletons";
import { useSimpleLocale, getTranslation } from "@/i18n/SimpleTranslationProvider";

export function BillsSkeleton() {
  const { locale } = useSimpleLocale();
  return (
    <div className="space-y-6 animate-pulse" role="status" aria-busy="true">
      <span className="sr-only">
        {getTranslation("common.loadingBills", locale) as string}
      </span>
      {/* Filters — the title and bill actions stay live on the shell's
          PageHeader above this slot, so no header placeholder here (S-9). */}
      <div className="flex gap-2">
        <SkeletonLine width="10rem" height="2.25rem" className="rounded-lg" />
        <SkeletonLine width="10rem" height="2.25rem" className="rounded-lg" />
      </div>
      {/* Table */}
      <SkeletonTable rows={8} columns={6} />
    </div>
  );
}
