"use client";

import React, { useCallback, useEffect, useId, useState } from "react";
import { Card, CardHeader, CardBody, Button, SelectItem, Slider, Divider, Input } from "@nextui-org/react";
import { NamedSwitch } from "@/components/ui/NamedSwitch";
import { NamedSelect } from "@/components/ui/NamedSelect";
import { Palette, Type, Layout, RotateCcw, Paintbrush, Smartphone } from "lucide-react";
import { useToast } from "@/contexts/ToastContext";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { BusinessDesignSettings } from "@/api/business";
import {
  SHIPPED_BRAND_PRESETS,
  validatePrimaryBrandColor,
  validateSecondaryBrandColor,
  BRAND_ON_PRIMARY_TEXT,
  BRAND_SECONDARY_SURFACE,
} from "@/lib/contrast";
import BusinessPageLivePreview, {
  type BusinessPageLivePreviewModel,
} from "@/components/business-page/BusinessPageLivePreview";
import StorefrontPreviewFrame from "@/components/business-page/StorefrontPreviewFrame";
import {
  STOREFRONT_PATTERN_IDS,
  getPatternDataUri,
  isStorefrontPattern,
} from "@/lib/storefront/theme";

interface DesignCustomizationProps {
  businessId: number;
  designSettings: BusinessDesignSettings;
  onDesignSettingsChange: (settings: BusinessDesignSettings) => void;
  customUrl?: string;
  /**
   * In-progress Business Page form state used to drive the live storefront
   * preview (name, hours, contact). When omitted the preview still mounts
   * with empty identity so design-only callers keep working.
   */
  previewModel?: BusinessPageLivePreviewModel;
}

/** Shipped quick-presets — AA-validated pairs from `@/lib/contrast`. */
const COLOR_PRESETS = SHIPPED_BRAND_PRESETS;

// Labels are looked up at render time via t(`fontOptions.${labelKey}`) so the
// font-family options render in the operator's locale instead of leaking the
// hardcoded English strings (I18N-9).
const FONT_OPTIONS = [
  { value: "Inter", labelKey: "fontOptions.sans" },
  { value: "Serif", labelKey: "fontOptions.serif" },
];

// Option values are stable enum strings; the human label is looked up at
// render time via the matching option-label key so the Diseño y Marca tab
// renders in the operator's locale instead of leaking English labels.
const RADIUS_OPTIONS = ["none", "small", "medium", "large"] as const;
const SHADOW_OPTIONS = ["none", "subtle", "medium", "strong"] as const;
// Swatch list + artwork come from the shared storefront theme module so the
// editor preview renders the SAME patterns guests get on the public page
// (Plan 2.4). This also exposes `mandala`, which existed publicly but was
// never selectable here.
const PATTERN_OPTIONS = ["none", ...STOREFRONT_PATTERN_IDS] as const;

// Custom color inputs must be a full 6-digit hex (e.g. #1a6b6a). Anything else
// silently renders as browser-default black in the inline styles and the native
// <input type="color">, so we validate before propagating up and buffer the raw
// text locally so the operator can keep typing an in-progress value.
const HEX_COLOR_RE = /^#[0-9a-fA-F]{6}$/;

// Design-system note (M30): DesignCustomization is a DELIBERATE exemption from
// the shared DashboardTabShell/PageHeader tab contract. It is a preview-dominant
// surface (a large live storefront preview driving most of the viewport) whose
// split editor+preview layout does not fit the shell's single-column title-header
// idiom. Do not migrate it onto DashboardTabShell.
export default function DesignCustomization({
  businessId,
  designSettings,
  onDesignSettingsChange,
  customUrl,
  previewModel,
}: DesignCustomizationProps) {
  const { showSuccess: _showSuccess, showError: _showError } = useToast();
  const { locale } = useSimpleLocale();
  const settingsA11yId = useId();

  // Translation helper
  const t = useCallback(
    (key: string): string => {
      const fullKey = `businessDashboard.dashboard.designCustomization.${key}`;
      const result = getTranslation(fullKey, locale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [locale],
  );

  // Empty model keeps design-only callers rendering without identity data.
  // businessId is stitched in here (not by every caller) — the preview mounts
  // the real storefront tree, which reads the public menu / delivery /
  // reservation settings for this business (#591).
  const livePreviewModel: BusinessPageLivePreviewModel = {
    ...(previewModel ?? {
      name: "",
      operatingHours: [],
      showOperatingHours: false,
      customUrl,
    }),
    businessId,
  };

  // Update setting helper function. Enum selects must never accept "" — an empty
  // option is never a valid design choice (NextUI can emit "" on clear).
  const updateSetting = (key: keyof BusinessDesignSettings, value: any) => {
    if (typeof value === "string" && value.trim() === "") {
      return;
    }
    onDesignSettingsChange({ ...designSettings, [key]: value });
  };

  // Buffer the raw text of each color field locally so the operator can type an
  // in-progress value (e.g. "#1a") without us pushing a malformed color up to
  // the parent (which feeds inline styles and <input type="color">, where a bad
  // value renders as silent browser-default black). We only propagate values
  // that match a full 6-digit hex. The buffers re-sync whenever the persisted
  // props change externally (presets, reset, initial load).
  const [primaryColorText, setPrimaryColorText] = useState(
    designSettings.primary_color || "",
  );
  const [secondaryColorText, setSecondaryColorText] = useState(
    designSettings.secondary_color || "",
  );

  useEffect(() => {
    setPrimaryColorText(designSettings.primary_color || "");
  }, [designSettings.primary_color]);
  useEffect(() => {
    setSecondaryColorText(designSettings.secondary_color || "");
  }, [designSettings.secondary_color]);

  const primaryFormatInvalid = !HEX_COLOR_RE.test(primaryColorText);
  const secondaryFormatInvalid = !HEX_COLOR_RE.test(secondaryColorText);

  // Contrast is only evaluated for well-formed hex; incomplete typing must not
  // surface a false WCAG failure while the operator is mid-edit.
  const primaryContrast = primaryFormatInvalid
    ? null
    : validatePrimaryBrandColor(primaryColorText);
  const secondaryContrast = secondaryFormatInvalid
    ? null
    : validateSecondaryBrandColor(secondaryColorText);

  const primaryColorInvalid =
    primaryFormatInvalid || primaryContrast?.ok === false;
  const secondaryColorInvalid =
    secondaryFormatInvalid || secondaryContrast?.ok === false;

  const formatContrastFail = (ratio: number, fallback?: string): string => {
    const template = t("contrastFail");
    // Missing keys fall back to the leaf name or full path — prefer lib message.
    if (
      !template ||
      template === "contrastFail" ||
      template.endsWith(".contrastFail")
    ) {
      return (
        fallback ||
        `Contrast ${ratio.toFixed(1)}:1 is below the WCAG AA minimum of 4.5:1`
      );
    }
    return template
      .replace("{{ratio}}", ratio.toFixed(1))
      .replace("{{minimum}}", "4.5");
  };

  const primaryErrorMessage = primaryFormatInvalid
    ? t("colorFormatHint")
    : primaryContrast?.ok === false
      ? formatContrastFail(primaryContrast.ratio, primaryContrast.error)
      : undefined;
  const secondaryErrorMessage = secondaryFormatInvalid
    ? t("colorFormatHint")
    : secondaryContrast?.ok === false
      ? formatContrastFail(secondaryContrast.ratio, secondaryContrast.error)
      : undefined;

  const handlePrimaryColorText = (value: string) => {
    setPrimaryColorText(value);
    // Commit only full hex that clears AA (white body text on the fill).
    if (!HEX_COLOR_RE.test(value)) return;
    if (!validatePrimaryBrandColor(value).ok) return;
    updateSetting("primary_color", value);
  };
  const handleSecondaryColorText = (value: string) => {
    setSecondaryColorText(value);
    // Commit only full hex that clears AA as body text on a light surface.
    if (!HEX_COLOR_RE.test(value)) return;
    if (!validateSecondaryBrandColor(value).ok) return;
    updateSetting("secondary_color", value);
  };

  const applyColorPreset = (preset: (typeof COLOR_PRESETS)[0]) => {
    onDesignSettingsChange({
      ...designSettings,
      primary_color: preset.primary,
      secondary_color: preset.secondary
    });
  };

  const resetToDefaults = () => {
    /* eslint-disable no-restricted-syntax -- API-stored design defaults; not Tailwind.
       Secondary is teal-700 (#0f766e) rather than the older #2a8b8a so white-surface
       body text clears WCAG AA 4.5:1 (legacy secondary was ~4.07:1). */
    onDesignSettingsChange({
      ...designSettings,
      primary_color: "#1a6b6a",
      secondary_color: "#0f766e",
      font_family: "Inter",
      theme: "light",
      menu_layout: "grid",
      show_images: true,
      show_descriptions: true,
      header_style: "banner",
      corner_radius: "medium",
      shadow_intensity: "subtle",
      background_pattern: "none",
      pattern_opacity: 0.1,
      hero_layout: "centered",
      section_density: "comfortable",
    });
    /* eslint-enable no-restricted-syntax */
  };

  const cardClassName =
    "rounded-3xl border border-warm-200/80 bg-white/85 shadow-sm shadow-warm-900/5";
  const cardHeaderClassName = "pb-3 border-b border-warm-200/70";
  const labelClassName = "text-sm font-semibold text-ink-800 mb-2 block";
  const selectClassNames = {
    trigger: "border-warm-200 bg-white/90 shadow-sm",
    value: "text-ink-900",
  };

  return (
    <div className="space-y-8 w-full">
      {/* Compact action row — header is provided by the parent BusinessSettings shell */}
      <div className="flex items-center justify-end gap-2">
        <Button
          size="sm"
          variant="light"
          startContent={<RotateCcw className="w-3.5 h-3.5" />}
          onPress={resetToDefaults}
          className="rounded-xl font-semibold text-ink-700"
        >
          {t("resetDefaults") || "Reset"}
        </Button>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-8">
        {/* Settings Panel */}
        <div className="space-y-6">

          {/* Color Scheme */}
          <Card className={cardClassName}>
            <CardHeader className={cardHeaderClassName}>
              <div className="flex items-center gap-2">
                <Palette className="w-5 h-5 text-brand" />
                <h3 className="text-lg font-semibold text-ink-950">{t("colorScheme") || "Colors"}</h3>
              </div>
            </CardHeader>
            <CardBody className="space-y-6 p-6">
              {/* Presets */}
              <div>
                <label className={labelClassName}>
                  {t("quickPresets") || "Quick Presets"}
                </label>
                <div className="grid grid-cols-2 sm:grid-cols-4 gap-2">
                  {COLOR_PRESETS.map((preset, index) => (
                    <button
                      key={index}
                      onClick={() => applyColorPreset(preset)}
                      className="group flex flex-col items-center gap-2 rounded-2xl border border-warm-200 bg-white/80 p-2 shadow-sm shadow-warm-900/5 transition hover:-translate-y-0.5 hover:border-brand/30 hover:bg-brand/5 hover:shadow-md hover:shadow-brand/10"
                    >
                      <div className="flex gap-1 shadow-sm rounded-full overflow-hidden">
                        <div
                          className="w-4 h-8"
                          style={{ backgroundColor: preset.primary }}
                        />
                        <div
                          className="w-4 h-8"
                          style={{ backgroundColor: preset.secondary }}
                        />
                      </div>
                      <span className="text-xs font-semibold text-ink-700 group-hover:text-brand-dark">
                        {t(`colorPresets.${preset.key}`)}
                      </span>
                    </button>
                  ))}
                </div>
              </div>

              <Divider />

              {/* Custom Colors */}
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className={labelClassName}>
                    {t("primaryColor") || "Primary Color"}
                  </label>
                  <div className="flex gap-2 items-start">
                    <input
                      type="color"
                      aria-label={t("primaryColor") || "Primary Color"}
                      value={
                        HEX_COLOR_RE.test(primaryColorText)
                          ? primaryColorText
                          : designSettings.primary_color
                      }
                      onChange={(e) => handlePrimaryColorText(e.target.value)}
                      className="w-10 h-10 rounded-xl cursor-pointer border border-warm-300 bg-white p-1 shadow-sm shadow-warm-900/5 shrink-0"
                    />
                    <Input
                      type="text"
                      aria-label={t("primaryColor") || "Primary Color"}
                      value={primaryColorText}
                      onValueChange={handlePrimaryColorText}
                      isInvalid={primaryColorInvalid}
                      errorMessage={primaryErrorMessage}
                      variant="bordered"
                      classNames={{ input: "uppercase text-sm text-ink-900" }}
                      className="flex-1"
                    />
                  </div>
                  {/* Inline body-text preview: white label on the primary fill. */}
                  {HEX_COLOR_RE.test(primaryColorText) && (
                    <div
                      className="mt-2 rounded-xl border border-warm-200 px-3 py-2 text-sm"
                      style={{
                        backgroundColor: primaryColorText,
                        color: BRAND_ON_PRIMARY_TEXT,
                      }}
                      data-testid="primary-contrast-preview"
                    >
                      <span className="font-medium">
                        {t("contrastPreviewPrimary") ||
                          "Body text on primary"}
                      </span>
                      {primaryContrast && (
                        <span className="ml-2 text-xs opacity-90 tabular-nums">
                          {primaryContrast.ratio.toFixed(1)}:1
                        </span>
                      )}
                    </div>
                  )}
                </div>
                <div>
                  <label className={labelClassName}>
                    {t("secondaryColor") || "Secondary Color"}
                  </label>
                  <div className="flex gap-2 items-start">
                    <input
                      type="color"
                      aria-label={t("secondaryColor") || "Secondary Color"}
                      value={
                        HEX_COLOR_RE.test(secondaryColorText)
                          ? secondaryColorText
                          : designSettings.secondary_color
                      }
                      onChange={(e) => handleSecondaryColorText(e.target.value)}
                      className="w-10 h-10 rounded-xl cursor-pointer border border-warm-300 bg-white p-1 shadow-sm shadow-warm-900/5 shrink-0"
                    />
                    <Input
                      type="text"
                      aria-label={t("secondaryColor") || "Secondary Color"}
                      value={secondaryColorText}
                      onValueChange={handleSecondaryColorText}
                      isInvalid={secondaryColorInvalid}
                      errorMessage={secondaryErrorMessage}
                      variant="bordered"
                      classNames={{ input: "uppercase text-sm text-ink-900" }}
                      className="flex-1"
                    />
                  </div>
                  {/* Inline body-text preview: secondary as text on a light surface. */}
                  {HEX_COLOR_RE.test(secondaryColorText) && (
                    <div
                      className="mt-2 rounded-xl border border-warm-200 px-3 py-2 text-sm"
                      style={{
                        backgroundColor: BRAND_SECONDARY_SURFACE,
                        color: secondaryColorText,
                      }}
                      data-testid="secondary-contrast-preview"
                    >
                      <span className="font-medium">
                        {t("contrastPreviewSecondary") ||
                          "Body text as secondary"}
                      </span>
                      {secondaryContrast && (
                        <span className="ml-2 text-xs opacity-80 tabular-nums">
                          {secondaryContrast.ratio.toFixed(1)}:1
                        </span>
                      )}
                    </div>
                  )}
                </div>
              </div>
            </CardBody>
          </Card>

          {/* Typography & Visuals */}
          <Card className={cardClassName}>
            <CardHeader className={cardHeaderClassName}>
              <div className="flex items-center gap-2">
                <Paintbrush className="w-5 h-5 text-brand" />
                <h3 className="text-lg font-semibold text-ink-950">{t("visualStyle") || "Styles"}</h3>
              </div>
            </CardHeader>
            <CardBody className="space-y-6 p-6">
              {/* Font Family */}
              <div>
                <label className={labelClassName}>
                  {t("fontFamily") || "Font Family"}
                </label>
                <NamedSelect
                  name={t("fontFamily") || "Font Family"}
                  valueLabel={t(
                    FONT_OPTIONS.find(
                      (font) =>
                        font.value === (designSettings.font_family || "Inter"),
                    )?.labelKey ?? "fontOptions.sans",
                  )}
                  selectedKeys={[designSettings.font_family || "Inter"]}
                  onChange={(e) => updateSetting("font_family", e.target.value)}
                  className="max-w-xs"
                  classNames={selectClassNames}
                  disallowEmptySelection
                >
                  {FONT_OPTIONS.map((font) => (
                    <SelectItem key={font.value} value={font.value}>
                      {t(font.labelKey)}
                    </SelectItem>
                  ))}
                </NamedSelect>
              </div>

              <div className="grid grid-cols-2 gap-6">
                {/* Corner Radius */}
                <div>
                  <label className={labelClassName}>
                    {t("cornerRadius") || "Corner Radius"}
                  </label>
                  <NamedSelect
                    name={t("cornerRadius") || "Corner Radius"}
                    valueLabel={t(
                      `cornerRadiusOptions.${designSettings.corner_radius || "medium"}`,
                    )}
                    selectedKeys={[designSettings.corner_radius || "medium"]}
                    onChange={(e) => updateSetting("corner_radius", e.target.value)}
                    classNames={selectClassNames}
                    disallowEmptySelection
                  >
                    {RADIUS_OPTIONS.map((value) => (
                      <SelectItem key={value} value={value}>
                        {t(`cornerRadiusOptions.${value}`)}
                      </SelectItem>
                    ))}
                  </NamedSelect>
                </div>
                {/* Shadow */}
                <div>
                  <label className={labelClassName}>
                    {t("shadowIntensity") || "Shadow Depth"}
                  </label>
                  <NamedSelect
                    name={t("shadowIntensity") || "Shadow Intensity"}
                    valueLabel={t(
                      `shadowOptions.${designSettings.shadow_intensity || "subtle"}`,
                    )}
                    selectedKeys={[designSettings.shadow_intensity || "subtle"]}
                    onChange={(e) => updateSetting("shadow_intensity", e.target.value)}
                    classNames={selectClassNames}
                    disallowEmptySelection
                  >
                    {SHADOW_OPTIONS.map((value) => (
                      <SelectItem key={value} value={value}>
                        {t(`shadowOptions.${value}`)}
                      </SelectItem>
                    ))}
                  </NamedSelect>
                </div>
              </div>

              <div className="grid grid-cols-2 gap-6">
                {/* Hero Layout */}
                <div>
                  <label className={labelClassName}>
                    {t("heroLayoutLabel") || "Hero layout"}
                  </label>
                  <NamedSelect
                    name={t("heroLayoutLabel") || "Hero layout"}
                    valueLabel={
                      designSettings.hero_layout === "split-left"
                        ? t("heroLayout.splitLeft")
                        : designSettings.hero_layout === "split-right"
                          ? t("heroLayout.splitRight")
                          : t("heroLayout.centered")
                    }
                    selectedKeys={[designSettings.hero_layout || "centered"]}
                    onChange={(e) => updateSetting("hero_layout", e.target.value)}
                    classNames={selectClassNames}
                    disallowEmptySelection
                  >
                    <SelectItem key="centered" value="centered">{t("heroLayout.centered")}</SelectItem>
                    <SelectItem key="split-left" value="split-left">{t("heroLayout.splitLeft")}</SelectItem>
                    <SelectItem key="split-right" value="split-right">{t("heroLayout.splitRight")}</SelectItem>
                  </NamedSelect>
                </div>
                {/* Section Density */}
                <div>
                  <label className={labelClassName}>
                    {t("sectionDensityLabel") || "Section density"}
                  </label>
                  <NamedSelect
                    name={t("sectionDensityLabel") || "Section density"}
                    valueLabel={t(
                      `sectionDensityOptions.${designSettings.section_density || "comfortable"}`,
                    )}
                    selectedKeys={[designSettings.section_density || "comfortable"]}
                    onChange={(e) => updateSetting("section_density", e.target.value)}
                    classNames={selectClassNames}
                    disallowEmptySelection
                  >
                    <SelectItem key="comfortable" value="comfortable">{t("sectionDensityOptions.comfortable")}</SelectItem>
                    <SelectItem key="compact" value="compact">{t("sectionDensityOptions.compact")}</SelectItem>
                  </NamedSelect>
                </div>
              </div>

              <div className="grid grid-cols-2 gap-6">
                {/* Menu Layout */}
                <div>
                  <label className={labelClassName}>
                    {t("menuLayoutLabel") || "Menu layout"}
                  </label>
                  <NamedSelect
                    name={t("menuLayoutLabel") || "Menu layout"}
                    valueLabel={t(
                      `menuLayout.${designSettings.menu_layout || "grid"}`,
                    )}
                    selectedKeys={[designSettings.menu_layout || "grid"]}
                    onChange={(e) => updateSetting("menu_layout", e.target.value)}
                    classNames={selectClassNames}
                    disallowEmptySelection
                  >
                    <SelectItem key="grid" value="grid">{t("menuLayout.grid")}</SelectItem>
                    <SelectItem key="list" value="list">{t("menuLayout.list")}</SelectItem>
                  </NamedSelect>
                </div>
                {/* Header Style */}
                <div>
                  <label className={labelClassName}>
                    {t("headerStyleLabel") || "Header style"}
                  </label>
                  <NamedSelect
                    name={t("headerStyleLabel") || "Header style"}
                    valueLabel={t(
                      `headerStyle.${designSettings.header_style || "banner"}`,
                    )}
                    selectedKeys={[designSettings.header_style || "banner"]}
                    onChange={(e) => updateSetting("header_style", e.target.value)}
                    classNames={selectClassNames}
                    disallowEmptySelection
                  >
                    <SelectItem key="banner" value="banner">{t("headerStyle.banner")}</SelectItem>
                    <SelectItem key="minimal" value="minimal">{t("headerStyle.minimal")}</SelectItem>
                    <SelectItem key="classic" value="classic">{t("headerStyle.classic")}</SelectItem>
                  </NamedSelect>
                </div>
              </div>
            </CardBody>
          </Card>

          {/* Background Pattern */}
          <Card className={cardClassName}>
            <CardHeader className={cardHeaderClassName}>
              <div className="flex items-center gap-2">
                <Layout className="w-5 h-5 text-brand" />
                <h3 className="text-lg font-semibold text-ink-950">{t("background") || "Background Pattern"}</h3>
              </div>
            </CardHeader>
            <CardBody className="space-y-6 p-6">
              <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
                {PATTERN_OPTIONS.map((patternValue) => (
                  <button
                    key={patternValue}
                    data-pattern={patternValue}
                    onClick={() => updateSetting("background_pattern", patternValue)}
                    className={`relative overflow-hidden h-20 rounded-2xl border text-sm transition flex flex-col items-center justify-center gap-1 shadow-sm ${designSettings.background_pattern === patternValue
                      ? "border-brand bg-brand/10 text-brand-dark font-semibold ring-2 ring-brand/30 ring-offset-1 shadow-brand/10"
                      : "border-warm-200 bg-white hover:-translate-y-0.5 hover:border-brand/30 hover:bg-brand/5 hover:shadow-md hover:shadow-brand/10 text-ink-700"
                      }`}
                  >
                    {isStorefrontPattern(patternValue) && (
                      <div
                        className="absolute inset-0 opacity-20 pointer-events-none"
                        data-pattern-artwork={patternValue}
                        style={{
                          /* eslint-disable-next-line no-restricted-syntax -- swatch tint for the shared SVG data-URI artwork; not a Tailwind class */
                          backgroundImage: getPatternDataUri(patternValue, '#000000'),
                          backgroundRepeat: 'repeat',
                        }}
                      />
                    )}
                    <span className="relative z-10 bg-white/70 px-2 py-0.5 rounded-lg backdrop-blur-sm">
                      {t(`backgroundOptions.${patternValue}`)}
                    </span>
                  </button>
                ))}
              </div>

              {designSettings.background_pattern !== "none" && (
                <div>
                  <label className={labelClassName}>
                    {t("patternOpacity") || "Pattern Opacity"}
                  </label>
                  <Slider
                    aria-label={t("patternOpacity") || "Pattern Opacity"}
                    step={0.05}
                    maxValue={1}
                    minValue={0.05}
                    defaultValue={0.2}
                    value={designSettings.pattern_opacity || 0.2}
                    onChange={(value) => updateSetting("pattern_opacity", Number(value))}
                    className="max-w-md"
                  />
                  <p className="text-xs text-warm-700 mt-1">
                    {t("patternOpacityHint").replace(
                      "{percent}",
                      String(Math.round((designSettings.pattern_opacity || 0.2) * 100)),
                    )}
                  </p>
                </div>
              )}
            </CardBody>
          </Card>

          {/* Layout Options */}
          <Card className={cardClassName}>
            <CardHeader className={cardHeaderClassName}>
              <div className="flex items-center gap-2">
                <Type className="w-5 h-5 text-brand" />
                <h3 className="text-lg font-semibold text-ink-950">{t("contentDisplay") || "Content"}</h3>
              </div>
            </CardHeader>
            <CardBody className="space-y-4 p-6">
              <div className="flex items-center justify-between">
                <div>
                  <p
                    id={`${settingsA11yId}-show-images-label`}
                    className="font-semibold text-ink-950"
                  >
                    {t("showImages") || "Show Item Images"}
                  </p>
                  <p
                    id={`${settingsA11yId}-show-images-desc`}
                    className="text-sm text-ink-600"
                  >
                    {t("showImagesDesc") || "Display photos of your menu items"}
                  </p>
                </div>
                <NamedSwitch
                  name={t("showImages") || "Show Item Images"}
                  labelId={`${settingsA11yId}-show-images-label`}
                  descriptionId={`${settingsA11yId}-show-images-desc`}
                  isSelected={designSettings.show_images}
                  onValueChange={(checked) => updateSetting("show_images", checked)}
                />
              </div>
              <Divider />
              <div className="flex items-center justify-between">
                <div>
                  <p
                    id={`${settingsA11yId}-show-desc-label`}
                    className="font-semibold text-ink-950"
                  >
                    {t("showDescriptions") || "Show Descriptions"}
                  </p>
                  <p
                    id={`${settingsA11yId}-show-desc-desc`}
                    className="text-sm text-ink-600"
                  >
                    {t("showDescriptionsDesc") || "Show detailed item descriptions"}
                  </p>
                </div>
                <NamedSwitch
                  name={t("showDescriptions") || "Show Descriptions"}
                  labelId={`${settingsA11yId}-show-desc-label`}
                  descriptionId={`${settingsA11yId}-show-desc-desc`}
                  isSelected={designSettings.show_descriptions}
                  onValueChange={(checked) => updateSetting("show_descriptions", checked)}
                />
              </div>
            </CardBody>
          </Card>

        </div>

        {/* Live Preview Panel — the REAL public page tree (announcement bar,
            hero, sticky tabs, About/Menu/Delivery/Reservations/Contact panels,
            footer, language selector) in preview mode, driven by the editor's
            in-progress form state (#591). aria-hidden lives on
            BusinessPageLivePreview so the nested <h1> and tablist do not
            hijack the operator document outline. */}
        <div className="hidden lg:block">
          <div className="sticky top-6">
            <Card className="shadow-none bg-transparent border-0">
              <CardHeader className="px-0 pb-4">
                <div className="flex items-center gap-2 text-ink-700">
                  <Smartphone className="w-5 h-5" />
                  <h3 className="text-sm font-semibold uppercase tracking-wider">{t("livePreview") || "Live Preview"}</h3>
                </div>
              </CardHeader>
              {/* Simulated Mobile Device chrome around the real page tree.
                  The screen is a same-origin iframe with a REAL 380px
                  viewport (#600): the page tree is portaled into it, so
                  Tailwind md:/lg: and every raw @media resolve against the
                  phone frame instead of the operator's desktop browser.
                  Chrome width = 380px screen + 2×16px padding + 2×4px
                  border. Scrolling happens inside the frame document. */}
              <div className="bg-ink-950 rounded-[2.5rem] p-4 shadow-2xl shadow-warm-900/20 border-4 border-ink-800 max-w-[420px] mx-auto">
                <div className="bg-white rounded-[2rem] overflow-hidden relative">
                  <StorefrontPreviewFrame
                    title={t("livePreview") || "Live Preview"}
                    width={380}
                    height={700}
                  >
                    <BusinessPageLivePreview
                      model={livePreviewModel}
                      designSettings={designSettings}
                      operatorLocale={locale}
                    />
                  </StorefrontPreviewFrame>
                </div>
              </div>
            </Card>
          </div>
        </div>

      </div>
    </div>
  );
}
