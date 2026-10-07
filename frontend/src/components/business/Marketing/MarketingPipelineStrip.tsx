"use client";

import React from "react";
import {
  ArrowRight,
  Camera,
  MessageSquareText,
  Share2,
  Sparkles,
} from "lucide-react";
import { PremiumPanel } from "../premium";
import IconTile from "@/components/ui/IconTile";

interface Props {
  t: (key: string, params?: Record<string, string | number>) => string;
  conceptsCount: number;
  readyCount: number;
  handledCount: number;
  isExample: boolean;
}

const STEP_ICONS = [Camera, MessageSquareText, Share2] as const;
const STEP_KEYS = [
  "hero.stepPhoto",
  "hero.stepCaption",
  "hero.stepPost",
] as const;

export function MarketingPipelineStrip({
  t,
  conceptsCount,
  readyCount,
  handledCount,
  isExample,
}: Props) {
  return (
    <PremiumPanel
      tone="accent"
      className="overflow-hidden p-0"
      withTexture={false}
    >
      <div className="grid gap-0 lg:grid-cols-[minmax(0,1fr)_auto]">
        <div className="border-b border-brand/10 p-5 sm:p-6 lg:border-b-0 lg:border-r">
          <div className="flex items-start gap-3">
            <IconTile icon={Sparkles} size="lg" />
            <div className="min-w-0 space-y-1">
              <p className="text-xs font-semibold uppercase tracking-[0.14em] text-brand-700">
                {t("hero.eyebrow")}
              </p>
              <p className="text-sm leading-6 text-ink-700">
                {isExample ? t("example.subtitle") : t("feed.subtitle")}
              </p>
            </div>
          </div>

          <div className="mt-5 flex flex-wrap items-center gap-2">
            {STEP_KEYS.map((key, index) => {
              const Icon = STEP_ICONS[index];
              const isLast = index === STEP_KEYS.length - 1;
              return (
                <React.Fragment key={key}>
                  <div className="inline-flex items-center gap-2 rounded-full border border-white/80 bg-white/90 px-3 py-1.5 text-xs font-medium text-ink-700 shadow-sm">
                    <Icon
                      className="h-3.5 w-3.5 text-brand"
                      aria-hidden="true"
                    />
                    {t(key)}
                  </div>
                  {!isLast ? (
                    <ArrowRight
                      className="hidden h-3.5 w-3.5 text-brand/50 sm:block"
                      aria-hidden="true"
                    />
                  ) : null}
                </React.Fragment>
              );
            })}
          </div>
        </div>

        <div className="grid grid-cols-2 divide-x divide-brand/10 bg-white/50 sm:grid-cols-3 lg:w-[min(100%,320px)] lg:grid-cols-1 lg:divide-x-0 lg:divide-y">
          <div className="px-4 py-4 sm:px-5">
            <p className="text-[11px] font-semibold uppercase tracking-[0.12em] text-ink-500">
              {t("hero.ideas")}
            </p>
            <p className="mt-1 font-title text-2xl tabular-nums text-ink-900">
              {conceptsCount}
            </p>
          </div>
          <div className="px-4 py-4 sm:px-5">
            <p className="text-[11px] font-semibold uppercase tracking-[0.12em] text-ink-500">
              {t("hero.ready")}
            </p>
            <p
              className="mt-1 font-title text-2xl tabular-nums text-ink-900"
              data-testid="marketing-ready-count"
            >
              {readyCount}
            </p>
          </div>
          <div className="col-span-2 border-t border-brand/10 px-4 py-4 sm:col-span-1 sm:border-t-0 sm:px-5 lg:border-t">
            <p className="text-[11px] font-semibold uppercase tracking-[0.12em] text-ink-500">
              {t("hero.handledRecently")}
            </p>
            <p
              className="mt-1 font-title text-2xl tabular-nums text-ink-900"
              data-testid="marketing-handled-count"
            >
              {handledCount}
            </p>
          </div>
        </div>
      </div>
    </PremiumPanel>
  );
}
