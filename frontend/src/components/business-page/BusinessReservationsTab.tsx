"use client";

import React from "react";
import GuestReservationForm from "./GuestReservationForm";
import SectionHeader from "./SectionHeader";
import PartnerLinkRow from "./PartnerLinkRow";
import { getSectionPadding } from "./designClasses";
import {
  withResolvedIcon,
  type ExternalPartnerLink,
} from "./partnerIconResolver";

interface BusinessReservationsTabProps {
  customUrl: string;
  businessName: string;
  /** IANA timezone of the business; reservation slots render in it. */
  timezone?: string;
  reservationsEnabled: boolean;
  reservationPartnerLinks: ExternalPartnerLink[];
  reservationLoading?: boolean;
  designSettings: any;
  t: (key: string, params?: Record<string, string | number>) => string;
  /** Switch the storefront to the menu tab (post-booking cross-sell CTA). */
  onViewMenu?: () => void;
}

export default function BusinessReservationsTab({
  customUrl,
  businessName,
  timezone,
  reservationsEnabled,
  reservationPartnerLinks,
  reservationLoading = false,
  designSettings,
  t,
  onViewMenu,
}: BusinessReservationsTabProps) {
  const sectionPadding = getSectionPadding(designSettings?.section_density);

  if (reservationLoading) {
    return (
      <div
        role="tabpanel"
        id="tabpanel-reservations"
        aria-labelledby="tab-reservations"
        tabIndex={-1}
      >
        <section className={`${sectionPadding} relative z-10`}>
          <div className="max-w-6xl mx-auto px-6">
            <p className="text-sm text-gray-600">
              {t("businessPage.loadingAria")}
            </p>
          </div>
        </section>
      </div>
    );
  }

  if (reservationsEnabled) {
    return (
      <div
        role="tabpanel"
        id="tabpanel-reservations"
        aria-labelledby="tab-reservations"
        tabIndex={-1}
      >
        <section className={`${sectionPadding} relative z-10`}>
          <div className="max-w-6xl mx-auto px-6">
            <SectionHeader
              title={t("businessPage.reservations")}
              subtitle={t("businessPage.reservationsDescription")}
              badge={t("businessPage.bookingBadge")}
              designSettings={designSettings}
              centered={false}
            />
            <GuestReservationForm
              customUrl={customUrl}
              businessName={businessName}
              timezone={timezone}
              designSettings={designSettings}
              onViewMenu={onViewMenu}
            />
            {/* Partner escape hatches (OpenTable/Resy) only render when native
                booking is off — never under a working booking form. */}
          </div>
        </section>
      </div>
    );
  }

  if (reservationPartnerLinks.length > 0) {
    return (
      <div
        role="tabpanel"
        id="tabpanel-reservations"
        aria-labelledby="tab-reservations"
        tabIndex={-1}
      >
        <section className={`${sectionPadding} relative z-10`}>
          <div className="max-w-3xl mx-auto px-6">
            <SectionHeader
              title={t("businessPage.partnerLinks.reservationsTitle")}
              subtitle={t("businessPage.partnerLinks.reservationsDescription")}
              badge={t("businessPage.bookingBadge")}
              designSettings={designSettings}
              centered={false}
            />
            <ul className="divide-y divide-gray-200 border-y border-gray-200">
              {reservationPartnerLinks.map((link, index) => (
                <PartnerLinkRow
                  key={`${link.name}-${link.url}-${index}`}
                  link={withResolvedIcon(link)}
                  designSettings={designSettings}
                  ctaLabel={t("businessPage.partnerLinks.openPartner", {
                    name: link.name,
                  })}
                />
              ))}
            </ul>
          </div>
        </section>
      </div>
    );
  }

  return (
    <div
      role="tabpanel"
      id="tabpanel-reservations"
      aria-labelledby="tab-reservations"
      tabIndex={-1}
    />
  );
}
