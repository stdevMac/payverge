"use client";

import React from "react";
import { SkeletonLine } from "@/components/ui/skeletons";
import { useSimpleLocale, getTranslation } from "@/i18n/SimpleTranslationProvider";

export function SettingsSkeleton() {
  const { locale } = useSimpleLocale();
  return (
    <div className="space-y-8 animate-pulse" role="status" aria-busy="true">
      <span className="sr-only">
        {getTranslation("common.loadingSettings", locale) as string}
      </span>
      {Array.from({ length: 3 }, (_, section) => (
        <div key={section} className="space-y-4">
          <SkeletonLine width="10rem" height="1.25rem" />
          <div className="space-y-3">
            {Array.from({ length: 4 }, (_, field) => (
              <div key={field} className="space-y-1">
                <SkeletonLine width="8rem" height="0.75rem" />
                <SkeletonLine width="100%" height="2.5rem" className="rounded-lg" />
              </div>
            ))}
          </div>
        </div>
      ))}
    </div>
  );
}
