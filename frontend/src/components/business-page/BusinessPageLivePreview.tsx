"use client";

/**
 * Live storefront preview for the Business Page editor (#591).
 *
 * This mounts the REAL public page tree — `ConvertingBusinessLandingPage` in
 * `previewMode` — fed by the editor's in-progress draft state. There is no
 * second storefront implementation here to drift out of sync: announcement
 * bar, hero, sticky tab navigation, About / Menu / Delivery / Reservations /
 * Contact panels, footer and the language selector are the same components
 * `/b/<slug>` renders, wrapped in the same `storefront-theme` +
 * `StorefrontThemeStyle` scope so brand color / radius edits paint exactly as
 * they will for guests.
 *
 * Preview-only concessions (see `previewMode` on the landing page):
 *   - the storefront's public reads live in their OWN QueryClient so the
 *     preview never writes into the dashboard's query cache,
 *   - no URL/hash mutation, no document scroll or focus moves,
 *   - no AI waiter session, no analytics, no data mutations.
 *
 * Marked aria-hidden: this is a visual preview inside the operator dashboard
 * and must not contribute a second <h1> or tablist to the a11y tree.
 */

import React, { useMemo, useState } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { BusinessDesignSettings } from "@/api/business";
import { GuestTranslationProvider } from "@/i18n/GuestTranslationProvider";
import ConvertingBusinessLandingPage from "./ConvertingBusinessLandingPage";
import {
  buildPreviewStorefrontBusiness,
  resolvePreviewLanguage,
  type BusinessPageLivePreviewModel,
} from "./previewStorefrontBusiness";

export type { BusinessPageLivePreviewModel };

export type BusinessPageLivePreviewProps = {
  model: BusinessPageLivePreviewModel;
  designSettings: BusinessDesignSettings;
  /** Operator dashboard locale — used when the storefront has no default. */
  operatorLocale?: string;
  className?: string;
};

/**
 * Isolated cache: the preview's public storefront reads (menu, delivery and
 * reservation settings) must not collide with — or invalidate — the
 * dashboard's own queries for the same business.
 */
function createPreviewQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        refetchOnWindowFocus: false,
        staleTime: 60_000,
      },
    },
  });
}

export default function BusinessPageLivePreview({
  model,
  designSettings,
  operatorLocale,
  className,
}: BusinessPageLivePreviewProps) {
  const [queryClient] = useState(createPreviewQueryClient);

  const business = useMemo(
    () => buildPreviewStorefrontBusiness(model, designSettings),
    [model, designSettings],
  );

  const previewLanguage = resolvePreviewLanguage(
    model.defaultLanguage,
    operatorLocale,
  );

  return (
    <div
      data-testid="business-page-live-preview"
      aria-hidden="true"
      className={className}
    >
      <QueryClientProvider client={queryClient}>
        <GuestTranslationProvider
          initialLanguage={previewLanguage}
          preferInitialLanguage
        >
          <ConvertingBusinessLandingPage
            business={business}
            customUrl={model.customUrl || ""}
            seedLanguage={previewLanguage}
            previewMode
          />
        </GuestTranslationProvider>
      </QueryClientProvider>
    </div>
  );
}
