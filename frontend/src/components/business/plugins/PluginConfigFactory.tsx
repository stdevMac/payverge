import React, { useCallback } from "react";
import Image from "next/image";
import { PLUGIN } from "@/constants/plugins";
import { pluginAPI } from "@/api/plugins";

import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import StripeConfig from "./StripeConfig";
import MercadoPagoConfig from "./MercadoPagoConfig";
import TrustpilotConfig from "./TrustpilotConfig";
import TelegramConfig from "./TelegramConfig";
import PayPalConfig from "./PayPalConfig";
import USDCPaymentConfig from "./USDCPaymentConfig";
import CrossChainPaymentConfig from "./CrossChainPaymentConfig";
import WeeklyReportConfig from "./WeeklyReportConfig";
import DailyReportConfig from "./DailyReportConfig";
import { PluginConfigFooter } from "./PluginConfigFooter";
import FeatureUnavailable from "@/components/instance/FeatureUnavailable";
import { useInstance } from "@/hooks/useInstance";

// Simple fallback component for plugins without custom configuration
export function GenericPluginConfig({ plugin, onSave, onCancel }: any) {
  const { locale } = useSimpleLocale();

  // Translation helper
  const t = useCallback(
    (key: string): string => {
      const fullKey = `businessDashboard.dashboard.pluginManager.config.${key}`;
      const result = getTranslation(fullKey, locale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [locale],
  );

  const displayName = pluginAPI.utils.getTranslatedContent(
    plugin,
    "display_name",
    locale as any,
  );
  const description = pluginAPI.utils.getTranslatedContent(
    plugin,
    "description",
    locale as any,
  );

  return (
    <div className="space-y-6">
      <div className="flex items-start gap-3 rounded-2xl border border-warm-200 bg-warm-50/70 p-3 shadow-sm shadow-warm-900/5">
        <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl border border-warm-200 bg-white shadow-inner shadow-warm-900/5">
          <Image
            src={plugin.image || "/images/plugins/default.png"}
            alt={displayName}
            width={24}
            height={24}
            className="rounded"
          />
        </div>
        <div className="min-w-0">
          <h3 className="text-lg font-semibold text-ink-950">{displayName}</h3>
          <p className="text-sm leading-5 text-ink-600">{description}</p>
        </div>
      </div>

      <div className="rounded-3xl border border-warm-200 bg-white p-6 shadow-sm shadow-warm-900/5">
        <p className="text-center text-sm font-medium text-ink-600">
          {t("noConfigRequired")}
        </p>
      </div>

      <PluginConfigFooter
        onSave={onSave}
        onCancel={onCancel}
        isEnabled={Boolean(plugin.is_enabled)}
        cancelLabel={t("cancel")}
        saveLabel={t("save")}
        enableLabel={t("enable")}
      />
    </div>
  );
}


interface Plugin {
  id: number;
  name: string;
  display_name: string;
  description: string;
  image: string;
  category: string;
  version: string;
  features: string;
  is_enabled: boolean;
  config: string;
  config_schema?: string;
}

interface PluginConfigFactoryProps {
  plugin: Plugin;
  config: Record<string, any>;
  onConfigChange: (config: Record<string, any>) => void;
  onSave: () => void;
  onCancel: () => void;
  businessId?: string;
  businessProfile?: any;
}

export default function PluginConfigFactory({
  plugin,
  config,
  onConfigChange,
  onSave,
  onCancel,
  businessId,
  businessProfile,
}: PluginConfigFactoryProps) {
  const { isOff } = useInstance();
  // Integrations this install has not configured: explain, do not offer setup.
  if (
    isOff("crypto") &&
    (plugin.name === PLUGIN.usdcPayment ||
      plugin.name === PLUGIN.crossChainPayment)
  ) {
    return <FeatureUnavailable feature="crypto" />;
  }
  if (isOff("telegram") && plugin.name === PLUGIN.telegram) {
    return <FeatureUnavailable feature="telegram" />;
  }
  if (
    isOff("email") &&
    (plugin.name === PLUGIN.dailyEmailReport ||
      plugin.name === PLUGIN.weeklyEmailReport)
  ) {
    return <FeatureUnavailable feature="email" />;
  }
  // Route to specific plugin configuration components
  switch (plugin.name) {
    case PLUGIN.usdcPayment:
      return (
        <USDCPaymentConfig
          plugin={plugin}
          config={config}
          onConfigChange={onConfigChange}
          onSave={onSave}
          onCancel={onCancel}
          businessProfile={businessProfile}
        />
      );

    case PLUGIN.crossChainPayment:
      return (
        <CrossChainPaymentConfig
          plugin={plugin}
          config={config}
          onConfigChange={onConfigChange}
          onSave={onSave}
          onCancel={onCancel}
          businessProfile={businessProfile}
        />
      );

    case PLUGIN.stripe:
      return (
        <StripeConfig
          plugin={plugin}
          config={config}
          onConfigChange={onConfigChange}
          onSave={onSave}
          onCancel={onCancel}
          businessId={businessId}
        />
      );

    case PLUGIN.mercadopago:
      return (
        <MercadoPagoConfig
          plugin={plugin}
          config={config}
          onConfigChange={onConfigChange}
          onSave={onSave}
          onCancel={onCancel}
          businessId={businessId}
        />
      );

    case PLUGIN.paypal:
      return (
        <PayPalConfig
          plugin={plugin}
          config={config}
          onConfigChange={onConfigChange}
          onSave={onSave}
          onCancel={onCancel}
        />
      );

    case PLUGIN.telegram:
      return (
        <TelegramConfig
          businessId={businessId}
          plugin={plugin}
          config={config}
          onConfigChange={onConfigChange}
          onSave={onSave}
          onCancel={onCancel}
          businessTimezone={businessProfile?.timezone ?? null}
          businessName={businessProfile?.name ?? ""}
        />
      );

    case PLUGIN.trustpilot:
      return (
        <TrustpilotConfig
          plugin={plugin}
          config={config}
          onConfigChange={onConfigChange}
          onSave={onSave}
          onCancel={onCancel}
        />
      );

    case PLUGIN.weeklyEmailReport:
      return (
        <WeeklyReportConfig
          plugin={plugin}
          config={config}
          onConfigChange={onConfigChange}
          onSave={onSave}
          onCancel={onCancel}
          businessTimezone={businessProfile?.timezone ?? null}
        />
      );

    case PLUGIN.dailyEmailReport:
      return (
        <DailyReportConfig
          plugin={plugin}
          config={config}
          onConfigChange={onConfigChange}
          onSave={onSave}
          onCancel={onCancel}
          businessTimezone={businessProfile?.timezone ?? null}
        />
      );

    default:
      // Fall back to generic configuration for unknown plugins
      return (
        <GenericPluginConfig
          plugin={plugin}
          config={config}
          onConfigChange={onConfigChange}
          onSave={onSave}
          onCancel={onCancel}
        />
      );
  }
}
