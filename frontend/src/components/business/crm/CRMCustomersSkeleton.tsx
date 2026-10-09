"use client";

import React from "react";
import { SkeletonKPI, SkeletonTable, SkeletonLine } from "@/components/ui/skeletons";
import { useSimpleLocale, getTranslation } from "@/i18n/SimpleTranslationProvider";

export function CRMCustomersSkeleton() {
  const { locale } = useSimpleLocale();
  return (
    <div className="space-y-4 animate-pulse" role="status" aria-busy="true">
      <span className="sr-only">
        {getTranslation("common.loadingCustomers", locale) as string}
      </span>
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
        <SkeletonKPI />
        <SkeletonKPI />
        <SkeletonKPI />
        <SkeletonKPI />
      </div>
      <div className="flex gap-2 items-center">
        <SkeletonLine width="100%" height="2.5rem" className="rounded-lg max-w-lg" />
        <SkeletonLine width="10rem" height="2.5rem" className="rounded-lg" />
      </div>
      <SkeletonTable rows={8} columns={7} />
    </div>
  );
}
