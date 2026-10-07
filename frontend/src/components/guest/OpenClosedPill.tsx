"use client";

import React from "react";
import { useGuestTranslation } from "@/i18n/GuestTranslationProvider";
import { formatGuestClockTime } from "@/utils/guestClockTime";

interface OpenClosedHours {
  day_of_week: number; // 0=Sunday … 6=Saturday
  open_time: string; // HH:MM (24h)
  close_time: string; // HH:MM (24h)
  is_closed?: boolean;
}

interface OpenClosedPillProps {
  hours?: OpenClosedHours[] | null;
  /**
   * IANA timezone of the business. When provided the open/closed decision is
   * computed in the venue's local time rather than the device's. Without it
   * the device timezone is used.
   */
  timezone?: string;
  /**
   * Override "now" for deterministic tests. Defaults to new Date().
   */
  now?: Date;
}

const WEEKDAY_INDEX: Record<string, number> = {
  Sun: 0,
  Mon: 1,
  Tue: 2,
  Wed: 3,
  Thu: 4,
  Fri: 5,
  Sat: 6,
};

// Compute the weekday (0=Sun) and minutes-since-midnight for `date` in the
// business timezone, so the pill is correct regardless of the diner's device
// timezone. Normalizes the ICU/Node "24:xx" midnight-hour quirk back to 0.
function partsInTimezone(
  date: Date,
  timezone?: string,
): { weekday: number; minutes: number } {
  try {
    // "en-US" here is a STABLE PARSING locale, not user-facing display: the
    // weekday/hour/minute parts are read back as numbers (WEEKDAY_INDEX lookup +
    // Number.parseInt), so they must stay English/Latin-digit. A localized digit
    // system (ar/hi) would break the parse. Guest-facing times use formatTime in
    // the active locale (currentLanguage).
    const fmt = new Intl.DateTimeFormat("en-US", {
      timeZone: timezone || undefined,
      weekday: "short",
      hour: "2-digit",
      minute: "2-digit",
      hour12: false,
    });
    const lookup = Object.fromEntries(
      fmt
        .formatToParts(date)
        .filter((p) => p.type !== "literal")
        .map((p) => [p.type, p.value]),
    ) as Record<string, string>;
    const hour = Number.parseInt(lookup.hour || "0", 10) % 24; // "24" -> 0
    const minute = Number.parseInt(lookup.minute || "0", 10);
    return {
      weekday: WEEKDAY_INDEX[lookup.weekday] ?? date.getDay(),
      minutes: hour * 60 + minute,
    };
  } catch (e) { console.error("Failed to parse opening hours", e);
    return {
      weekday: date.getDay(),
      minutes: date.getHours() * 60 + date.getMinutes(),
    };
  }
}

function clockMinutes(time: string): number | null {
  const [hStr, mStr] = (time || "").split(":");
  const h = Number(hStr);
  const m = Number(mStr);
  if (Number.isNaN(h) || Number.isNaN(m)) return null;
  return h * 60 + m;
}

// True when `minutes` falls inside [open, close), with overnight windows
// (close <= open, e.g. 18:00 -> 02:00) treated as wrapping past midnight.
function withinWindow(
  minutes: number,
  open: number | null,
  close: number | null,
): boolean {
  if (open === null || close === null) return false;
  if (close > open) {
    return minutes >= open && minutes < close;
  }
  // Overnight (or 24h when open === close): open in the evening tail OR the
  // early-morning tail.
  return minutes >= open || minutes < close;
}

function formatTime(time: string, locale: string): string {
  return formatGuestClockTime(time, locale);
}

function OpenClosedPill({ hours, timezone, now }: OpenClosedPillProps) {
  const { t, currentLanguage } = useGuestTranslation();

  if (!hours || hours.length === 0) {
    return null;
  }

  const current = now ?? new Date();
  const { weekday: today, minutes } = partsInTimezone(current, timezone);
  const todayHours = hours.find((h) => h.day_of_week === today);
  // The previous day matters for overnight venues whose window spills past
  // midnight (open 18:00, close 02:00) — after midnight the venue is "open"
  // under yesterday's row, not today's.
  const previousDayHours = hours.find((h) => h.day_of_week === (today + 6) % 7);

  const openNow =
    todayHours && !todayHours.is_closed
      ? withinWindow(
          minutes,
          clockMinutes(todayHours.open_time),
          clockMinutes(todayHours.close_time),
        )
      : false;

  // Open because of the previous day's overnight tail (we're in the early
  // morning before yesterday's window closed).
  const openFromPreviousOvernight = (() => {
    if (!previousDayHours || previousDayHours.is_closed) return false;
    const open = clockMinutes(previousDayHours.open_time);
    const close = clockMinutes(previousDayHours.close_time);
    if (open === null || close === null) return false;
    return close <= open && minutes < close;
  })();

  if (openNow || openFromPreviousOvernight) {
    const activeClose = openNow
      ? todayHours!.close_time
      : previousDayHours?.close_time ?? "";
    return (
      <span
        data-testid="landing-open-pill"
        className="inline-flex items-center gap-1.5 rounded-full border border-emerald-200 bg-emerald-50 px-3 py-1 text-label uppercase tracking-wider text-emerald-700"
      >
        <span className="inline-flex h-1.5 w-1.5 rounded-full bg-emerald-500" />
        {t("landing.statusOpenUntil", {
          time: formatTime(activeClose, currentLanguage),
        })}
      </span>
    );
  }

  // Not open. If today has usable hours, tell the guest when it opens;
  // otherwise the venue is closed for the day.
  if (!todayHours || todayHours.is_closed || clockMinutes(todayHours.open_time) === null) {
    return (
      <span
        data-testid="landing-open-pill"
        className="inline-flex items-center gap-1.5 rounded-full border border-rose-200 bg-rose-50 px-3 py-1 text-label uppercase tracking-wider text-rose-700"
      >
        <span className="inline-flex h-1.5 w-1.5 rounded-full bg-rose-500" />
        {t("landing.statusClosedToday")}
      </span>
    );
  }

  return (
    <span
      data-testid="landing-open-pill"
      className="inline-flex items-center gap-1.5 rounded-full border border-rose-200 bg-rose-50 px-3 py-1 text-label uppercase tracking-wider text-rose-700"
    >
      <span className="inline-flex h-1.5 w-1.5 rounded-full bg-rose-500" />
      {t("landing.statusOpensAt", {
        time: formatTime(todayHours.open_time, currentLanguage),
      })}
    </span>
  );
}

export default OpenClosedPill;
