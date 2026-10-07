"use client";

import React from "react";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";

export interface StaleWidgetBannerProps {
  /** Optional override title; defaults to urlState.widgetStaleTitle */
  title?: string;
  /** Optional override body; defaults to urlState.widgetStaleBody */
  body?: string;
  /** Retry callback — when omitted, no retry control is shown */
  onRetry?: () => void;
  /** Optional retry label override */
  retryLabel?: string;
  className?: string;
}

/**
 * Shared honest stale-widget banner (L9-2 UX half).
 *
 * Prefer this over fabricating zero counts or silently substituting empty
 * arrays when an overview widget request fails. Extracted from the inline
 * OnboardingHub recovery pattern so all widgets share one voice.
 */
export default function StaleWidgetBanner({
  title,
  body,
  onRetry,
  retryLabel,
  className = "",
}: StaleWidgetBannerProps) {
  const { locale } = useSimpleLocale();
  const t = (key: string): string => {
    const result = getTranslation(`urlState.${key}`, locale);
    return Array.isArray(result) ? result[0] || key : (result as string);
  };

  const resolvedTitle = title || t("widgetStaleTitle");
  const resolvedBody = body || t("widgetStaleBody");
  const resolvedRetry = retryLabel || t("widgetStaleRetry");

  return (
    <div
      role="alert"
      data-testid="stale-widget-banner"
      className={`rounded-xl border border-amber-200 bg-amber-50 p-3 text-sm text-ink-700 ${className}`}
    >
      <p className="font-medium text-ink-800">{resolvedTitle}</p>
      {resolvedBody ? <p className="mt-1 text-ink-600">{resolvedBody}</p> : null}
      {onRetry ? (
        <button
          type="button"
          onClick={onRetry}
          className="mt-2 font-medium text-brand underline-offset-2 hover:underline"
        >
          {resolvedRetry}
        </button>
      ) : null}
    </div>
  );
}
