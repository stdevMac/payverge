"use client";

import React from "react";
import { Lock } from "lucide-react";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";
import { PremiumPanel } from "./premium";

interface BusinessLockNoticeProps {
  businessId: number | string;
  /** Rendered when the business is operational (or its state is unknown). */
  children?: React.ReactNode;
}

/**
 * The only dashboard lock: a server administrator suspended or closed the
 * business. Anything else renders `children` unchanged.
 */
export const BusinessLockNotice: React.FC<BusinessLockNoticeProps> = ({
  businessId,
  children,
}) => {
  const { locale } = useSimpleLocale();
  const { lockState, isError, loading, refetch } = useBusinessAccess(businessId);

  const t = (key: string): string => {
    const fullKey = `businessLock.${key}`;
    const result = getTranslation(fullKey, locale);
    return Array.isArray(result) ? result[0] : (result as string);
  };

  // A failed status fetch must never read as a lock: show last-good children
  // or a retry.
  if (loading || isError) {
    if (children) return <>{children}</>;
    if (loading) return null;
    return (
      <div
        data-testid="business-lock-retry"
        className="mx-auto flex min-h-[240px] w-full max-w-xl flex-col items-center justify-center gap-3 px-4 py-10 text-center"
      >
        <p className="text-sm leading-6 text-ink-700">{t("loadFailed.title")}</p>
        <button
          type="button"
          onClick={() => {
            void refetch();
          }}
          className="rounded-full bg-brand px-4 py-2 text-sm font-semibold text-white hover:bg-brand-dark"
        >
          {t("loadFailed.retry")}
        </button>
      </div>
    );
  }

  if (lockState === "active") {
    return <>{children ?? null}</>;
  }

  const state = lockState === "closed" ? "closed" : "suspended";
  return (
    <div
      data-testid="admin-lock-notice"
      className="mx-auto flex min-h-[320px] w-full max-w-xl items-center justify-center px-4 py-8"
    >
      <PremiumPanel as="section" tone="urgent" className="w-full">
        <div className="p-6 text-center sm:p-8">
          <Lock className="mx-auto mb-4 h-6 w-6 text-brand" aria-hidden="true" />
          <h2 className="font-title text-2xl font-semibold text-ink-950">
            {t(`${state}.title`)}
          </h2>
          <p className="mt-3 text-sm leading-6 text-ink-600 sm:text-base">
            {t(`${state}.subtitle`)}
          </p>
        </div>
      </PremiumPanel>
    </div>
  );
};
