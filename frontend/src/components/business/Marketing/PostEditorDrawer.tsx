"use client";

import React from "react";
import {
  Button,
  Drawer,
  DrawerBody,
  DrawerContent,
  DrawerFooter,
  DrawerHeader,
  Input,
  Select,
  SelectItem,
  Spinner,
  Tab,
  Tabs,
  Textarea,
  useDisclosure,
} from "@nextui-org/react";
import {
  Copy,
  Download,
  ImagePlus,
  RefreshCw,
  RotateCcw,
  Share2,
  Shuffle,
} from "lucide-react";
import type { Business, BusinessGalleryImage } from "@/api/business";
import { getBusinessGalleryImages } from "@/api/business";
import { resetWindowCopy } from "@/lib/dailyLimitReset";
import type {
  CampaignSuggestion,
  MarketingCreativeSnapshot,
  MarketingCreativeProfile,
  MarketingImageSource,
} from "@/api/marketing";
import { normalizeMarketingRenderFontFamily } from "@/api/marketing";
import { usePostComposer, type Tone } from "./hooks/usePostComposer";
import { isCleanupEligiblePhotoUrl } from "./cleanupPhotoEligibility";
import { generationRetryWarning } from "./generatedImageAcceptance";
import { btnSecondaryNextUI } from "@/components/ui/buttonStyles";
import { KIT_ORDER } from "./artDirection/kits";
import { PLATFORM_CONTENT_BOUNDS, type SlotKey } from "./templates/types";
import {
  resolveArtDirection,
  type RenderPostInput,
} from "./templates/renderPost";
import { COMPOSITIONS, type CompositionId } from "./composition/compositions";
import { FORMATS, FORMAT_ORDER, type FormatId } from "./formats/formats";
import {
  DEFAULT_NARRATIVE_ROLES,
  NARRATIVE_ROLES,
} from "./formats/narrativeRoles";
import { useCampaignKitExport } from "./hooks/useCampaignKitExport";
import { PostPreview, type PreviewRenderState } from "./PostPreview";
import { PlatformPreviewFrame, platformLabelKey } from "./PlatformPreviewFrame";
import { PostReadinessChecklist } from "./PostReadinessChecklist";
import { localizedSuggestionTitle } from "./PostCard";
import SimpleImageUpload from "../SimpleImageUpload";
import ConfirmationModal from "../modals/ConfirmationModal";
import type { MarketingCreativeHandoff } from "./ExportOutcomeDialog";
import toast from "react-hot-toast";
import { downloadPostVideo } from "./postContent";
import { detectMotionSupport } from "./motion/capabilities";
import { MOTION_PRESET_ORDER } from "./motion/presets";
import { attachAttributionQuery, buildAttributionTag } from "./attribution";
import {
  captionBudgetLevel,
  captionProfileFor,
  DESTINATION_ORDER,
  destinationFor,
  destinationPackContents,
  destinationPlatformLabelKey,
  packFormatsForDestination,
  simpleModeFormatsForDestination,
} from "./destinations/destinations";
import { type EditorMode, readEditorMode, writeEditorMode } from "./editorMode";
import { printPresetForDestination } from "./formats/printPresets";
import { computeExportReady } from "./PostReadinessChecklist";
import { isPublicGuestUrl } from "./guestLinks";

const TONES: Tone[] = ["warm", "playful", "elegant", "punchy"];
/** Caption languages offered in the editor (house voice + common guest locales). */
const CAPTION_LANGUAGES = [
  "en",
  "es",
  "es-AR",
  "pt",
  "fr",
  "de",
  "it",
  "ja",
  "zh",
];
const EDITABLE_SLOTS: SlotKey[] = [
  "badge",
  "dishName",
  "price",
  "cta",
  "handle",
];

/**
 * The slot keys this post's render will actually paint.
 *
 * The band set is a property of the COMPOSITION, not of the drawer: the scene
 * pipeline lays out exactly `COMPOSITIONS[id].bandsFor(aspect)` and nothing
 * else, so an editor offered outside that set collects text no render will
 * ever show. `renderInput.template` cannot answer this any more — it is
 * optional now, and a kit-path post carries no `TemplateDef` at all.
 *
 * WHICH composition that is comes from `resolveArtDirection`, the renderer's
 * own resolver, called rather than restated. The precedence it encodes (a
 * `kit` selects the kit path where the composition owns the bands; anything
 * else is the legacy path where the template's pinned legacy family owns them)
 * used to be re-implemented here, and two copies of one rule drift: the moment
 * they disagree the editor collects text the render drops, or hides a field
 * the render paints empty. Mapping composition -> editable keys stays here,
 * because that is the drawer's own question.
 *
 * Two arguments are shaped for the resolver rather than passed straight
 * through:
 *
 * `composition` is pre-filled with the composer's when the input carries none,
 * so the kit path resolves to the id the composer will persist on the
 * snapshot. That is the same `chooseComposition` the resolver's own fallback
 * would run, but with the real `play` and `hasDiscount` signals the composer
 * holds and `renderPost` cannot see — the better-informed answer to the same
 * question, and the one the preview beside these editors is already using.
 *
 * `hasPhoto` is derived from the URL, which is the only truth available before
 * a render: the resolver prefers the LOADED image because a photo that 404s
 * must not shape the layout, but the drawer has no loaded image to ask. It is
 * never load-bearing here — `composition` is always supplied by the line
 * above, so the resolver's chooser fallback is unreachable from this call.
 */
function paintedSlotKeys(
  renderInput: RenderPostInput,
  composerComposition: CompositionId,
  aspect: FormatId,
): ReadonlySet<string> {
  const { compositionId } = resolveArtDirection(
    {
      ...renderInput,
      composition: renderInput.composition ?? composerComposition,
    },
    renderInput.photoUrl.trim().length > 0,
  );
  return new Set(
    COMPOSITIONS[compositionId]
      .bandsFor(FORMATS[aspect] ?? FORMATS["4:5"])
      .map((band) => band.key),
  );
}
type GalleryStatus = "idle" | "loading" | "loaded" | "failed";

function useFocusReturn(isOpen: boolean) {
  const returnFocusRef = React.useRef<HTMLElement | null>(null);
  const wasOpenRef = React.useRef(false);

  React.useLayoutEffect(() => {
    if (isOpen && !wasOpenRef.current) {
      const active = document.activeElement;
      returnFocusRef.current =
        active instanceof HTMLElement && active !== document.body
          ? active
          : null;
    } else if (!isOpen && wasOpenRef.current) {
      const returnTarget = returnFocusRef.current;
      queueMicrotask(() => returnTarget?.focus());
    }
    wasOpenRef.current = isOpen;
  }, [isOpen]);
}

function isNestedEscapeTarget(target: EventTarget | null) {
  return (
    target instanceof Element &&
    Boolean(
      target.closest(
        'select,[role="combobox"],[role="listbox"],[role="option"],[aria-haspopup="listbox"]',
      ),
    )
  );
}

/** Narrow viewports use a full-screen editor sheet; desktop keeps the side drawer. */
function useNarrowViewport(breakpointPx = 1024): boolean {
  const [narrow, setNarrow] = React.useState(false);
  React.useEffect(() => {
    if (typeof window === "undefined" || !window.matchMedia) return;
    const mq = window.matchMedia(`(max-width: ${breakpointPx - 1}px)`);
    const apply = () => setNarrow(mq.matches);
    apply();
    mq.addEventListener("change", apply);
    return () => mq.removeEventListener("change", apply);
  }, [breakpointPx]);
  return narrow;
}

interface Props {
  isOpen: boolean;
  onClose: () => void;
  business: Business;
  suggestion: CampaignSuggestion | null;
  /** Exact activity snapshot for a local-only Reuse session. */
  initialCreative?: MarketingCreativeSnapshot | null;
  creativeProfile?: MarketingCreativeProfile;
  canEdit: boolean;
  canGenerate: boolean;
  canUpload: boolean;
  onHandoff?: (handoff: MarketingCreativeHandoff) => void;
  t: (key: string, params?: Record<string, string | number>) => string;
}

function Section({
  title,
  children,
}: {
  title: string;
  children: React.ReactNode;
}) {
  return (
    <section className="space-y-3 rounded-2xl border border-warm-200 bg-white p-4 shadow-sm shadow-warm-900/5">
      <h3 className="font-title text-base font-semibold text-ink-900">
        {title}
      </h3>
      {children}
    </section>
  );
}

export function PostEditorDrawer(props: Props) {
  if (!props.canEdit) return null;
  return <EditablePostEditorDrawer {...props} />;
}

function EditablePostEditorDrawer({
  isOpen,
  onClose,
  business,
  suggestion,
  initialCreative,
  creativeProfile,
  canEdit,
  canGenerate,
  canUpload,
  onHandoff,
  t,
}: Props) {
  useFocusReturn(isOpen);
  const isNarrow = useNarrowViewport();
  const c = usePostComposer({
    business,
    suggestion,
    initialSnapshot: initialCreative,
    creativeProfile,
    t,
  });
  const editorLocked = c.isGeneratingPhoto;
  const dailyLimitReset = c.dailyLimitReached
    ? resetWindowCopy(c.dailyLimitReached.resetsInSeconds)
    : null;
  const paintedSlots = React.useMemo(
    () => paintedSlotKeys(c.renderInput, c.composition, c.aspect),
    [c.renderInput, c.composition, c.aspect],
  );
  const titleId = React.useId();
  const styleLabelId = React.useId();
  const modeLabelId = React.useId();
  const discardConfirm = useDisclosure();
  const generationConfirm = useDisclosure();
  const confirmationOpen = discardConfirm.isOpen || generationConfirm.isOpen;
  const [photoTab, setPhotoTab] = React.useState("this");
  const [gallery, setGallery] = React.useState<BusinessGalleryImage[]>([]);
  const [galleryStatus, setGalleryStatus] =
    React.useState<GalleryStatus>("idle");
  const [previewState, setPreviewState] =
    React.useState<PreviewRenderState>("rendering");
  const [editorMode, setEditorMode] = React.useState<EditorMode>(() =>
    readEditorMode(business.id),
  );
  const galleryRequest = React.useRef(0);

  // Re-read preference when opening for a different business (Reuse/Tweak).
  React.useEffect(() => {
    if (!isOpen) return;
    setEditorMode(readEditorMode(business.id));
  }, [isOpen, business.id]);

  const setMode = React.useCallback(
    (mode: EditorMode) => {
      setEditorMode(mode);
      writeEditorMode(business.id, mode);
    },
    [business.id],
  );
  const isCraft = editorMode === "craft";

  React.useEffect(
    () => () => {
      galleryRequest.current += 1;
    },
    [],
  );

  const loadGallery = React.useCallback(() => {
    const request = galleryRequest.current + 1;
    galleryRequest.current = request;
    setGalleryStatus("loading");
    // Bound picker fan-out; thumbnails are CSS-sized, not full-res canvases.
    getBusinessGalleryImages(business.id, { limit: 24 })
      .then((images) => {
        if (galleryRequest.current !== request) return;
        setGallery(
          images.filter((image) => image.is_active && image.image_url),
        );
        setGalleryStatus("loaded");
      })
      .catch(() => {
        if (galleryRequest.current === request) setGalleryStatus("failed");
      });
  }, [business.id]);

  React.useEffect(() => {
    if (photoTab !== "gallery" || galleryStatus !== "idle") return;
    loadGallery();
  }, [photoTab, galleryStatus, loadGallery]);

  const requestClose = React.useCallback(() => {
    if (editorLocked) return;
    if (c.dirty) {
      discardConfirm.onOpen();
      return;
    }
    onClose();
  }, [c.dirty, editorLocked, discardConfirm, onClose]);

  const confirmDiscard = React.useCallback(() => {
    if (editorLocked) return;
    c.resetCreative();
    setPreviewState("rendering");
    onClose();
  }, [c, editorLocked, onClose]);

  const onPreviewStateChange = React.useCallback(
    (state: PreviewRenderState) => {
      setPreviewState(state);
    },
    [],
  );
  const previewBroken = previewState === "failed";
  const photoReady = Boolean(c.photoUrl) && !previewBroken;
  // The deterministic starter caption is valid export copy while an optional
  // AI refinement is still loading.
  const captionReady = Boolean(c.caption.trim());
  // Never treat a failed or mid-render preview as ready — export stays disabled.
  const previewReady =
    previewState === "ready" && !editorLocked && !previewBroken;
  const exportReady = computeExportReady({
    photoReady,
    captionReady,
    previewReady,
    previewBroken,
  });
  const simpleFormats = simpleModeFormatsForDestination(c.destinationId);
  const sourceText =
    c.imageSource === "generated"
      ? t("provenance.ai")
      : c.imageSource === "none"
        ? t("provenance.none")
        : t("provenance.free", {
            source: t(`provenance.sources.${c.imageSource}`),
          });
  const formatLabel = (id: FormatId) =>
    t(FORMATS[id]?.labelKey ?? `preview.aspects.${id}`);

  const retryWarning = generationRetryWarning({
    hasReadyGeneratedDraft: c.imageSource === "generated" && !!c.photoUrl,
    lastAcceptanceFailed:
      c.error === "image_failed" || c.error === "image_decode_failed",
    imageSource: c.imageSource,
  });
  const generationDescription = [
    t("generationConfirm.direction", {
      name: c.slots.dishName || business.name,
    }),
    t("generationConfirm.ratio", { ratio: formatLabel(c.aspect) }),
    retryWarning === "already_ready"
      ? t("generationConfirm.paidRetryWarning")
      : retryWarning === "failed_delivery"
        ? t("generationConfirm.failedDeliveryWarning")
        : t("generationConfirm.freeAlternative"),
  ].join(" ");

  const activeDestination = destinationFor(c.destinationId);
  const captionProfile = captionProfileFor(activeDestination);
  const captionChars = c.caption.length;
  const captionLevel = captionBudgetLevel(captionChars, captionProfile);
  const destinationPack = destinationPackContents(activeDestination, {
    caption: c.caption,
  });
  const printShopPreset = printPresetForDestination(c.destinationId);

  /** S3-Reach: promo code + attributed guest URL for pack QR payloads. */
  const packAttribution = React.useMemo(() => {
    if (!suggestion) return null;
    return buildAttributionTag({
      play: suggestion.play,
      suggestionId: suggestion.id,
      destinationId: c.destinationId,
    });
  }, [suggestion, c.destinationId]);

  const attributedGuestUrl = React.useMemo(() => {
    if (!packAttribution) return undefined;
    const base = suggestion?.guest_url;
    if (!isPublicGuestUrl(base)) return undefined;
    return (
      attachAttributionQuery(base, {
        promoCode: packAttribution.promoCode,
        play: packAttribution.play,
        destinationId: packAttribution.destinationId,
      }) ?? undefined
    );
  }, [packAttribution, suggestion?.guest_url]);

  const reportHandoff = React.useCallback(
    (
      mode: MarketingCreativeHandoff["mode"],
      /**
       * Override for the media kind recorded on the snapshot. Required on the
       * video-export path because `markMotionExported` is a setState and has not
       * re-rendered yet when this fires in the same tick.
       */
      opts?: { mediaKind?: "image" | "video" },
    ) => {
      if (!suggestion || !onHandoff || c.imageSource === "none") return;
      // The outbound half of the motion round trip. `media_kind` follows the
      // export the operator actually took, not the picker — see
      // `markMotionExported` in usePostComposer.ts. The preset ships only
      // with a video, because the backend rejects a video without one and
      // has no use for one without a video.
      const mediaKind = opts?.mediaKind ?? c.mediaKind;
      const dest = destinationFor(c.destinationId);
      const attr = buildAttributionTag({
        play: suggestion.play,
        suggestionId: suggestion.id,
        destinationId: c.destinationId,
      });
      const printPreset = printPresetForDestination(c.destinationId);
      onHandoff({
        mode,
        suggestion,
        destinationLabel: t(dest.labelKey),
        creative: {
          aspect: c.aspect,
          template: c.templateStyle,
          crop: { ...c.crop },
          slots: Object.fromEntries(
            Object.entries(c.slots).map(([key, value]) => [key, value ?? ""]),
          ),
          caption: c.caption.trim(),
          image_url: c.photoUrl,
          image_source: c.imageSource as MarketingImageSource,
          font_family: normalizeMarketingRenderFontFamily(c.palette.fontFamily),
          // The outbound half of the art-direction round trip. The composer
          // holding a kit is not enough — this literal is the only thing that
          // reaches the backend, so omitting these two discards the operator's
          // choice the moment the drawer closes. `usePostComposer` reads them
          // back on reopen.
          kit: c.kit,
          composition: c.composition,
          kit_formats: c.kitFormats,
          destination_id: c.destinationId,
          media_kind: mediaKind,
          ...(mediaKind === "video" ? { motion_preset: c.motionPreset } : {}),
          // S3-Reach: correlation code + print preset (backend-first fields).
          promo_code: attr.promoCode,
          ...(printPreset ? { print_preset: printPreset.id } : {}),
        },
      });
    },
    [c, onHandoff, suggestion, t],
  );

  const kitExport = useCampaignKitExport();
  const [singleBusy, setSingleBusy] = React.useState(false);
  const [exportNotice, setExportNotice] = React.useState<{
    tone: "success" | "error";
    key: string;
    params?: Record<string, string | number>;
  } | null>(null);
  const exportInFlight = singleBusy || kitExport.progress !== null;

  const announceExport = React.useCallback(
    (
      tone: "success" | "error",
      key: string,
      params?: Record<string, string | number>,
    ) => {
      setExportNotice({ tone, key, params });
      const message = t(key, params);
      if (tone === "success") toast.success(message);
      else toast.error(message);
    },
    [t],
  );

  const downloadKitAndHandoff = React.useCallback(async () => {
    setExportNotice(null);
    try {
      const ok = await kitExport.exportKit({
        base: c.renderInput,
        formats: c.kitFormats,
        caption: c.caption,
        targetName: c.slots.dishName || "post",
        origin: typeof window !== "undefined" ? window.location.origin : "",
        lang: c.language,
        guestUrl: attributedGuestUrl,
        attribution: packAttribution ?? undefined,
      });
      if (ok) {
        reportHandoff("downloaded");
        announceExport("success", "campaignKit.success");
        return;
      }
      announceExport(
        "error",
        c.kitFormats.length ? "campaignKit.failed" : "campaignKit.empty",
      );
    } catch (error) {
      if (error instanceof DOMException && error.name === "AbortError") return;
      announceExport("error", "campaignKit.failed");
    }
  }, [
    kitExport,
    c,
    reportHandoff,
    attributedGuestUrl,
    packAttribution,
    announceExport,
  ]);

  /** Export only the selected destination's format pack (+ caption.txt). */
  const downloadDestinationPackAndHandoff = React.useCallback(async () => {
    setExportNotice(null);
    try {
      // Print destinations with a shop preset use the print-shop zip (HTML@300dpi
      // or high-dpi PNG + instructions + attribution). Social destinations keep
      // the standard multi-format kit path.
      if (printShopPreset) {
        const instructionsKey = `printShop.presets.${printShopPreset.id}.instructions`;
        const translatedInstructions = t(instructionsKey);
        const ok = await kitExport.exportPrintShopPack({
          base: c.renderInput,
          presetId: printShopPreset.id,
          caption: c.caption,
          targetName: c.slots.dishName || "post",
          origin: typeof window !== "undefined" ? window.location.origin : "",
          lang: c.language,
          guestUrl: attributedGuestUrl,
          attribution: packAttribution ?? undefined,
          // Fall back to English preset defaults when a locale is missing the key.
          instructionsText:
            translatedInstructions && translatedInstructions !== instructionsKey
              ? translatedInstructions
              : printShopPreset.defaultInstructions,
        });
        if (ok) {
          reportHandoff("downloaded");
          announceExport("success", "destinations.exportSuccess");
          return;
        }
        announceExport("error", "destinations.exportFailed");
        return;
      }
      const formats = packFormatsForDestination(c.destinationId);
      const ok = await kitExport.exportKit({
        base: c.renderInput,
        formats,
        caption: c.caption,
        targetName: c.slots.dishName || "post",
        origin: typeof window !== "undefined" ? window.location.origin : "",
        lang: c.language,
        guestUrl: attributedGuestUrl,
        attribution: packAttribution ?? undefined,
      });
      if (ok) {
        reportHandoff("downloaded");
        announceExport("success", "destinations.exportSuccess");
        return;
      }
      announceExport("error", "destinations.exportFailed");
    } catch (error) {
      if (error instanceof DOMException && error.name === "AbortError") return;
      announceExport("error", "destinations.exportFailed");
    }
  }, [
    kitExport,
    c,
    reportHandoff,
    printShopPreset,
    attributedGuestUrl,
    packAttribution,
    t,
    announceExport,
  ]);

  /**
   * Narrative pack (S2-C): five story roles in one zip, immediately.
   * Caption angles are text-only. No schedule metadata.
   */
  const downloadNarrativePackAndHandoff = React.useCallback(async () => {
    const captionAngles = [];
    if (c.caption.trim()) {
      captionAngles.push({ id: "primary", text: c.caption.trim() });
    }
    // Optional CTA-line angle without a second LLM call.
    const cta = c.slots.cta?.trim();
    if (cta && cta !== c.caption.trim()) {
      captionAngles.push({ id: "cta", text: cta });
    }
    setExportNotice(null);
    try {
      const ok = await kitExport.exportNarrativeKit({
        base: c.renderInput,
        caption: c.caption,
        captionAngles,
        roles: DEFAULT_NARRATIVE_ROLES,
        targetName: c.slots.dishName || "post",
        origin: typeof window !== "undefined" ? window.location.origin : "",
        lang: c.language,
        readmeText: t("narrativeKit.readme"),
        manifestNote: t("narrativeKit.manifestNote"),
        guestUrl: attributedGuestUrl,
        attribution: packAttribution ?? undefined,
      });
      if (ok) {
        reportHandoff("downloaded");
        announceExport("success", "narrativeKit.success");
        return;
      }
      announceExport("error", "narrativeKit.failed");
    } catch (error) {
      if (error instanceof DOMException && error.name === "AbortError") return;
      announceExport("error", "narrativeKit.failed");
    }
  }, [
    kitExport,
    c,
    reportHandoff,
    t,
    attributedGuestUrl,
    packAttribution,
    announceExport,
  ]);

  const motionSupport = React.useMemo(() => detectMotionSupport(), []);
  const [videoProgress, setVideoProgress] = React.useState<number | null>(null);
  const [videoError, setVideoError] = React.useState<
    "failed" | "cancelled" | null
  >(null);
  const videoAbortRef = React.useRef<AbortController | null>(null);

  React.useEffect(
    () => () => {
      // A drawer closed mid-export must not leave an encoder running against a
      // canvas nobody is looking at.
      videoAbortRef.current?.abort();
    },
    [],
  );

  /**
   * Motion export is client-side only (Decision 2.1 / Task 24) — no model
   * call and no daily fair-use metering.
   */
  const downloadVideoAndHandoff = React.useCallback(async () => {
    const controller = new AbortController();
    videoAbortRef.current = controller;
    setVideoError(null);
    setVideoProgress(0);
    try {
      await downloadPostVideo({
        renderInput: c.renderInput,
        preset: c.motionPreset,
        caption: c.caption,
        filename: `${(c.slots.dishName || "post").replace(/\s+/g, "-").toLowerCase()}-${c.aspect.replace(":", "x")}.mp4`,
        onProgress: setVideoProgress,
        signal: controller.signal,
      });
      c.markMotionExported();
      reportHandoff("downloaded", { mediaKind: "video" });
    } catch (error) {
      const cancelled = (error as { name?: string }).name === "AbortError";
      setVideoError(cancelled ? "cancelled" : "failed");
      if (!cancelled) toast.error(t("motion.failed"));
    } finally {
      setVideoProgress(null);
      videoAbortRef.current = null;
    }
  }, [c, reportHandoff, t]);

  const copyAndHandoff = React.useCallback(async () => {
    if (await c.copyCaption()) reportHandoff("copied");
  }, [c, reportHandoff]);

  const downloadAndHandoff = React.useCallback(async () => {
    setSingleBusy(true);
    setExportNotice(null);
    try {
      const ok = await c.downloadPack();
      if (ok) {
        reportHandoff("downloaded");
        announceExport("success", "card.downloadPackSuccess");
        return;
      }
      announceExport("error", "errors.download_failed");
    } catch (error) {
      if (error instanceof DOMException && error.name === "AbortError") return;
      announceExport("error", "errors.download_failed");
    } finally {
      setSingleBusy(false);
    }
  }, [c, reportHandoff, announceExport]);

  const shareAndHandoff = React.useCallback(async () => {
    setExportNotice(null);
    try {
      const mode = await c.sharePack();
      if (mode) {
        reportHandoff(mode);
        announceExport(
          "success",
          mode === "shared" ? "card.shareSuccess" : "card.shareFallbackSuccess",
        );
        return;
      }
    } catch (error) {
      if (error instanceof DOMException && error.name === "AbortError") return;
      announceExport("error", "errors.download_failed");
    }
  }, [c, reportHandoff, announceExport]);

  return (
    <>
      <Drawer
        isOpen={isOpen && !confirmationOpen}
        onOpenChange={(open) => !open && !confirmationOpen && requestClose()}
        placement={isNarrow ? "bottom" : "right"}
        size={isNarrow ? "full" : "4xl"}
        isDismissable={!editorLocked}
        isKeyboardDismissDisabled
        hideCloseButton={editorLocked}
        classNames={{
          base: isNarrow ? "max-h-[100dvh]" : undefined,
          body: isNarrow ? "pb-24" : undefined,
        }}
      >
        <DrawerContent
          data-testid="post-editor-drawer"
          data-editor-mode={editorMode}
          data-narrow={String(isNarrow)}
          aria-label={
            suggestion
              ? localizedSuggestionTitle(suggestion, t)
              : t("manualStudio.title")
          }
          aria-labelledby={titleId}
          onKeyDown={(event) => {
            if (event.key !== "Escape") return;
            if (event.defaultPrevented) return;
            if (isNestedEscapeTarget(event.target)) {
              event.stopPropagation();
              return;
            }
            event.preventDefault();
            event.stopPropagation();
            requestClose();
          }}
        >
          <DrawerHeader className="flex flex-col gap-1">
            <h2 id={titleId} className="font-title text-xl text-ink-900">
              {suggestion
                ? localizedSuggestionTitle(suggestion, t)
                : t("manualStudio.title")}
            </h2>
            <span className="text-sm text-ink-700">{t("editor.subtitle")}</span>
          </DrawerHeader>

          <DrawerBody className="grid grid-cols-1 gap-6 lg:grid-cols-[minmax(320px,1fr)_minmax(320px,420px)] lg:items-start">
            <div
              data-testid="editor-preview-column"
              className="order-1 space-y-3 lg:sticky lg:top-4 lg:self-start"
            >
              <PostReadinessChecklist
                photoReady={photoReady}
                captionReady={captionReady}
                previewReady={previewReady}
                previewBroken={previewBroken}
                sticky
                t={t}
              />
              <PlatformPreviewFrame
                aspect={c.aspect}
                // Prefer the format registry's safe envelope; fall back to the
                // legacy PLATFORM_CONTENT_BOUNDS table only for ids that still
                // live there (the three original aspects).
                contentBounds={
                  FORMATS[c.aspect]?.safe ??
                  PLATFORM_CONTENT_BOUNDS[
                    c.aspect === "1:1" ||
                    c.aspect === "4:5" ||
                    c.aspect === "9:16"
                      ? c.aspect
                      : "4:5"
                  ]
                }
                platformLabel={t(
                  destinationPlatformLabelKey(c.destinationId) ||
                    platformLabelKey(c.aspect),
                )}
              >
                <PostPreview
                  renderInput={c.renderInput}
                  isGenerating={editorLocked}
                  emptyLabel={t("editor.noPhoto")}
                  previewErrorLabel={t("preview.renderError")}
                  previewRetryLabel={t("preview.retry")}
                  renderingLabel={t("preview.rendering")}
                  generatingLabel={t("preview.generating")}
                  onRenderStateChange={onPreviewStateChange}
                />
              </PlatformPreviewFrame>
              {previewBroken ? (
                <div
                  className="space-y-2 rounded-xl border border-amber-200 bg-amber-50 px-3 py-3 text-center"
                  role="alert"
                  data-testid="editor-preview-broken"
                >
                  <p className="text-xs font-medium text-amber-800">
                    {t("editor.previewBrokenHint")}
                  </p>
                  {canGenerate ? (
                    <div className="flex flex-wrap items-center justify-center gap-2">
                      <Button
                        size="sm"
                        radius="full"
                        color="primary"
                        startContent={
                          editorLocked ? (
                            <Spinner size="sm" color="white" />
                          ) : (
                            <ImagePlus className="h-4 w-4" />
                          )
                        }
                        isDisabled={editorLocked}
                        onPress={generationConfirm.onOpen}
                      >
                        {c.photoUrl
                          ? t("editor.newPhoto")
                          : t("editor.generatePhoto")}
                      </Button>
                      {c.photoUrl && isCleanupEligiblePhotoUrl(c.photoUrl) ? (
                        <Button
                          size="sm"
                          radius="full"
                          className={btnSecondaryNextUI}
                          isDisabled={editorLocked || !canEdit}
                          onPress={() => void c.cleanupPhoto()}
                        >
                          {t("editor.cleanupPhoto")}
                        </Button>
                      ) : null}
                    </div>
                  ) : (
                    <p className="text-xs text-amber-700">
                      {t("editor.previewBrokenNoGenerate")}
                    </p>
                  )}
                </div>
              ) : null}
              <p className="text-center text-[10px] leading-4 text-ink-500">
                {t("preview.safeZoneHint")}
              </p>
              {c.logoStatus === "failed" ? (
                <p
                  className="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-center text-xs leading-5 text-amber-800"
                  role="status"
                  data-testid="editor-logo-failed"
                >
                  {t("brand.logoLoadFailed")}
                </p>
              ) : null}
              {c.logoStatus === "missing" ? (
                <p
                  className="text-center text-xs leading-5 text-ink-500"
                  role="status"
                  data-testid="editor-logo-missing"
                >
                  {t("brand.logoMissing")}
                </p>
              ) : null}
              <p
                className="text-center text-xs font-semibold text-ink-600"
                aria-live="polite"
              >
                {sourceText}
              </p>
            </div>

            <fieldset
              data-testid="editor-controls-column"
              disabled={editorLocked}
              aria-busy={editorLocked}
              aria-disabled={editorLocked}
              className="order-2 min-w-0 space-y-4 border-0 p-0"
            >
              <Section title={t("editor.mode.label")}>
                <div
                  role="group"
                  aria-labelledby={modeLabelId}
                  data-testid="editor-mode-toggle"
                  className="flex flex-wrap gap-2"
                >
                  <p id={modeLabelId} className="sr-only">
                    {t("editor.mode.label")}
                  </p>
                  <button
                    type="button"
                    aria-pressed={editorMode === "simple"}
                    data-testid="editor-mode-simple"
                    disabled={editorLocked}
                    className={`min-h-11 min-w-[7rem] rounded-full px-4 py-2 text-sm font-semibold transition-colors ${
                      editorMode === "simple"
                        ? "bg-brand text-white"
                        : "bg-warm-100 text-ink-600 hover:bg-warm-200"
                    }`}
                    onClick={() => setMode("simple")}
                  >
                    {t("editor.mode.simple")}
                  </button>
                  <button
                    type="button"
                    aria-pressed={editorMode === "craft"}
                    data-testid="editor-mode-craft"
                    disabled={editorLocked}
                    className={`min-h-11 min-w-[7rem] rounded-full px-4 py-2 text-sm font-semibold transition-colors ${
                      editorMode === "craft"
                        ? "bg-brand text-white"
                        : "bg-warm-100 text-ink-600 hover:bg-warm-200"
                    }`}
                    onClick={() => setMode("craft")}
                  >
                    {t("editor.mode.craft")}
                  </button>
                </div>
                <p className="text-xs text-ink-500">
                  {isCraft
                    ? t("editor.mode.craftHint")
                    : t("editor.mode.simpleHint")}
                </p>
              </Section>

              <Section title={t("sections.image")}>
                <Tabs
                  aria-label={t("photoSource.heading")}
                  selectedKey={photoTab}
                  onSelectionChange={(key) => setPhotoTab(String(key))}
                  size="sm"
                  fullWidth
                  isDisabled={editorLocked}
                >
                  <Tab key="this" title={t("photoSource.thisPhoto")}>
                    <div className="space-y-2 pt-2">
                      {suggestion?.image_url ? (
                        <Button
                          fullWidth
                          variant="flat"
                          onPress={() =>
                            c.setPhoto(
                              suggestion.image_url ?? "",
                              c.initialCreative.imageSource === "none"
                                ? "menu"
                                : c.initialCreative.imageSource,
                            )
                          }
                        >
                          {t("photoSource.useThis")}
                        </Button>
                      ) : (
                        <p className="text-center text-xs text-ink-500">
                          {t("photoSource.noneSelected")}
                        </p>
                      )}
                    </div>
                  </Tab>
                  {canUpload ? (
                    <Tab key="gallery" title={t("photoSource.gallery")}>
                      <div className="space-y-2 pt-2">
                        {galleryStatus === "loading" && (
                          <div
                            className="flex items-center justify-center gap-2 py-6 text-xs text-ink-500"
                            role="status"
                          >
                            <Spinner size="sm" />{" "}
                            {t("photoSource.galleryLoading")}
                          </div>
                        )}
                        {galleryStatus === "failed" && (
                          <div className="space-y-2 text-center" role="alert">
                            <p className="text-xs text-amber-700">
                              {t("photoSource.galleryFailed")}
                            </p>
                            <Button
                              size="sm"
                              variant="flat"
                              onPress={loadGallery}
                            >
                              {t("photoSource.galleryRetry")}
                            </Button>
                          </div>
                        )}
                        {galleryStatus === "loaded" && gallery.length === 0 && (
                          <div className="space-y-2 text-center">
                            <p className="text-xs text-ink-500">
                              {t("photoSource.galleryEmpty")}
                            </p>
                            <Button
                              size="sm"
                              variant="light"
                              onPress={loadGallery}
                            >
                              {t("photoSource.galleryRetry")}
                            </Button>
                          </div>
                        )}
                        {galleryStatus === "loaded" && gallery.length > 0 && (
                          <div className="grid max-h-48 grid-cols-3 gap-2 overflow-y-auto">
                            {gallery.map((image) => (
                              <button
                                key={image.id}
                                type="button"
                                aria-label={
                                  image.caption || t("photoSource.useThis")
                                }
                                className="aspect-square overflow-hidden rounded-lg border border-warm-200 hover:border-brand focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
                                onClick={() =>
                                  c.setPhoto(image.image_url, "gallery")
                                }
                              >
                                {/* eslint-disable-next-line @next/next/no-img-element */}
                                <img
                                  src={image.image_url}
                                  alt=""
                                  loading="lazy"
                                  decoding="async"
                                  width={160}
                                  height={160}
                                  className="h-full w-full object-cover"
                                />
                              </button>
                            ))}
                          </div>
                        )}
                      </div>
                    </Tab>
                  ) : null}
                  {canUpload ? (
                    <Tab key="upload" title={t("photoSource.upload")}>
                      <div className="pt-2">
                        <SimpleImageUpload
                          businessId={business.id}
                          type="gallery"
                          onImageUploaded={(url) => c.setPhoto(url, "upload")}
                        />
                      </div>
                    </Tab>
                  ) : null}
                  {canGenerate ? (
                    <Tab key="ai" title={t("photoSource.ai")}>
                      <div className="space-y-2 pt-2">
                        <Button
                          fullWidth
                          color="primary"
                          startContent={
                            editorLocked ? (
                              <Spinner size="sm" color="white" />
                            ) : (
                              <ImagePlus className="h-4 w-4" />
                            )
                          }
                          isDisabled={editorLocked}
                          onPress={generationConfirm.onOpen}
                        >
                          {c.photoUrl
                            ? t("editor.newPhoto")
                            : t("editor.generatePhoto")}
                        </Button>
                        <Button
                          className={btnSecondaryNextUI}
                          radius="full"
                          size="sm"
                          fullWidth
                          isDisabled={
                            !c.photoUrl ||
                            c.isGeneratingPhoto ||
                            !canEdit ||
                            !isCleanupEligiblePhotoUrl(c.photoUrl)
                          }
                          onPress={() => void c.cleanupPhoto()}
                        >
                          {t("editor.cleanupPhoto")}
                        </Button>
                        <p className="text-xs text-ink-500">
                          {t("editor.cleanupHint")}
                        </p>
                        {c.promptRejected && (
                          <p className="text-center text-xs text-amber-700">
                            {t("editor.promptRejected")}
                          </p>
                        )}
                      </div>
                    </Tab>
                  ) : null}
                </Tabs>

                {canGenerate && c.dailyLimitReached && dailyLimitReset && (
                  <div
                    role="status"
                    className="rounded-lg border border-warm-200 bg-warm-50 p-3 text-sm text-warm-700"
                  >
                    {t("editor.dailyLimit.title", {
                      limit: String(c.dailyLimitReached.dailyLimit),
                    })}{" "}
                    {dailyLimitReset.key === "resets"
                      ? t("editor.dailyLimit.resets", {
                          hours: String(dailyLimitReset.hours),
                        })
                      : t(`editor.dailyLimit.${dailyLimitReset.key}`)}{" "}
                    {t("editor.dailyLimit.contact")}
                  </div>
                )}

                <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
                  <label className="space-y-1 text-xs font-medium text-ink-700">
                    <span>{t("crop.focalX")}</span>
                    <input
                      aria-label={t("crop.focalX")}
                      type="range"
                      min="0"
                      max="1"
                      step="0.01"
                      value={c.crop.x}
                      onChange={(event) =>
                        c.setCrop({ ...c.crop, x: Number(event.target.value) })
                      }
                      className="w-full accent-brand"
                    />
                  </label>
                  <label className="space-y-1 text-xs font-medium text-ink-700">
                    <span>{t("crop.focalY")}</span>
                    <input
                      aria-label={t("crop.focalY")}
                      type="range"
                      min="0"
                      max="1"
                      step="0.01"
                      value={c.crop.y}
                      onChange={(event) =>
                        c.setCrop({ ...c.crop, y: Number(event.target.value) })
                      }
                      className="w-full accent-brand"
                    />
                  </label>
                  <label className="space-y-1 text-xs font-medium text-ink-700">
                    <span>{t("crop.zoom")}</span>
                    <input
                      aria-label={t("crop.zoom")}
                      type="range"
                      min="1"
                      max="3"
                      step="0.05"
                      value={c.crop.zoom}
                      onChange={(event) =>
                        c.setCrop({
                          ...c.crop,
                          zoom: Number(event.target.value),
                        })
                      }
                      className="w-full accent-brand"
                    />
                  </label>
                </div>
                <Button
                  size="sm"
                  variant="light"
                  startContent={<RotateCcw className="h-3.5 w-3.5" />}
                  onPress={c.resetCrop}
                >
                  {t("crop.reset")}
                </Button>
              </Section>

              <Section title={t("sections.destination")}>
                <p className="text-xs text-ink-500">
                  {t("destinations.subtitle")}
                </p>
                <div
                  role="group"
                  aria-label={t("sections.destination")}
                  className="flex flex-wrap gap-2"
                >
                  {DESTINATION_ORDER.map((id) => {
                    const dest = destinationFor(id);
                    const selected = c.destinationId === id;
                    return (
                      <button
                        key={id}
                        type="button"
                        aria-pressed={selected}
                        disabled={editorLocked}
                        className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors ${selected ? "bg-brand text-white" : "bg-warm-100 text-ink-600 hover:bg-warm-200"}`}
                        onClick={() => c.setDestination(id)}
                      >
                        {t(dest.labelKey)}
                      </button>
                    );
                  })}
                </div>
                <p className="text-xs text-ink-500">
                  {t(activeDestination.hintKey)}
                </p>
                <div
                  className="flex flex-wrap gap-2 pt-1"
                  data-testid="destination-format-chips"
                >
                  {(isCraft
                    ? packFormatsForDestination(activeDestination)
                    : simpleFormats
                  ).map((formatId) => (
                    <button
                      key={formatId}
                      type="button"
                      aria-pressed={c.aspect === formatId}
                      className={`min-h-10 rounded-full px-3 py-1.5 text-xs font-semibold transition-colors ${c.aspect === formatId ? "bg-brand text-white" : "bg-warm-100 text-ink-600 hover:bg-warm-200"}`}
                      onClick={() => c.setAspect(formatId)}
                    >
                      {t(FORMATS[formatId].labelKey)}
                    </button>
                  ))}
                </div>
                <p className="text-[11px] text-ink-500">
                  {t("destinations.formatHint", {
                    format: t(
                      FORMATS[activeDestination.primaryFormat].labelKey,
                    ),
                  })}
                </p>
              </Section>

              <Section title={t("sections.layout")}>
                {/*
                  Craft mode: full format registry + kit / composition /
                  motion. Simple mode: on-post text slots only (photo +
                  destination already cover format). Same Scene IR either way.
                */}
                {isCraft ? (
                  <>
                    <div
                      className="flex flex-wrap gap-2"
                      data-testid="craft-format-chips"
                    >
                      {FORMAT_ORDER.map((formatId) => (
                        <button
                          key={formatId}
                          type="button"
                          aria-pressed={c.aspect === formatId}
                          className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors ${c.aspect === formatId ? "bg-brand text-white" : "bg-warm-100 text-ink-600 hover:bg-warm-200"}`}
                          onClick={() => c.setAspect(formatId)}
                        >
                          {t(FORMATS[formatId].labelKey)}
                        </button>
                      ))}
                    </div>
                    <div
                      className="space-y-2"
                      data-testid="craft-style-controls"
                    >
                      <p
                        id={styleLabelId}
                        className="text-[11px] font-semibold uppercase tracking-[0.12em] text-ink-500"
                      >
                        {t("editor.styleLabel")}
                      </p>
                      <div className="flex flex-wrap items-center gap-2">
                        <div
                          role="group"
                          aria-labelledby={styleLabelId}
                          className="flex flex-wrap gap-2"
                        >
                          {KIT_ORDER.map((id) => (
                            <button
                              key={id}
                              type="button"
                              aria-pressed={c.kit === id}
                              className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors ${c.kit === id ? "bg-brand text-white" : "bg-warm-100 text-ink-600 hover:bg-warm-200"}`}
                              onClick={() => c.setKit(id)}
                            >
                              {t(`kits.${id}`)}
                            </button>
                          ))}
                        </div>
                        <Button
                          size="sm"
                          variant="flat"
                          startContent={<Shuffle className="h-3.5 w-3.5" />}
                          onPress={c.reshuffle}
                          isDisabled={!c.canReshuffle}
                        >
                          {t("editor.reshuffle")}
                        </Button>
                      </div>
                      <div
                        role="group"
                        aria-label={t("motion.label")}
                        data-testid="craft-motion-presets"
                        className="mt-3 flex flex-wrap gap-1.5"
                      >
                        {MOTION_PRESET_ORDER.map((id) => (
                          <button
                            key={id}
                            type="button"
                            aria-pressed={c.motionPreset === id}
                            className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors ${c.motionPreset === id ? "bg-brand text-white" : "bg-warm-100 text-ink-600 hover:bg-warm-200"}`}
                            onClick={() => c.setMotionPreset(id)}
                          >
                            {t(`motion.presets.${id}`)}
                          </button>
                        ))}
                      </div>
                      <p className="mt-1.5 text-xs text-ink-500">
                        {activeDestination.motionAllowed
                          ? t("motion.hint")
                          : t("destinations.motionOptional")}
                      </p>
                    </div>
                  </>
                ) : null}
                <div className="space-y-3">
                  {EDITABLE_SLOTS.filter((key) => paintedSlots.has(key)).map(
                    (key) => (
                      <Input
                        key={key}
                        size="sm"
                        label={t(`slots.${key}`)}
                        value={c.slots[key] ?? ""}
                        onValueChange={(value) => c.setSlot(key, value)}
                      />
                    ),
                  )}
                </div>
              </Section>

              <Section title={t("sections.copy")}>
                {canGenerate ? (
                  <>
                    <div className="flex flex-wrap gap-1.5">
                      {TONES.map((tone) => (
                        <button
                          key={tone}
                          type="button"
                          aria-pressed={c.tone === tone}
                          className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors ${c.tone === tone ? "bg-brand text-white" : "bg-warm-100 text-ink-600 hover:bg-warm-200"}`}
                          onClick={() => c.setTone(tone)}
                        >
                          {t(`tones.${tone}`)}
                        </button>
                      ))}
                    </div>
                    <Select
                      size="sm"
                      label={t("editor.captionLanguage")}
                      selectedKeys={[c.language]}
                      onChange={(event) => c.setLanguage(event.target.value)}
                    >
                      {CAPTION_LANGUAGES.map((language) => (
                        <SelectItem key={language}>
                          {t(`languages.${language}`)}
                        </SelectItem>
                      ))}
                    </Select>
                  </>
                ) : null}
                <Textarea
                  minRows={4}
                  label={t("editor.caption")}
                  value={c.caption}
                  onValueChange={c.setCaption}
                  isReadOnly={c.isGeneratingCaption}
                  placeholder={t("editor.captionPlaceholder")}
                />
                <div className="flex flex-wrap items-center justify-between gap-2 text-xs">
                  <div className="flex flex-wrap items-center gap-2">
                    <span
                      className="rounded-full bg-warm-100 px-2 py-0.5 text-[11px] font-semibold text-ink-600"
                      data-caption-length={captionProfile.length}
                    >
                      {t(`destinations.captionLength.${captionProfile.length}`)}
                    </span>
                    <span
                      className={
                        captionLevel === "hard"
                          ? "font-semibold text-rose-700"
                          : captionLevel === "soft"
                            ? "font-semibold text-amber-700"
                            : "text-ink-500"
                      }
                      data-caption-budget={captionLevel}
                    >
                      {t("destinations.captionBudget", {
                        count: captionChars,
                        soft: captionProfile.softMaxChars,
                        hard: captionProfile.hardMaxChars,
                      })}
                    </span>
                  </div>
                  {captionLevel === "soft" ? (
                    <span className="text-amber-700">
                      {t("destinations.captionSoftWarn")}
                    </span>
                  ) : null}
                  {captionLevel === "hard" ? (
                    <span className="text-rose-700">
                      {t("destinations.captionHardWarn")}
                    </span>
                  ) : null}
                </div>
                {activeDestination.captionProfile.hashtags === "none" ? (
                  <p className="text-[11px] text-ink-500">
                    {t("destinations.hashtagsOff")}
                  </p>
                ) : null}
                <p className="text-[11px] text-ink-500">
                  {t("destinations.captionHouseVoiceHint")}
                </p>
                <div className="flex flex-wrap gap-2">
                  {canGenerate ? (
                    <Button
                      size="sm"
                      variant="flat"
                      startContent={
                        c.isGeneratingCaption ? (
                          <Spinner size="sm" />
                        ) : (
                          <RefreshCw className="h-4 w-4" />
                        )
                      }
                      onPress={c.regenerateCaption}
                      isDisabled={c.isGeneratingCaption}
                    >
                      {t("editor.regenerateCaption")}
                    </Button>
                  ) : null}
                  <Button
                    size="sm"
                    variant="flat"
                    startContent={<Copy className="h-4 w-4" />}
                    onPress={() => void copyAndHandoff()}
                  >
                    {t("editor.copyCaption")}
                  </Button>
                </div>
              </Section>

              <Section title={t("sections.export")}>
                <p
                  className={
                    exportNotice?.tone === "error"
                      ? "text-xs text-amber-700"
                      : "text-xs text-ink-500"
                  }
                  data-testid="export-outcome"
                  role={exportNotice?.tone === "error" ? "alert" : undefined}
                >
                  {exportNotice
                    ? t(exportNotice.key, exportNotice.params)
                    : exportInFlight
                      ? t("editor.downloading")
                      : exportReady
                        ? t("editor.exportReady")
                        : previewBroken
                          ? t("editor.exportPreviewBroken")
                          : t("editor.exportNotReady")}
                </p>
                <div className="rounded-xl border border-warm-200 bg-warm-50/80 p-3">
                  <p className="text-[11px] font-semibold uppercase tracking-[0.12em] text-ink-500">
                    {t("destinations.exportChecklist")}
                  </p>
                  <ul className="mt-2 space-y-1.5 text-xs text-ink-700">
                    {destinationPack.checklistKeys.map((key) => (
                      <li key={key} className="flex gap-2">
                        <span className="text-brand" aria-hidden>
                          •
                        </span>
                        <span>{t(`destinations.checklist.${key}`)}</span>
                      </li>
                    ))}
                  </ul>
                  <p className="mt-2 text-[11px] text-ink-500">
                    {t("destinations.packContents", {
                      formats: destinationPack.formats
                        .map((id) => t(FORMATS[id].labelKey))
                        .join(", "),
                      caption: destinationPack.includeCaptionFile
                        ? t("destinations.packWithCaption")
                        : t("destinations.packWithoutCaption"),
                    })}
                  </p>
                  {packAttribution ? (
                    <p
                      className="mt-2 text-[11px] text-ink-600"
                      data-testid="pack-promo-code"
                    >
                      {t("attribution.promoCodeLabel", {
                        code: packAttribution.promoCode,
                      })}
                    </p>
                  ) : null}
                  {printShopPreset ? (
                    <p className="mt-1 text-[11px] text-ink-500">
                      {t(printShopPreset.hintKey)}
                    </p>
                  ) : null}
                  <Button
                    className="mt-3 min-h-11"
                    color="primary"
                    startContent={<Download className="h-4 w-4" />}
                    isDisabled={!exportReady || exportInFlight}
                    isLoading={kitExport.progress !== null}
                    onPress={() => void downloadDestinationPackAndHandoff()}
                  >
                    {t(activeDestination.exportKey)}
                  </Button>
                </div>
                <div
                  className="mt-3 rounded-xl border border-warm-200 bg-warm-50/80 p-3"
                  data-testid="narrative-campaign-pack"
                >
                  <p className="text-sm font-semibold text-ink-900">
                    {t("narrativeKit.title")}
                  </p>
                  <p className="mt-1 text-xs text-ink-500">
                    {t("narrativeKit.subtitle")}
                  </p>
                  <p className="mt-2 text-[11px] font-medium text-brand">
                    {t("narrativeKit.readyNow")}
                  </p>
                  <ul
                    className="mt-2 space-y-1"
                    aria-label={t("narrativeKit.rolesLabel")}
                  >
                    {DEFAULT_NARRATIVE_ROLES.map((roleId) => {
                      const role = NARRATIVE_ROLES[roleId];
                      return (
                        <li
                          key={roleId}
                          className="flex gap-2 text-xs text-ink-700"
                        >
                          <span className="text-brand" aria-hidden>
                            •
                          </span>
                          <span>
                            <span className="font-semibold">
                              {t(role.labelKey)}
                            </span>
                            <span className="block text-ink-500">
                              {t(role.useKey)}
                            </span>
                          </span>
                        </li>
                      );
                    })}
                  </ul>
                  <p className="mt-2 text-[11px] text-ink-500">
                    {t("narrativeKit.captionAnglesNote")}
                  </p>
                  {kitExport.progress?.roleId ? (
                    <p className="mt-2 text-xs text-ink-600">
                      {t("narrativeKit.progress", {
                        current: t(
                          NARRATIVE_ROLES[kitExport.progress.roleId].labelKey,
                        ),
                        done: kitExport.progress.done + 1,
                        total: kitExport.progress.total,
                      })}
                    </p>
                  ) : null}
                  <Button
                    className="mt-3 min-h-11"
                    color="primary"
                    variant="flat"
                    startContent={<Download className="h-4 w-4" />}
                    isDisabled={!exportReady || exportInFlight}
                    isLoading={kitExport.progress !== null}
                    onPress={() => void downloadNarrativePackAndHandoff()}
                    data-testid="export-narrative-pack"
                  >
                    {t("narrativeKit.download")}
                  </Button>
                </div>
                {isCraft ? (
                  <Section title={t("campaignKit.title")}>
                    <p className="text-xs text-ink-500">
                      {t("campaignKit.subtitle")}
                    </p>
                    <div
                      role="group"
                      aria-label={t("campaignKit.formatsLabel")}
                      data-testid="craft-campaign-kit"
                      className="mt-3 flex flex-col gap-2"
                    >
                      {FORMAT_ORDER.map((id) => {
                        const format = FORMATS[id];
                        const isHero = id === c.aspect;
                        return (
                          <label
                            key={id}
                            className="flex items-start gap-2 text-xs text-ink-700"
                          >
                            <input
                              type="checkbox"
                              checked={c.kitFormats.includes(id)}
                              disabled={isHero || editorLocked}
                              onChange={() => c.toggleKitFormat(id)}
                              aria-label={t(format.labelKey)}
                            />
                            <span>
                              <span className="font-semibold">
                                {t(format.labelKey)}
                              </span>
                              <span className="block text-ink-500">
                                {t(format.useKey)}
                              </span>
                            </span>
                          </label>
                        );
                      })}
                    </div>
                    <p className="mt-2 text-[11px] text-ink-500">
                      {t("campaignKit.printHint")}
                    </p>
                    {kitExport.progress && !kitExport.progress.roleId ? (
                      <p className="mt-2 text-xs text-ink-600">
                        {t("campaignKit.progress", {
                          current: t(
                            FORMATS[kitExport.progress.current].labelKey,
                          ),
                          done: kitExport.progress.done + 1,
                          total: kitExport.progress.total,
                        })}
                      </p>
                    ) : null}
                    <Button
                      className="mt-3 min-h-11"
                      color="primary"
                      variant="flat"
                      startContent={<Download className="h-4 w-4" />}
                      isDisabled={!exportReady || exportInFlight}
                      isLoading={kitExport.progress !== null}
                      onPress={() => void downloadKitAndHandoff()}
                    >
                      {t("campaignKit.download")}
                    </Button>
                  </Section>
                ) : null}
              </Section>
            </fieldset>
          </DrawerBody>

          <DrawerFooter className="flex flex-wrap gap-2">
            {editorLocked && (
              <p
                className="mr-auto self-center text-xs font-semibold text-ink-600"
                role="status"
                aria-live="polite"
              >
                {t("editor.generationBusy")}
              </p>
            )}
            {c.error && (
              <p
                className="mr-auto self-center text-xs text-amber-700"
                role="alert"
              >
                {t(`errors.${c.error}`)}
              </p>
            )}
            <Button
              variant="flat"
              className="min-h-11"
              onPress={requestClose}
              isDisabled={editorLocked}
            >
              {t("editor.close")}
            </Button>
            <Button
              variant="flat"
              className="min-h-11"
              startContent={<Share2 className="h-4 w-4" />}
              isDisabled={!exportReady || exportInFlight}
              onPress={() => void shareAndHandoff()}
            >
              {t("editor.sharePost")}
            </Button>
            <Button
              color="primary"
              className="min-h-11"
              startContent={<Download className="h-4 w-4" />}
              isDisabled={!exportReady || exportInFlight}
              isLoading={singleBusy}
              onPress={() => void downloadAndHandoff()}
            >
              {t("editor.downloadPack")}
            </Button>
            {isCraft ? (
              !motionSupport.supported ? (
                <p className="text-xs text-amber-700">
                  {t("motion.unsupported")}
                </p>
              ) : videoProgress === null ? (
                <Button
                  variant="bordered"
                  className="min-h-11"
                  onPress={() => void downloadVideoAndHandoff()}
                  isDisabled={editorLocked || !c.photoUrl}
                >
                  {t("motion.download")}
                </Button>
              ) : (
                <div
                  className="flex items-center gap-2"
                  role="status"
                  aria-live="polite"
                >
                  <span className="text-xs font-semibold text-ink-600">
                    {t("motion.rendering", {
                      percent: String(Math.round(videoProgress * 100)),
                    })}
                  </span>
                  <Button
                    size="sm"
                    variant="light"
                    onPress={() => videoAbortRef.current?.abort()}
                  >
                    {t("motion.cancel")}
                  </Button>
                </div>
              )
            ) : null}
            {isCraft && videoError ? (
              <p className="text-xs text-amber-700" role="alert">
                {t(
                  videoError === "cancelled"
                    ? "motion.cancelled"
                    : "motion.failed",
                )}
              </p>
            ) : null}
          </DrawerFooter>
        </DrawerContent>
      </Drawer>

      <ConfirmationModal
        isOpen={discardConfirm.isOpen}
        onOpenChange={discardConfirm.onOpenChange}
        title={t("discard.title")}
        description={t("discard.body")}
        confirmLabel={t("discard.confirm")}
        cancelLabel={t("discard.cancel")}
        isDanger
        onConfirm={confirmDiscard}
      />
      <ConfirmationModal
        isOpen={generationConfirm.isOpen}
        onOpenChange={generationConfirm.onOpenChange}
        title={t("generationConfirm.title")}
        description={generationDescription}
        confirmLabel={t("generationConfirm.confirm")}
        cancelLabel={t("generationConfirm.cancel")}
        onConfirm={() => {
          void c.regeneratePhoto();
        }}
      />
    </>
  );
}
