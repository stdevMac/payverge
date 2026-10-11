"use client";

import NextImage from "next/image";
import React, { useEffect, useMemo, useRef, useState } from "react";

import type { MenuPrintAudit } from "@/lib/menuPrint/audit";
import {
  MENU_TREATMENTS,
  type FeaturedImageSelection,
  type ImageFocalPoint,
  type ImageRole,
  type MenuTreatment,
  type PrintMenuItem,
  type PrintMenuModel,
  type RegisteredMenuDesignFamily,
} from "@/lib/menuPrint/types";

const TREATMENT_KEY_PARTS: Record<MenuTreatment, string> = {
  "type-led": "typeLed",
  balanced: "balanced",
  "photo-led": "photoLed",
  compact: "compact",
};

const FAMILY_KEY_PARTS: Record<RegisteredMenuDesignFamily["id"], string> = {
  atelier: "atelier",
  maison: "maison",
  osteria: "osteria",
  "night-house": "nightHouse",
  counter: "counter",
  street: "street",
  field: "field",
  gallery: "gallery",
};

const ROLE_KEY_PARTS: Record<ImageRole, string> = {
  cover: "cover",
  "full-width": "fullWidth",
  "half-page": "halfPage",
  "editorial-crop": "editorialCrop",
  "category-opener": "categoryOpener",
  paired: "paired",
  "compact-tile": "compactTile",
};

const FOCAL_POSITIONS = [
  { key: "topLeft", point: { x: 0, y: 0 } },
  { key: "top", point: { x: 0.5, y: 0 } },
  { key: "topRight", point: { x: 1, y: 0 } },
  { key: "left", point: { x: 0, y: 0.5 } },
  { key: "center", point: { x: 0.5, y: 0.5 } },
  { key: "right", point: { x: 1, y: 0.5 } },
  { key: "bottomLeft", point: { x: 0, y: 1 } },
  { key: "bottom", point: { x: 0.5, y: 1 } },
  { key: "bottomRight", point: { x: 1, y: 1 } },
] as const;

interface PrintImageMeasurement {
  url?: string;
  status: "loading" | "ready" | "failed" | "timeout";
  width?: number;
  height?: number;
}

export interface PrintPhotoControlsProps {
  family: RegisteredMenuDesignFamily;
  treatment: MenuTreatment;
  model: PrintMenuModel;
  audit: MenuPrintAudit;
  measurements: Record<string, PrintImageMeasurement>;
  selections: readonly FeaturedImageSelection[];
  focalPoints: Record<string, ImageFocalPoint>;
  tString: (key: string) => string;
  onTreatmentChange: (treatment: MenuTreatment) => void;
  onSelectionChange: (selections: FeaturedImageSelection[]) => void;
  onFocalPointChange: (url: string, point: ImageFocalPoint) => void;
}

interface ItemPhoto {
  item: PrintMenuItem;
  sectionId: string;
  url: string;
  measurement?: PrintImageMeasurement;
  role: ImageRole;
  failed: boolean;
  timedOut: boolean;
  lowResolution: boolean;
  selectable: boolean;
}

function selectionsEqual(
  first: readonly FeaturedImageSelection[],
  second: readonly FeaturedImageSelection[],
): boolean {
  return (
    first.length === second.length &&
    first.every(
      (selection, index) =>
        selection.itemId === second[index]?.itemId &&
        selection.url === second[index]?.url &&
        selection.role === second[index]?.role,
    )
  );
}

function normalizeRenderableSelections(
  selections: readonly FeaturedImageSelection[],
  photos: readonly ItemPhoto[],
  family: RegisteredMenuDesignFamily,
): FeaturedImageSelection[] {
  const photosByItem = new Map(
    photos.map((photo) => [photo.item.id, photo] as const),
  );
  const usedSections = new Set<string>();
  let coverUsed = false;
  const normalized: FeaturedImageSelection[] = [];

  for (const selection of selections) {
    if (normalized.length >= family.photography.maxFeatureImagesPerPage) break;
    const photo = photosByItem.get(selection.itemId);
    if (!photo || photo.url !== selection.url) continue;
    const measurementReady = photo.measurement?.status === "ready";
    if (photo.failed || photo.timedOut) continue;
    let role =
      measurementReady && photo.lowResolution ? "compact-tile" : selection.role;
    if (measurementReady && !photo.selectable) continue;
    if (!family.photography.roles.includes(role)) continue;
    if (role === "cover") {
      if (coverUsed) {
        const nonCoverRole = family.photography.roles.find(
          (candidate) => candidate !== "cover",
        );
        if (!nonCoverRole) continue;
        role = nonCoverRole;
      } else {
        coverUsed = true;
      }
    }
    if (role !== "cover") {
      if (usedSections.has(photo.sectionId)) continue;
      usedSections.add(photo.sectionId);
    }
    normalized.push({ ...selection, role });
  }

  return normalized;
}

function samePoint(
  first: ImageFocalPoint | undefined,
  second: ImageFocalPoint,
): boolean {
  return first?.x === second.x && first.y === second.y;
}

function compatibilityKey(
  family: RegisteredMenuDesignFamily,
  treatment: MenuTreatment,
): string {
  const treatmentPart = TREATMENT_KEY_PARTS[treatment];
  return `print.compatibility.${FAMILY_KEY_PARTS[family.id]}${treatmentPart[0].toUpperCase()}${treatmentPart.slice(1)}Unavailable`;
}

function treatmentClasses(active: boolean, disabled: boolean): string {
  if (disabled) {
    return "cursor-not-allowed border-warm-200 bg-warm-100 text-ink-500";
  }
  return active
    ? "border-brand bg-brand-50 text-brand-800 ring-1 ring-brand/20"
    : "border-warm-200 bg-white text-ink-800 hover:border-warm-300 hover:bg-warm-50";
}

function measurementLabel(
  photo: ItemPhoto,
  tString: (key: string) => string,
): string {
  if (photo.timedOut) return tString("print.photos.timeout");
  if (photo.failed) return tString("print.photos.failed");
  if (photo.lowResolution) return tString("print.photos.lowResolution");
  // Raw pixel dimensions are internal diagnostics; operators only need to know
  // whether the photo will hold up in print.
  if (photo.measurement?.status === "ready") {
    return tString("print.photos.printReady");
  }
  return tString("print.photos.measuring");
}

interface FocalGridProps {
  url: string;
  value?: ImageFocalPoint;
  tString: (key: string) => string;
  onChange: (url: string, point: ImageFocalPoint) => void;
}

function FocalGrid({ url, value, tString, onChange }: FocalGridProps) {
  const buttonRefs = useRef<Array<HTMLButtonElement | null>>([]);

  const selectIndex = (index: number) => {
    const normalized = Math.max(0, Math.min(FOCAL_POSITIONS.length - 1, index));
    const { point } = FOCAL_POSITIONS[normalized];
    buttonRefs.current[normalized]?.focus();
    onChange(url, point);
  };

  const onKeyDown = (
    event: React.KeyboardEvent<HTMLButtonElement>,
    index: number,
  ) => {
    const row = Math.floor(index / 3);
    const column = index % 3;
    let nextIndex: number | undefined;
    if (event.key === "ArrowRight") nextIndex = row * 3 + ((column + 1) % 3);
    if (event.key === "ArrowLeft") nextIndex = row * 3 + ((column + 2) % 3);
    if (event.key === "ArrowDown") nextIndex = ((row + 1) % 3) * 3 + column;
    if (event.key === "ArrowUp") nextIndex = ((row + 2) % 3) * 3 + column;
    if (event.key === "Home") nextIndex = 0;
    if (event.key === "End") nextIndex = 8;
    if (nextIndex === undefined) return;
    event.preventDefault();
    selectIndex(nextIndex);
  };

  return (
    <div className="space-y-2">
      <p className="text-xs font-semibold text-ink-700">
        {tString("print.focal.label")}
      </p>
      <div
        role="group"
        aria-label={tString("print.focal.label")}
        className="grid w-32 grid-cols-3 gap-1"
      >
        {FOCAL_POSITIONS.map(({ key, point }, index) => {
          const active = samePoint(value, point);
          return (
            <button
              key={key}
              ref={(node) => {
                buttonRefs.current[index] = node;
              }}
              type="button"
              aria-label={tString(`print.focal.${key}`)}
              aria-pressed={active}
              onClick={() => onChange(url, point)}
              onKeyDown={(event) => onKeyDown(event, index)}
              className={[
                "flex h-9 w-9 items-center justify-center rounded-md border transition-colors",
                "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-1",
                active
                  ? "border-brand bg-brand text-white"
                  : "border-warm-300 bg-white text-ink-700 hover:bg-warm-50",
              ].join(" ")}
            >
              <span
                aria-hidden="true"
                className={[
                  "h-1.5 w-1.5 rounded-full",
                  active ? "bg-white" : "bg-ink-500",
                ].join(" ")}
              />
            </button>
          );
        })}
      </div>
    </div>
  );
}

export function PrintPhotoControls({
  family,
  treatment,
  model,
  audit,
  measurements,
  selections,
  focalPoints,
  tString,
  onTreatmentChange,
  onSelectionChange,
  onFocalPointChange,
}: PrintPhotoControlsProps) {
  const [activeItemId, setActiveItemId] = useState<string | undefined>(
    selections[0]?.itemId,
  );
  const failedUrls = useMemo(
    () => new Set(audit.failedUrls),
    [audit.failedUrls],
  );
  const lowResolutionUrls = useMemo(
    () => new Set(audit.lowResolutionUrls),
    [audit.lowResolutionUrls],
  );
  const supportsCompactTile = family.photography.roles.includes("compact-tile");
  const itemPhotos = useMemo<ItemPhoto[]>(
    () =>
      model.sections.flatMap((section) =>
        section.items.flatMap((item) => {
          const selected = selections.find(({ itemId }) => itemId === item.id);
          const url = selected?.url ?? item.imageCandidates?.[0];
          if (!url) return [];
          const measurement = measurements[url];
          const failed =
            failedUrls.has(url) || measurement?.status === "failed";
          const timedOut = measurement?.status === "timeout";
          const lowResolution = lowResolutionUrls.has(url);
          const defaultRole = family.photography.roles[0];
          const role: ImageRole = lowResolution
            ? "compact-tile"
            : (selected?.role ??
              (defaultRole === "cover" &&
              selections.some((selection) => selection.role === "cover")
                ? (family.photography.roles.find(
                    (candidate) => candidate !== "cover",
                  ) ?? defaultRole)
                : defaultRole));
          return [
            {
              item,
              sectionId: section.id,
              url,
              measurement,
              role,
              failed,
              timedOut,
              lowResolution,
              selectable:
                measurement?.status === "ready" &&
                !failed &&
                !timedOut &&
                (!lowResolution || supportsCompactTile),
            },
          ];
        }),
      ),
    [
      failedUrls,
      family.photography.roles,
      lowResolutionUrls,
      measurements,
      model.sections,
      selections,
      supportsCompactTile,
    ],
  );
  const renderableSelections = useMemo(
    () => normalizeRenderableSelections(selections, itemPhotos, family),
    [family, itemPhotos, selections],
  );
  const selectedItemIds = useMemo(
    () => new Set(renderableSelections.map(({ itemId }) => itemId)),
    [renderableSelections],
  );

  useEffect(() => {
    if (!selectionsEqual(selections, renderableSelections)) {
      onSelectionChange(renderableSelections);
    }
  }, [onSelectionChange, renderableSelections, selections]);

  useEffect(() => {
    if (
      activeItemId &&
      !itemPhotos.some(({ item }) => item.id === activeItemId)
    ) {
      setActiveItemId(selections[0]?.itemId);
    }
  }, [activeItemId, itemPhotos, selections]);

  const showImageControls =
    treatment === "balanced" || treatment === "photo-led";
  const changeTreatment = (option: MenuTreatment) => {
    if (option === "type-led" && selections.length > 0) {
      onSelectionChange([]);
      setActiveItemId(undefined);
    }
    onTreatmentChange(option);
  };
  const togglePhoto = (photo: ItemPhoto) => {
    if (!photo.selectable) return;
    if (selectedItemIds.has(photo.item.id)) {
      onSelectionChange(
        renderableSelections.filter(({ itemId }) => itemId !== photo.item.id),
      );
      if (activeItemId === photo.item.id) setActiveItemId(undefined);
      return;
    }
    setActiveItemId(photo.item.id);
    const itemSection = new Map(
      itemPhotos.map(({ item, sectionId }) => [item.id, sectionId] as const),
    );
    const role = photo.lowResolution ? "compact-tile" : photo.role;
    let next = renderableSelections.filter((selection) => {
      if (selection.itemId === photo.item.id) return false;
      if (role === "cover") return selection.role !== "cover";
      return (
        selection.role === "cover" ||
        itemSection.get(selection.itemId) !== photo.sectionId
      );
    });
    const capacity = family.photography.maxFeatureImagesPerPage;
    while (next.length >= capacity) {
      const removable = next.findIndex(({ role: currentRole }) =>
        role === "cover" ? currentRole === "cover" : currentRole !== "cover",
      );
      next.splice(removable < 0 ? 0 : removable, 1);
    }
    next.push({ itemId: photo.item.id, url: photo.url, role });
    onSelectionChange(normalizeRenderableSelections(next, itemPhotos, family));
  };

  return (
    <div className="space-y-8 text-ink-950">
      <fieldset className="space-y-3">
        <legend className="text-heading-sm text-ink-950">
          {tString("print.treatment.label")}
        </legend>
        <p className="max-w-2xl text-sm leading-relaxed text-ink-600">
          {tString("print.treatment.help")}
        </p>
        <div className="grid gap-3 sm:grid-cols-2">
          {MENU_TREATMENTS.map((option) => {
            const supported = family.supportedTreatments.includes(option);
            const active = treatment === option;
            const descriptionId = `treatment-${option}-compatibility`;
            return (
              <div key={option} className="space-y-2">
                <button
                  type="button"
                  aria-label={tString(
                    `print.treatment.${TREATMENT_KEY_PARTS[option]}`,
                  )}
                  aria-pressed={active}
                  aria-describedby={supported ? undefined : descriptionId}
                  disabled={!supported}
                  onClick={() => changeTreatment(option)}
                  className={[
                    "flex min-h-20 w-full flex-col items-start justify-center rounded-xl border px-4 py-3 text-left",
                    "transition-[border-color,background-color,transform] active:translate-y-px",
                    "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-2",
                    treatmentClasses(active, !supported),
                  ].join(" ")}
                >
                  <span className="text-sm font-semibold">
                    {tString(`print.treatment.${TREATMENT_KEY_PARTS[option]}`)}
                  </span>
                  <span className="mt-1 text-xs leading-relaxed opacity-80">
                    {tString(
                      `print.treatmentDesc.${TREATMENT_KEY_PARTS[option]}`,
                    )}
                  </span>
                </button>
                {!supported ? (
                  <p
                    id={descriptionId}
                    className="text-xs leading-relaxed text-ink-600"
                  >
                    {tString(compatibilityKey(family, option))}
                  </p>
                ) : null}
              </div>
            );
          })}
        </div>
      </fieldset>

      {showImageControls ? (
        <section aria-labelledby="print-photo-selection" className="space-y-4">
          <div className="space-y-1">
            <h3
              id="print-photo-selection"
              className="text-heading-sm text-ink-950"
            >
              {tString("print.photos.selection")}
            </h3>
            <p className="text-sm leading-relaxed text-ink-600">
              {tString("print.photos.selectionHelp")}
            </p>
          </div>

          {itemPhotos.length === 0 ? (
            <div className="rounded-xl border border-dashed border-warm-300 bg-warm-50 p-6 text-center">
              <p className="font-medium text-ink-800">
                {tString("print.photos.empty")}
              </p>
              <p className="mt-1 text-sm text-ink-600">
                {tString("print.photos.emptyHelp")}
              </p>
            </div>
          ) : (
            <div className="space-y-3">
              {itemPhotos.map((photo) => {
                const selected = selectedItemIds.has(photo.item.id);
                const focalVisible = activeItemId === photo.item.id || selected;
                return (
                  <article
                    key={photo.item.id}
                    className={[
                      "grid gap-4 rounded-xl border p-4 sm:grid-cols-[7rem_minmax(0,1fr)_auto]",
                      selected
                        ? "border-brand bg-brand-50/40"
                        : "border-warm-200 bg-white",
                    ].join(" ")}
                  >
                    <div className="relative aspect-[4/3] overflow-hidden rounded-lg bg-warm-100">
                      {photo.failed || photo.timedOut ? (
                        <span className="flex h-full items-center justify-center px-3 text-center text-xs font-medium text-ink-600">
                          {photo.timedOut
                            ? tString("print.photos.timeout")
                            : tString("print.photos.failed")}
                        </span>
                      ) : (
                        <NextImage
                          src={photo.url}
                          alt={photo.item.name}
                          fill
                          sizes="112px"
                          unoptimized
                          className="object-cover"
                        />
                      )}
                    </div>
                    <div className="min-w-0 space-y-2">
                      <div>
                        <h4 className="truncate text-sm font-semibold text-ink-900">
                          {photo.item.name}
                        </h4>
                        <p className="text-xs text-ink-600">
                          {measurementLabel(photo, tString)}
                        </p>
                      </div>
                      <p className="text-xs font-medium text-ink-700">
                        {tString("print.photos.role")}:{" "}
                        <span>
                          {tString(
                            `print.imageRole.${ROLE_KEY_PARTS[photo.role]}`,
                          )}
                        </span>
                      </p>
                      {photo.lowResolution && !photo.selectable ? (
                        <p className="text-xs leading-relaxed text-ink-600">
                          {tString("print.photos.compactTileUnavailable")}
                        </p>
                      ) : null}
                    </div>
                    <div className="flex items-start justify-end">
                      <label className="flex cursor-pointer items-center gap-2 text-sm font-medium text-ink-800">
                        <input
                          type="checkbox"
                          aria-label={photo.item.name}
                          checked={selected}
                          disabled={!photo.selectable}
                          onChange={() => togglePhoto(photo)}
                          className="h-4 w-4 rounded border-warm-300 text-brand focus:ring-brand/40 disabled:cursor-not-allowed"
                        />
                      </label>
                    </div>
                    {focalVisible && photo.selectable ? (
                      <div className="sm:col-start-2 sm:col-end-4">
                        <FocalGrid
                          url={photo.url}
                          value={focalPoints[photo.url]}
                          tString={tString}
                          onChange={onFocalPointChange}
                        />
                      </div>
                    ) : null}
                  </article>
                );
              })}
            </div>
          )}
        </section>
      ) : (
        <p className="rounded-xl border border-warm-200 bg-warm-50 p-4 text-sm text-ink-700">
          {tString(`print.photos.${TREATMENT_KEY_PARTS[treatment]}NoSelection`)}
        </p>
      )}
    </div>
  );
}
