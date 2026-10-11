"use client";

import React from "react";
import { PublicBusiness } from "@/api/publicBusiness";
import { MapPin, Phone, Globe, Clock, ExternalLink } from "lucide-react";
// Social-network brand marks (Instagram, Facebook, X, LinkedIn, YouTube,
// TikTok) have no faithful lucide equivalent, so they ship as dependency-free
// inline SVG glyphs (simple-icons paths) instead. The legacy Twitter slot now
// renders the X mark since Twitter rebranded to X.
import {
  InstagramIcon,
  FacebookIcon,
  XBrandIcon,
  LinkedinIcon,
  YoutubeIcon,
  TiktokIcon,
} from "@/components/icons/brands";
import { parseSocialMedia } from "@/utils/businessDataParsers";
import {
  normalize,
  socialProfileUrl,
  type SocialNetwork,
} from "@/lib/socialHandle";
// I18N-4: read the active guest locale to localize operating-hours times.
// BusinessContactTab always renders inside the storefront's
// GuestTranslationProvider (via ConvertingBusinessLandingPage), but several
// unit tests mount it provider-less, so use the no-throw context read with an
// undefined (browser-default) fallback rather than the throwing hook.
import { GuestTranslationContext } from "@/i18n/GuestTranslationProvider";
import SectionHeader from "./SectionHeader";
import BusinessMapEmbed from "./BusinessMapEmbed";
import ContactEmptyState from "./ContactEmptyState";
import { getRadiusClass, getSectionPadding } from "./designClasses";
import { hexWithAlpha, sanitizeCssColor } from "@/lib/storefront/theme";
import { guestHourCycle, isAllDayWindow } from "@/utils/guestClockTime";

interface BusinessContactTabProps {
  business: PublicBusiness;
  designSettings: {
    primary_color: string;
    corner_radius?: string;
    section_density?: string;
  };
  operatingHours: any[];
  t: (key: string) => string;
}

// Order matches the day_of_week index returned by the backend (0 = Sunday).
const DAY_KEYS = [
  "sunday",
  "monday",
  "tuesday",
  "wednesday",
  "thursday",
  "friday",
  "saturday",
];

type SocialKey = SocialNetwork;

/**
 * Merchants commonly enter a bare domain ("myrestaurant.com"). Without a
 * protocol the browser treats href as a relative path and resolves it inside
 * the app (/b/[customUrl]/myrestaurant.com), 404ing instead of opening the
 * external site. Mirror the social-link guard and default to https.
 */
function normalizeWebsiteHref(raw: string): string {
  const trimmed = raw.trim();
  return /^https?:\/\//i.test(trimmed) ? trimmed : `https://${trimmed}`;
}

/**
 * Build a social profile href from a (possibly dirty) stored value.
 * Prefers the canonical bare-handle form so already-dirty rows that still
 * hold full URLs do not double-prefix with SOCIAL_LINKS base.
 */
function socialHref(network: SocialKey, raw: string): string {
  const result = normalize(network, raw);
  if (result.ok && result.handle) {
    return socialProfileUrl(network, result.handle);
  }
  // Fallback: if it's already an absolute URL (e.g. wrong-network leftover),
  // open it as-is; otherwise leave empty so we don't invent a bad link.
  const trimmed = raw.trim();
  if (/^https?:\/\//i.test(trimmed)) return trimmed;
  return "";
}

/** Group split-shift rows (multiple periods per day) for display. */
function groupOperatingHoursByDay(
  hours: Array<{
    day_of_week: number;
    open_time?: string;
    close_time?: string;
    is_closed?: boolean;
  }>,
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

/**
 * Operating-hours values are HH:mm strings ("17:00") from an <input type=time>.
 * Render them with the guest's locale convention (12h vs 24h) so the contact
 * tab matches the reservation flow's time formatting instead of always showing
 * raw 24-hour times. Falls back to the raw string if it can't be parsed.
 */
function formatHoursTime(value: string | undefined, locale?: string): string {
  if (!value) return "";
  const match = value.trim().match(/^(\d{1,2}):(\d{2})/);
  if (!match) return value;
  const hour = Number(match[1]);
  const minute = Number(match[2]);
  if (Number.isNaN(hour) || Number.isNaN(minute)) return value;
  const date = new Date();
  date.setHours(hour, minute, 0, 0);
  // I18N-4: render in the active guest locale (12h vs 24h), not the browser default.
  // #949: es-AR must read 23:00, not "11:00 p. m." — one shared hour-cycle
  // rule for every guest-facing clock string.
  return date.toLocaleTimeString(locale || [], {
    hour: "numeric",
    minute: "2-digit",
    hourCycle: guestHourCycle(locale),
  });
}

const SOCIAL_LINKS: Array<{
  key: SocialKey;
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

export default function BusinessContactTab({
  business,
  designSettings,
  operatingHours,
  t,
}: BusinessContactTabProps) {
  const socialMedia = parseSocialMedia(business.social_media) as Record<
    SocialKey,
    string | undefined
  >;

  // I18N-4: localize operating-hours times in the active guest locale. Falls
  // back to the browser default when rendered outside a GuestTranslationProvider.
  const guestLocale = React.useContext(
    GuestTranslationContext,
  )?.currentLanguage;

  const hasAddress = Boolean(
    business.address?.street ||
    business.address?.city ||
    business.address?.state,
  );
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

  const todayDay = new Date().getDay();
  const activeSocials = SOCIAL_LINKS.filter((entry) =>
    Boolean(socialMedia[entry.key]),
  );

  const sectionPadding = getSectionPadding(designSettings.section_density);
  const radiusClass = getRadiusClass(designSettings.corner_radius);

  const hasHours = Boolean(
    business.show_operating_hours && operatingHours.length > 0,
  );
  // Plan 1.4: with no address, phone, website, socials, or hours the tab used
  // to render an empty two-column shell. Show a friendly empty state instead.
  const isEmpty =
    !hasAddress &&
    !business.phone &&
    !business.website &&
    activeSocials.length === 0 &&
    !hasHours;

  // Plan 1.9: social chip hover is pure CSS driven by these custom
  // properties (resting = primary tint; hover/focus = solid primary). No JS
  // event handlers — the old onMouseEnter/onMouseLeave inline-style mutation
  // was dead on touch and a React anti-pattern. sanitizeCssColor guards the
  // merchant-controlled value before it reaches a CSS context.
  const socialChipVars = {
    "--chip-bg": hexWithAlpha(designSettings.primary_color, "14"),
    "--chip-fg": sanitizeCssColor(designSettings.primary_color),
    "--chip-border": hexWithAlpha(designSettings.primary_color, "26"),
  } as React.CSSProperties;

  return (
    <div
      role="tabpanel"
      id="tabpanel-contact"
      aria-labelledby="tab-contact"
      tabIndex={-1}
    >
      <section className={`${sectionPadding} relative z-10`}>
        <div className="max-w-4xl mx-auto px-6">
          <SectionHeader
            title={t("businessPage.contact")}
            subtitle={t("businessPage.contactDescription")}
            designSettings={designSettings}
            centered={false}
          />

          {isEmpty ? (
            <ContactEmptyState
              primaryColor={designSettings.primary_color}
              radiusClass={radiusClass}
              title={t("businessPage.contactEmptyTitle")}
              body={t("businessPage.contactEmptyBody")}
            />
          ) : (
          <div className="grid gap-12 md:grid-cols-[minmax(0,1fr)_auto] md:gap-16">
            {/* Contact list */}
            <div className="space-y-6">
              {hasAddress && (
                <ContactRow
                  Icon={MapPin}
                  label={t("businessPage.address")}
                  value={fullAddress}
                  primaryColor={designSettings.primary_color}
                />
              )}
              {business.phone && (
                <ContactRow
                  Icon={Phone}
                  label={t("businessPage.phone")}
                  value={business.phone}
                  href={`tel:${business.phone}`}
                  primaryColor={designSettings.primary_color}
                />
              )}
              {business.website && (
                <ContactRow
                  Icon={Globe}
                  label={t("businessPage.website")}
                  value={t("businessPage.visitWebsite")}
                  href={normalizeWebsiteHref(business.website)}
                  external
                  primaryColor={designSettings.primary_color}
                />
              )}

              {activeSocials.length > 0 && (
                <div className="pt-4 border-t border-gray-200">
                  <p className="text-xs font-semibold uppercase tracking-[0.2em] text-gray-500 mb-3">
                    {t("businessPage.followUs")}
                  </p>
                  <div className="flex flex-wrap gap-2">
                    {activeSocials.map(({ key, Icon, label }) => {
                      const raw = socialMedia[key]!;
                      const href = socialHref(key, raw);
                      if (!href) return null;
                      return (
                        <a
                          key={key}
                          href={href}
                          target="_blank"
                          rel="noopener noreferrer"
                          aria-label={label}
                          style={socialChipVars}
                          className={`w-10 h-10 ${radiusClass} flex items-center justify-center border transition-colors bg-[var(--chip-bg)] text-[var(--chip-fg)] border-[var(--chip-border)] hover:bg-[var(--chip-fg)] hover:text-white hover:border-[var(--chip-fg)] focus-visible:bg-[var(--chip-fg)] focus-visible:text-white focus-visible:border-[var(--chip-fg)]`}
                        >
                          <Icon className="w-4 h-4" />
                        </a>
                      );
                    })}
                  </div>
                </div>
              )}

              {/* Plan 3.3: keyless Google Maps embed + directions CTA under
                  the address block. */}
              {hasAddress && (
                <BusinessMapEmbed
                  address={fullAddress}
                  primaryColor={designSettings.primary_color}
                  radiusClass={radiusClass}
                  t={t}
                />
              )}
            </div>

            {/* Operating hours */}
            {hasHours && (
              <div className="md:min-w-[280px]">
                <div className="flex items-center gap-2 mb-4 text-xs font-semibold uppercase tracking-[0.2em] text-gray-500">
                  <Clock className="w-3.5 h-3.5" />
                  {t("businessPage.openingHours")}
                </div>
                <ul className="divide-y divide-gray-200 border-y border-gray-200">
                  {groupOperatingHoursByDay(operatingHours).map((day) => {
                    const isToday = todayDay === day.day_of_week;
                    return (
                      <li
                        key={day.day_of_week}
                        className={`flex justify-between items-baseline gap-3 py-2.5 text-sm ${
                          isToday ? "font-semibold" : ""
                        }`}
                      >
                        <span
                          className={
                            isToday ? "text-gray-900" : "text-gray-700"
                          }
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
                              ? "text-right text-gray-500"
                              : "text-right text-gray-900"
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
          </div>
          )}
        </div>
      </section>
    </div>
  );
}

function ContactRow({
  Icon,
  label,
  value,
  href,
  external = false,
  primaryColor,
}: {
  Icon: React.ComponentType<{ className?: string }>;
  label: string;
  value: string;
  href?: string;
  external?: boolean;
  primaryColor: string;
}) {
  const labelEl = (
    <p className="text-xs font-semibold uppercase tracking-[0.2em] text-gray-500 mb-1">
      {label}
    </p>
  );
  const body = href ? (
    <a
      href={href}
      target={external ? "_blank" : undefined}
      rel={external ? "noopener noreferrer" : undefined}
      className="text-base font-medium hover:underline inline-flex items-center gap-1"
      style={{ color: primaryColor }}
    >
      {value}
      {external && <ExternalLink className="w-3 h-3" />}
    </a>
  ) : (
    <p className="text-base text-gray-900 font-medium">{value}</p>
  );

  return (
    <div className="flex items-start gap-4">
      <Icon className="w-4 h-4 mt-1 text-gray-400 flex-shrink-0" />
      <div className="flex-1 min-w-0">
        {labelEl}
        {body}
      </div>
    </div>
  );
}
