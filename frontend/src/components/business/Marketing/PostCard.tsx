"use client";

import React from "react";
import {
  Button,
  Dropdown,
  DropdownItem,
  DropdownMenu,
  DropdownTrigger,
} from "@nextui-org/react";
import {
  CheckCircle2,
  ChevronDown,
  ChevronUp,
  Copy,
  Download,
  ImagePlus,
  MoreHorizontal,
  Share2,
  Sparkles,
  Wand2,
  X,
} from "lucide-react";
import type { Business } from "@/api/business";
import type {
  CampaignSuggestion,
  MarketingCreativeProfile,
  MarketingCreativeSnapshot,
  MarketingImageSource,
} from "@/api/marketing";
import { normalizeMarketingRenderFontFamily } from "@/api/marketing";
import { intlLocaleFor } from "@/utils/intlLocale";
import toast from "react-hot-toast";
import { StatusChip } from "@/components/ui/StatusChip";
import { PLATFORM_CONTENT_BOUNDS } from "./templates/types";
import { PostPreview, type PreviewRenderState } from "./PostPreview";
import { PlatformPreviewFrame, platformLabelKey } from "./PlatformPreviewFrame";
import {
  buildSlots,
  buildPostFilename,
  composeLocalizedFallbackCaption,
  copyCaption,
  defaultTemplateForPlay,
  downloadPostPack,
  shareOrDownloadPostPack,
} from "./postContent";
import { resolveGuestLink } from "./guestLinks";
import {
  brandLogoUrl,
  ctaLabelForBrand,
  resolveBrandPalette,
  seedKitForCreative,
} from "./brandLock";
import type { CaptionResource } from "./hooks/useSuggestionCaptions";
import {
  btnGhostIcon,
  btnPrimaryNextUI,
  btnSecondaryNextUI,
} from "@/components/ui/buttonStyles";
import { PremiumPanel } from "../premium";
import { DEFAULT_CROP, type RenderPostInput } from "./templates/renderPost";
import type {
  MarketingCreativeHandoff,
  MarketingCreativeSelection,
} from "./ExportOutcomeDialog";
import { buildWhyExplain, hasWhyExplain } from "./whyExplain";
import {
  exportBlockedReason,
  exportBlockedReasonKey,
  isExportReady,
} from "./exportReadiness";
import { photoQualityGateForUrl } from "./photoQualityGate";
import { browserReadablePhotoUrl } from "./generatedImageAcceptance";

const CARD_ASPECT = "4:5" as const;
/** Collapsed factor list shows this many rows before "show more". */
const WHY_COLLAPSED_ROWS = 2;

const PLAY_TONE: Record<
  CampaignSuggestion["play"],
  "info" | "success" | "warn" | "neutral"
> = {
  happy_hour: "warn",
  featured_dish: "success",
  move_item: "info",
  win_back: "neutral",
  combo_deal: "info",
  offer: "warn",
};

/** Resolve suggestion card title from play_key (+ params) when present. */
export function localizedSuggestionTitle(
  suggestion: CampaignSuggestion,
  t: (key: string, params?: Record<string, string | number>) => string,
): string {
  const playKey = suggestion.play_key || suggestion.play;
  if (!playKey) return suggestion.title;
  const name = suggestion.target_name || "";
  const count =
    typeof suggestion.metrics?.marketable_lapsed === "number"
      ? suggestion.metrics.marketable_lapsed
      : "";
  switch (playKey) {
    case "featured_dish":
      return t("playTitles.featured_dish", { name });
    case "move_item":
      return t("playTitles.move_item", { name });
    case "happy_hour":
      return suggestion.daypart_key
        ? t("playTitles.happy_hour", { daypart: suggestion.daypart_key })
        : t("playTitles.happy_hour_generic");
    case "win_back":
      return t("playTitles.win_back", { count: count === "" ? 0 : count });
    case "combo_deal":
      return t("playTitles.combo_deal", { name });
    case "offer":
      return t("playTitles.offer", { name });
    default:
      return suggestion.title;
  }
}

function inferredSnapshotImageSource(
  suggestion: CampaignSuggestion,
): MarketingImageSource {
  if (suggestion.image_source) return suggestion.image_source;
  if (suggestion.play === "offer") return "offer";
  if (suggestion.play === "combo_deal") return "bundle";
  return "menu";
}

interface Props {
  suggestion: CampaignSuggestion;
  business: Business;
  t: (key: string, params?: Record<string, string | number>) => string;
  onTweak?: (suggestion: CampaignSuggestion) => void;
  onDismiss?: (suggestion: CampaignSuggestion) => void;
  onHandoff?: (handoff: MarketingCreativeHandoff) => void;
  onMarkPosted?: (selection: MarketingCreativeSelection) => void;
  /** S2-E: staff/owner mark craft ready for owner review (no due date). */
  onMarkReady?: (selection: MarketingCreativeSelection) => void;
  onReadinessChange?: (suggestionId: string, ready: boolean) => void;
  captionResource?: CaptionResource;
  isExample?: boolean;
  canEdit?: boolean;
  /** Creative profile so card kit/CTA match composer brand lock. */
  creativeProfile?: MarketingCreativeProfile;
}

export function PostCard({
  suggestion,
  business,
  t,
  onTweak,
  onDismiss,
  onHandoff,
  onMarkPosted,
  onMarkReady,
  onReadinessChange,
  captionResource,
  isExample = false,
  canEdit = true,
  creativeProfile,
}: Props) {
  const [actionError, setActionError] = React.useState<string | null>(null);
  const [captionExpanded, setCaptionExpanded] = React.useState(false);
  const [whyExpanded, setWhyExpanded] = React.useState(false);
  const [previewState, setPreviewState] =
    React.useState<PreviewRenderState>("rendering");
  const captionId = React.useId();
  const whyId = React.useId();
  const whyExplain = React.useMemo(
    () => buildWhyExplain(suggestion, t, previewState),
    [suggestion, t, previewState],
  );
  const showWhy = hasWhyExplain(whyExplain);
  const whyCanExpand =
    whyExplain.mode === "factors" &&
    whyExplain.rows.length > WHY_COLLAPSED_ROWS;
  const visibleWhyRows =
    whyExplain.mode === "factors"
      ? whyExpanded || !whyCanExpand
        ? whyExplain.rows
        : whyExplain.rows.slice(0, WHY_COLLAPSED_ROWS)
      : [];
  const currency =
    business.display_currency || business.default_currency || "USD";
  const intlLocale = intlLocaleFor(business.default_language || "en");
  const logoUrl = brandLogoUrl(business);
  const templateStyle = defaultTemplateForPlay(suggestion.play);
  const visualMood = creativeProfile?.visual_mood || "";
  const kit = React.useMemo(
    () =>
      seedKitForCreative({
        visualMood,
        templateStyle,
      }),
    [visualMood, templateStyle],
  );
  const palette = React.useMemo(
    () => resolveBrandPalette(business, { kit }),
    [business, kit],
  );
  const slots = React.useMemo(
    () =>
      buildSlots({
        suggestion,
        business,
        currency,
        intlLocale,
        ctaLabel: ctaLabelForBrand(
          t,
          suggestion.play,
          creativeProfile?.cta_style,
        ),
        badgeLabel:
          suggestion.play === "offer"
            ? ""
            : t(`defaults.badge.${suggestion.play}`),
        offLabel: t("discount.off"),
      }),
    [suggestion, business, currency, intlLocale, t, creativeProfile?.cta_style],
  );
  const fallbackCaption = React.useMemo(
    () =>
      composeLocalizedFallbackCaption({
        suggestion,
        business,
        locale: business.default_language,
      }),
    [suggestion, business],
  );
  const displayCaption = (
    captionResource?.source === "unavailable"
      ? ""
      : captionResource?.caption || fallbackCaption
  ).trim();
  const photoUrl = suggestion.image_url
    ? browserReadablePhotoUrl(suggestion.image_url)
    : "";
  const hasPhoto = !!photoUrl;
  const previewBroken = previewState === "failed";
  // After the preview paints, reuse the renderer's cached photo analysis so
  // "Too dark" / reshoot library grades cannot still show Ready to post.
  const photoQuality =
    previewState === "ready"
      ? photoQualityGateForUrl(suggestion.image_url)
      : "unknown";
  // A URL alone is not readiness: CORS-blocked or decode-failed photos leave an
  // empty/broken card. Never green "ready" or enable export on a failed preview.
  const readinessInput = {
    hasPhoto,
    previewState,
    displayCaption,
    captionGenerating: !!captionResource?.generating,
    captionError: !!captionResource?.error,
    photoQuality,
  };
  const blockedReason = exportBlockedReason(readinessInput);
  const isReady = isExportReady(readinessInput);
  const blockedExplain = blockedReason
    ? t(exportBlockedReasonKey(blockedReason))
    : null;
  React.useEffect(() => {
    onReadinessChange?.(suggestion.id, isReady);
  }, [isReady, onReadinessChange, suggestion.id]);
  const renderInput = React.useMemo<RenderPostInput>(
    () => ({
      // The kit path, not `template`. The two are not equivalent: a `template`
      // pins a legacy composition that reproduces a pre-scene-system layout,
      // so the feed rendered families the editor had already stopped using.
      // Deliberately NO `composition`: absent lets `resolveArtDirection` choose
      // with the real photo signals, which is the whole point of Wave 2, and it
      // matches the snapshot below, which records none either.
      kit,
      aspect: CARD_ASPECT,
      photoUrl,
      slots,
      palette,
      logoUrl,
      // No `crop`: absent means "crop around whatever the analysis found", and
      // the feed thumbnail has no crop UI for an operator to have chosen with.
      // The recorded snapshot below still carries DEFAULT_CROP, because the
      // wire shape requires one and a centred crop is the honest record of
      // "nobody chose" — the editor's analysis effect replaces it on Reuse.
    }),
    [kit, photoUrl, slots, palette, logoUrl],
  );
  const creative = React.useMemo<MarketingCreativeSnapshot>(
    () => ({
      caption: displayCaption,
      image_url: photoUrl,
      image_source: inferredSnapshotImageSource(suggestion),
      template: templateStyle,
      aspect: CARD_ASPECT,
      slots: Object.fromEntries(
        Object.entries(slots).map(([key, value]) => [key, value ?? ""]),
      ),
      crop: { ...DEFAULT_CROP },
      // Recorded so Library reuse opens on the kit the card rendered. The
      // composition is deliberately absent — see the renderInput comment.
      kit,
      font_family: normalizeMarketingRenderFontFamily(palette.fontFamily),
    }),
    [
      displayCaption,
      suggestion,
      photoUrl,
      templateStyle,
      kit,
      slots,
      palette.fontFamily,
    ],
  );
  const selection = React.useMemo<MarketingCreativeSelection>(
    () => ({ suggestion, creative }),
    [suggestion, creative],
  );
  const packArgs = React.useMemo(
    () => ({
      renderInput,
      caption: displayCaption,
      filename: buildPostFilename(suggestion.target_name, CARD_ASPECT),
    }),
    [renderInput, displayCaption, suggestion.target_name],
  );

  const reportHandoff = React.useCallback(
    (mode: MarketingCreativeHandoff["mode"]) => {
      onHandoff?.({ ...selection, mode });
    },
    [onHandoff, selection],
  );

  const downloadPack = () => {
    if (!isReady) return;
    setActionError(null);
    downloadPostPack(packArgs)
      .then(() => {
        toast.success(t("card.downloadPackSuccess"));
        reportHandoff("downloaded");
      })
      .catch(() => setActionError("download_failed"));
  };

  const sharePack = () => {
    if (!isReady) return;
    setActionError(null);
    shareOrDownloadPostPack(packArgs)
      .then((mode) => {
        toast.success(
          t(
            mode === "shared"
              ? "card.shareSuccess"
              : "card.shareFallbackSuccess",
          ),
        );
        reportHandoff(mode);
      })
      .catch((error) => {
        if (error instanceof DOMException && error.name === "AbortError")
          return;
        setActionError("download_failed");
      });
  };

  const guestLink = React.useMemo(
    () =>
      resolveGuestLink({
        guestUrl: suggestion.guest_url,
        guestUrlKind: suggestion.guest_url_kind,
        customUrl: business.custom_url,
        pageEnabled: business.business_page_enabled,
        play: suggestion.play,
      }),
    [
      suggestion.guest_url,
      suggestion.guest_url_kind,
      suggestion.play,
      business.custom_url,
      business.business_page_enabled,
    ],
  );

  const copyGuestLink = () => {
    if (!guestLink?.url) return;
    setActionError(null);
    copyCaption(guestLink.url)
      .then(() => {
        toast.success(t("guestLink.copySuccess"));
      })
      .catch(() => setActionError("copy_failed"));
  };

  const copyCaptionOnly = () => {
    setActionError(null);
    copyCaption(displayCaption)
      .then(() => {
        toast.success(t("card.copyCaptionSuccess"));
        reportHandoff("copied");
      })
      .catch(() => setActionError("copy_failed"));
  };

  return (
    <PremiumPanel
      as="article"
      tone="default"
      className="group flex min-h-full flex-col gap-0 p-0"
      withTexture={false}
    >
      <div className="flex items-start justify-between gap-3 border-b border-warm-100 px-4 py-3">
        <div className="min-w-0 space-y-1">
          <div className="flex flex-wrap items-center gap-2">
            <StatusChip
              tone={PLAY_TONE[suggestion.play] ?? "info"}
              label={t(`plays.${suggestion.play}`)}
            />
            {!isExample && isReady ? (
              <StatusChip tone="success" label={t("card.readyBadge")} />
            ) : null}
            {isExample ? (
              <StatusChip tone="warn" label={t("example.badge")} />
            ) : null}
          </div>
          <h3 className="line-clamp-2 break-words text-balance text-sm font-semibold leading-snug text-ink-900">
            {localizedSuggestionTitle(suggestion, t)}
          </h3>
        </div>
        {!isExample && canEdit && onDismiss ? (
          <button
            type="button"
            aria-label={t("card.dismiss")}
            className={`${btnGhostIcon} opacity-0 transition-opacity group-hover:opacity-100`}
            onClick={() => onDismiss(suggestion)}
          >
            <X className="h-4 w-4" />
          </button>
        ) : null}
      </div>

      <div className="space-y-3 px-4 pt-3">
        <PlatformPreviewFrame
          aspect={CARD_ASPECT}
          // Not an approximation of the template's value — it IS that value.
          // `layout()` in templates.ts sets every layout's `contentBounds` to
          // `PLATFORM_CONTENT_BOUNDS[aspect]` and takes its options as
          // `Omit<TemplateLayout, "contentBounds" | ...>`, so no template can
          // supply its own. Reading it here instead of off `renderInput`
          // survives a kit-path input, which carries no `TemplateDef` at all.
          // PostCard.test.tsx pins the equality this relies on.
          contentBounds={PLATFORM_CONTENT_BOUNDS[CARD_ASPECT]}
          platformLabel={t(platformLabelKey(CARD_ASPECT))}
          showSafeZones={!isExample}
          className="max-w-none"
        >
          <PostPreview
            renderInput={renderInput}
            isGenerating={false}
            emptyLabel={t("card.addPhotoHint")}
            previewErrorLabel={t("preview.renderError")}
            previewRetryLabel={t("preview.retry")}
            renderingLabel={t("preview.rendering")}
            generatingLabel={t("preview.generating")}
            onRenderStateChange={setPreviewState}
          />
        </PlatformPreviewFrame>
        {previewBroken && !isExample ? (
          <div
            className="rounded-xl border border-amber-200 bg-amber-50 px-3 py-2.5 text-center"
            role="alert"
            data-testid="card-photo-failed"
          >
            <p className="text-xs font-medium text-amber-800">
              {t("card.photoLoadFailed")}
            </p>
            {canEdit ? (
              <Button
                size="sm"
                radius="full"
                className={`${btnPrimaryNextUI} mt-2`}
                startContent={<ImagePlus className="h-4 w-4" />}
                onPress={() => onTweak?.(suggestion)}
              >
                {t("card.fixPhoto")}
              </Button>
            ) : null}
          </div>
        ) : null}
        {!isExample ? (
          <div className="flex justify-center">
            <StatusChip
              tone={
                previewBroken
                  ? "warn"
                  : hasPhoto
                    ? "info"
                    : "neutral"
              }
              label={
                previewBroken
                  ? t("card.photoBrokenBadge")
                  : t(`sourceTag.${suggestion.image_source || "none"}`)
              }
            />
          </div>
        ) : null}
      </div>

      <div className="flex flex-1 flex-col gap-3 px-4 py-4">
        {showWhy ? (
          <blockquote
            className="border-l-[3px] border-brand/35 bg-brand-50/50 px-3 py-2.5 text-xs leading-5 text-ink-700"
            data-testid="why-this-post"
            data-why-mode={whyExplain.mode}
          >
            <span className="mb-1.5 flex items-center gap-1.5 text-[11px] font-semibold uppercase tracking-[0.12em] text-brand-700">
              <Sparkles className="h-3 w-3" aria-hidden="true" />
              {t("why.heading")}
            </span>
            {whyExplain.mode === "factors" ? (
              <>
                <dl
                  id={whyId}
                  className="m-0 space-y-1.5"
                  data-testid="why-factors"
                >
                  {visibleWhyRows.map((row) => (
                    <div
                      key={row.key}
                      className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-0.5"
                      data-why-key={row.key}
                    >
                      <dt className="text-[11px] font-medium text-ink-500">
                        {row.label}
                      </dt>
                      <dd className="m-0 text-xs font-semibold text-ink-800">
                        {row.value}
                      </dd>
                    </div>
                  ))}
                </dl>
                {whyCanExpand ? (
                  <button
                    type="button"
                    aria-expanded={whyExpanded}
                    aria-controls={whyId}
                    className="mt-2 inline-flex items-center gap-1 text-[11px] font-semibold text-brand-700"
                    onClick={() => setWhyExpanded((open) => !open)}
                  >
                    {whyExpanded ? (
                      <>
                        {t("why.collapse")}
                        <ChevronUp className="h-3 w-3" aria-hidden="true" />
                      </>
                    ) : (
                      <>
                        {t("why.expand")}
                        <ChevronDown className="h-3 w-3" aria-hidden="true" />
                      </>
                    )}
                  </button>
                ) : null}
              </>
            ) : (
              <p className="m-0" data-testid="why-legacy">
                {whyExplain.legacyText}
              </p>
            )}
          </blockquote>
        ) : null}

        {!isExample ? (
          <div className="space-y-2">
            <p className="text-[11px] font-semibold uppercase tracking-[0.12em] text-ink-500">
              {t("card.captionLabel")}
            </p>
            <div
              className="rounded-xl border border-warm-200 bg-white px-3 py-2.5"
              data-caption-source={captionResource?.source ?? "fallback"}
            >
              <p
                id={captionId}
                className={`text-xs leading-5 text-ink-700 ${captionExpanded ? "" : "line-clamp-3"}`}
              >
                {displayCaption}
              </p>
              {captionResource?.generating ? (
                <p className="mt-1 text-[11px] text-ink-500" aria-live="polite">
                  {t("card.captionLoading")}
                </p>
              ) : null}
              {captionResource?.error ? (
                <div
                  className="mt-2 flex flex-wrap items-center justify-between gap-2 border-t border-warm-100 pt-2"
                  role="alert"
                >
                  <span className="text-[11px] text-amber-700">
                    {t(
                      captionResource.error?.message === "item_unavailable"
                        ? "errors.caption_unavailable"
                        : "errors.caption_failed",
                    )}
                  </span>
                  <button
                    type="button"
                    className="text-[11px] font-semibold text-brand-700 hover:text-brand-800"
                    onClick={captionResource.retry}
                  >
                    {t("card.retryCaption")}
                  </button>
                </div>
              ) : null}
              {displayCaption.length > 120 ? (
                <button
                  type="button"
                  aria-expanded={captionExpanded}
                  aria-controls={captionId}
                  className="mt-2 inline-flex items-center gap-1 text-[11px] font-semibold text-brand-700"
                  onClick={() => setCaptionExpanded((open) => !open)}
                >
                  {captionExpanded ? (
                    <>
                      {t("card.collapseCaption")}
                      <ChevronUp className="h-3 w-3" />
                    </>
                  ) : (
                    <>
                      {t("card.expandCaption")}
                      <ChevronDown className="h-3 w-3" />
                    </>
                  )}
                </button>
              ) : null}
            </div>
          </div>
        ) : null}

        <div className="mt-auto space-y-2 pt-1">
          {isExample && canEdit ? (
            <Button
              radius="full"
              className={`${btnPrimaryNextUI} w-full`}
              startContent={<Wand2 className="h-4 w-4" />}
              onPress={() => onTweak?.(suggestion)}
            >
              {t("example.cta")}
            </Button>
          ) : !isExample && hasPhoto && previewBroken && canEdit ? (
            <Button
              radius="full"
              className={`${btnPrimaryNextUI} w-full`}
              startContent={<ImagePlus className="h-4 w-4" />}
              onPress={() => onTweak?.(suggestion)}
            >
              {t("card.fixPhoto")}
            </Button>
          ) : !isExample && hasPhoto ? (
            <>
              {canEdit ? (
                <Button
                  radius="full"
                  className={`${btnPrimaryNextUI} w-full`}
                  startContent={<Wand2 className="h-4 w-4" />}
                  onPress={() => onTweak?.(suggestion)}
                >
                  {t("card.reviewExport")}
                </Button>
              ) : null}
              <p className="text-xs text-ink-500">{t("card.localExportHint")}</p>
              {canEdit ? (
                <Dropdown placement="bottom-end">
                  <DropdownTrigger>
                    <Button
                      size="sm"
                      variant="bordered"
                      radius="full"
                      className={`${btnSecondaryNextUI} w-full`}
                      startContent={<MoreHorizontal className="h-4 w-4" />}
                      aria-label={t("card.moreActions")}
                      data-testid="post-card-more-actions"
                    >
                      {t("card.moreActions")}
                    </Button>
                  </DropdownTrigger>
                  <DropdownMenu
                    aria-label={t("card.moreActions")}
                    onAction={(key) => {
                      if (key === "share") sharePack();
                      if (key === "download") downloadPack();
                      if (key === "copyCaption") copyCaptionOnly();
                      if (key === "copyGuest") copyGuestLink();
                      if (key === "markReady" && onMarkReady)
                        onMarkReady(selection);
                      if (key === "markPosted" && onMarkPosted)
                        onMarkPosted(selection);
                    }}
                  >
                    <DropdownItem
                      key="share"
                      startContent={<Share2 className="h-4 w-4" />}
                      isDisabled={!isReady}
                      textValue={t("card.sharePost")}
                      title={blockedExplain ?? undefined}
                      aria-label={
                        blockedExplain
                          ? `${t("card.sharePost")}: ${blockedExplain}`
                          : t("card.sharePost")
                      }
                    >
                      {t("card.sharePost")}
                    </DropdownItem>
                    <DropdownItem
                      key="download"
                      startContent={<Download className="h-4 w-4" />}
                      isDisabled={!isReady}
                      textValue={t("card.downloadPack")}
                      title={blockedExplain ?? undefined}
                      aria-label={
                        blockedExplain
                          ? `${t("card.downloadPack")}: ${blockedExplain}`
                          : t("card.downloadPack")
                      }
                    >
                      {t("card.downloadPack")}
                    </DropdownItem>
                    <DropdownItem
                      key="copyCaption"
                      startContent={<Copy className="h-4 w-4" />}
                      isDisabled={!displayCaption}
                      textValue={t("card.copyCaption")}
                    >
                      {t("card.copyCaption")}
                    </DropdownItem>
                    {guestLink ? (
                      <DropdownItem
                        key="copyGuest"
                        startContent={<Copy className="h-4 w-4" />}
                        textValue={t("guestLink.copy")}
                        data-testid="copy-guest-link"
                      >
                        {t("guestLink.copy")}
                      </DropdownItem>
                    ) : null}
                    {onMarkReady && isReady ? (
                      <DropdownItem
                        key="markReady"
                        startContent={<CheckCircle2 className="h-4 w-4" />}
                        textValue={t("handoff.markReady")}
                        data-testid="mark-ready"
                      >
                        {t("handoff.markReady")}
                      </DropdownItem>
                    ) : null}
                    {onMarkPosted && isReady ? (
                      <DropdownItem
                        key="markPosted"
                        startContent={<CheckCircle2 className="h-4 w-4" />}
                        textValue={t("card.markPosted")}
                      >
                        {t("card.markPosted")}
                      </DropdownItem>
                    ) : null}
                  </DropdownMenu>
                </Dropdown>
              ) : (
                <Button
                  size="sm"
                  variant="bordered"
                  radius="full"
                  className={`${btnSecondaryNextUI} w-full`}
                  startContent={<Copy className="h-4 w-4" />}
                  onPress={copyCaptionOnly}
                  isDisabled={!displayCaption}
                >
                  {t("card.copyCaption")}
                </Button>
              )}
            </>
          ) : !isExample && canEdit ? (
            <Button
              radius="full"
              className={`${btnPrimaryNextUI} w-full`}
              startContent={<ImagePlus className="h-4 w-4" />}
              onPress={() => onTweak?.(suggestion)}
            >
              {t("card.addPhoto")}
            </Button>
          ) : null}
        </div>
      </div>

      {actionError ? (
        <p className="px-4 pb-4 text-xs text-amber-700" role="alert">
          {t(`errors.${actionError}`)}
        </p>
      ) : null}
    </PremiumPanel>
  );
}
