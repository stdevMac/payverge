"use client";
import React, { useState, useEffect, useCallback } from "react";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  Tooltip,
  useDisclosure,
  Image,
} from "@nextui-org/react";
import { useDirtyForm } from "@/hooks/useDirtyForm";
import ConfirmationModal from "@/components/business/modals/ConfirmationModal";
import {
  Puzzle,
  Settings,
  CreditCard,
  BarChart3,
  Plug,
  Search,
  CalendarClock,
} from "lucide-react";
import { pluginAPI, type Plugin as BasePlugin } from "@/api/plugins";
import { PLUGIN } from "@/constants/plugins";
import { resolvePluginImageSrc } from "@/utils/pluginImages";
import {
  isCardPaymentPlugin,
  paymentPluginRank,
  sortPluginsForOperatorMarketplace,
} from "@/utils/pluginMarketplaceOrder";

// Extended plugin type with business-specific properties
interface APIPlugin extends BasePlugin {
  is_enabled?: boolean;
  business_config?: Record<string, any>;
  last_status?: string;
  last_error?: string;
  last_error_at?: string | null;
}
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import PluginConfigFactory from "./plugins/PluginConfigFactory";
import toast from "react-hot-toast";
import {
  getApiErrorMessage,
  getSafeApiErrorMessage,
  isRawValidatorDump,
} from "@/utils/apiError";

import { useBusinessAccess } from "@/hooks/useBusinessAccess";
import DashboardLockedTabView from "./DashboardLockedTabView";
import DashboardTabShell from "./shared/DashboardTabShell";
import DashboardTabLoadingSkeleton from "./shared/DashboardTabLoadingSkeleton";
import { PremiumPanel } from "./premium";
import { EmptyState } from "@/components/ui/EmptyState";
import Toolbar, { ViewToggle } from "./shared/Toolbar";
import IconTile from "@/components/ui/IconTile";
import { StatusChip, type StatusTone } from "@/components/ui/StatusChip";
import { brandLinks } from "@/config/brand";
import {
  btnPrimary,
  btnPrimaryCompact,
  btnSecondary,
} from "@/components/ui/buttonStyles";

// Plugin categories with icons
const PLUGIN_CATEGORIES = {
  payment: { label: "Payment Processing", icon: CreditCard, color: "primary" },
  analytics: {
    label: "Analytics & Reporting",
    icon: BarChart3,
    color: "secondary",
  },
  integration: {
    label: "Third-party Integration",
    icon: Plug,
    color: "success",
  },
  reporting: { label: "Tax & Compliance", icon: Settings, color: "warning" },
  marketing: { label: "Marketing & CRM", icon: Puzzle, color: "danger" },
};

type PluginStatusTone = "available" | "comingSoon" | "enabled" | "error";

const pluginStatusChipTone: Record<PluginStatusTone, StatusTone> = {
  available: "info",
  comingSoon: "warn",
  enabled: "success",
  error: "danger",
};

interface PluginManagerProps {
  businessId: string;
}

function getPluginStatus(plugin: APIPlugin): PluginStatusTone {
  if (plugin.coming_soon) return "comingSoon";
  if (plugin.is_enabled && plugin.last_status === "error") return "error";
  if (plugin.is_enabled) return "enabled";
  return "available";
}

/** Footer readiness copy is derived from the same single status as the chip. */
function footerStatusKey(tone: PluginStatusTone): "waitlist" | "configured" | "ready" {
  switch (tone) {
    case "comingSoon":
      return "waitlist";
    case "enabled":
    case "error":
      return "configured";
    default:
      return "ready";
  }
}

function PluginStatusBadge({
  plugin,
  label,
}: {
  plugin: APIPlugin;
  label: string;
}) {
  const tone = getPluginStatus(plugin);
  const chip = <StatusChip tone={pluginStatusChipTone[tone]} label={label} />;

  // An erroring plugin carries its diagnostic in last_error — surface it on
  // hover like the old badge's title attribute did.
  if (tone === "error") {
    return (
      <Tooltip content={plugin.last_error ?? label}>
        <span className="inline-flex">{chip}</span>
      </Tooltip>
    );
  }

  return chip;
}

export default function PluginManager({ businessId }: PluginManagerProps) {
  const [plugins, setPlugins] = useState<APIPlugin[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState(false);
  const [selectedPlugin, setSelectedPlugin] = useState<APIPlugin | null>(null);
  const [pluginConfig, setPluginConfig] = useState<Record<string, any>>({});
  const [searchQuery, setSearchQuery] = useState("");
  const [viewMode, setViewMode] = useState<"grid" | "list">("grid");
  // Disabling a live plugin (esp. payments) is destructive — confirm first and
  // guard against double-clicks with a per-plugin busy id.
  const [disableTarget, setDisableTarget] = useState<APIPlugin | null>(null);
  const [disablingId, setDisablingId] = useState<number | null>(null);
  // Inline "Test connection" state for the config modal. Lets owners verify
  // stored payment credentials before a guest's payment fails.
  const [testState, setTestState] = useState<{
    status: "idle" | "testing" | "ok" | "fail";
    message?: string;
  }>({ status: "idle" });

  // Translation setup
  const { locale } = useSimpleLocale();
  const currentLanguage = locale;

  // Operational lock (admin suspend/close) and RBAC access
  const { hasAccess, loading: accessLoading } = useBusinessAccess(businessId);

  const {
    isOpen: isConfigOpen,
    onOpen: onConfigOpen,
    onClose: onConfigCloseRaw,
  } = useDisclosure();

  // L6-32: baseline config when the modal opens; guard Esc/backdrop close.
  const { dirty: configDirty, markClean: markConfigClean } =
    useDirtyForm(pluginConfig);
  const [confirmDiscardConfig, setConfirmDiscardConfig] = useState(false);
  const onConfigClose = useCallback(() => {
    if (configDirty) {
      setConfirmDiscardConfig(true);
      return;
    }
    onConfigCloseRaw();
  }, [configDirty, onConfigCloseRaw]);

  // Translation helper
  const t = useCallback(
    (key: string): string => {
      const fullKey = `businessDashboard.dashboard.pluginManager.${key}`;
      const result = getTranslation(fullKey, currentLanguage);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [currentLanguage],
  );

  // Like `t`, but falls back to an English default when the key is missing
  // (getTranslation echoes the key on miss). Used for keys not yet present in
  // businessDashboard.json.
  const tOr = useCallback(
    (key: string, fallback: string): string => {
      const value = t(key);
      return value === key || value.endsWith(key) ? fallback : value;
    },
    [t],
  );

  // Load plugins with translations for current language
  const loadPlugins = useCallback(async () => {
    if (!businessId) return;

    try {
      setLoading(true);
      setLoadError(false);

      // Get all available platform plugins
      const platformResponse =
        await pluginAPI.protected.getAllPlugins(currentLanguage);
      const availablePlugins = (platformResponse.plugins || []).filter(
        (plugin) => plugin.is_active,
      );

      // Get business-specific plugin states
      const businessResponse =
        await pluginAPI.business.getBusinessPlugins(businessId);
      const businessPlugins = businessResponse.plugins || [];

      // Merge platform plugins with business plugin states
      const mergedPlugins = availablePlugins.map((plugin) => {
        const businessPlugin = businessPlugins.find(
          (bp) => bp.plugin_id === plugin.id,
        );
        // Parse per-plugin so ONE malformed config blob doesn't throw out of
        // the whole merge and blank the entire integrations tab.
        let businessConfig: Record<string, any> = {};
        if (businessPlugin?.config) {
          try {
            businessConfig = JSON.parse(businessPlugin.config);
          } catch (parseError) {
            console.error(
              `Failed to parse config for plugin ${plugin.name}:`,
              parseError,
            );
          }
        }
        return {
          ...plugin,
          is_enabled: businessPlugin?.is_enabled || false,
          business_config: businessConfig,
          last_status: businessPlugin?.last_status,
          last_error: businessPlugin?.last_error,
          last_error_at: businessPlugin?.last_error_at,
        };
      });

      setPlugins(mergedPlugins);
    } catch (error) {
      console.error("Failed to load plugins:", error);
      // Distinguish a real fetch failure from a genuinely-empty catalog so the
      // UI can offer Retry instead of the misleading "no plugins" empty state.
      setLoadError(true);
    } finally {
      setLoading(false);
    }
  }, [currentLanguage, businessId]);

  // Get translated content for a plugin using the API utility
  const getTranslatedContent = (
    plugin: APIPlugin,
    fieldName: string,
  ): string => {
    return pluginAPI.utils.getTranslatedContent(
      plugin,
      fieldName,
      currentLanguage,
    );
  };

  // Fetch business profile for payments configuration
  const [businessProfile, setBusinessProfile] = useState<any>(null);

  useEffect(() => {
    const fetchBusinessProfile = async () => {
      if (!businessId) return;
      try {
        const { businessApi } = await import("@/api/business");
        const profile = await businessApi.getBusiness(businessId);
        setBusinessProfile(profile);
      } catch (error) {
        console.error("Failed to fetch business profile:", error);
      }
    };
    fetchBusinessProfile();
  }, [businessId]);

  useEffect(() => {
    if (!accessLoading && hasAccess) {
      loadPlugins();
    }
  }, [loadPlugins, accessLoading, hasAccess]);

  // Handle plugin enable/configure
  const handlePluginEnable = async (plugin: APIPlugin) => {
    if (plugin.coming_soon) {
      return; // Should not be clickable for coming soon plugins
    }

    setSelectedPlugin(plugin);
    setTestState({ status: "idle" });

    // If plugin is already enabled, load existing configuration
    let nextConfig: Record<string, any> = {};
    if (plugin.is_enabled && businessId) {
      try {
        const configResponse = await pluginAPI.business.getPluginConfig(
          businessId,
          plugin.id,
        );
        nextConfig = configResponse.config || {};
      } catch (error) {
        console.error("Failed to load plugin config:", error);
        nextConfig = plugin.business_config || {};
      }
    }
    setPluginConfig(nextConfig);
    markConfigClean(nextConfig);

    onConfigOpen();
  };

  // Handle plugin configuration save
  const handleConfigSave = async () => {
    if (!selectedPlugin || !businessId) return;

    try {
      const isFirstTelegramEnable =
        selectedPlugin.name === PLUGIN.telegram && !selectedPlugin.is_enabled;

      if (selectedPlugin.is_enabled) {
        // Update existing plugin configuration
        await pluginAPI.business.updatePluginConfig(
          businessId,
          selectedPlugin.id,
          {
            config: pluginConfig,
          },
        );
        toast.success(
          t("toasts.configUpdated").replace(
            "{name}",
            selectedPlugin.display_name,
          ),
        );
      } else {
        // Enable the plugin with the configuration
        await pluginAPI.business.enablePlugin(businessId, selectedPlugin.id, {
          config: pluginConfig,
        });
        if (isFirstTelegramEnable) {
          toast.success(t("toasts.telegramEnabled"));
        } else {
          toast.success(
            t("toasts.enabled").replace("{name}", selectedPlugin.display_name),
          );
        }
      }

      // Reload plugins to show updated state
      await loadPlugins();

      if (isFirstTelegramEnable) {
        setSelectedPlugin({
          ...selectedPlugin,
          is_enabled: true,
        });
        markConfigClean(pluginConfig);
        return;
      }

      markConfigClean(pluginConfig);
      onConfigCloseRaw();
    } catch (error) {
      console.error("Failed to save plugin configuration:", error);
      // Never surface raw gin binding dumps (FIND-031).
      const raw = getApiErrorMessage(error);
      const fallback = t("toasts.failedToSaveConfig").replace(
        "{name}",
        selectedPlugin.display_name,
      );
      const message =
        raw && !isRawValidatorDump(raw) && raw.length <= 200 ? raw : fallback;
      toast.error(message);
    }
  };

  // Payment providers exposing a cheap read-only credential probe. Others have
  // no testable endpoint, so the "Test connection" button is hidden for them.
  const testableProviders = new Set<string>([
    PLUGIN.stripe,
    PLUGIN.mercadopago,
    PLUGIN.paypal,
  ]);

  const handleTestConnection = async () => {
    if (!selectedPlugin || !businessId) return;
    setTestState({ status: "testing" });
    try {
      const result = await pluginAPI.business.testPluginConnection(
        businessId,
        selectedPlugin.name,
      );
      setTestState({
        status: result.ok ? "ok" : "fail",
        message: result.message,
      });
    } catch (error) {
      const fallback = tOr(
        "test.error",
        "Couldn't reach the payment provider. Please try again.",
      );
      const message = getSafeApiErrorMessage(error, fallback);
      setTestState({ status: "fail", message });
    }
  };

  // Handle plugin disable — invoked only after the confirmation dialog. The
  // per-plugin busy id prevents a double-click from firing two disable calls.
  const handlePluginDisable = async (plugin: APIPlugin) => {
    if (!businessId || disablingId) return;

    setDisablingId(plugin.id);
    try {
      await pluginAPI.business.disablePlugin(businessId, plugin.id);
      toast.success(
        t("toasts.disabled").replace("{name}", plugin.display_name),
      );
      setDisableTarget(null);

      // Reload plugins to show updated state
      await loadPlugins();
    } catch (error) {
      console.error("Failed to disable plugin:", error);
      toast.error(
        t("toasts.failedToDisable").replace("{name}", plugin.display_name),
      );
    } finally {
      setDisablingId(null);
    }
  };

  // Filter plugins based on search query
  const filteredPlugins = plugins.filter((plugin) => {
    if (!searchQuery.trim()) return true;

    const query = searchQuery.toLowerCase();
    const pluginName = getTranslatedContent(
      plugin,
      "display_name",
    ).toLowerCase();
    const pluginDescription = getTranslatedContent(
      plugin,
      "description",
    ).toLowerCase();
    const pluginMessage = getTranslatedContent(plugin, "message").toLowerCase();

    // Get translated features and search within them
    let features: string[] = [];
    try {
      const parsed = JSON.parse(plugin.features || "[]");
      if (Array.isArray(parsed)) {
        features = parsed.filter((f): f is string => typeof f === "string");
      }
    } catch (error) {
      console.error(
        `Failed to parse features for plugin ${plugin.name}:`,
        error,
      );
    }
    const featuresText = features.join(" ").toLowerCase();

    return (
      pluginName.includes(query) ||
      pluginDescription.includes(query) ||
      pluginMessage.includes(query) ||
      featuresText.includes(query)
    );
  });

  // Group filtered plugins by category, then apply operator marketplace order:
  // Mercado Pago / real checkout first, crypto last, Coming Soon at the bottom.
  const pluginsByCategory = filteredPlugins.reduce(
    (acc, plugin) => {
      if (!acc[plugin.category]) {
        acc[plugin.category] = [];
      }
      acc[plugin.category].push(plugin);
      return acc;
    },
    {} as Record<string, APIPlugin[]>,
  );
  for (const category of Object.keys(pluginsByCategory)) {
    pluginsByCategory[category] = sortPluginsForOperatorMarketplace(
      pluginsByCategory[category],
    );
  }

  // Define category order with payment first
  const categoryOrder = [
    "payment",
    "analytics",
    "integration",
    "reporting",
    "marketing",
  ];

  // Sort categories to put payment first
  const sortedCategories = Object.keys(pluginsByCategory).sort((a, b) => {
    const aIndex = categoryOrder.indexOf(a);
    const bIndex = categoryOrder.indexOf(b);
    if (aIndex === -1) return 1;
    if (bIndex === -1) return -1;
    return aIndex - bIndex;
  });
  const cardRailPlugins = plugins.filter(
    (plugin) =>
      plugin.category === "payment" &&
      !plugin.coming_soon &&
      isCardPaymentPlugin(plugin.name),
  );
  const hasEnabledCardRail = cardRailPlugins.some((plugin) => plugin.is_enabled);
  const primaryCardPlugin = [...cardRailPlugins].sort(
    (a, b) => paymentPluginRank(a.name) - paymentPluginRank(b.name),
  )[0];
  const enabledCount = plugins.filter(
    (plugin) => plugin.is_enabled && !plugin.coming_soon,
  ).length;
  const availableCount = plugins.filter(
    (plugin) => !plugin.coming_soon && !plugin.is_enabled,
  ).length;
  const comingSoonCount = plugins.filter((plugin) => plugin.coming_soon).length;
  const attentionCount = plugins.filter(
    (plugin) =>
      plugin.is_enabled &&
      !plugin.coming_soon &&
      plugin.last_status === "error",
  ).length;
  const visibleCount = filteredPlugins.length;
  const hasSearch = searchQuery.trim().length > 0;
  // "Need a custom plugin?" belongs at the end of the full catalog and in place
  // of a zero-result search — an operator searching for an integration we don't
  // carry is the strongest signal of unmet need the marketplace ever gets. It
  // does not belong under a filtered view that found something (keeps results
  // tight), nor under a failed load, where an empty screen means "we didn't
  // ask", not "we don't have it". (L6-34) It also needs somewhere to send the
  // operator: a self-hosted install ships no booking link, so the card stays
  // hidden until one is configured in `@/config/brand`.
  const customPluginBookingUrl = brandLinks.bookCallUrl;
  const showCustomPluginCard =
    Boolean(customPluginBookingUrl) &&
    !loadError &&
    (!hasSearch || visibleCount === 0);
  const statusLabelFor = (plugin: APIPlugin) =>
    t(`status.${getPluginStatus(plugin)}`);

  return (
    <DashboardTabShell
      locked={
        !accessLoading && !hasAccess ? (
          <DashboardLockedTabView
            title={t("title")}
            subtitle={t("subtitle")}
            businessId={businessId}
          />
        ) : null
      }
      loading={loading ? <DashboardTabLoadingSkeleton withPageChrome={false} /> : null}
      header={{
        title: t("title"),
        subtitle: t("subtitle"),
        /* Quiet by default — the dot only appears when an enabled plugin is
           erroring. One home per number: enabled/payment counts live here; the
           toolbar's "showing X of Y" owns visible/available. */
        status:
          attentionCount > 0
            ? {
                label: t("shell.signals.attention"),
                tone: "attention",
              }
            : undefined,
        stats: [
          { label: t("shell.signals.enabled"), value: enabledCount },
          {
            label: tOr("shell.signals.available", "Available"),
            value: availableCount,
          },
          {
            label: tOr("shell.signals.comingSoon", "Coming Soon"),
            value: comingSoonCount,
          },
        ],
      }}
    >
      <Toolbar
        search={{
          value: searchQuery,
          onChange: setSearchQuery,
          placeholder: t("searchPlaceholder"),
        }}
      >
        <span className="text-xs text-ink-500">
          {t("showing")
            .replace("{visible}", String(visibleCount))
            .replace("{total}", String(plugins.length))}
        </span>
        <ViewToggle
          value={viewMode}
          onChange={setViewMode}
          labels={{ grid: t("viewMode.grid"), list: t("viewMode.list") }}
        />
      </Toolbar>

      {/* Load failed — offer Retry instead of the "no plugins" empty state,
          which would falsely imply the catalog is genuinely empty. */}
      {loadError ? (
        <EmptyState
          icon={Plug}
          title={tOr("loadError.title", "Couldn't load integrations")}
          subtitle={tOr(
            "loadError.description",
            "Something went wrong loading your plugins. Please try again.",
          )}
          panel
          actionLabel={tOr("loadError.retry", "Retry")}
          onAction={() => loadPlugins()}
        />
      ) : /* Show search results or no results message */
      Object.keys(pluginsByCategory).length === 0 && hasSearch ? (
        <EmptyState
          icon={Search}
          title={t("noSearchResults").replace("{searchQuery}", searchQuery)}
          subtitle={t("noSearchResultsDescription")}
          panel
          actionLabel={t("clearSearch")}
          onAction={() => setSearchQuery("")}
        />
      ) : Object.keys(pluginsByCategory).length === 0 ? (
        <EmptyState
          icon={Puzzle}
          title={t("noPlugins")}
          subtitle={t("noPluginsDescription")}
          panel
        />
      ) : (
        // Show plugin categories
        sortedCategories.map((category) => {
          const categoryPlugins = pluginsByCategory[category];
          const categoryInfo =
            PLUGIN_CATEGORIES[category as keyof typeof PLUGIN_CATEGORIES];
          const IconComponent = categoryInfo?.icon || Puzzle;

          return (
            <section key={category} className="space-y-4">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div className="flex min-w-0 items-center gap-3">
                  <IconTile icon={IconComponent} />
                  <div className="min-w-0">
                    <h2 className="truncate text-lg font-semibold tracking-normal text-ink-900">
                      {t(`categories.${category}`) ||
                        categoryInfo?.label ||
                        category}
                    </h2>
                    <p className="text-xs font-medium text-ink-500">
                      {t("categorySubtitle")}
                    </p>
                  </div>
                </div>
                <span className="rounded-full border border-warm-200 bg-white px-3 py-1.5 text-xs font-semibold text-ink-600 shadow-sm">
                  {categoryPlugins.length}{" "}
                  {categoryPlugins.length === 1 ? t("plugin") : t("plugins")}
                </span>
              </div>

              {category === "payment" &&
              !hasEnabledCardRail &&
              primaryCardPlugin ? (
                <PremiumPanel
                  tone="urgent"
                  className="p-4"
                  withTexture={false}
                  data-testid="card-rail-callout"
                >
                  <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                    <div className="min-w-0">
                      <p className="text-sm font-semibold text-ink-900">
                        {tOr(
                          "cardRailCallout.title",
                          "No card processor enabled",
                        )}
                      </p>
                      <p className="mt-1 text-sm leading-6 text-ink-600">
                        {tOr(
                          "cardRailCallout.description",
                          "Guests can pay at the counter once the cash drawer is open. Enable a card processor to take cards — we will not turn it on until you connect your account.",
                        )}
                      </p>
                    </div>
                    <button
                      type="button"
                      onClick={() => handlePluginEnable(primaryCardPlugin)}
                      className={btnPrimaryCompact}
                    >
                      {tOr("cardRailCallout.action", "Enable {name}").replace(
                        "{name}",
                        getTranslatedContent(primaryCardPlugin, "display_name"),
                      )}
                    </button>
                  </div>
                </PremiumPanel>
              ) : null}

              {viewMode === "grid" ? (
                <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
                  {categoryPlugins.map((plugin) => {
                    const features = pluginAPI.utils.getTranslatedFeatures(
                      plugin,
                      currentLanguage,
                    );
                    const pluginImageSrc = resolvePluginImageSrc(plugin);
                    const pluginTitle = getTranslatedContent(
                      plugin,
                      "display_name",
                    );

                    return (
                      <PremiumPanel
                        key={plugin.id}
                        interactive={!plugin.coming_soon}
                        className={`group flex h-full flex-col p-5${
                          plugin.coming_soon
                            ? " opacity-70 saturate-50"
                            : ""
                        }`}
                        withTexture={false}
                      >
                        <div className="mb-5 space-y-3">
                          <div className="flex min-w-0 items-start gap-3">
                            <div className="flex h-12 w-12 flex-shrink-0 items-center justify-center rounded-xl border border-warm-200 bg-white shadow-sm">
                              {pluginImageSrc ? (
                                <Image
                                  src={pluginImageSrc}
                                  alt={plugin.display_name}
                                  width={28}
                                  height={28}
                                  className="rounded"
                                />
                              ) : (
                                <Puzzle
                                  className="h-6 w-6 text-ink-600"
                                  aria-hidden="true"
                                />
                              )}
                            </div>
                            <div className="min-w-0 flex-1">
                              <Tooltip content={pluginTitle}>
                                <h3 className="break-words text-base font-semibold tracking-normal text-ink-900">
                                  {pluginTitle}
                                </h3>
                              </Tooltip>
                              <p className="text-xs font-medium text-ink-500">
                                v{plugin.version}
                              </p>
                            </div>
                          </div>
                          <PluginStatusBadge
                            plugin={plugin}
                            label={statusLabelFor(plugin)}
                          />
                        </div>

                        <div className="flex-1">
                          <p className="mb-4 line-clamp-3 text-sm leading-6 text-ink-600">
                            {getTranslatedContent(plugin, "message")}
                          </p>

                          {features.length > 0 && (
                            <div className="mb-4">
                              <p className="mb-2 text-[11px] font-semibold uppercase tracking-normal text-ink-500">
                                {t("features")}
                              </p>
                              <div className="flex flex-wrap gap-1.5">
                                {features
                                  .slice(0, 3)
                                  .map((feature: string, index: number) => (
                                    <span
                                      key={index}
                                      className="rounded-full border border-warm-200 bg-warm-50 px-2.5 py-1 text-xs font-medium text-ink-600"
                                    >
                                      {feature}
                                    </span>
                                  ))}
                                {features.length > 3 && (
                                  <Tooltip
                                    content={features.slice(3).join(" · ")}
                                  >
                                    <span className="rounded-full border border-brand/15 bg-brand/5 px-2.5 py-1 text-xs font-semibold text-brand">
                                      {t("moreFeatures").replace(
                                        "{count}",
                                        String(features.length - 3),
                                      )}
                                    </span>
                                  </Tooltip>
                                )}
                              </div>
                            </div>
                          )}
                        </div>

                        <div className="mt-auto flex items-center justify-between gap-3 border-t border-warm-200/70 pt-4">
                          <span className="text-xs font-medium text-ink-500">
                            {t(`status.${footerStatusKey(getPluginStatus(plugin))}`)}
                          </span>
                          {plugin.coming_soon ? (
                            <button
                              type="button"
                              disabled
                              aria-disabled="true"
                              className="cursor-not-allowed rounded-full border border-warm-200 bg-warm-50 px-3 py-1.5 text-xs font-medium text-ink-500 opacity-80"
                            >
                              {t("comingSoon")}
                            </button>
                          ) : plugin.is_enabled ? (
                            <div className="flex items-center gap-2">
                              <button
                                onClick={() => setDisableTarget(plugin)}
                                disabled={disablingId === plugin.id}
                                className="rounded-full px-3 py-1.5 text-xs font-medium text-rose-700 transition-colors hover:bg-rose-50 hover:text-rose-800 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-rose-400/50 disabled:opacity-50 disabled:cursor-not-allowed"
                              >
                                {t("disable")}
                              </button>
                              <button
                                onClick={() => handlePluginEnable(plugin)}
                                className={btnPrimaryCompact}
                              >
                                {t("configure")}
                              </button>
                            </div>
                          ) : (
                            <button
                              onClick={() => handlePluginEnable(plugin)}
                              className={btnPrimaryCompact}
                            >
                              {t("enable")}
                            </button>
                          )}
                        </div>
                      </PremiumPanel>
                    );
                  })}
                </div>
              ) : (
                <div className="flex flex-col gap-3">
                  {categoryPlugins.map((plugin) => {
                    const features = pluginAPI.utils.getTranslatedFeatures(
                      plugin,
                      currentLanguage,
                    );
                    const pluginImageSrc = resolvePluginImageSrc(plugin);
                    const pluginTitle = getTranslatedContent(
                      plugin,
                      "display_name",
                    );

                    return (
                      <PremiumPanel
                        key={plugin.id}
                        interactive={!plugin.coming_soon}
                        className={`p-4${
                          plugin.coming_soon
                            ? " opacity-70 saturate-50"
                            : ""
                        }`}
                        withTexture={false}
                      >
                        <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
                          <div className="flex min-w-0 flex-1 items-start gap-4">
                            <div className="flex h-12 w-12 flex-shrink-0 items-center justify-center rounded-xl border border-warm-200 bg-white shadow-sm">
                              {pluginImageSrc ? (
                                <Image
                                  src={pluginImageSrc}
                                  alt={plugin.display_name}
                                  width={28}
                                  height={28}
                                  className="rounded"
                                />
                              ) : (
                                <Puzzle
                                  className="h-6 w-6 text-ink-600"
                                  aria-hidden="true"
                                />
                              )}
                            </div>
                            <div className="min-w-0 flex-1">
                              <div className="flex flex-wrap items-center gap-2">
                                <Tooltip content={pluginTitle}>
                                  <h3 className="max-w-full break-words text-base font-semibold tracking-normal text-ink-900">
                                    {pluginTitle}
                                  </h3>
                                </Tooltip>
                                <span className="rounded-full border border-warm-200 bg-warm-50 px-2 py-0.5 text-[10px] font-semibold text-ink-500">
                                  v{plugin.version}
                                </span>
                                <PluginStatusBadge
                                  plugin={plugin}
                                  label={statusLabelFor(plugin)}
                                />
                              </div>
                              <p className="mt-1 line-clamp-2 text-sm leading-6 text-ink-600">
                                {getTranslatedContent(plugin, "message")}
                              </p>
                              {features.length > 0 && (
                                <p className="mt-2 text-xs font-medium text-ink-500">
                                  {t("featureCount").replace(
                                    "{count}",
                                    String(features.length),
                                  )}
                                </p>
                              )}
                            </div>
                          </div>

                          <div className="flex w-full items-center justify-between gap-3 border-t border-warm-100 pt-3 sm:w-auto sm:justify-end sm:border-t-0 sm:pt-0">
                            <span className="text-xs font-medium text-ink-500 sm:hidden">
                              {t(`status.${footerStatusKey(getPluginStatus(plugin))}`)}
                            </span>
                            {plugin.coming_soon ? (
                              <button
                                type="button"
                                disabled
                                aria-disabled="true"
                                className="cursor-not-allowed rounded-full border border-warm-200 bg-warm-50 px-3 py-1.5 text-xs font-medium text-ink-500 opacity-80"
                              >
                                {t("comingSoon")}
                              </button>
                            ) : plugin.is_enabled ? (
                              <div className="flex items-center gap-2">
                                <button
                                  onClick={() => setDisableTarget(plugin)}
                                  disabled={disablingId === plugin.id}
                                  className="rounded-full px-3 py-1.5 text-xs font-medium text-rose-700 transition-colors hover:bg-rose-50 hover:text-rose-800 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-rose-400/50 disabled:opacity-50 disabled:cursor-not-allowed"
                                >
                                  {t("disable")}
                                </button>
                                <button
                                  onClick={() => handlePluginEnable(plugin)}
                                  className={btnPrimaryCompact}
                                >
                                  {t("configure")}
                                </button>
                              </div>
                            ) : (
                              <button
                                onClick={() => handlePluginEnable(plugin)}
                                className={btnPrimaryCompact}
                              >
                                {t("enable")}
                              </button>
                            )}
                          </div>
                        </div>
                      </PremiumPanel>
                    );
                  })}
                </div>
              )}
            </section>
          );
        })
      )}

      {/* Book a Call — see showCustomPluginCard above for when it appears. */}
      {showCustomPluginCard && (
        <PremiumPanel as="section" tone="accent" className="overflow-hidden">
          <div className="grid gap-6 p-6 sm:p-8 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-center">
            <div className="min-w-0">
              <h2 className="font-title text-heading-md text-ink-950 sm:text-heading-lg">
                {t("customPlugin.title")}
              </h2>
              <p className="mt-3 max-w-2xl text-sm leading-6 text-ink-600">
                {t("customPlugin.description")}
              </p>
              <div className="mt-4 flex flex-wrap gap-2">
                <span className="rounded-full border border-white/80 bg-white/75 px-3 py-1.5 text-xs font-semibold text-ink-600">
                  {t("customPlugin.chips.scope")}
                </span>
                <span className="rounded-full border border-white/80 bg-white/75 px-3 py-1.5 text-xs font-semibold text-ink-600">
                  {t("customPlugin.chips.security")}
                </span>
                <span className="rounded-full border border-white/80 bg-white/75 px-3 py-1.5 text-xs font-semibold text-ink-600">
                  {t("customPlugin.chips.launch")}
                </span>
              </div>
            </div>
            <div className="flex flex-col items-start gap-3 lg:items-end">
              <button
                onClick={() =>
                  window.open(
                    customPluginBookingUrl,
                    "_blank",
                    "noopener,noreferrer",
                  )
                }
                className={btnPrimary}
              >
                <CalendarClock className="h-4 w-4" aria-hidden="true" />
                {t("customPlugin.bookCall")}
              </button>
              <p className="max-w-xs text-xs font-medium leading-5 text-ink-500 lg:text-right">
                {t("customPlugin.benefits")}
              </p>
            </div>
          </div>
        </PremiumPanel>
      )}

      {/* Plugin Configuration Modal — L6-32 dirty guard on Esc/backdrop */}
      <Modal
        isOpen={isConfigOpen}
        onClose={onConfigClose}
        size="4xl"
        scrollBehavior="inside"
      >
        <ModalContent>
          <ModalHeader className="text-lg font-semibold tracking-normal text-ink-900">
            {selectedPlugin
              ? t("configurePlugin").replace(
                  "{name}",
                  getTranslatedContent(selectedPlugin, "display_name"),
                )
              : t("modal.title")}
          </ModalHeader>
          <ModalBody className="p-6">
            {selectedPlugin &&
              selectedPlugin.is_enabled &&
              testableProviders.has(selectedPlugin.name) && (
                <div className="mb-4 flex flex-col gap-2 rounded-xl border border-warm-200 bg-warm-50 p-4 sm:flex-row sm:items-center sm:justify-between">
                  <div className="min-w-0">
                    <p className="text-sm font-semibold text-ink-900">
                      {tOr("test.title", "Test connection")}
                    </p>
                    <p className="text-xs text-ink-500">
                      {tOr(
                        "test.subtitle",
                        "Verify your saved credentials with a live check.",
                      )}
                    </p>
                    {testState.status === "ok" && (
                      <p className="mt-1 text-xs font-semibold text-emerald-600">
                        {testState.message ||
                          tOr("test.ok", "Connection successful.")}
                      </p>
                    )}
                    {testState.status === "fail" && (
                      <p className="mt-1 text-xs font-semibold text-red-600">
                        {testState.message ||
                          tOr(
                            "test.fail",
                            "Connection failed. Please check your credentials.",
                          )}
                      </p>
                    )}
                  </div>
                  <button
                    type="button"
                    onClick={handleTestConnection}
                    disabled={testState.status === "testing"}
                    className="flex-shrink-0 rounded-lg border border-ink-300 px-4 py-2 text-sm font-semibold text-ink-800 transition-colors hover:border-ink-900 disabled:cursor-not-allowed disabled:opacity-50"
                  >
                    {testState.status === "testing"
                      ? tOr("test.testing", "Testing…")
                      : tOr("test.button", "Test connection")}
                  </button>
                </div>
              )}
            {selectedPlugin && (
              <PluginConfigFactory
                plugin={{
                  id: selectedPlugin.id,
                  name: selectedPlugin.name,
                  display_name: selectedPlugin.display_name,
                  description: selectedPlugin.description,
                  image: resolvePluginImageSrc(selectedPlugin) || "",
                  category: selectedPlugin.category,
                  version: selectedPlugin.version,
                  features: selectedPlugin.features,
                  is_enabled: selectedPlugin.is_enabled || false,
                  config: JSON.stringify(pluginConfig),
                  config_schema: selectedPlugin.config_schema,
                }}
                config={pluginConfig}
                onConfigChange={setPluginConfig}
                onSave={handleConfigSave}
                onCancel={onConfigClose}
                businessId={businessId}
                businessProfile={businessProfile}
              />
            )}
          </ModalBody>
        </ModalContent>
      </Modal>

      <ConfirmationModal
        isOpen={confirmDiscardConfig}
        onOpenChange={() => setConfirmDiscardConfig(false)}
        title={tOr("discardConfig.title", "Discard configuration changes?")}
        description={tOr(
          "discardConfig.description",
          "You have unsaved plugin configuration edits. Closing now will discard them.",
        )}
        confirmLabel={tOr("discardConfig.confirm", "Discard")}
        cancelLabel={tOr("discardConfig.cancel", "Keep editing")}
        isDanger
        onConfirm={() => {
          setConfirmDiscardConfig(false);
          markConfigClean(pluginConfig);
          onConfigCloseRaw();
        }}
      />

      {/* Disable-plugin confirmation — disabling a live payment plugin stops
          real transactions, so require an explicit confirm (matches the
          delete-offer pattern). */}
      <Modal
        isOpen={disableTarget !== null}
        onClose={() => {
          if (!disablingId) setDisableTarget(null);
        }}
        size="md"
      >
        <ModalContent>
          <ModalHeader className="text-lg font-semibold tracking-normal text-ink-900">
            {tOr("disableModal.title", "Disable plugin?")}
          </ModalHeader>
          <ModalBody className="p-6">
            <p className="text-sm text-ink-600">
              {disableTarget?.category === "payment"
                ? tOr(
                    "disableModal.paymentBody",
                    "This is a live payment integration. Disabling it will stop processing payments through it immediately.",
                  )
                : tOr(
                    "disableModal.body",
                    "Disabling this plugin will turn off its integration immediately.",
                  )}
            </p>
            <div className="mt-6 flex justify-end gap-2">
              <button
                onClick={() => setDisableTarget(null)}
                disabled={!!disablingId}
                className={btnSecondary}
              >
                {tOr("disableModal.cancel", "Keep enabled")}
              </button>
              <button
                onClick={() =>
                  disableTarget && handlePluginDisable(disableTarget)
                }
                disabled={!!disablingId}
                className="rounded-full bg-rose-600 px-4 py-2 text-sm font-semibold text-white transition-colors hover:bg-rose-700 disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {disablingId
                  ? tOr("disableModal.disabling", "Disabling…")
                  : tOr("disableModal.confirm", "Disable")}
              </button>
            </div>
          </ModalBody>
        </ModalContent>
      </Modal>
    </DashboardTabShell>
  );
}
