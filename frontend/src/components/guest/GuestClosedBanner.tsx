"use client";

import React from "react";
import { Info } from "lucide-react";
import { useGuestTranslation } from "@/i18n/GuestTranslationProvider";

/**
 * Single closed-hours notice for guest landing + menu.
 * Priority vs kitchen-off is owned by the parent (kitchen off first).
 */
export default function GuestClosedBanner({
  className = "",
}: {
  className?: string;
}) {
  const { t } = useGuestTranslation();
  const title =
    t("menu.businessClosed") || "Restaurant is currently closed";
  const description =
    t("menu.businessClosedDescription") ||
    "You can still browse the menu and pay an open bill.";

  return (
    <div
      role="status"
      data-testid="guest-business-closed-banner"
      className={`flex items-start gap-3 rounded-2xl border border-warm-200 bg-warm-50 px-4 py-3 ${className}`}
    >
      <Info
        className="mt-0.5 h-5 w-5 flex-shrink-0 text-brand"
        strokeWidth={1.75}
        aria-hidden
      />
      <div className="min-w-0 flex-1">
        <p className="text-sm font-semibold text-ink-900">{title}</p>
        <p className="mt-0.5 text-sm text-ink-700">{description}</p>
      </div>
    </div>
  );
}
