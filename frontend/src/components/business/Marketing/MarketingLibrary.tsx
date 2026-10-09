"use client";

import React from "react";
import {
  Button,
  DatePicker,
  Select,
  SelectItem,
  Spinner,
} from "@nextui-org/react";
import { parseDate, type DateValue } from "@internationalized/date";
import { Megaphone, RotateCcw, SearchX, WandSparkles } from "lucide-react";
import type { Business } from "@/api/business";
import {
  isMarketingPlay,
  type MarketingActivity,
  type MarketingActivityStatus,
  type MarketingCreativeSnapshot,
  type MarketingPlay,
} from "@/api/marketing";
import { StatusChip } from "@/components/ui/StatusChip";
import { EmptyState } from "@/components/ui/EmptyState";
import { useSimpleLocale } from "@/i18n/SimpleTranslationProvider";
import { intlLocaleFor } from "@/utils/intlLocale";
import { PremiumPanel } from "../premium";
import {
  ActivityDetailDrawer,
  ViewActivityButton,
} from "./ActivityDetailDrawer";
import {
  LIBRARY_THUMBNAIL_WIDTH,
  PostPreview,
} from "./PostPreview";
import { brandLogoUrl, resolveBrandPalette } from "./brandLock";
import { DEFAULT_CROP } from "./templates/renderPost";
import { TEMPLATES } from "./templates/templates";
import { useMarketingActivity } from "./hooks/useMarketingActivity";

function toDateString(value: DateValue | null): string {
  return value ? value.toString() : "";
}

function fromDateString(value: string): DateValue | null {
  if (!value) return null;
  try {
    return parseDate(value);
  } catch {
    return null;
  }
}

const PLAY_OPTIONS = [
  "happy_hour",
  "featured_dish",
  "move_item",
  "win_back",
  "combo_deal",
  "offer",
] as const;

// Single collection for the NextUI Select: an "all plays" sentinel followed by
// each play. Built as one array (mapped via the `items` prop) rather than a
// static <SelectItem> beside a mapped array, which NextUI's CollectionElement
// typing rejects (TS2322).
const PLAY_FILTER_OPTIONS = [
  { key: "__all__" },
  ...PLAY_OPTIONS.map((play) => ({ key: play })),
] as const;

interface Props {
  businessId: number | string;
  business?: Business;
  canEdit?: boolean;
  /** S2-E owner gate for approve. */
  canApprove?: boolean;
  onRestore?: (suggestionId: string) => void;
  restorePendingSuggestionId?: string | null;
  restoreFailedSuggestionId?: string | null;
  onReuse?: (
    activity: MarketingActivity,
    creative: MarketingCreativeSnapshot,
  ) => void;
  onApprove?: (activity: MarketingActivity) => void;
  t: (key: string, params?: Record<string, string | number>) => string;
}

function statusLabelKey(status: MarketingActivityStatus): string {
  switch (status) {
    case "posted":
      return "library.statusPosted";
    case "ready":
      return "library.statusReady";
    case "approved":
      return "library.statusApproved";
    case "dismissed":
    default:
      return "library.statusDismissed";
  }
}

/**
 * Old activity rows predate creative snapshots. These values reconstruct a
 * useful editor starting point; the UI labels them as defaults, never as the
 * historical creative that was posted.
 */
export function creativeForActivity(
  row: MarketingActivity,
): MarketingCreativeSnapshot {
  if (row.creative_snapshot) return row.creative_snapshot;
  return {
    caption: row.caption || "",
    image_url: row.image_url || "",
    image_source: "menu",
    template: "editorial",
    aspect: "4:5",
    slots: {
      dishName: row.title || row.target_name || "",
      price: "",
      badge: "",
      cta: "",
      handle: "",
    },
    crop: { ...DEFAULT_CROP },
    font_family: "Sans",
  };
}

export function MarketingLibrary({
  businessId,
  business,
  canEdit = false,
  canApprove = false,
  onRestore,
  restorePendingSuggestionId = null,
  restoreFailedSuggestionId = null,
  onReuse,
  onApprove,
  t,
}: Props) {
  const { locale } = useSimpleLocale();
  const [status, setStatus] = React.useState<MarketingActivityStatus | "">("");
  const [play, setPlay] = React.useState<MarketingPlay | "">("");
  const [from, setFrom] = React.useState("");
  const [to, setTo] = React.useState("");
  const {
    items,
    total,
    loading,
    loadingMore,
    error,
    hasMore,
    loadMore,
    refetch,
  } = useMarketingActivity(businessId, { status, play, from, to });
  const hasActiveFilters = Boolean(status || play || from || to);
  const [detailActivity, setDetailActivity] =
    React.useState<MarketingActivity | null>(null);
  const playLabel = (value: MarketingPlay | "unknown") =>
    value === "unknown" ? t("library.unknownPlay") : t(`plays.${value}`);
  const detailCreative = detailActivity
    ? creativeForActivity(detailActivity)
    : null;

  const statusChip = (key: MarketingActivityStatus | "", label: string) => (
    <Button
      key={label}
      size="sm"
      radius="full"
      variant={status === key ? "solid" : "flat"}
      onPress={() => setStatus(key)}
    >
      {label}
    </Button>
  );

  const listState = loading ? (
    <div
      className="flex items-center justify-center gap-2 px-5 py-10 text-sm text-ink-600"
      role="status"
      aria-live="polite"
    >
      <Spinner size="sm" />
      <span>{t("library.loading")}</span>
    </div>
  ) : error && items.length === 0 ? (
    <div className="space-y-3 px-5 py-10 text-center" role="alert">
      <p className="text-sm text-amber-800">{t("library.loadFailed")}</p>
      <Button size="sm" variant="flat" onPress={() => void refetch()}>
        {t("library.retry")}
      </Button>
    </div>
  ) : items.length === 0 ? (
    hasActiveFilters ? (
      <EmptyState
        compact
        icon={SearchX}
        title={t("library.filteredEmptyTitle")}
        subtitle={t("library.filteredEmpty")}
        className="px-5"
      />
    ) : (
      <EmptyState
        compact
        icon={Megaphone}
        title={t("library.emptyTitle")}
        subtitle={t("library.empty")}
        className="px-5"
      />
    )
  ) : (
    <>
      {error ? (
        <div
          className="flex flex-wrap items-center justify-between gap-2 border-b border-amber-200 bg-amber-50 px-5 py-3"
          role="alert"
        >
          <p className="text-sm text-amber-800">{t("library.loadFailed")}</p>
          <Button size="sm" variant="flat" onPress={() => void refetch()}>
            {t("library.retry")}
          </Button>
        </div>
      ) : null}
      <ul className="divide-y divide-warm-100">
        {items.map((row) => {
          const creative = creativeForActivity(row);
          const captured = row.creative_snapshot != null;
          const restorePending =
            restorePendingSuggestionId === row.suggestion_id;
          const restoreFailed = restoreFailedSuggestionId === row.suggestion_id;
          const palette = business
            ? resolveBrandPalette(business, {
                kit: creative.kit,
                storedFontFamily: creative.font_family,
              })
            : {
                primary: "",
                secondary: "",
                fontFamily: creative.font_family || "Sans",
              };
          const renderInput = {
            template: TEMPLATES[creative.template],
            aspect: creative.aspect,
            photoUrl: creative.image_url,
            slots: creative.slots,
            palette,
            // Re-apply live brand logo on Library thumbs (S1 brand lock).
            logoUrl: business ? brandLogoUrl(business) : undefined,
            crop: creative.crop,
          };

          return (
            <li
              key={row.id}
              className="grid gap-4 px-5 py-4 sm:grid-cols-[7rem_minmax(0,1fr)_auto] sm:items-start"
            >
              <div className="relative overflow-hidden rounded-xl border border-warm-200 bg-warm-50">
                <PostPreview
                  renderInput={renderInput}
                  isGenerating={false}
                  emptyLabel={t("editor.noPhoto")}
                  previewErrorLabel={t("preview.renderError")}
                  previewRetryLabel={t("preview.retry")}
                  renderingLabel={t("preview.rendering")}
                  generatingLabel={t("preview.generating")}
                  lazy
                  targetWidth={LIBRARY_THUMBNAIL_WIDTH}
                  cacheKey={`activity-${row.id}`}
                />
                {/*
                  Exact-match on media_kind: an unknown id from a newer backend
                  falls through to the still treatment rather than being badged
                  as something this build cannot play.
                */}
                {row.creative_snapshot?.media_kind === "video" && (
                  <span className="absolute left-2 top-2 rounded-full bg-ink-900/75 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-white">
                    {t("motion.libraryBadge")}
                  </span>
                )}
              </div>

              <div className="min-w-0 space-y-2">
                <div className="flex flex-wrap items-center gap-2">
                  <p className="line-clamp-2 min-w-0 break-words text-balance text-sm font-semibold text-ink-900">
                    {row.title || playLabel(row.play)}
                  </p>
                  <StatusChip
                    tone={
                      row.status === "posted" || row.status === "approved"
                        ? "success"
                        : row.status === "ready"
                          ? "info"
                          : "neutral"
                    }
                    label={t(statusLabelKey(row.status))}
                  />
                </div>
                <p className="line-clamp-1 min-w-0 break-words text-xs text-ink-500">
                  <span>{playLabel(row.play)}</span>
                  {row.target_name ? ` · ${row.target_name}` : ""}
                </p>
                {creative.caption ? (
                  <p className="line-clamp-2 text-sm leading-5 text-ink-700">
                    {creative.caption}
                  </p>
                ) : null}
                <div className="flex flex-wrap items-center gap-2 text-[11px] text-ink-500">
                  <span
                    className={
                      captured
                        ? "rounded-full bg-emerald-50 px-2 py-0.5 text-emerald-800"
                        : "rounded-full bg-amber-50 px-2 py-0.5 text-amber-800"
                    }
                  >
                    {t(
                      captured
                        ? "library.capturedPreview"
                        : "library.reconstructedPreview",
                    )}
                  </span>
                  <time>
                    {new Intl.DateTimeFormat(intlLocaleFor(locale), {
                      year: "numeric",
                      month: "short",
                      day: "numeric",
                    }).format(new Date(row.created_at))}
                  </time>
                </div>
              </div>

              <div className="flex flex-wrap gap-2 sm:flex-col">
                <ViewActivityButton
                  label={t("library.viewDetail")}
                  onPress={() => setDetailActivity(row)}
                />
                <span className="inline-flex items-center rounded-full border border-warm-200 bg-warm-50 px-2 py-0.5 text-[11px] font-medium text-ink-600">
                  {t(statusLabelKey(row.status))}
                </span>
                {canApprove &&
                onApprove &&
                row.status === "ready" &&
                row.creative_snapshot ? (
                  <Button
                    size="sm"
                    color="primary"
                    variant="flat"
                    radius="full"
                    data-testid="library-row-approve"
                    onPress={() => onApprove(row)}
                  >
                    {t("handoff.approve")}
                  </Button>
                ) : null}
                {canEdit && onReuse && row.play !== "unknown" ? (
                  <Button
                    size="sm"
                    variant="flat"
                    radius="full"
                    startContent={<WandSparkles className="h-3.5 w-3.5" />}
                    onPress={() => onReuse(row, creative)}
                  >
                    {t("library.reuse")}
                  </Button>
                ) : null}
                {canEdit && row.status === "dismissed" && onRestore ? (
                  restoreFailed ? (
                    <div
                      className="space-y-2 rounded-xl border border-amber-200 bg-amber-50 p-2 text-center"
                      role="alert"
                    >
                      <p className="text-xs font-medium text-amber-800">
                        {t("library.restoreFailed")}
                      </p>
                      <Button
                        size="sm"
                        variant="flat"
                        radius="full"
                        onPress={() => onRestore(row.suggestion_id)}
                      >
                        {t("library.retryRestore")}
                      </Button>
                    </div>
                  ) : (
                    <Button
                      size="sm"
                      variant="flat"
                      radius="full"
                      startContent={<RotateCcw className="h-3.5 w-3.5" />}
                      isLoading={restorePending}
                      isDisabled={restorePendingSuggestionId != null}
                      onPress={() => onRestore(row.suggestion_id)}
                    >
                      {t("library.restore")}
                    </Button>
                  )
                ) : null}
              </div>
            </li>
          );
        })}
      </ul>
    </>
  );

  return (
    <PremiumPanel
      tone="default"
      className="overflow-hidden p-0"
      withTexture={false}
    >
      <div className="flex flex-col gap-3 border-b border-warm-100 px-5 py-4">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div>
            <h2 className="text-base font-semibold text-ink-900">
              {t("library.heading")}
            </h2>
            <p className="text-sm text-ink-500">{t("library.subtitle")}</p>
          </div>
          <span className="text-xs font-semibold tabular-nums text-ink-600">
            {t(total === 1 ? "library.countOne" : "library.count", {
              count: total,
            })}
          </span>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {statusChip("", t("library.filterAll"))}
          {statusChip("ready", t("library.filterReady"))}
          {statusChip("approved", t("library.filterApproved"))}
          {statusChip("posted", t("library.filterPosted"))}
          {statusChip("dismissed", t("library.filterDismissed"))}
          <Select
            aria-label={t("library.filterPlay")}
            size="sm"
            radius="full"
            selectedKeys={[play || "__all__"]}
            className="w-full max-w-[10rem]"
            classNames={{
              trigger:
                "min-h-8 border border-warm-200 bg-warm-50 shadow-sm hover:border-brand/35 data-[open=true]:border-brand",
            }}
            onChange={(event) =>
              setPlay(
                isMarketingPlay(event.target.value) ? event.target.value : "",
              )
            }
            items={PLAY_FILTER_OPTIONS}
          >
            {(option) => (
              <SelectItem key={option.key}>
                {option.key === "__all__"
                  ? t("library.allPlays")
                  : t(`plays.${option.key}`)}
              </SelectItem>
            )}
          </Select>
          <DatePicker
            aria-label={t("library.filterFrom")}
            label={t("library.filterFrom")}
            value={fromDateString(from)}
            maxValue={to ? fromDateString(to) ?? undefined : undefined}
            onChange={(value) => setFrom(toDateString(value))}
            granularity="day"
            size="sm"
            className="w-full max-w-[11rem]"
          />
          <DatePicker
            aria-label={t("library.filterTo")}
            label={t("library.filterTo")}
            value={fromDateString(to)}
            minValue={from ? fromDateString(from) ?? undefined : undefined}
            onChange={(value) => setTo(toDateString(value))}
            granularity="day"
            size="sm"
            className="w-full max-w-[11rem]"
          />
        </div>
      </div>

      {listState}

      {hasMore ? (
        <div className="border-t border-warm-100 px-5 py-3 text-center">
          <Button
            size="sm"
            variant="flat"
            radius="full"
            aria-label={t(
              loadingMore ? "library.loadingMore" : "library.loadMore",
            )}
            isLoading={loadingMore}
            isDisabled={loadingMore}
            onPress={() => void loadMore()}
          >
            {t(loadingMore ? "library.loadingMore" : "library.loadMore")}
          </Button>
        </div>
      ) : null}

      <ActivityDetailDrawer
        isOpen={detailActivity != null}
        onClose={() => setDetailActivity(null)}
        activity={detailActivity}
        creative={detailCreative}
        business={business}
        canEdit={canEdit}
        canApprove={canApprove}
        onReuse={onReuse}
        onApprove={onApprove}
        t={t}
        playLabel={playLabel}
      />
    </PremiumPanel>
  );
}
