import React from "react";

interface HeroSkeletonProps {
  /**
   * Localized loading announcement for screen readers. The skeleton renders
   * before any guest translation provider mounts, so the caller passes the
   * already-resolved label (operator-tier). Defaults to English.
   */
  label?: string;
}

export default function HeroSkeleton({
  label = "Loading business page",
}: HeroSkeletonProps) {
  return (
    <div
      data-testid="hero-skeleton"
      role="status"
      aria-label={label}
      className="min-h-[56vh] md:min-h-[68vh] bg-warm-50 relative overflow-hidden"
    >
      {/* Top sticky-bar placeholder */}
      <div className="absolute inset-x-0 top-0 h-14 bg-warm-100/80 backdrop-blur-sm border-b border-warm-200" />

      {/* Content placeholders */}
      <div className="relative z-10 max-w-7xl mx-auto px-6 pt-32 pb-16 flex flex-col items-center text-center gap-6">
        <div className="w-28 h-28 md:w-32 md:h-32 rounded-lg bg-warm-200 animate-pulse" />
        <div className="h-10 md:h-14 w-2/3 max-w-xl rounded-lg bg-warm-200 animate-pulse" />
        <div className="h-4 w-1/2 max-w-md rounded bg-warm-200 animate-pulse" />
        <div className="flex gap-3 mt-4">
          <div className="h-12 w-32 rounded-lg bg-warm-200 animate-pulse" />
          <div className="h-12 w-24 rounded-lg bg-warm-200 animate-pulse" />
        </div>
      </div>
    </div>
  );
}
