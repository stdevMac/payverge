"use client";

import React from "react";
import type { Business } from "@/api/business";
import type {
  CampaignSuggestion,
  MarketingCreativeProfile,
} from "@/api/marketing";
import { PostCard } from "./PostCard";
import { useSuggestionCaption } from "./hooks/useSuggestionCaptions";
import type {
  MarketingCreativeHandoff,
  MarketingCreativeSelection,
} from "./ExportOutcomeDialog";

interface Props {
  suggestions: CampaignSuggestion[];
  business: Business;
  t: (key: string, params?: Record<string, string | number>) => string;
  onTweak?: (s: CampaignSuggestion) => void;
  onDismiss?: (s: CampaignSuggestion) => void;
  onHandoff?: (handoff: MarketingCreativeHandoff) => void;
  onMarkPosted?: (selection: MarketingCreativeSelection) => void;
  onMarkReady?: (selection: MarketingCreativeSelection) => void;
  onReadinessChange?: (suggestionId: string, ready: boolean) => void;
  /**
   * When set (live queue), cards are visually ordered ready-first via CSS
   * `order` under a single stable React parent. DOM order of cards never
   * changes, so PostCard state (preview readiness) is never remounted.
   */
  readinessBySuggestion?: Record<string, boolean>;
  captionLocale: string;
  creativeProfile?: MarketingCreativeProfile;
  captionsEnabled?: boolean;
  isExample?: boolean;
  canEdit?: boolean;
}

function CaptionedPostCard({
  suggestion,
  business,
  t,
  onTweak,
  onDismiss,
  onHandoff,
  onMarkPosted,
  onMarkReady,
  onReadinessChange,
  captionLocale,
  creativeProfile,
  captionsEnabled,
  priority,
  isExample,
  canEdit,
}: Omit<Props, "suggestions"> & {
  suggestion: CampaignSuggestion;
  priority: boolean;
}) {
  const captionResource = useSuggestionCaption({
    businessId: business.id,
    business,
    suggestion,
    locale: captionLocale,
    creativeProfile,
    enabled: !!captionsEnabled && !isExample,
    priority,
  });
  return (
    <div ref={captionResource.observeRef} className="min-h-full">
      <PostCard
        suggestion={suggestion}
        business={business}
        t={t}
        onTweak={onTweak}
        onDismiss={onDismiss}
        onHandoff={onHandoff}
        onMarkPosted={onMarkPosted}
        onMarkReady={onMarkReady}
        onReadinessChange={onReadinessChange}
        captionResource={captionResource}
        isExample={isExample}
        canEdit={canEdit}
        creativeProfile={creativeProfile}
      />
    </div>
  );
}

export function SuggestionFeed({
  suggestions,
  business,
  t,
  onTweak,
  onDismiss,
  onHandoff,
  onMarkPosted,
  onMarkReady,
  onReadinessChange,
  readinessBySuggestion,
  captionLocale,
  creativeProfile,
  captionsEnabled = false,
  isExample = false,
  canEdit = true,
}: Props) {
  const partitionLive = !isExample && readinessBySuggestion != null;
  const readyIds = React.useMemo(() => {
    if (!partitionLive || !readinessBySuggestion) return new Set<string>();
    return new Set(
      suggestions.filter((s) => readinessBySuggestion[s.id]).map((s) => s.id),
    );
  }, [partitionLive, readinessBySuggestion, suggestions]);
  const readyCount = readyIds.size;
  const needsCount = partitionLive ? suggestions.length - readyCount : 0;

  return (
    <div className="grid grid-cols-1 gap-6 xl:grid-cols-2 2xl:grid-cols-3">
      {partitionLive && readyCount > 0 ? (
        <div
          className="col-span-full"
          style={{ order: -2 }}
          data-testid="marketing-ready-group"
          role="group"
          aria-label={t("feed.readyGroup")}
        >
          <h3 className="text-sm font-semibold text-ink-900">
            {t("feed.readyGroup")}
          </h3>
        </div>
      ) : null}
      {partitionLive && needsCount > 0 && readyCount > 0 ? (
        <div
          className="col-span-full"
          style={{ order: 0 }}
          data-testid="marketing-needs-work-group"
          role="group"
          aria-label={t("feed.needsWorkGroup")}
        >
          <h3 className="text-sm font-semibold text-ink-900">
            {t("feed.needsWorkGroup")}
          </h3>
        </div>
      ) : partitionLive && needsCount > 0 ? (
        <div
          className="col-span-full sr-only"
          style={{ order: 0 }}
          data-testid="marketing-needs-work-group"
          role="group"
          aria-label={t("feed.needsWorkGroup")}
        />
      ) : null}
      {suggestions.map((s, index) => {
        const isReady = readyIds.has(s.id);
        return (
          <div
            key={s.id}
            data-testid={`marketing-card-${s.id}`}
            data-export-ready={isReady ? "true" : "false"}
            // Ready cards: order -1 (after ready heading -2). Needs work: order 1
            // (after needs heading 0). Card DOM order stays fixed for React state.
            style={partitionLive ? { order: isReady ? -1 : 1 } : undefined}
          >
            <CaptionedPostCard
              suggestion={s}
              business={business}
              t={t}
              onTweak={onTweak}
              onDismiss={onDismiss}
              onHandoff={onHandoff}
              onMarkPosted={onMarkPosted}
              onMarkReady={onMarkReady}
              onReadinessChange={onReadinessChange}
              captionLocale={captionLocale}
              creativeProfile={creativeProfile}
              captionsEnabled={captionsEnabled}
              priority={index === 0}
              isExample={isExample}
              canEdit={canEdit}
            />
          </div>
        );
      })}
    </div>
  );
}
