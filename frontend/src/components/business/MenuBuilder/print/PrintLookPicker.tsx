"use client";

import React, { useEffect, useMemo, useState } from "react";
import { Check } from "lucide-react";
import type { MenuPrintAudit } from "@/lib/menuPrint/audit";
import { selectDefaultPrintImages } from "@/lib/menuPrint/directionDefaults";
import { ALL_MENU_DESIGN_FAMILIES } from "@/lib/menuPrint/families";
import { resolvePrintPalette } from "@/lib/menuPrint/palette";
import { planMenuDocument } from "@/lib/menuPrint/planner/planDocument";
import { spaciousPrintFormat } from "@/lib/menuPrint/recommend";
import { renderPlannedMenuHtml } from "@/lib/menuPrint/renderPlannedHtml";
import {
  PRINT_TYPOGRAPHY_BY_FAMILY,
  type MenuArtDirection,
  type MenuDesignFamilyId,
  type MenuDirectionRecommendation,
  type MenuOutputFormat,
  type MenuTreatment,
  type PaperFormat,
  type PrintMenuModel,
  type RegisteredMenuDesignFamily,
} from "@/lib/menuPrint/types";

export interface PrintLookPickerProps {
  model: PrintMenuModel;
  audit: MenuPrintAudit;
  selectedFamilyId: MenuDesignFamilyId;
  recommendations: MenuDirectionRecommendation[];
  /**
   * The paper stock previews plan against. Recommendations don't carry a
   * paper size of their own, so this mirrors whatever the studio currently
   * has selected (defaulting to the business's country default).
   */
  paperFormat: PaperFormat;
  tString: (key: string) => string;
  onSelect: (
    familyId: MenuDesignFamilyId,
    treatment: MenuTreatment,
    outputFormat: MenuOutputFormat,
  ) => void;
}

interface LookCandidate {
  family: RegisteredMenuDesignFamily;
  treatment: MenuTreatment;
  outputFormat: MenuOutputFormat;
  recommendation?: MenuDirectionRecommendation;
}

interface LookPreview extends LookCandidate {
  /**
   * The format the candidate asked for before any blocked-plan escalation.
   * Cache hits key on this, not the escalated outputFormat, so an escalated
   * preview stays cached instead of re-planning on every visibility change.
   */
  requestedFormat: MenuOutputFormat;
  srcDoc: string;
  /** Real planned-page geometry, in millimeters, used to fit the frame. */
  pageWidthMm: number;
  pageHeightMm: number;
}

/** How many excellent directions are surfaced before the quiet disclosure. */
const SHORTLIST_SIZE = 3;

const CSS_PX_PER_MM = 96 / 25.4;
/** Portrait A4 fallback, used only until a preview's real geometry is known. */
const FALLBACK_PREVIEW_WIDTH_PX = 210 * CSS_PX_PER_MM;
const FALLBACK_PREVIEW_HEIGHT_PX = 297 * CSS_PX_PER_MM;
const INITIAL_PREVIEW_SCALE = 0.25;

interface FittedPreviewFrameProps {
  srcDoc: string;
  title: string;
  /** Planned page size, in millimeters, so the frame fits its real shape. */
  pageWidthMm: number;
  pageHeightMm: number;
}

function fitPreviewScale(
  containerWidth: number,
  containerHeight: number,
  frameWidthPx: number,
  frameHeightPx: number,
): number | undefined {
  if (
    containerWidth <= 0 ||
    containerHeight <= 0 ||
    frameWidthPx <= 0 ||
    frameHeightPx <= 0
  ) {
    return undefined;
  }
  const scale = Math.min(
    containerWidth / frameWidthPx,
    containerHeight / frameHeightPx,
  );
  return Number.isFinite(scale) && scale > 0 ? scale : undefined;
}

function FittedPreviewFrame({
  srcDoc,
  title,
  pageWidthMm,
  pageHeightMm,
}: FittedPreviewFrameProps) {
  const wrapperRef = React.useRef<HTMLSpanElement>(null);
  const frameWidthPx =
    pageWidthMm > 0 ? pageWidthMm * CSS_PX_PER_MM : FALLBACK_PREVIEW_WIDTH_PX;
  const frameHeightPx =
    pageHeightMm > 0
      ? pageHeightMm * CSS_PX_PER_MM
      : FALLBACK_PREVIEW_HEIGHT_PX;
  const [scale, setScale] = useState(INITIAL_PREVIEW_SCALE);

  useEffect(() => {
    const wrapper = wrapperRef.current;
    if (!wrapper) return;

    const updateScale = (width: number, height: number): void => {
      const nextScale = fitPreviewScale(
        width,
        height,
        frameWidthPx,
        frameHeightPx,
      );
      if (nextScale === undefined) return;
      setScale((currentScale) =>
        currentScale === nextScale ? currentScale : nextScale,
      );
    };

    updateScale(wrapper.clientWidth, wrapper.clientHeight);
    if (typeof ResizeObserver === "undefined") return;

    const observer = new ResizeObserver((entries) => {
      const rect = entries[0]?.contentRect;
      if (rect) updateScale(rect.width, rect.height);
    });
    observer.observe(wrapper);
    return () => observer.disconnect();
  }, [frameHeightPx, frameWidthPx]);

  return (
    <span
      ref={wrapperRef}
      data-testid="print-look-preview-frame"
      className="absolute inset-0 overflow-hidden bg-warm-100"
    >
      <iframe
        srcDoc={srcDoc}
        // Same-origin is required for root-relative, self-hosted print fonts.
        // Scripts remain disabled, so srcDoc cannot execute or relax its sandbox.
        sandbox="allow-same-origin"
        title={title}
        tabIndex={-1}
        aria-hidden="true"
        loading="lazy"
        className="pointer-events-none absolute left-0 top-0 bg-white"
        style={{
          width: frameWidthPx,
          height: frameHeightPx,
          transform: `scale(${scale})`,
          transformOrigin: "top left",
          border: 0,
        }}
      />
    </span>
  );
}

function compatibleTreatment(
  family: RegisteredMenuDesignFamily,
  recommended: MenuTreatment,
): MenuTreatment {
  return family.supportedTreatments.includes(recommended)
    ? recommended
    : family.supportedTreatments[0];
}

function compatibleFormat(
  family: RegisteredMenuDesignFamily,
  recommended: MenuOutputFormat,
): MenuOutputFormat {
  return family.supportedFormats.includes(recommended)
    ? recommended
    : family.supportedFormats[0];
}

function buildPreview(
  candidate: LookCandidate,
  model: PrintMenuModel,
  audit: MenuPrintAudit,
  paperFormat: PaperFormat,
): LookPreview {
  const { family, treatment } = candidate;
  const makeDirection = (outputFormat: MenuOutputFormat): MenuArtDirection => ({
    familyId: family.id,
    treatment,
    outputFormat,
    paperFormat,
    palette: resolvePrintPalette({
      familyId: family.id,
      primaryColor: model.business.primaryColor,
      secondaryColor: model.business.secondaryColor,
    }),
    imageSelections: selectDefaultPrintImages({
      family,
      model,
      audit,
      treatment,
    }),
    imageFocalPoints: {},
    logoTreatment: "contained",
    coverMode: "none",
    ornamentIntensity: "restrained",
    typographyPersonality: PRINT_TYPOGRAPHY_BY_FAMILY[family.id][0],
    contactPlacement: "footer",
  });
  let { outputFormat } = candidate;
  let document = planMenuDocument({
    model,
    audit,
    direction: makeDirection(outputFormat),
  });
  // Only a handful of families carry a real recommendation; the rest inherit
  // the top pick's format, which a fuller menu can overflow. When the planner
  // blocks, retry once on the family's spacious format so every look card
  // previews — and, on select, applies — a menu that actually fits.
  const spacious = spaciousPrintFormat(family.id);
  if (
    document.readiness === "blocked" &&
    spacious &&
    spacious !== outputFormat &&
    family.supportedFormats.includes(spacious)
  ) {
    outputFormat = spacious;
    document = planMenuDocument({
      model,
      audit,
      direction: makeDirection(outputFormat),
    });
  }
  return {
    ...candidate,
    outputFormat,
    requestedFormat: candidate.outputFormat,
    srcDoc: renderPlannedMenuHtml(model, document, { origin: "" }),
    pageWidthMm: document.geometry.widthMm,
    pageHeightMm: document.geometry.heightMm,
  };
}

interface LookCardProps {
  preview: LookPreview;
  selected: boolean;
  best: boolean;
  tString: (key: string) => string;
  onSelect: (
    familyId: MenuDesignFamilyId,
    treatment: MenuTreatment,
    outputFormat: MenuOutputFormat,
  ) => void;
}

function LookCard({
  preview,
  selected,
  best,
  tString,
  onSelect,
}: LookCardProps) {
  const familyName = tString(preview.family.nameKey);

  return (
    <label
      className={[
        "group relative flex w-full cursor-pointer flex-col overflow-hidden rounded-2xl border bg-white text-left",
        "transition-[border-color,box-shadow,transform] duration-200 active:translate-y-px",
        "focus-within:outline-none focus-within:ring-2 focus-within:ring-brand/40 focus-within:ring-offset-2",
        selected
          ? "border-brand shadow-md shadow-brand/10"
          : "border-warm-200 hover:border-warm-300 hover:shadow-md hover:shadow-ink-950/5",
      ].join(" ")}
    >
      <input
        type="radio"
        name="print-menu-family"
        value={preview.family.id}
        checked={selected}
        aria-label={familyName}
        onChange={() =>
          onSelect(preview.family.id, preview.treatment, preview.outputFormat)
        }
        className="sr-only"
      />
      <span className="relative block aspect-[210/297] w-full overflow-hidden border-b border-warm-200 bg-warm-100">
        <FittedPreviewFrame
          srcDoc={preview.srcDoc}
          title={tString("print.look.previewTitle")}
          pageWidthMm={preview.pageWidthMm}
          pageHeightMm={preview.pageHeightMm}
        />
        {best ? (
          <span
            data-testid="print-look-best-badge"
            className="absolute left-3 top-3 rounded-full bg-brand px-3 py-1 text-[11px] font-semibold text-white shadow-sm"
          >
            {tString("print.look.recommended")}
          </span>
        ) : null}
        {selected ? (
          <span
            data-testid="print-look-selected-check"
            aria-hidden="true"
            className="absolute right-3 top-3 inline-flex h-8 w-8 items-center justify-center rounded-full bg-brand text-white shadow-sm"
          >
            <Check size={17} strokeWidth={2.25} />
          </span>
        ) : null}
      </span>

      <span className="flex w-full flex-col gap-1.5 p-4">
        <span className="text-base font-semibold leading-tight text-ink-950">
          {familyName}
        </span>
        <span className="text-xs leading-relaxed text-ink-600">
          {preview.recommendation
            ? tString(preview.recommendation.reasonKey)
            : tString(preview.family.descriptionKey)}
        </span>
      </span>
    </label>
  );
}

export function PrintLookPicker({
  model,
  audit,
  selectedFamilyId,
  recommendations,
  paperFormat,
  tString,
  onSelect,
}: PrintLookPickerProps) {
  const [showAll, setShowAll] = useState(false);

  const orderedCandidates = useMemo<LookCandidate[]>(() => {
    const recommendationsByFamily = new Map(
      recommendations.map((recommendation) => [
        recommendation.familyId,
        recommendation,
      ]),
    );
    const defaultTreatment = recommendations[0]?.treatment ?? "balanced";
    const defaultFormat = recommendations[0]?.outputFormat ?? "single-sheet";
    const registryOrder = new Map(
      ALL_MENU_DESIGN_FAMILIES.map((family, index) => [family.id, index]),
    );
    return ALL_MENU_DESIGN_FAMILIES.map((family) => {
      const recommendation = recommendationsByFamily.get(family.id);
      return {
        family,
        recommendation,
        treatment: compatibleTreatment(
          family,
          recommendation?.treatment ?? defaultTreatment,
        ),
        outputFormat: compatibleFormat(
          family,
          recommendation?.outputFormat ?? defaultFormat,
        ),
      };
    }).sort((first, second) => {
      const firstScore = first.recommendation?.score;
      const secondScore = second.recommendation?.score;
      if (firstScore !== undefined && secondScore !== undefined) {
        return (
          secondScore - firstScore ||
          (registryOrder.get(first.family.id) ?? 0) -
            (registryOrder.get(second.family.id) ?? 0)
        );
      }
      if (firstScore !== undefined) return -1;
      if (secondScore !== undefined) return 1;
      return (
        (registryOrder.get(first.family.id) ?? 0) -
        (registryOrder.get(second.family.id) ?? 0)
      );
    });
  }, [recommendations]);

  // Previews are expensive, so each family is planned and rendered at most once
  // per menu. The cache is scoped to the current content and resets with it.
  const cacheRef = React.useRef<{
    model: PrintMenuModel;
    audit: MenuPrintAudit;
    paperFormat: PaperFormat;
    previews: Map<MenuDesignFamilyId, LookPreview>;
  }>({ model, audit, paperFormat, previews: new Map() });
  if (
    cacheRef.current.model !== model ||
    cacheRef.current.audit !== audit ||
    cacheRef.current.paperFormat !== paperFormat
  ) {
    cacheRef.current = { model, audit, paperFormat, previews: new Map() };
  }
  const previewCache = cacheRef.current.previews;
  const visibleCandidates = useMemo(() => {
    const shortlist = orderedCandidates.slice(0, SHORTLIST_SIZE);
    if (showAll) return orderedCandidates;
    return shortlist.some(({ family }) => family.id === selectedFamilyId) ||
      !orderedCandidates.some(({ family }) => family.id === selectedFamilyId)
      ? shortlist
      : [
          ...shortlist,
          ...orderedCandidates.filter(
            ({ family }) => family.id === selectedFamilyId,
          ),
        ];
  }, [orderedCandidates, selectedFamilyId, showAll]);
  const previews = useMemo(
    () =>
      visibleCandidates.map((candidate) => {
        const cached = previewCache.get(candidate.family.id);
        if (
          cached &&
          cached.treatment === candidate.treatment &&
          cached.requestedFormat === candidate.outputFormat
        ) {
          return cached;
        }
        const preview = buildPreview(candidate, model, audit, paperFormat);
        previewCache.set(candidate.family.id, preview);
        return preview;
      }),
    [audit, model, paperFormat, previewCache, visibleCandidates],
  );
  const bestFamilyId = orderedCandidates[0]?.recommendation
    ? orderedCandidates[0].family.id
    : undefined;

  return (
    <section className="space-y-6">
      <div className="space-y-1">
        <h3 className="text-lg font-semibold text-ink-950">
          {tString("print.look.title")}
        </h3>
        <p className="text-sm leading-relaxed text-ink-600">
          {model.business.name}
        </p>
        <p className="text-sm leading-relaxed text-ink-600">
          {tString("print.look.help")}
        </p>
      </div>

      <div
        className="grid gap-5 sm:grid-cols-2 xl:grid-cols-3"
        role="radiogroup"
        aria-label={tString("print.look.familyLabel")}
      >
        {previews.map((preview) => (
          <LookCard
            key={preview.family.id}
            preview={preview}
            selected={preview.family.id === selectedFamilyId}
            best={preview.family.id === bestFamilyId}
            tString={tString}
            onSelect={onSelect}
          />
        ))}
      </div>

      {orderedCandidates.length > visibleCandidates.length || showAll ? (
        <div>
          <button
            type="button"
            onClick={() => setShowAll((current) => !current)}
            className="min-h-9 rounded-full border border-warm-200 bg-white px-4 text-sm font-medium text-ink-700 transition-colors hover:border-warm-300 hover:bg-warm-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-2"
          >
            {tString(showAll ? "print.look.showFewer" : "print.look.showAll")}
          </button>
        </div>
      ) : null}
    </section>
  );
}
