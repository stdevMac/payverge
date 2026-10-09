"use client";

import React from "react";
import { MapPin, Phone, Clock, Navigation, Zap } from "lucide-react";
import {
  InstagramIcon,
  FacebookIcon,
  XBrandIcon,
  LinkedinIcon,
  YoutubeIcon,
  TiktokIcon,
} from "@/components/icons/brands";
import type { PublicBusiness } from "@/api/publicBusiness";
import { parseSocialMedia } from "@/utils/businessDataParsers";
import {
  normalize,
  socialProfileUrl,
  type SocialNetwork,
} from "@/lib/socialHandle";
// Same provider-less context read as BusinessContactTab: localize hours in
// the active guest locale, fall back to the browser default in unit tests.
import { GuestTranslationContext } from "@/i18n/GuestTranslationProvider";
import { getRadiusClass } from "./designClasses";
import { guestHourCycle, isAllDayWindow } from "@/utils/guestClockTime";

type OperatingHourRow = {
  day_of_week: number;
  open_time?: string;
  close_time?: string;
  is_closed?: boolean;
};

interface BusinessFooterProps {
  business: PublicBusiness;
  operatingHours: OperatingHourRow[];
  /** YYYY-MM-DD "today" in the business timezone (from useBusinessOpenStatus). */
  businessDate?: string;
  designSettings: {
    primary_color: string;
    secondary_color: string;
    corner_radius?: string;
  };
  t: (key: string) => string;
}

// Order matches the backend day_of_week index (0 = Sunday).
const DAY_KEYS = [
  "sunday",
  "monday",
  "tuesday",
  "wednesday",
  "thursday",
  "friday",
  "saturday",
] as const;

const SOCIAL_LINKS: Array<{
  key: SocialNetwork;
  Icon: React.ComponentType<{ className?: string }>;
  label: string;
}> = [
  { key: "instagram", Icon: InstagramIcon, label: "Instagram" },
  { key: "facebook", Icon: FacebookIcon, label: "Facebook" },
  { key: "twitter", Icon: XBrandIcon, label: "Twitter" },
  { key: "linkedin", Icon: LinkedinIcon, label: "LinkedIn" },
  { key: "youtube", Icon: YoutubeIcon, label: "YouTube" },
  { key: "tiktok", Icon: TiktokIcon, label: "TikTok" },
];

/** Same dirty-input guard as BusinessContactTab: prefer the canonical bare
 *  handle, fall back to an absolute URL as-is, otherwise no link. */
function socialHref(network: SocialNetwork, raw: string): string {
  const result = normalize(network, raw);
  if (result.ok && result.handle) {
    return socialProfileUrl(network, result.handle);
  }
  const trimmed = raw.trim();
  if (/^https?:\/\//i.test(trimmed)) return trimmed;
  return "";
}

/** Group split-shift rows (multiple periods per day) for display. */
function groupOperatingHoursByDay(
  hours: OperatingHourRow[],
): Array<{
  day_of_week: number;
  is_closed: boolean;
  periods: Array<{ open_time?: string; close_time?: string }>;
}> {
  const byDay = new Map<
    number,
    {
      day_of_week: number;
      is_closed: boolean;
      periods: Array<{ open_time?: string; close_time?: string }>;
    }
  >();
  for (const h of hours) {
    const existing = byDay.get(h.day_of_week);
    if (!existing) {
      byDay.set(h.day_of_week, {
        day_of_week: h.day_of_week,
        is_closed: Boolean(h.is_closed),
        periods: h.is_closed
          ? []
          : [{ open_time: h.open_time, close_time: h.close_time }],
      });
      continue;
    }
    if (h.is_closed) {
      existing.is_closed = true;
      existing.periods = [];
    } else if (!existing.is_closed) {
      existing.periods.push({
        open_time: h.open_time,
        close_time: h.close_time,
      });
    }
  }
  return Array.from(byDay.values()).sort(
    (a, b) => a.day_of_week - b.day_of_week,
  );
}

/** HH:mm API strings rendered with the guest's locale convention (12h vs 24h). */
function formatHoursTime(value: string | undefined, locale?: string): string {
  if (!value) return "";
  const match = value.trim().match(/^(\d{1,2}):(\d{2})/);
  if (!match) return value;
  const hour = Number(match[1]);
  const minute = Number(match[2]);
  if (Number.isNaN(hour) || Number.isNaN(minute)) return value;
  const date = new Date();
  date.setHours(hour, minute, 0, 0);
  return date.toLocaleTimeString(locale || [], {
    hour: "numeric",
    minute: "2-digit",
    // #949: es-AR must read 23:00, not "11:00 p. m." — one shared hour-cycle
    // rule for every guest-facing clock string.
    hourCycle: guestHourCycle(locale),
  });
}

/** Day-of-week (0=Sunday) for a YYYY-MM-DD business date key; null on garbage. */
function parseBusinessDateDow(businessDate?: string): number | null {
  if (!businessDate || !/^\d{4}-\d{2}-\d{2}$/.test(businessDate)) return null;
  const parsed = new Date(`${businessDate}T12:00:00Z`);
  return Number.isNaN(parsed.getTime()) ? null : parsed.getUTCDay();
}

/**
 * Business footer (plan 3.4): NAP block + hours + socials/directions, so the
 * page reads as the restaurant's own site rather than a hosted profile. The
 * Payverge strip stays subtle at the bottom. Columns with no data are hidden;
 * when there is no business content at all, only the slim strip renders.
 */
export default function BusinessFooter({
  business,
  operatingHours,
  businessDate,
  designSettings,
  t,
}: BusinessFooterProps) {
  const guestLocale = React.useContext(
    GuestTranslationContext,
  )?.currentLanguage;
  const radiusClass = getRadiusClass(designSettings.corner_radius);

  const addressParts = [
    business.address?.street,
    [business.address?.city, business.address?.state]
      .filter(Boolean)
      .join(", "),
    [business.address?.postal_code, business.address?.country]
      .filter(Boolean)
      .join(" "),
  ].filter((part): part is string => Boolean(part));
  const hasAddress = addressParts.length > 0;
  const fullAddress = hasAddress
    ? [
        business.address?.street,
        business.address?.city,
        business.address?.state,
        business.address?.postal_code,
        business.address?.country,
      ]
        .filter(Boolean)
        .join(", ")
    : "";

  const socialMedia = parseSocialMedia(business.social_media) as Record<
    SocialNetwork,
    string | undefined
  >;
  const activeSocials = SOCIAL_LINKS.filter((entry) =>
    Boolean(socialMedia[entry.key]),
  );

  const showHours =
    Boolean(business.show_operating_hours) && operatingHours.length > 0;
  const todayDow = parseBusinessDateDow(businessDate) ?? new Date().getDay();

  const showContactColumn = hasAddress || Boolean(business.phone);
  const showSocialColumn = activeSocials.length > 0 || hasAddress;
  const hasBusinessContent =
    showContactColumn || showHours || showSocialColumn;

  const directionsHref = hasAddress
    ? `https://www.google.com/maps/search/?api=1&query=${encodeURIComponent(fullAddress)}`
    : null;

  const payvergeStrip = (
    <div className="flex flex-col sm:flex-row items-center justify-between gap-3 text-xs text-ink-500">
      <div className="flex items-center gap-2">
        <div
          className="w-5 h-5 rounded-md flex items-center justify-center"
          style={{
            background: `linear-gradient(135deg, ${designSettings.primary_color}, ${designSettings.secondary_color})`,
          }}
        >
          <Zap className="w-3 h-3 text-white" />
        </div>
        <span className="font-semibold text-ink-800 tracking-wide">
          Payverge
        </span>
      </div>
      <span className="text-ink-700 sm:text-right">
        {t("businessPage.poweringHospitality")}
      </span>
    </div>
  );

  if (!hasBusinessContent) {
    return (
      <footer className="bg-warm-100 border-t border-warm-200 py-8">
        <div className="max-w-6xl mx-auto px-6">{payvergeStrip}</div>
      </footer>
    );
  }

  return (
    <footer className="bg-warm-100 border-t border-warm-200">
      <div className="max-w-6xl mx-auto px-6 pt-10 pb-8">
        <div className="grid gap-10 md:grid-cols-3 md:gap-8">
          {showContactColumn && (
            <div data-testid="footer-contact" className="space-y-3">
              <p className="font-title text-lg text-ink-900">{business.name}</p>
              {hasAddress && (
                <div className="flex items-start gap-2 text-sm text-ink-600">
                  <MapPin
                    className="w-4 h-4 mt-0.5 shrink-0 text-ink-400"
                    aria-hidden
                  />
                  <div>
                    {addressParts.map((line) => (
                      <p key={line}>{line}</p>
                    ))}
                  </div>
                </div>
              )}
              {business.phone && (
                <div className="flex items-center gap-2 text-sm">
                  <Phone
                    className="w-4 h-4 shrink-0 text-ink-400"
                    aria-hidden
                  />
                  <a
                    href={`tel:${business.phone}`}
                    className="font-medium hover:underline"
                    style={{ color: designSettings.primary_color }}
                  >
                    {business.phone}
                  </a>
                </div>
              )}
            </div>
          )}

          {showHours && (
            <div data-testid="footer-hours">
              <div className="flex items-center gap-2 mb-3 text-xs font-semibold uppercase tracking-[0.2em] text-ink-500">
                <Clock className="w-3.5 h-3.5" aria-hidden />
                {t("businessPage.openingHours")}
              </div>
              <ul className="divide-y divide-warm-200 border-y border-warm-200">
                {groupOperatingHoursByDay(operatingHours).map((day) => {
                  const isToday = todayDow === day.day_of_week;
                  return (
                    <li
                      key={day.day_of_week}
                      className={`flex justify-between items-baseline gap-3 py-2 text-sm ${
                        isToday ? "font-semibold" : ""
                      }`}
                    >
                      <span
                        className={isToday ? "text-ink-900" : "text-ink-600"}
                      >
                        {t(`businessPage.days.${DAY_KEYS[day.day_of_week]}`)}
                        {isToday && (
                          <span
                            className="ml-2 text-[10px] uppercase tracking-[0.18em]"
                            style={{ color: designSettings.primary_color }}
                          >
                            · {t("businessPage.today")}
                          </span>
                        )}
                      </span>
                      <span
                        className={
                          day.is_closed
                            ? "text-right text-ink-500"
                            : "text-right text-ink-900"
                        }
                      >
                        {day.is_closed
                          ? t("businessPage.closed")
                          : day.periods
                              .map(
                                (p) =>
                                  isAllDayWindow(p.open_time, p.close_time)
                                    ? t("businessPage.open24Hours")
                                    : `${formatHoursTime(p.open_time, guestLocale)} – ${formatHoursTime(p.close_time, guestLocale)}`,
                              )
                              .join(", ")}
                      </span>
                    </li>
                  );
                })}
              </ul>
            </div>
          )}

          {showSocialColumn && (
            <div data-testid="footer-social" className="space-y-4">
              {activeSocials.length > 0 && (
                <div>
                  <p className="text-xs font-semibold uppercase tracking-[0.2em] text-ink-500 mb-3">
                    {t("businessPage.followUs")}
                  </p>
                  <div className="flex flex-wrap gap-2">
                    {activeSocials.map(({ key, Icon, label }) => {
                      const href = socialHref(key, socialMedia[key]!);
                      if (!href) return null;
                      return (
                        <a
                          key={key}
                          href={href}
                          target="_blank"
                          rel="noopener noreferrer"
                          aria-label={label}
                          className={`w-9 h-9 ${radiusClass} flex items-center justify-center border border-warm-200 bg-white text-ink-600 transition-colors hover:bg-[var(--primary)] hover:border-[var(--primary)] hover:text-white`}
                        >
                          <Icon className="w-4 h-4" />
                        </a>
                      );
                    })}
                  </div>
                </div>
              )}
              {directionsHref && (
                <a
                  href={directionsHref}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="inline-flex items-center gap-2 text-sm font-semibold hover:underline"
                  style={{ color: designSettings.primary_color }}
                >
                  <Navigation className="w-4 h-4" aria-hidden />
                  {t("businessPage.footer.getDirections")}
                </a>
              )}
            </div>
          )}
        </div>
      </div>
      <div className="border-t border-warm-200 py-5">
        <div className="max-w-6xl mx-auto px-6">{payvergeStrip}</div>
      </div>
    </footer>
  );
}
