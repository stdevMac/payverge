"use client";

import React from "react";
import {
  Button,
  Drawer,
  DrawerBody,
  DrawerContent,
  DrawerHeader,
} from "@nextui-org/react";
import { Download, Eye, WandSparkles } from "lucide-react";
import type { Business } from "@/api/business";
import type {
  MarketingActivity,
  MarketingCreativeSnapshot,
} from "@/api/marketing";
import { StatusChip } from "@/components/ui/StatusChip";
import { useSimpleLocale } from "@/i18n/SimpleTranslationProvider";
import { intlLocaleFor } from "@/utils/intlLocale";
import { PostPreview } from "./PostPreview";
import { brandLogoUrl, resolveBrandPalette } from "./brandLock";
import { isKitId, kitForLegacyTemplate } from "./artDirection/kits";
import { isCompositionId } from "./composition/chooser";
import { DEFAULT_NARRATIVE_ROLES } from "./formats/narrativeRoles";
import { resolveGuestLink } from "./guestLinks";
import toast from "react-hot-toast";
import { useCampaignKitExport } from "./hooks/useCampaignKitExport";
import { TEMPLATES } from "./templates/templates";
import type { TemplateStyle } from "./templates/types";
import type { RenderPostInput } from "./templates/renderPost";

interface Props {
  isOpen: boolean;
  onClose: () => void;
  activity: MarketingActivity | null;
  creative: MarketingCreativeSnapshot | null;
  business?: Business;
  canEdit?: boolean;
  /** S2-E: owner may approve ready creatives. */
  canApprove?: boolean;
  onReuse?: (
    activity: MarketingActivity,
    creative: MarketingCreativeSnapshot,
  ) => void;
  onApprove?: (activity: MarketingActivity) => void;
  t: (key: string, params?: Record<string, string | number>) => string;
  playLabel: (play: MarketingActivity["play"]) => string;
}

/**
 * Build a RenderPostInput that reuses the scene system (kit path preferred).
 * Library rows without kit fall back to the legacy template → kit mapping.
 */
function renderInputFromLibraryCreative(
  creative: MarketingCreativeSnapshot,
  business?: Business,
): RenderPostInput {
  const templateStyle = (creative.template || "editorial") as TemplateStyle;
  const template = TEMPLATES[templateStyle] ?? TEMPLATES.editorial;
  const kit = isKitId(creative.kit)
    ? creative.kit
    : kitForLegacyTemplate(templateStyle);
  const composition = isCompositionId(creative.composition)
    ? creative.composition
    : undefined;
  const palette = business
    ? resolveBrandPalette(business, {
        kit,
        storedFontFamily: creative.font_family,
      })
    : {
        primary: "",
        secondary: "",
        fontFamily: creative.font_family || "Sans",
      };
  return {
    kit,
    composition,
    template,
    aspect: creative.aspect,
    photoUrl: creative.image_url,
    slots: creative.slots,
    palette,
    logoUrl: business ? brandLogoUrl(business) : undefined,
    crop: creative.crop,
  };
}

/**
 * Read-only detail view for library rows. Available to all viewers (including
 * marketing:read-without-write staff) so they can inspect the full creative
 * and caption. Reuse remains edit-gated. Narrative pack export reuses the
 * scene system and is available whenever a photo URL exists (viewers can
 * download assets; reuse still requires canEdit).
 */
function libraryStatusLabel(
  status: MarketingActivity["status"],
  t: (key: string) => string,
): { tone: "success" | "neutral" | "info" | "warn"; label: string } {
  switch (status) {
    case "posted":
      return { tone: "success", label: t("library.statusPosted") };
    case "ready":
      return { tone: "info", label: t("library.statusReady") };
    case "approved":
      return { tone: "success", label: t("library.statusApproved") };
    case "dismissed":
    default:
      return { tone: "neutral", label: t("library.statusDismissed") };
  }
}

export function ActivityDetailDrawer({
  isOpen,
  onClose,
  activity,
  creative,
  business,
  canEdit = false,
  canApprove = false,
  onReuse,
  onApprove,
  t,
  playLabel,
}: Props) {
  const { locale } = useSimpleLocale();
  const kitExport = useCampaignKitExport();

  const exportNarrative = React.useCallback(async () => {
    if (!creative?.image_url) return;
    const captionAngles = [];
    if (creative.caption?.trim()) {
      captionAngles.push({ id: "primary", text: creative.caption.trim() });
    }
    const cta = creative.slots?.cta?.trim();
    if (cta && cta !== creative.caption?.trim()) {
      captionAngles.push({ id: "cta", text: cta });
    }
    const guest = resolveGuestLink({
      customUrl: business?.custom_url,
      pageEnabled: business?.business_page_enabled,
      play: activity?.play,
    });
    try {
      const ok = await kitExport.exportNarrativeKit({
        base: renderInputFromLibraryCreative(creative, business),
        caption: creative.caption || "",
        captionAngles,
        roles: DEFAULT_NARRATIVE_ROLES,
        targetName:
          creative.slots?.dishName ||
          activity?.target_name ||
          activity?.title ||
          "post",
        origin: typeof window !== "undefined" ? window.location.origin : "",
        lang: locale || "en",
        readmeText: t("narrativeKit.readme"),
        manifestNote: t("narrativeKit.manifestNote"),
        guestUrl: guest?.url,
      });
      if (ok) {
        toast.success(t("narrativeKit.success"));
        return;
      }
      toast.error(t("narrativeKit.failed"));
    } catch (error) {
      if (error instanceof DOMException && error.name === "AbortError") return;
      toast.error(t("narrativeKit.failed"));
    }
  }, [creative, business, activity, kitExport, locale, t]);

  if (!activity || !creative) return null;

  const captured = activity.creative_snapshot != null;
  const renderInput = renderInputFromLibraryCreative(creative, business);
  const canExportNarrative = Boolean(creative.image_url);
  const statusMeta = libraryStatusLabel(activity.status, t);

  return (
    <Drawer
      isOpen={isOpen}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      placement="right"
      size="lg"
      classNames={{
        base: "w-full max-w-lg border-l border-warm-200 bg-white shadow-2xl shadow-warm-900/15",
        closeButton:
          "text-ink-500 hover:bg-warm-100 focus-visible:ring-2 focus-visible:ring-brand",
      }}
    >
      <DrawerContent aria-label={t("library.detailTitle")}>
        <DrawerHeader className="flex flex-col items-start gap-1 border-b border-warm-100 px-5 py-4">
          <div className="flex flex-wrap items-center gap-2">
            <h2 className="text-base font-semibold text-ink-900">
              {activity.title || playLabel(activity.play)}
            </h2>
            <StatusChip tone={statusMeta.tone} label={statusMeta.label} />
          </div>
          <p className="text-sm text-ink-500">
            {playLabel(activity.play)}
            {activity.target_name ? ` · ${activity.target_name}` : ""}
          </p>
        </DrawerHeader>
        <DrawerBody className="gap-5 px-5 py-5">
          <div className="overflow-hidden rounded-2xl border border-warm-200 bg-warm-50">
            <PostPreview
              renderInput={renderInput}
              isGenerating={false}
              emptyLabel={t("editor.noPhoto")}
              previewErrorLabel={t("preview.renderError")}
              previewRetryLabel={t("preview.retry")}
              renderingLabel={t("preview.rendering")}
              generatingLabel={t("preview.generating")}
            />
          </div>

          {creative.caption ? (
            <section className="space-y-2">
              <h3 className="text-xs font-semibold uppercase tracking-wide text-ink-500">
                {t("library.detailCaption")}
              </h3>
              <p className="whitespace-pre-wrap text-sm leading-6 text-ink-800">
                {creative.caption}
              </p>
            </section>
          ) : null}

          <section className="space-y-2 text-sm text-ink-600">
            <h3 className="text-xs font-semibold uppercase tracking-wide text-ink-500">
              {t("library.detailMeta")}
            </h3>
            <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1.5">
              <dt className="text-ink-500">{t("library.detailCreated")}</dt>
              <dd>
                <time>
                  {new Intl.DateTimeFormat(intlLocaleFor(locale), {
                    year: "numeric",
                    month: "short",
                    day: "numeric",
                    hour: "numeric",
                    minute: "2-digit",
                  }).format(new Date(activity.created_at))}
                </time>
              </dd>
              <dt className="text-ink-500">{t("library.detailProvenance")}</dt>
              <dd>
                {t(
                  captured
                    ? "library.capturedPreview"
                    : "library.reconstructedPreview",
                )}
              </dd>
              {activity.created_by ? (
                <>
                  <dt className="text-ink-500">{t("library.detailBy")}</dt>
                  <dd className="truncate">{activity.created_by}</dd>
                </>
              ) : null}
            </dl>
          </section>

          {canExportNarrative ? (
            <section
              className="space-y-2 rounded-xl border border-warm-200 bg-warm-50/80 p-3"
              data-testid="library-narrative-export"
            >
              <p className="text-xs text-ink-600">
                {t("library.exportNarrativeHint")}
              </p>
              <p className="text-[11px] font-medium text-brand">
                {t("narrativeKit.readyNow")}
              </p>
              <Button
                size="sm"
                color="primary"
                variant="flat"
                radius="full"
                startContent={<Download className="h-3.5 w-3.5" />}
                isDisabled={kitExport.progress !== null}
                isLoading={kitExport.progress !== null}
                onPress={() => void exportNarrative()}
                data-testid="library-export-narrative-pack"
              >
                {t("library.exportNarrative")}
              </Button>
            </section>
          ) : null}

          <div className="flex flex-wrap gap-2 border-t border-warm-100 pt-4">
            {canApprove &&
            onApprove &&
            activity.status === "ready" &&
            activity.creative_snapshot ? (
              <Button
                size="sm"
                color="primary"
                radius="full"
                data-testid="library-approve-creative"
                onPress={() => {
                  onApprove(activity);
                  onClose();
                }}
              >
                {t("handoff.approve")}
              </Button>
            ) : null}
            {canEdit && onReuse && activity.play !== "unknown" ? (
              <Button
                size="sm"
                color="primary"
                radius="full"
                startContent={<WandSparkles className="h-3.5 w-3.5" />}
                onPress={() => {
                  onReuse(activity, creative);
                  onClose();
                }}
              >
                {t("library.reuse")}
              </Button>
            ) : null}
            <Button size="sm" variant="flat" radius="full" onPress={onClose}>
              {t("library.detailClose")}
            </Button>
          </div>
        </DrawerBody>
      </DrawerContent>
    </Drawer>
  );
}

export function ViewActivityButton({
  onPress,
  label,
}: {
  onPress: () => void;
  label: string;
}) {
  return (
    <Button
      size="sm"
      variant="flat"
      radius="full"
      startContent={<Eye className="h-3.5 w-3.5" />}
      onPress={onPress}
      aria-label={label}
    >
      {label}
    </Button>
  );
}
