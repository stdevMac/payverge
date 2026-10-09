"use client";

import React from "react";
import { useSimpleLocale, getTranslation } from "@/i18n/SimpleTranslationProvider";
import { PremiumPanel } from "../premium";

interface MarketingSkeletonProps {
  showHero?: boolean;
  showPipeline?: boolean;
}

export function MarketingSkeleton({ showHero = true, showPipeline = true }: MarketingSkeletonProps) {
  const { locale } = useSimpleLocale();
  return (
    <div className="space-y-6" role="status" aria-busy="true">
      <span className="sr-only">
        {getTranslation("marketingDashboard.loading", locale) as string}
      </span>
      {showHero && showPipeline ? (
        <PremiumPanel tone="accent" className="animate-pulse p-5 sm:p-6" withTexture={false}>
          <div className="space-y-4">
            <div className="flex items-start gap-3">
              <div className="h-12 w-12 rounded-2xl bg-brand/10" />
              <div className="space-y-2">
                <div className="h-3 w-28 rounded-full bg-brand/10" />
                <div className="h-4 w-full max-w-lg rounded-full bg-warm-200/80" />
              </div>
            </div>
            <div className="flex gap-2">
              {[0, 1, 2].map((item) => (
                <div key={item} className="h-8 w-24 rounded-full bg-white/80" />
              ))}
            </div>
          </div>
        </PremiumPanel>
      ) : null}
      <PremiumPanel className="overflow-hidden p-0" withTexture={false}>
        <div className="animate-pulse border-b border-warm-100 px-5 py-4">
          <div className="h-5 w-40 rounded-full bg-warm-200" />
        </div>
        <div className="grid animate-pulse grid-cols-1 gap-6 p-5 xl:grid-cols-2 2xl:grid-cols-3">
          {[0, 1, 2].map((item) => (
            <div key={item} className="overflow-hidden rounded-2xl border border-warm-200/90 bg-white">
              <div className="border-b border-warm-100 px-4 py-3">
                <div className="h-4 w-32 rounded-full bg-brand/10" />
                <div className="mt-2 h-4 w-4/5 rounded-full bg-warm-200" />
              </div>
              <div className="aspect-[4/5] bg-warm-100" />
              <div className="space-y-3 p-4">
                <div className="h-12 rounded-xl bg-brand-50" />
                <div className="h-16 rounded-xl bg-warm-100" />
                <div className="h-10 rounded-full bg-brand/10" />
              </div>
            </div>
          ))}
        </div>
      </PremiumPanel>
    </div>
  );
}
