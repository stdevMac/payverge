"use client";

import React from "react";
import { useInstance } from "@/hooks/useInstance";
import { Button, useDisclosure } from "@nextui-org/react";
import { CheckCircle2, Inbox, RefreshCw, Settings2, Wand2 } from "lucide-react";
import toast from "react-hot-toast";
import { StatusChip } from "@/components/ui/StatusChip";
import { EmptyState } from "@/components/ui/EmptyState";
import { btnPrimaryNextUI, btnSecondaryNextUI } from "@/components/ui/buttonStyles";
import type { Business } from "@/api/business";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { getSafeApiErrorMessage } from "@/utils/apiError";
import DashboardLockedTabView from "../DashboardLockedTabView";
import { AiProviderNotice } from "../AiProviderNotice";
import { PremiumPanel } from "../premium";
import { MarketingSkeleton } from "./MarketingSkeleton";
import { MarketingPipelineStrip } from "./MarketingPipelineStrip";
import { SuggestionFeed } from "./SuggestionFeed";
import { PostEditorDrawer } from "./PostEditorDrawer";
import { buildExampleSuggestions } from "./exampleSuggestions";
import { isHonestHappyHourSuggestion } from "./postContent";
import { useMarketingSuggestions } from "./hooks/useMarketingSuggestions";
import {
  useMarketingActivity,
  useMarketingActivityMutations,
} from "./hooks/useMarketingActivity";
import { useMarketingSettings } from "./hooks/useMarketingSettings";
import { MarketingLibrary } from "./MarketingLibrary";
import { PhotoReadinessPanel } from "./PhotoReadinessPanel";
import { MarketingSettingsDrawer } from "./MarketingSettingsDrawer";
import { brandHandle, brandLogoUrl } from "./brandLock";
import {
  ExportOutcomeDialog,
  type MarketingCreativeHandoff,
  type MarketingCreativeSelection,
} from "./ExportOutcomeDialog";
import { getMarketingCapabilities } from "./marketingCapabilities";
import { useAuth } from "@/providers/HybridAuthProvider";
import { useStaffPermissionsContext } from "@/contexts/StaffPermissionsContext";
import type {
  CampaignSuggestion,
  MarketingActivity,
  MarketingActivitySnapshot,
  MarketingCreativeSnapshot,
} from "@/api/marketing";
import DashboardTabShell from "../shared/DashboardTabShell";
import { applyGeneratedImageDrafts } from "./generatedImageAcceptance";

interface Props {
  business: Business;
  /** Dashboard cross-tab seam for photo QA → Menu Builder. */
  onNavigateToTab?: (tab: string) => void;
}

function MarketingFeedHeader({
  isExample,
  count,
  readyCount,
  t,
  onRefresh,
  isRefreshing,
}: {
  isExample: boolean;
  count: number;
  readyCount?: number;
  t: (key: string, params?: Record<string, string | number>) => string;
  onRefresh?: () => void;
  isRefreshing?: boolean;
}) {
  return (
    <div className="flex flex-col gap-3 border-b border-warm-100 px-5 py-4 sm:flex-row sm:items-center sm:justify-between">
      <div className="space-y-1">
        <div className="flex flex-wrap items-center gap-2">
          <h2 className="text-base font-semibold text-ink-900">
            {isExample ? t("example.heading") : t("feed.heading")}
          </h2>
          <StatusChip
            tone={isExample ? "warn" : "info"}
            label={isExample ? t("feed.exampleBadge") : t("feed.liveBadge")}
          />
        </div>
        {!isExample && readyCount != null && readyCount > 0 ? (
          <p className="text-sm text-ink-500">
            {t(readyCount === 1 ? "feed.readyCountOne" : "feed.readyCount", {
              count: readyCount,
            })}
          </p>
        ) : null}
      </div>
      <div className="flex flex-wrap items-center gap-2">
        {!isExample && onRefresh && (
          <Button
            size="sm"
            variant="flat"
            radius="full"
            className={btnSecondaryNextUI}
            startContent={
              <RefreshCw
                className={`h-3.5 w-3.5 ${isRefreshing ? "animate-spin" : ""}`}
              />
            }
            onPress={onRefresh}
            isDisabled={isRefreshing}
          >
            {t("feed.refresh")}
          </Button>
        )}
        <span className="inline-flex items-center rounded-full border border-warm-200 bg-warm-50 px-3 py-1.5 text-xs font-semibold tabular-nums text-ink-700">
          {t(count === 1 ? "feed.countOne" : "feed.count", { count })}
        </span>
      </div>
    </div>
  );
}

export default function MarketingDashboard({
  business,
  onNavigateToTab,
}: Props) {
  const { locale } = useSimpleLocale();
  const { staffData } = useAuth();
  const { permissions } = useStaffPermissionsContext();
  const baseCapabilities = getMarketingCapabilities(permissions, !staffData);
  // AI off on this install: hide the generate/regenerate actions instead of
  // offering calls the backend refuses.
  const { isOff: instanceFeatureOff } = useInstance();
  const marketingCapabilities = {
    ...baseCapabilities,
    canGenerate: baseCapabilities.canGenerate && !instanceFeatureOff("ai"),
  };
  const t = React.useCallback(
    (key: string, params?: Record<string, string | number>) => {
      const r = getTranslation(`marketingDashboard.${key}`, locale, params);
      return Array.isArray(r) ? r[0] || key : (r as string);
    },
    [locale],
  );

  const {
    hasAccess,
    aiConfigured,
    loading: accessLoading,
  } = useBusinessAccess(business.id);
  const marketingEnabled = !accessLoading && hasAccess;
  const recentActivityFrom = React.useMemo(() => {
    const from = new Date();
    from.setUTCDate(from.getUTCDate() - 30);
    return from.toISOString();
  }, []);
  // Count-only: request a single row so we don't hydrate creative_snapshot
  // blobs just to read `total` for the KPI strip.
  const { total: handledRecentlyCount } = useMarketingActivity(
    business.id,
    { handled_from: recentActivityFrom },
    marketingEnabled,
    1,
  );
  const {
    suggestions: rawSuggestions,
    paused,
    emptyReason,
    inventoryBlocked = [],
    loading: suggestionsLoading,
    error: suggestionsError,
    refetch: refetchSuggestions,
  } = useMarketingSuggestions(business.id, marketingEnabled);
  const {
    settings,
    hasSnapshot: hasSettingsSnapshot,
    loading: settingsLoading,
    retryingLoad: settingsRetryingLoad,
    saving: settingsSaving,
    loadError: settingsLoadError,
    saveError: settingsSaveError,
    rollbackOccurred: settingsRollbackOccurred,
    saved: settingsSaved,
    save: saveSettings,
    retryLoad: retrySettingsLoad,
    retrySave: retrySettingsSave,
  } = useMarketingSettings(business.id, marketingEnabled);
  // Optimistic hide + rollback ownership lives in the mutation hook so a failed
  // record un-hides the card AND surfaces a toast instead of silently dropping
  // it (only to have it reappear on the next refetch, never reaching Library).
  const { record, restore, locallyHidden } = useMarketingActivityMutations(
    business.id,
    (vars) =>
      toast.error(
        t(
          vars.action === "post"
            ? "errors.mark_posted_failed"
            : vars.action === "mark_ready"
              ? "errors.mark_ready_failed"
              : vars.action === "approve"
                ? "errors.approve_failed"
                : "errors.dismiss_failed",
        ),
      ),
    refetchSuggestions,
  );

  const toSnapshot = (s: CampaignSuggestion): MarketingActivitySnapshot => ({
    id: s.id,
    play: s.play,
    title: s.title,
    target_name: s.target_name ?? "",
    image_url: s.image_url ?? "",
    caption: "",
  });
  const handleDismiss = (s: CampaignSuggestion) => {
    record.mutate({ action: "dismiss", suggestion: toSnapshot(s) });
  };
  const handleMarkPosted = (selection: MarketingCreativeSelection) => {
    record.mutate({
      action: "post",
      suggestion: toSnapshot(selection.suggestion),
      creative_snapshot: selection.creative,
    });
  };
  const handleMarkReady = (selection: MarketingCreativeSelection) => {
    record.mutate(
      {
        action: "mark_ready",
        suggestion: toSnapshot(selection.suggestion),
        creative_snapshot: selection.creative,
      },
      {
        onSuccess: () => toast.success(t("handoff.markReadySuccess")),
      },
    );
  };
  const handleApproveActivity = (activity: MarketingActivity) => {
    if (!marketingCapabilities.canApprove) return;
    if (activity.play === "unknown") return;
    record.mutate(
      {
        action: "approve",
        suggestion: {
          id: activity.suggestion_id,
          play: activity.play,
          title: activity.title,
          target_name: activity.target_name,
          image_url: activity.image_url,
          caption: activity.caption,
          creative_snapshot: activity.creative_snapshot,
        },
        ...(activity.creative_snapshot
          ? { creative_snapshot: activity.creative_snapshot }
          : {}),
      },
      {
        onSuccess: () => toast.success(t("handoff.approveSuccess")),
      },
    );
  };
  const visibleSuggestions = React.useMemo(
    () =>
      applyGeneratedImageDrafts(
        rawSuggestions.filter(
          (s) =>
            !(locallyHidden?.has(s.id) ?? false) &&
            isHonestHappyHourSuggestion(s),
        ),
        String(business.id),
      ),
    [rawSuggestions, locallyHidden, business.id],
  );

  const exampleSuggestions = React.useMemo(
    () => buildExampleSuggestions(t),
    [t],
  );

  const showingExamples =
    !suggestionsLoading &&
    !suggestionsError &&
    !paused &&
    emptyReason === "no_data" &&
    visibleSuggestions.length === 0 &&
    rawSuggestions.length === 0;
  const conceptsCount = showingExamples
    ? exampleSuggestions.length
    : visibleSuggestions.length;

  const [readinessBySuggestion, setReadinessBySuggestion] = React.useState<
    Record<string, boolean>
  >({});
  const handleReadinessChange = React.useCallback(
    (suggestionId: string, ready: boolean) => {
      setReadinessBySuggestion((current) =>
        current[suggestionId] === ready
          ? current
          : { ...current, [suggestionId]: ready },
      );
    },
    [],
  );
  const readyCount = React.useMemo(() => {
    if (showingExamples) return 0;
    return visibleSuggestions.filter((suggestion) =>
      Boolean(readinessBySuggestion[suggestion.id]),
    ).length;
  }, [visibleSuggestions, showingExamples, readinessBySuggestion]);

  const settingsDrawer = useDisclosure();
  const [drawerOpen, setDrawerOpen] = React.useState(false);
  const [activeSuggestion, setActiveSuggestion] =
    React.useState<CampaignSuggestion | null>(null);
  const [activeCreative, setActiveCreative] =
    React.useState<MarketingCreativeSnapshot | null>(null);
  const [editorSession, setEditorSession] = React.useState(0);
  const [isRefreshing, setIsRefreshing] = React.useState(false);
  const [handoff, setHandoff] = React.useState<MarketingCreativeHandoff | null>(
    null,
  );
  const [handoffError, setHandoffError] = React.useState<string | null>(null);

  const openEditor = (s: CampaignSuggestion | null) => {
    if (!marketingCapabilities.canEdit) return;
    setActiveSuggestion(s);
    setActiveCreative(null);
    setEditorSession((session) => session + 1);
    setDrawerOpen(true);
  };

  const reuseActivity = React.useCallback(
    (activity: MarketingActivity, creative: MarketingCreativeSnapshot) => {
      if (!marketingCapabilities.canEdit || activity.play === "unknown") return;
      setActiveSuggestion({
        id: activity.suggestion_id,
        play: activity.play,
        play_key: activity.play,
        title: activity.title,
        why_data: activity.target_name || activity.title,
        source: "activity",
        target_name: activity.target_name,
        copy_angle: creative.caption,
        rank: 0,
        image_url: creative.image_url,
      });
      setActiveCreative(creative);
      setEditorSession((session) => session + 1);
      setDrawerOpen(true);
    },
    [marketingCapabilities.canEdit],
  );

  const handleRefresh = async () => {
    setIsRefreshing(true);
    try {
      await refetchSuggestions();
    } finally {
      setIsRefreshing(false);
    }
  };

  const handleHandoff = React.useCallback(
    (nextHandoff: MarketingCreativeHandoff) => {
      setHandoff(nextHandoff);
      setHandoffError(null);
      setDrawerOpen(false);
    },
    [],
  );

  const confirmPublished = React.useCallback(
    (confirmedHandoff: MarketingCreativeHandoff) => {
      setHandoffError(null);
      const channel = confirmedHandoff.postedChannel?.trim();
      // Persist freeform channel on the snapshot so Director / Library can show it.
      const creative: MarketingCreativeSnapshot = channel
        ? { ...confirmedHandoff.creative, posted_channel: channel }
        : confirmedHandoff.creative;
      record.mutate(
        {
          action: "post",
          suggestion: toSnapshot(confirmedHandoff.suggestion),
          creative_snapshot: creative,
          ...(channel ? { posted_channel: channel } : {}),
        },
        {
          onSuccess: () => {
            setHandoff(null);
            setHandoffError(null);
          },
          onError: (error) => {
            // FIND-058: safe product copy; handoff UI must not show dumps.
            setHandoffError(getSafeApiErrorMessage(error, "record_failed"));
          },
        },
      );
    },
    [record],
  );

  const closeHandoff = React.useCallback(() => {
    setHandoff(null);
    setHandoffError(null);
  }, []);

  const feedSection = suggestionsLoading ? (
    <MarketingSkeleton showHero={false} showPipeline={false} />
  ) : suggestionsError ? (
    <div className="space-y-3 px-5 py-10 text-center">
      <p className="text-sm text-amber-800" role="alert">
        {t("errors.suggestions_failed")}
      </p>
      <Button
        size="sm"
        variant="flat"
        onPress={handleRefresh}
        isLoading={isRefreshing}
      >
        {t("feed.retry")}
      </Button>
    </div>
  ) : visibleSuggestions.length === 0 && rawSuggestions.length > 0 ? (
    <>
      <MarketingFeedHeader
        isExample={false}
        count={0}
        t={t}
        onRefresh={handleRefresh}
        isRefreshing={isRefreshing}
      />
      <EmptyState
        compact
        icon={Inbox}
        title={t("feed.allDismissedTitle")}
        subtitle={t("feed.allDismissed")}
        className="px-5"
      />
    </>
  ) : !showingExamples && visibleSuggestions.length === 0 ? (
    <>
      <MarketingFeedHeader isExample={false} count={0} t={t} />
      {emptyReason === "no_enabled_plays" ? (
        <EmptyState
          compact
          icon={Settings2}
          title={t("feed.noEnabledPlaysTitle")}
          subtitle={t("feed.noEnabledPlays")}
          className="px-5"
        />
      ) : (
        <EmptyState
          compact
          icon={CheckCircle2}
          title={t("feed.allHandledTitle")}
          subtitle={t("feed.allHandled")}
          className="px-5"
        />
      )}
    </>
  ) : (
    <>
      <MarketingFeedHeader
        isExample={showingExamples}
        count={conceptsCount}
        readyCount={readyCount}
        t={t}
        onRefresh={showingExamples ? undefined : handleRefresh}
        isRefreshing={isRefreshing}
      />
      <div className="p-5">
        {!showingExamples && inventoryBlocked.length > 0 ? (
          <div
            className="mb-4 rounded-xl border border-amber-200 bg-amber-50 px-3 py-2.5"
            role="status"
            data-testid="marketing-inventory-blocked"
          >
            <p className="text-xs font-semibold uppercase tracking-[0.12em] text-amber-800">
              {t("feed.inventoryBlockedTitle")}
            </p>
            <p className="mt-1 text-sm text-amber-900">
              {inventoryBlocked.length === 1
                ? t("feed.inventoryBlockedOne", {
                    name: inventoryBlocked[0],
                    reason: t("feed.inventoryBlockedReason"),
                  })
                : t("feed.inventoryBlocked", {
                    names: inventoryBlocked.join(", "),
                    reason: t("feed.inventoryBlockedReason"),
                  })}
            </p>
          </div>
        ) : null}
        <SuggestionFeed
          suggestions={
            showingExamples ? exampleSuggestions : visibleSuggestions
          }
          business={business}
          t={t}
          onTweak={
            marketingCapabilities.canEdit
              ? showingExamples
                ? // #831: examples are placeholders — "Make it yours" starts a
                  // blank post instead of prefilling a dish this venue may not
                  // serve.
                  () => openEditor(null)
                : openEditor
              : undefined
          }
          onDismiss={
            !showingExamples && marketingCapabilities.canEdit
              ? handleDismiss
              : undefined
          }
          onHandoff={marketingCapabilities.canEdit ? handleHandoff : undefined}
          onMarkPosted={
            !showingExamples && marketingCapabilities.canEdit
              ? handleMarkPosted
              : undefined
          }
          onMarkReady={
            !showingExamples && marketingCapabilities.canEdit
              ? handleMarkReady
              : undefined
          }
          onReadinessChange={handleReadinessChange}
          readinessBySuggestion={
            showingExamples ? undefined : readinessBySuggestion
          }
          canEdit={marketingCapabilities.canEdit}
          captionLocale={locale || business.default_language || "en"}
          creativeProfile={settings.creative_profile}
          captionsEnabled={
            marketingEnabled &&
            aiConfigured &&
            !showingExamples &&
            hasSettingsSnapshot &&
            !settingsSaving &&
            marketingCapabilities.canGenerate
          }
          isExample={showingExamples}
        />
      </div>
    </>
  );

  return (
    <DashboardTabShell
      loading={accessLoading ? <MarketingSkeleton /> : null}
      locked={
        !accessLoading && !hasAccess ? (
          <DashboardLockedTabView
            title={t("title")}
            subtitle={t("subtitle")}
            businessId={business.id}
          />
        ) : null
      }
      header={{
        title: t("title"),
        subtitle: t("subtitle"),
        dense: true,
        actions: (
          <div className="flex items-center gap-2">
            <Button
              size="sm"
              variant="flat"
              radius="full"
              className={btnSecondaryNextUI}
              startContent={<Settings2 className="h-3.5 w-3.5" />}
              onPress={settingsDrawer.onOpen}
            >
              {t("automation.button")}
            </Button>
            {marketingCapabilities.canEdit ? (
              <Button
                radius="full"
                className={btnPrimaryNextUI}
                startContent={<Wand2 className="h-4 w-4" />}
                onPress={() => openEditor(null)}
              >
                {t("hero.cta")}
              </Button>
            ) : null}
          </div>
        ),
      }}
    >
      <AiProviderNotice businessId={business.id} />

      {!marketingCapabilities.canEdit && marketingCapabilities.canView ? (
        <p className="rounded-xl border border-warm-200 bg-warm-50 px-4 py-3 text-sm text-ink-600">
          {t("feed.readOnly")}
        </p>
      ) : null}

      {!paused ? (
        <MarketingPipelineStrip
          t={t}
          conceptsCount={conceptsCount}
          readyCount={readyCount}
          handledCount={handledRecentlyCount}
          isExample={showingExamples}
        />
      ) : null}

      {paused ? (
        <PremiumPanel tone="default" className="p-8 text-center">
          <p className="text-base font-semibold text-ink-900">
            {t("paused.heading")}
          </p>
          <p className="mx-auto mt-1 max-w-md text-sm text-ink-500">
            {t("paused.body")}
          </p>
          {marketingCapabilities.canEdit ? (
            <Button
              className={`${btnPrimaryNextUI} mt-4`}
              radius="full"
              isLoading={settingsSaving}
              onPress={() => saveSettings({ ...settings, enabled: true })}
            >
              {t("paused.resume")}
            </Button>
          ) : null}
        </PremiumPanel>
      ) : (
        <PremiumPanel
          tone="default"
          className="overflow-hidden p-0"
          withTexture={false}
        >
          {feedSection}
        </PremiumPanel>
      )}

      {!paused && marketingCapabilities.canEdit ? (
        <PhotoReadinessPanel
          business={business}
          t={t}
          onOpenMenuItem={
            onNavigateToTab
              ? (itemName) =>
                  onNavigateToTab(
                    `menu?menuSearch=${encodeURIComponent(itemName)}`,
                  )
              : undefined
          }
        />
      ) : null}

      <MarketingLibrary
        businessId={business.id}
        business={business}
        canEdit={marketingCapabilities.canEdit}
        canApprove={marketingCapabilities.canApprove}
        onRestore={(suggestionId) => restore.mutate(suggestionId)}
        restorePendingSuggestionId={
          restore.isPending ? restore.variables : null
        }
        restoreFailedSuggestionId={restore.isError ? restore.variables : null}
        onReuse={reuseActivity}
        onApprove={handleApproveActivity}
        t={t}
      />

      <PostEditorDrawer
        key={`${activeSuggestion?.id ?? "manual"}:${editorSession}`}
        isOpen={drawerOpen}
        onClose={() => setDrawerOpen(false)}
        business={business}
        suggestion={activeSuggestion}
        initialCreative={activeCreative}
        creativeProfile={settings.creative_profile}
        canEdit={marketingCapabilities.canEdit}
        canGenerate={marketingCapabilities.canGenerate}
        canUpload={marketingCapabilities.canUpload}
        onHandoff={handleHandoff}
        t={t}
      />

      <MarketingSettingsDrawer
        isOpen={settingsDrawer.isOpen}
        onClose={settingsDrawer.onClose}
        settings={settings}
        hasSnapshot={hasSettingsSnapshot}
        loading={settingsLoading}
        canEdit={marketingCapabilities.canEdit}
        saving={settingsSaving}
        loadError={settingsLoadError}
        saveError={settingsSaveError}
        rollbackOccurred={settingsRollbackOccurred}
        saved={settingsSaved}
        retryingLoad={settingsRetryingLoad}
        onSave={saveSettings}
        onRetryLoad={retrySettingsLoad}
        onRetrySave={retrySettingsSave}
        t={t}
        brandPreview={{
          logoUrl: brandLogoUrl(business),
          primaryColor: business.design_settings?.primary_color,
          secondaryColor: business.design_settings?.secondary_color,
          handle: brandHandle(business),
        }}
      />

      <ExportOutcomeDialog
        handoff={handoff}
        onNotYet={closeHandoff}
        onConfirmPublished={confirmPublished}
        error={handoffError}
        onRetry={() => handoff && confirmPublished(handoff)}
        isConfirming={record.isPending}
        t={t}
      />
    </DashboardTabShell>
  );
}
