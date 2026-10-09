import React, { useState, useEffect, useCallback } from "react";
import Image from "next/image";
import { Button, Chip, Link } from "@nextui-org/react";
import {
  CircleCheck,
  CircleAlert,
  Star,
  ExternalLink,
  Building2,
} from "lucide-react";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";

interface Plugin {
  id: number;
  name: string;
  display_name: string;
  description: string;
  image: string;
  category: string;
  version: string;
  features: string;
  config_schema?: string;
  is_enabled: boolean;
  config: string;
}

interface TrustpilotConfigProps {
  plugin: Plugin;
  config: Record<string, any>;
  onConfigChange: (config: Record<string, any>) => void;
  onSave: () => void;
  onCancel: () => void;
}

const trustpilotPanelClass =
  "rounded-2xl border border-warm-200/80 bg-white/90 p-5 shadow-sm shadow-warm-900/5 sm:p-6";
const trustpilotSectionTitleClass =
  "font-title text-base font-semibold tracking-0 text-ink-950";
const trustpilotCopyClass = "text-sm leading-6 text-ink-600";
const trustpilotLabelClass = "mb-2 block text-sm font-medium text-ink-700";
const trustpilotInputClass =
  "h-12 w-full rounded-xl border-2 border-warm-200 bg-white/90 px-4 text-sm text-ink-900 shadow-sm outline-none transition-colors hover:border-brand/30 focus:border-brand";
const trustpilotHelpBoxClass =
  "rounded-2xl border border-brand/15 bg-brand/5 p-3 text-xs text-ink-700";
const trustpilotStatusPillClass =
  "flex items-center gap-1 rounded-full border px-2.5 py-1 text-xs font-semibold";
const trustpilotIconTileClass =
  "flex h-11 w-11 items-center justify-center rounded-2xl border border-warm-200 bg-white shadow-sm";

export default function TrustpilotConfig({
  plugin,
  config,
  onConfigChange,
  onSave,
  onCancel,
}: TrustpilotConfigProps) {
  const { locale } = useSimpleLocale();

  // Translation helper
  const t = useCallback(
    (key: string): string => {
      const fullKey = `businessDashboard.dashboard.pluginManager.config.trustpilot.${key}`;
      const result = getTranslation(fullKey, locale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [locale],
  );

  const tCommon = useCallback(
    (key: string): string => {
      const fullKey = `businessDashboard.dashboard.pluginManager.config.${key}`;
      const result = getTranslation(fullKey, locale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [locale],
  );

  const [formData, setFormData] = useState({
    business_name: config.business_name || "",
    trustpilot_url: config.trustpilot_url || "",
  });
  const [fieldErrors, setFieldErrors] = useState<{
    business_name?: string;
    trustpilot_url?: string;
  }>({});

  // Track if this is an existing saved configuration
  const [isSavedConfig, setIsSavedConfig] = useState(
    !!(config.business_name && config.trustpilot_url),
  );

  // Update parent config when form data changes
  useEffect(() => {
    onConfigChange(formData);
  }, [formData, onConfigChange]);

  const handleInputChange = (field: string, value: any) => {
    const newFormData = { ...formData, [field]: value };
    setFormData(newFormData);
    onConfigChange(newFormData);
    setFieldErrors((prev) => ({ ...prev, [field]: undefined }));
  };

  const requiredMsg =
    (getTranslation("common.required", locale) as string) || "Required";

  const handleSave = () => {
    // L6-30: required-field gate — asterisks were decoration only.
    const nextErrors: typeof fieldErrors = {};
    if (!String(formData.business_name || "").trim()) {
      nextErrors.business_name = requiredMsg;
    }
    if (!String(formData.trustpilot_url || "").trim()) {
      nextErrors.trustpilot_url = requiredMsg;
    }
    if (Object.keys(nextErrors).length > 0) {
      setFieldErrors(nextErrors);
      return;
    }
    onSave();
    setIsSavedConfig(true);
  };

  const canSave =
    String(formData.business_name || "").trim().length > 0 &&
    String(formData.trustpilot_url || "").trim().length > 0;

  const isConnected =
    formData.business_name && formData.trustpilot_url && isSavedConfig;

  // If already connected, show connected state
  if (isConnected) {
    return (
      <div className="space-y-6">
        {/* Header */}
        <div className="flex items-center gap-3">
          <div className={trustpilotIconTileClass}>
            <Image
              src="/images/plugins/trustpilot.png"
              alt="Trustpilot"
              width={24}
              height={24}
              className="rounded"
            />
          </div>
          <div>
            <h3 className="font-title text-lg font-semibold text-ink-950">
              {t("title")}
            </h3>
            <p className={trustpilotCopyClass}>
              {t("description")}
            </p>
          </div>
        </div>

        {/* Connected status panel */}
        <div className={trustpilotPanelClass}>
            <div className="flex items-center justify-between mb-4">
              <div className="flex items-center gap-2">
                <Star className="w-5 h-5 text-success" />
                <h3 className={trustpilotSectionTitleClass}>
                  Connected to Trustpilot
                </h3>
              </div>
              <Chip color="success" variant="flat" size="sm">
                Active
              </Chip>
            </div>

            <div className="space-y-4 mb-6">
              <div>
                <p className="mb-1 text-sm text-ink-600">{t("businessProfile")}</p>
                <p className="font-medium text-ink-950">{formData.business_name}</p>
              </div>

              <div className="space-y-2">
                <p className="text-sm text-ink-600">{t("quickActions")}</p>

                <div className="flex flex-col gap-2">
                  <button
                    type="button"
                    onClick={() =>
                      window.open(
                        formData.trustpilot_url,
                        "_blank",
                        "noopener,noreferrer",
                      )
                    }
                    className="flex w-full cursor-pointer items-center gap-2 rounded-2xl border border-warm-200 bg-warm-50/70 p-3 text-left transition hover:border-brand/25 hover:bg-brand/5"
                  >
                    <Star className="w-4 h-4 text-brand" />
                    <div className="flex-1">
                      <p className="text-sm font-medium text-ink-950">{t("viewYourReviews")}</p>
                      <p className="text-xs text-ink-600">
                        {t("viewYourReviewsHint")}
                      </p>
                    </div>
                    <ExternalLink className="w-3 h-3 text-ink-400" />
                  </button>

                  <button
                    type="button"
                    onClick={() =>
                      window.open(
                        "https://business.trustpilot.com",
                        "_blank",
                        "noopener,noreferrer",
                      )
                    }
                    className="flex w-full cursor-pointer items-center gap-2 rounded-2xl border border-warm-200 bg-warm-50/70 p-3 text-left transition hover:border-brand/25 hover:bg-brand/5"
                  >
                    <Building2 className="w-4 h-4 text-brand" />
                    <div className="flex-1">
                      <p className="text-sm font-medium text-ink-950">
                        {t("manageBusinessProfile")}
                      </p>
                      <p className="text-xs text-ink-600">
                        {t("manageBusinessProfileHint")}
                      </p>
                    </div>
                    <ExternalLink className="w-3 h-3 text-ink-400" />
                  </button>
                </div>
              </div>
            </div>

            <div className="mb-4 border-t border-warm-200/80" />

            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm font-medium text-ink-950">{t("integrationStatus")}</p>
                <p className="text-xs text-ink-600">
                  {t("readyToCollect")}
                </p>
              </div>
              <Button
                size="sm"
                variant="flat"
                onPress={() => {
                  // Reset to setup mode
                  setIsSavedConfig(false);
                  handleInputChange("business_name", "");
                  handleInputChange("trustpilot_url", "");
                }}
              >
                Reconfigure
              </Button>
            </div>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center gap-3">
        <div className={trustpilotIconTileClass}>
          <Image
            src={plugin.image || "/images/plugins/trustpilot.png"}
            alt={plugin.display_name}
            width={24}
            height={24}
            className="rounded"
          />
        </div>
        <div>
          <h3 className="font-title text-lg font-semibold text-ink-950">
            {t("title")}
          </h3>
          <p className={trustpilotCopyClass}>{t("description")}</p>
        </div>
      </div>

      {/* Connection Status */}
      <div className={trustpilotPanelClass}>
        <div className="flex items-center justify-between mb-4">
          <h4 className={trustpilotSectionTitleClass}>
            {tCommon("connectionStatus")}
          </h4>
          {isConnected ? (
            <span className={`${trustpilotStatusPillClass} border-emerald-200 bg-emerald-50 text-emerald-700`}>
              <CircleCheck className="w-3 h-3" />
              {tCommon("connected")}
            </span>
          ) : (
            <span className={`${trustpilotStatusPillClass} border-warm-200 bg-warm-50 text-ink-700`}>
              <CircleAlert className="w-3 h-3" />
              {tCommon("notConnected")}
            </span>
          )}
        </div>
        <div className="space-y-3">
          {isConnected ? (
            <div className="space-y-2">
              <p className="flex items-center gap-2 text-sm text-ink-700">
                <CircleCheck size={16} className="text-emerald-600" />
                {t("integrationConfigured")}
              </p>
              <p className="flex items-center gap-2 text-sm text-ink-700">
                <Building2 size={16} className="text-brand" />
                {t("business")}:{" "}
                <span className="font-medium">{formData.business_name}</span>
              </p>
              <div className="flex items-center gap-2 text-sm text-ink-700">
                <Star size={16} className="text-amber-500" />
                <span>{t("reviewsCollectedAutomatically")}</span>
              </div>
            </div>
          ) : (
            <div className="space-y-3">
              <p className={trustpilotCopyClass}>
                {t("connectProfileMessage")}
              </p>
              <div className={trustpilotHelpBoxClass}>
                <p className="font-medium mb-1">{t("simpleSetupProcess")}:</p>
                <ol className="list-decimal list-inside space-y-1">
                  <li>{t("enterBusinessName")}</li>
                  <li>{t("searchTrustpilotProfile")}</li>
                  <li>{t("configureReviewSettings")}</li>
                  <li>{t("startCollectingReviews")}</li>
                </ol>
              </div>
            </div>
          )}
        </div>
      </div>

      {/* Business Setup */}
      <div className={trustpilotPanelClass}>
        <h4 className={`${trustpilotSectionTitleClass} mb-4`}>
          {t("configuration")}
        </h4>
        <div className="space-y-4">
          <div>
            <label className={trustpilotLabelClass}>
              {t("businessName")} <span className="text-rose-500">*</span>
            </label>
            <input
              type="text"
              placeholder={t("businessNamePlaceholder")}
              value={formData.business_name}
              onChange={(e) =>
                handleInputChange("business_name", e.target.value)
              }
              className={trustpilotInputClass}
              aria-invalid={Boolean(fieldErrors.business_name)}
              aria-required="true"
            />
            {fieldErrors.business_name ? (
              <p role="alert" className="mt-1 text-xs text-rose-600">
                {fieldErrors.business_name}
              </p>
            ) : (
              <p className="mt-1 text-xs text-ink-600">
                {t("businessNameDescription")}
              </p>
            )}
          </div>

          <div>
            <label className={trustpilotLabelClass}>
              {t("trustpilotUrl")} <span className="text-rose-500">*</span>
            </label>
            <input
              type="text"
              placeholder={t("trustpilotUrlPlaceholder")}
              value={formData.trustpilot_url}
              onChange={(e) =>
                handleInputChange("trustpilot_url", e.target.value)
              }
              className={trustpilotInputClass}
              aria-invalid={Boolean(fieldErrors.trustpilot_url)}
              aria-required="true"
            />
            {fieldErrors.trustpilot_url ? (
              <p role="alert" className="mt-1 text-xs text-rose-600">
                {fieldErrors.trustpilot_url}
              </p>
            ) : (
              <p className="mt-1 text-xs text-ink-600">
                {t("trustpilotUrlDescription")}
              </p>
            )}
          </div>

          <div className={trustpilotHelpBoxClass}>
            <p className="font-medium mb-2">{t("howToFindUrl")}:</p>
            <ol className="list-decimal list-inside space-y-1 mb-3">
              <li>
                {t("goToTrustpilot")}{" "}
                <Link
                  href="https://www.trustpilot.com"
                  target="_blank"
                  rel="noopener noreferrer"
                  size="sm"
                >
                  trustpilot.com
                </Link>
              </li>
              <li>{t("searchBusinessName")}</li>
              <li>{t("clickBusinessProfile")}</li>
              <li>{t("copyUrlFromBrowser")}</li>
            </ol>
            <p className="font-medium mb-1">{t("noProfileYet")}</p>
            <p>
              <Link
                href="https://business.trustpilot.com/signup"
                target="_blank"
                rel="noopener noreferrer"
                size="sm"
              >
                {t("createFreeAccount")}
              </Link>{" "}
              {t("toStartCollecting")}
            </p>
          </div>
        </div>
      </div>

      {/* Action Buttons */}
      <div className="flex gap-3 justify-end">
        <button
          onClick={onCancel}
          className="rounded-xl px-4 py-2 text-sm font-medium text-ink-600 transition hover:bg-brand/5 hover:text-brand-700"
        >
          {tCommon("cancel")}
        </button>
        <button
          onClick={handleSave}
          disabled={!canSave}
          className="rounded-xl bg-brand px-4 py-2 text-sm font-semibold text-white shadow-sm transition hover:bg-brand-dark focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-dark focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50"
        >
          {tCommon("save")}
        </button>
      </div>
    </div>
  );
}
