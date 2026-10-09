"use client";

import { AlertCircle, RefreshCw } from "lucide-react";
import { btnSecondary } from "./buttonStyles";
import { SkeletonLine } from "./skeletons";

export function RouteLoadingFallback() {
  return (
    <div
      role="status"
      aria-live="polite"
      aria-busy="true"
      className="mx-auto flex min-h-[60vh] w-full max-w-5xl items-center px-6 py-12"
    >
      <div aria-hidden="true" className="w-full space-y-5">
        <SkeletonLine
          width="8rem"
          height="0.875rem"
          className="rounded-full"
          data-skeleton-line
        />
        <SkeletonLine
          width="min(28rem, 90%)"
          height="2.5rem"
          className="rounded-xl"
          data-skeleton-line
        />
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {[0, 1, 2].map((item) => (
            <div
              key={item}
              data-skeleton-line
              className="h-32 animate-pulse rounded-2xl border border-warm-200 bg-warm-100"
            />
          ))}
        </div>
      </div>
    </div>
  );
}

interface PanelFailureProps {
  title: string;
  message: string;
  retryLabel: string;
  onRetry: () => void;
  isRetrying?: boolean;
  className?: string;
}

export function PanelFailure({
  title,
  message,
  retryLabel,
  onRetry,
  isRetrying = false,
  className = "",
}: PanelFailureProps) {
  return (
    <section
      role="alert"
      aria-busy={isRetrying}
      className={`rounded-2xl border border-rose-200 bg-rose-50 p-5 ${className}`.trim()}
    >
      <div className="flex items-start gap-3">
        <AlertCircle
          aria-hidden="true"
          className="mt-0.5 h-5 w-5 shrink-0 text-rose-700"
        />
        <div className="min-w-0">
          <h3 className="text-sm font-semibold text-rose-900">{title}</h3>
          <p className="mt-1 text-sm leading-6 text-rose-800">{message}</p>
          <button
            type="button"
            onClick={onRetry}
            disabled={isRetrying}
            className={`mt-4 ${btnSecondary}`}
          >
            <RefreshCw
              aria-hidden="true"
              className={`h-4 w-4 ${isRetrying ? "animate-spin" : ""}`}
            />
            {retryLabel}
          </button>
        </div>
      </div>
    </section>
  );
}
