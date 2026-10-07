"use client";

import React from "react";
import { SkeletonCard } from "@/components/ui/skeletons";
import { useSimpleLocale, getTranslation } from "@/i18n/SimpleTranslationProvider";

export function CashRegisterSkeleton() {
  const { locale } = useSimpleLocale();
  return (
    <div className="space-y-6 animate-pulse" role="status" aria-busy="true">
      <span className="sr-only">
        {
          getTranslation(
            "businessDashboard.cashRegisterDashboard.loading",
            locale,
          ) as string
        }
      </span>
      <SkeletonCard lines={4} hasHeader />
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-[minmax(0,1fr)_minmax(320px,420px)]">
        <SkeletonCard lines={6} hasHeader className="min-h-[20rem]" />
        <SkeletonCard lines={6} hasHeader className="min-h-[20rem]" />
      </div>
    </div>
  );
}
