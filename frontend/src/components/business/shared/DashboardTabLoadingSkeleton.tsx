"use client";

import React from "react";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { PremiumPanel } from "../premium";
import {
  SkeletonKPI,
  SkeletonLine,
  SkeletonTable,
} from "@/components/ui/skeletons";

type DashboardTabLoadingVariant =
  | "default"
  | "overview"
  | "table";

interface DashboardTabLoadingSkeletonProps {
  variant?: DashboardTabLoadingVariant;
  /** i18n key under `common.*` for screen-reader label */
  labelKey?: string;
  width?: "default" | "wide";
  /**
   * Page chrome = the centered page canvas *and* the PageHeader placeholder.
   * Pass `false` when this renders into `DashboardTabShell`'s `loading` slot:
   * the shell already keeps the real header mounted and owns the canvas, so
   * chrome here paints a grey title bar directly under the live one, inside a
   * second centered container (S-9). One knob, deliberately — the two halves
   * are never right apart.
   */
  withPageChrome?: boolean;
  className?: string;
}

const widthClass: Record<
  NonNullable<DashboardTabLoadingSkeletonProps["width"]>,
  string
> = {
  default: "max-w-6xl",
  wide: "max-w-7xl",
};

function HeaderSkeleton() {
  return (
    <div className="space-y-3" aria-hidden="true" data-skeleton-page-header="true">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div className="min-w-0 space-y-3">
          <SkeletonLine width="3.5rem" height="0.75rem" className="rounded-full" />
          <SkeletonLine width="14rem" height="2rem" className="rounded-xl" />
          <SkeletonLine
            width="18rem"
            height="0.875rem"
            className="max-w-full rounded-full"
          />
        </div>
        <SkeletonLine
          width="2.5rem"
          height="2.5rem"
          className="hidden rounded-2xl sm:block"
        />
      </div>
    </div>
  );
}

function DefaultBody() {
  return (
    <div className="grid gap-3 sm:grid-cols-3" aria-hidden="true">
      {Array.from({ length: 3 }).map((_, index) => (
        <div
          key={index}
          className="h-24 rounded-2xl border border-warm-200/80 bg-white/70 motion-safe:animate-pulse"
        />
      ))}
    </div>
  );
}

function OverviewBody() {
  return (
    <div className="space-y-5" aria-hidden="true">
      <PremiumPanel className="p-4 sm:p-5" withTexture>
        <div className="space-y-3">
          <SkeletonLine width="8rem" height="0.75rem" className="rounded-full" />
          <SkeletonLine width="100%" height="3rem" className="rounded-xl" />
        </div>
      </PremiumPanel>
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <SkeletonKPI />
        <SkeletonKPI />
        <SkeletonKPI />
        <SkeletonKPI />
      </div>
      <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
        {Array.from({ length: 3 }).map((_, index) => (
          <PremiumPanel key={index} className="p-5" withTexture>
            <div className="space-y-3">
              <SkeletonLine
                width="2.5rem"
                height="2.5rem"
                className="rounded-xl"
              />
              <SkeletonLine width="70%" height="1rem" className="rounded-lg" />
              <SkeletonLine width="90%" height="0.875rem" className="rounded-lg" />
            </div>
          </PremiumPanel>
        ))}
      </div>
    </div>
  );
}

function TableBody() {
  return (
    <div aria-hidden="true">
      <SkeletonTable rows={8} columns={6} />
    </div>
  );
}

const bodyByVariant: Record<DashboardTabLoadingVariant, React.ReactNode> = {
  default: <DefaultBody />,
  overview: <OverviewBody />,
  table: <TableBody />,
};

const defaultLabelByVariant: Record<DashboardTabLoadingVariant, string> = {
  default: "loadingDashboard",
  overview: "loadingOverview",
  table: "loadingTab",
};

/**
 * Shared first-paint skeleton for dashboard tabs. Matches the warm canvas,
 * serif-header budget, and PremiumPanel content rhythm used across the shell.
 */
export default function DashboardTabLoadingSkeleton({
  variant = "default",
  labelKey,
  width = "default",
  withPageChrome = true,
  className = "",
}: DashboardTabLoadingSkeletonProps) {
  const { locale } = useSimpleLocale();
  const resolvedLabelKey = labelKey ?? defaultLabelByVariant[variant];
  const label = getTranslation(`common.${resolvedLabelKey}`, locale) as string;

  // Width only means anything alongside the page canvas — inside the shell the
  // canvas is already sized, and a stray max-w here narrows the skeleton off
  // the content it replaces (Schedule is "wide"; the skeleton was max-w-6xl).
  const containerClass = [
    withPageChrome ? "mx-auto space-y-5 p-4 sm:p-6" : "space-y-5",
    withPageChrome ? widthClass[width] : "",
    className,
  ]
    .filter(Boolean)
    .join(" ");

  if (variant === "default") {
    return (
      <div
        className={containerClass}
        role="status"
        aria-busy="true"
        aria-live="polite"
      >
        <span className="sr-only">{label}</span>
        <PremiumPanel className="p-5 sm:p-6" withTexture>
          <div className="space-y-6">
            {withPageChrome ? <HeaderSkeleton /> : null}
            <DefaultBody />
          </div>
        </PremiumPanel>
      </div>
    );
  }

  return (
    <div
      className={containerClass}
      role="status"
      aria-busy="true"
      aria-live="polite"
    >
      <span className="sr-only">{label}</span>
      {withPageChrome ? (
        <HeaderSkeleton />
      ) : null}
      {bodyByVariant[variant]}
    </div>
  );
}
