"use client";

import React from "react";
import { SkeletonChart, SkeletonTable } from "@/components/ui/skeletons";
import { useSimpleLocale, getTranslation } from "@/i18n/SimpleTranslationProvider";

export function ReservationsSkeleton() {
  const { locale } = useSimpleLocale();
  return (
    <div className="space-y-6 animate-pulse" role="status" aria-busy="true">
      <span className="sr-only">
        {getTranslation("common.loadingReservations", locale) as string}
      </span>
      {/* No header placeholder: the shell keeps the real title, subtitle and
          actions mounted above this slot while reservations load (S-9). */}
      <SkeletonChart height="20rem" />
      <SkeletonTable rows={5} columns={5} />
    </div>
  );
}
