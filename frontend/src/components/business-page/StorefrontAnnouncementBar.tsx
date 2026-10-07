"use client";

import React, { useEffect, useState } from "react";
import { CalendarClock, X } from "lucide-react";
import { GuestTranslationContext } from "@/i18n/GuestTranslationProvider";
import {
  announcementStorageKey,
  type StorefrontOperatingException,
} from "./storefrontAnnouncement";

interface StorefrontAnnouncementBarProps {
  businessId: number;
  /** Nearest upcoming exception (today..+7d in the business tz), or null. */
  exception: (StorefrontOperatingException & { id?: number }) | null;
  t: (key: string, params?: Record<string, string | number>) => string;
}

/**
 * Slim dismissible banner above the hero (plan 3.5) surfacing an operating
 * exception (holiday closure, special hours) whose date is today or within
 * the next 7 days in the business timezone. Dismissal persists per
 * business+exception in sessionStorage.
 */
export default function StorefrontAnnouncementBar({
  businessId,
  exception,
  t,
}: StorefrontAnnouncementBarProps) {
  const [dismissed, setDismissed] = useState(false);
  const guestLocale = React.useContext(GuestTranslationContext)?.currentLanguage;

  const storageKey = exception
    ? announcementStorageKey(businessId, exception)
    : null;

  // Read dismissal after mount so SSR and first client render agree (the bar
  // shows, then hides if it was dismissed earlier this session).
  useEffect(() => {
    if (!storageKey) return;
    try {
      if (window.sessionStorage.getItem(storageKey)) setDismissed(true);
    } catch {
      // sessionStorage unavailable (private mode) — keep the bar visible.
    }
  }, [storageKey]);

  if (!exception || dismissed) return null;

  const dismiss = () => {
    setDismissed(true);
    if (storageKey) {
      try {
        window.sessionStorage.setItem(storageKey, "1");
      } catch {
        // Non-fatal: the bar simply reappears on next render.
      }
    }
  };

  const label =
    exception.label?.trim() || t("businessPage.announcement.specialHours");

  // Noon-UTC parse keeps the calendar day stable regardless of the viewer's
  // timezone; format in the guest locale.
  const parsed = new Date(`${exception.exception_date}T12:00:00Z`);
  const dateLabel = Number.isNaN(parsed.getTime())
    ? exception.exception_date
    : parsed.toLocaleDateString(guestLocale || undefined, {
        weekday: "short",
        month: "short",
        day: "numeric",
        timeZone: "UTC",
      });

  return (
    <div
      data-testid="storefront-announcement-bar"
      className="relative bg-amber-50 border-b border-amber-200 text-amber-800"
    >
      <div className="max-w-7xl mx-auto px-4 sm:px-6 py-2 flex items-center gap-3">
        <CalendarClock className="w-4 h-4 shrink-0" aria-hidden />
        <p className="flex-1 min-w-0 text-sm truncate">
          <span className="font-semibold">{label}</span>
          <span className="text-amber-700"> · {dateLabel}</span>
        </p>
        <button
          type="button"
          onClick={dismiss}
          aria-label={t("businessPage.announcement.dismiss")}
          className="shrink-0 inline-flex items-center justify-center w-7 h-7 rounded-full text-amber-700 transition-colors hover:bg-amber-100 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-amber-700"
        >
          <X className="w-4 h-4" aria-hidden />
        </button>
      </div>
    </div>
  );
}
