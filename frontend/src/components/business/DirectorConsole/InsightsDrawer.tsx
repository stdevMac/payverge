"use client";

import React from "react";
import { Drawer, DrawerContent, DrawerBody } from "@nextui-org/react";
import { useReducedMotion } from "framer-motion";
import { X } from "lucide-react";

import type { Business } from "@/api/business";
import type { BriefingResponse } from "@/api/directorConsole";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";
import { sortInsightCardsBySeverity, toPreShiftCards } from "./insightCopy";
import BriefingContent from "./BriefingContent";
import PreShiftCard from "./PreShiftCard";

interface InsightsDrawerProps {
  open: boolean;
  onClose: () => void;
  briefing: BriefingResponse;
  business: Business;
  onOpenTab: (tab: string) => void;
  /** Last successful briefing fetch — venue-local "as of" stamp. */
  asOf?: Date | null;
}

/**
 * The full briefing, in a right slide-over (full-screen sheet on mobile). NextUI
 * Drawer provides the focus trap, Esc/backdrop close, and aria-modal semantics —
 * we reuse it rather than hand-rolling a dialog. The briefing prose + play + win
 * lead; every insight then renders grouped by severity via PreShiftCard.
 *
 * Tapping a card deep-links into the relevant dashboard tab AND closes the
 * drawer, so the operator lands on the surface they need to act in.
 */
export default function InsightsDrawer({
  open,
  onClose,
  briefing,
  business,
  onOpenTab,
  asOf = null,
}: InsightsDrawerProps) {
  const { locale } = useSimpleLocale();
  // NextUI's Drawer (ModalContent under the hood) drives its slide via a
  // framer-motion transform, which ignores prefers-reduced-motion — only
  // `disableAnimation` skips it. framer's default MotionConfig reducedMotion is
  // "never", so we read the media query ourselves and pass it through.
  const prefersReducedMotion = !!useReducedMotion();
  const currency = business.default_currency || "USD";
  const t = (key: string, params?: Record<string, string | number>): string => {
    const value = getTranslation(`directorConsole.${key}`, locale, params);
    return Array.isArray(value) ? value[0] || key : (value as string);
  };

  const cards = React.useMemo(
    () => sortInsightCardsBySeverity(toPreShiftCards(briefing.insights)),
    [briefing.insights],
  );

  const handleOpen = (tab: string) => {
    onOpenTab(tab);
    onClose();
  };

  return (
    <Drawer
      isOpen={open}
      onClose={onClose}
      placement="right"
      size="lg"
      // NextUI defaults to an animated slide; disableAnimation removes it
      // entirely for users who asked for reduced motion (see above).
      disableAnimation={prefersReducedMotion}
      classNames={{
        base: "bg-warm-50",
        closeButton: "text-ink-500 hover:bg-warm-100",
      }}
    >
      <DrawerContent data-testid="dc-insights-drawer" aria-label={t("strip.drawerTitle")}>
        {() => (
          <DrawerBody className="px-5 py-6">
            <div className="mb-2 flex items-center justify-between">
              <h2 className="font-title text-heading-md text-ink-900">
                {t("strip.drawerTitle")}
              </h2>
              <button
                type="button"
                data-testid="dc-drawer-close"
                onClick={onClose}
                aria-label={t("strip.close")}
                className="rounded-full p-1.5 text-ink-500 transition-colors hover:bg-warm-100 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand"
              >
                <X className="h-5 w-5" />
              </button>
            </div>

            <BriefingContent
              briefing={briefing}
              business={business}
              onOpenTab={handleOpen}
              showCards={false}
              asOf={asOf}
            />

            {cards.length > 0 ? (
              <div className="mt-6">
                <p className="text-label font-semibold uppercase tracking-wide text-ink-500">
                  {t("strip.insightsHeading")}
                </p>
                <div className="mt-3 flex flex-col gap-3">
                  {cards.map((card) => (
                    <PreShiftCard
                      key={card.id}
                      model={card}
                      onOpen={handleOpen}
                      currency={currency}
                    />
                  ))}
                </div>
              </div>
            ) : null}
          </DrawerBody>
        )}
      </DrawerContent>
    </Drawer>
  );
}
