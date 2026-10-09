"use client";

import React from "react";
import DeliveryAvailableCard from "@/components/delivery/DeliveryAvailableCard";
import {
  GuestDeliveryEditorContext,
  GuestDeliveryQuoteContext,
} from "@/components/delivery/GuestDeliveryOrder";
import type { DeliverySettingsDto } from "@/api/delivery";
import SectionHeader from "./SectionHeader";
import PartnerLinkRow from "./PartnerLinkRow";
import { getSectionPadding } from "./designClasses";
import { withResolvedIcon, type ExternalPartnerLink } from "./partnerIconResolver";
import { isGuestFacingDeliveryNote } from "@/lib/deliveryGuestNote";

interface BusinessDeliveryTabProps {
  businessId: number;
  businessName: string;
  customUrl: string;
  deliveryEnabled: boolean;
  deliverySettings?: DeliverySettingsDto | null;
  deliveryLoading?: boolean;
  deliveryPartnerLinks: ExternalPartnerLink[];
  designSettings: any;
  onQuoteReady: (context: GuestDeliveryQuoteContext) => void;
  /** Business default currency (ISO 4217); falls back to USD. */
  currency?: string;
  /** Business address country, pre-filled into the delivery address form. */
  defaultCountry?: string;
  /** Closed Mode: when false, card must not claim delivery is available. */
  isBusinessOpen?: boolean;
  /** Saved guest delivery context — prefills the editor and labels a live quote. */
  fulfillmentContext?: GuestDeliveryEditorContext;
  /** Increment to open the address editor in one action from Edit. */
  openAddressEditorToken?: number;
  t: (key: string, params?: Record<string, string | number>) => string;
}

export default function BusinessDeliveryTab({
  businessId,
  businessName,
  customUrl: _customUrl,
  deliveryEnabled,
  deliverySettings,
  deliveryLoading,
  deliveryPartnerLinks,
  designSettings,
  onQuoteReady,
  currency = "USD",
  defaultCountry = "",
  isBusinessOpen,
  fulfillmentContext = null,
  openAddressEditorToken = 0,
  t,
}: BusinessDeliveryTabProps) {
  const sectionPadding = getSectionPadding(designSettings?.section_density);

  if (deliveryLoading) {
    return (
      <div role="tabpanel" id="tabpanel-delivery" aria-labelledby="tab-delivery" tabIndex={-1}>
        <section className={`${sectionPadding} relative z-10`}>
          <div className="max-w-6xl mx-auto px-6">
            <p className="text-sm text-gray-600">{t("businessPage.loadingAria")}</p>
          </div>
        </section>
      </div>
    );
  }

  if (deliveryEnabled) {
    return (
      <div role="tabpanel" id="tabpanel-delivery" aria-labelledby="tab-delivery" tabIndex={-1}>
        <section className={`${sectionPadding} relative z-10`}>
          <div className="max-w-6xl mx-auto px-6">
            <SectionHeader
              title={t("businessPage.deliveryService")}
              subtitle={t("businessPage.deliveryDescription")}
              badge={t("businessPage.deliveryTab")}
              designSettings={designSettings}
              centered={false}
            />
            <DeliveryAvailableCard
              businessId={businessId}
              businessName={businessName}
              deliverySettings={deliverySettings}
              loading={deliveryLoading}
              designSettings={designSettings}
              onQuoteReady={onQuoteReady}
              currency={currency}
              defaultCountry={defaultCountry}
              isBusinessOpen={isBusinessOpen}
              fulfillmentContext={fulfillmentContext}
              openAddressEditorToken={openAddressEditorToken}
            />

            {deliveryPartnerLinks.length > 0 && (
              <div className="mt-12 max-w-3xl">
                <p className="text-xs font-semibold uppercase tracking-[0.2em] text-gray-500 mb-2">
                  {t("businessPage.partnerLinks.fallbackHeading")}
                </p>
                <p className="text-sm text-gray-500 mb-4">
                  {t("businessPage.partnerLinks.fallbackBody")}
                </p>
                <ul className="divide-y divide-gray-200 border-y border-gray-200">
                  {deliveryPartnerLinks.map((link, index) => (
                    <PartnerLinkRow
                      key={`${link.name}-${link.url}-${index}`}
                      link={withResolvedIcon(link)}
                      designSettings={designSettings}
                      ctaLabel={t("businessPage.partnerLinks.openPartner", { name: link.name })}
                    />
                  ))}
                </ul>
              </div>
            )}
          </div>
        </section>
      </div>
    );
  }

  if (deliveryPartnerLinks.length > 0) {
    return (
      <div role="tabpanel" id="tabpanel-delivery" aria-labelledby="tab-delivery" tabIndex={-1}>
        <section className={`${sectionPadding} relative z-10`}>
          <div className="max-w-3xl mx-auto px-6">
            <SectionHeader
              title={t("businessPage.partnerLinks.deliveryTitle")}
              subtitle={t("businessPage.partnerLinks.deliveryDescription")}
              badge={t("businessPage.partnersBadge")}
              designSettings={designSettings}
              centered={false}
            />
            {isGuestFacingDeliveryNote(
              deliverySettings?.delivery_instructions,
            ) ? (
              <p dir="auto" className="mb-6 text-sm text-gray-700 whitespace-pre-line">
                {deliverySettings?.delivery_instructions}
              </p>
            ) : null}
            <ul className="divide-y divide-gray-200 border-y border-gray-200">
              {deliveryPartnerLinks.map((link, index) => (
                <PartnerLinkRow
                  key={`${link.name}-${link.url}-${index}`}
                  link={withResolvedIcon(link)}
                  designSettings={designSettings}
                  ctaLabel={t("businessPage.partnerLinks.openPartner", { name: link.name })}
                />
              ))}
            </ul>
          </div>
        </section>
      </div>
    );
  }

  return <div role="tabpanel" id="tabpanel-delivery" aria-labelledby="tab-delivery" tabIndex={-1} />;
}
