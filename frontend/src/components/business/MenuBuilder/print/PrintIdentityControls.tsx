"use client";

import React from "react";

import type {
  CoverMode,
  LogoTreatment,
  MenuArtDirection,
  OrnamentIntensity,
  PrintContactPlacement,
  PrintTypographyPersonality,
  ResolvedPrintPalette,
  RegisteredMenuDesignFamily,
} from "@/lib/menuPrint/types";
import {
  PRINT_CONTACT_PLACEMENTS,
  PRINT_TYPOGRAPHY_BY_FAMILY,
} from "@/lib/menuPrint/types";

const LOGO_TREATMENTS: readonly LogoTreatment[] = [
  "wordmark",
  "contained",
  "mark-only",
  "hidden",
];
const ORNAMENT_OPTIONS: readonly OrnamentIntensity[] = [
  "none",
  "restrained",
  "expressive",
];
const COVER_OPTIONS: readonly CoverMode[] = [
  "none",
  "typographic",
  "photographic",
];
const LOGO_KEY_PARTS: Record<LogoTreatment, string> = {
  wordmark: "wordmark",
  contained: "contained",
  "mark-only": "markOnly",
  hidden: "hidden",
};

export interface PrintIdentityControlsProps {
  family: RegisteredMenuDesignFamily;
  direction: MenuArtDirection;
  paletteSource?: ResolvedPrintPalette["source"];
  includeQr?: boolean;
  canIncludeQr: boolean;
  tString: (key: string) => string;
  onLogoTreatmentChange: (treatment: LogoTreatment) => void;
  onPaletteChange: (palette: ResolvedPrintPalette) => void;
  onTypographyChange: (personality: PrintTypographyPersonality) => void;
  onOrnamentChange: (intensity: OrnamentIntensity) => void;
  onCoverChange: (mode: CoverMode) => void;
  onQrChange: (include: boolean) => void;
  onContactPlacementChange: (placement: PrintContactPlacement) => void;
}

function optionClasses(active: boolean, disabled = false): string {
  if (disabled) {
    return "cursor-not-allowed border-warm-200 bg-warm-100 text-ink-500";
  }
  return active
    ? "border-brand bg-brand-50 text-brand-800 ring-1 ring-brand/20"
    : "border-warm-200 bg-white text-ink-800 hover:border-warm-300 hover:bg-warm-50";
}

interface OptionGroupProps<T extends string> {
  label: string;
  options: readonly T[];
  value: T;
  keyFor: (option: T) => string;
  isDisabled?: (option: T) => boolean;
  onChange: (option: T) => void;
}

function OptionGroup<T extends string>({
  label,
  options,
  value,
  keyFor,
  isDisabled = () => false,
  onChange,
}: OptionGroupProps<T>) {
  return (
    <fieldset className="space-y-3">
      <legend className="text-sm font-semibold text-ink-800">{label}</legend>
      <div className="flex flex-wrap gap-2">
        {options.map((option) => {
          const disabled = isDisabled(option);
          return (
            <button
              key={option}
              type="button"
              aria-label={keyFor(option)}
              aria-pressed={value === option}
              disabled={disabled}
              onClick={() => onChange(option)}
              className={[
                "min-h-10 rounded-full border px-4 py-2 text-sm font-medium transition-colors",
                "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-2",
                optionClasses(value === option, disabled),
              ].join(" ")}
            >
              {keyFor(option)}
            </button>
          );
        })}
      </div>
    </fieldset>
  );
}

export function PrintIdentityControls({
  family,
  direction,
  paletteSource = direction.palette.source,
  includeQr = false,
  canIncludeQr,
  tString,
  onLogoTreatmentChange,
  onPaletteChange,
  onTypographyChange,
  onOrnamentChange,
  onCoverChange,
  onQrChange,
  onContactPlacementChange,
}: PrintIdentityControlsProps) {
  const typographyOptions = PRINT_TYPOGRAPHY_BY_FAMILY[family.id];
  const paletteEntries = [
    ["ground", direction.palette.ground],
    ["ink", direction.palette.ink],
    ["accent", direction.palette.accent],
    ["muted", direction.palette.muted],
  ] as const;
  const supportsPhotographicCover = family.photography.roles.includes("cover");

  return (
    <div className="space-y-8 text-ink-950">
      <div className="space-y-2">
        <h3 className="text-heading-sm text-ink-950">
          {tString("print.identity.label")}
        </h3>
        <p className="max-w-2xl text-sm leading-relaxed text-ink-600">
          {tString("print.identity.help")}
        </p>
      </div>

      <OptionGroup
        label={tString("print.logo.label")}
        options={LOGO_TREATMENTS}
        value={direction.logoTreatment}
        keyFor={(option) => tString(`print.logo.${LOGO_KEY_PARTS[option]}`)}
        onChange={onLogoTreatmentChange}
      />

      <fieldset className="space-y-3">
        <legend className="text-sm font-semibold text-ink-800">
          {tString("print.palette.label")}
        </legend>
        <p className="text-sm leading-relaxed text-ink-600">
          {tString(
            `print.palette.${paletteSource === "restaurant" ? "restaurant" : "familyFallback"}`,
          )}
        </p>
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          {paletteEntries.map(([role, color]) => (
            <div
              key={role}
              data-testid="print-palette-swatch"
              className="overflow-hidden rounded-lg border border-warm-200 bg-white"
            >
              <span
                aria-label={tString(`print.palette.${role}`)}
                role="img"
                className="block h-12 w-full border-b border-warm-200"
                style={{ backgroundColor: color }}
              />
              <span className="block px-2 py-2 text-xs font-medium text-ink-700">
                {tString(`print.palette.${role}`)}
              </span>
            </div>
          ))}
        </div>
        <label className="flex max-w-sm items-center justify-between gap-4 rounded-lg border border-warm-200 bg-white p-3 text-sm font-medium text-ink-800">
          <span>{tString("print.palette.override")}</span>
          <input
            type="color"
            aria-label={tString("print.palette.override")}
            value={direction.palette.accent}
            onChange={(event) =>
              onPaletteChange({
                ...direction.palette,
                accent: event.target.value,
                source: "restaurant",
              })
            }
            className="h-9 w-12 cursor-pointer rounded border border-warm-300 bg-white p-1 focus:outline-none focus:ring-2 focus:ring-brand/40"
          />
        </label>
      </fieldset>

      <OptionGroup
        label={tString("print.typography.label")}
        options={typographyOptions}
        value={direction.typographyPersonality}
        keyFor={(option) => tString(`print.typography.${option}`)}
        onChange={onTypographyChange}
      />

      <OptionGroup
        label={tString("print.ornament.label")}
        options={ORNAMENT_OPTIONS}
        value={direction.ornamentIntensity}
        keyFor={(option) => tString(`print.ornament.${option}`)}
        onChange={onOrnamentChange}
      />

      <div className="space-y-2">
        <OptionGroup
          label={tString("print.cover.label")}
          options={COVER_OPTIONS}
          value={direction.coverMode}
          keyFor={(option) => tString(`print.cover.${option}`)}
          isDisabled={(option) =>
            option === "photographic" && !supportsPhotographicCover
          }
          onChange={onCoverChange}
        />
        {!supportsPhotographicCover ? (
          <p className="text-xs leading-relaxed text-ink-600">
            {tString("print.compatibility.photographicCoverUnavailable")}
          </p>
        ) : null}
      </div>

      <fieldset className="space-y-3">
        <legend className="text-sm font-semibold text-ink-800">
          {tString("print.qr.label")}
        </legend>
        <label
          className={[
            "flex min-h-12 items-center justify-between gap-4 rounded-lg border px-4 py-3 text-sm font-medium",
            canIncludeQr
              ? "cursor-pointer border-warm-200 bg-white text-ink-800 hover:bg-warm-50"
              : "cursor-not-allowed border-warm-200 bg-warm-100 text-ink-500",
          ].join(" ")}
        >
          <span>{tString("print.qr.include")}</span>
          <input
            type="checkbox"
            aria-label={tString("print.qr.include")}
            checked={includeQr}
            disabled={!canIncludeQr}
            onChange={(event) => onQrChange(event.target.checked)}
            className="h-4 w-4 rounded border-warm-300 text-brand focus:ring-brand/40 disabled:cursor-not-allowed"
          />
        </label>
        {!canIncludeQr ? (
          <p className="text-xs leading-relaxed text-ink-600">
            {tString("print.qr.unavailable")}
          </p>
        ) : null}
      </fieldset>

      <OptionGroup
        label={tString("print.contact.label")}
        options={PRINT_CONTACT_PLACEMENTS}
        value={direction.contactPlacement}
        keyFor={(option) => tString(`print.contact.${option}`)}
        onChange={onContactPlacementChange}
      />
    </div>
  );
}
