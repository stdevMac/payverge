import React, { useState, useEffect, useCallback, useRef } from "react";
import Image from "next/image";
import { Button, Input, Chip, Switch, Spinner, Code, Modal, ModalContent, ModalHeader, ModalBody, ModalFooter, useDisclosure } from "@nextui-org/react";
import { CircleCheck, CircleAlert, Send, ExternalLink, Bell, Copy } from "lucide-react";
import toast from "react-hot-toast";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { axiosInstance } from "@/api/tools/instance";
import { formatBusinessDateTime } from "@/utils/businessTime";
import { PluginConfigFooter } from "./PluginConfigFooter";

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

interface TelegramConfigProps {
  businessId?: string;
  plugin: Plugin;
  config: Record<string, any>;
  onConfigChange: (config: Record<string, any>) => void;
  onSave: () => void;
  onCancel: () => void;
  businessTimezone?: string | null;
  /** Loaded business display name — used to derive config.business_name (no input). */
  businessName?: string;
}

interface ConnectionStatus {
  connected: boolean;
  plugin_enabled: boolean;
  health?: "not_connected" | "pending" | "connected" | "degraded" | "disabled";
  chat_id?: string;
  chat_type?: string;
  chat_title?: string;
  telegram_username?: string;
  business_name?: string;
  connected_at?: string;
  pending_token_expires_at?: string;
  last_sent_at?: string;
  last_test_sent_at?: string;
  last_error?: string;
  last_error_at?: string;
  failure_count?: number;
  delivery_queue_depth?: number;
  last_activity?: string;
  error?: string;
}

interface NotificationSettings {
  order_notifications: boolean;
  payment_notifications: boolean;
  daily_summary: boolean;
  weekly_summary: boolean;
  monthly_summary: boolean;
  low_stock_alerts: boolean;
  new_customer_alerts: boolean;
  high_value_orders: boolean;
}

// Workforce alerts live in the primary `notifications` preference map read by
// the backend notification gate (telegramEventPreferenceKeys). They have no
// legacy `notification_settings` fallback, so they must be persisted under
// `notifications` to reach the gate. Opt-in (default off).
interface WorkforceNotifications {
  schedule_published: boolean;
  shift_reminder: boolean;
  coverage_decided: boolean;
  [key: string]: boolean;
}

const telegramPanelClass =
  "rounded-2xl border border-warm-200/80 bg-white/90 p-5 shadow-sm shadow-warm-900/5 sm:p-6";
const telegramSectionTitleClass =
  "font-title text-base font-semibold tracking-0 text-ink-950";
const telegramCopyClass = "text-sm leading-6 text-ink-600";
const telegramMutedCopyClass = "text-xs leading-5 text-ink-600";
const telegramIconTileClass =
  "flex h-11 w-11 items-center justify-center rounded-2xl border border-warm-200 bg-white shadow-sm";
const telegramHelpBoxClass =
  "rounded-2xl border border-warm-200 bg-warm-50/70 p-3 text-xs text-ink-700";
const telegramBrandBoxClass =
  "rounded-2xl border border-brand/20 bg-brand/5 p-3 text-xs text-brand-800";
const telegramSwitchRowClass =
  "flex items-center justify-between gap-4 rounded-2xl border border-warm-200/80 bg-warm-50/60 px-4 py-3";

export default function TelegramConfig({
  businessId,
  plugin,
  config,
  onConfigChange,
  onSave,
  onCancel,
  businessTimezone = null,
  businessName = "",
}: TelegramConfigProps) {
  const { locale } = useSimpleLocale();

  // Translation helper
  const t = useCallback(
    (key: string): string => {
      const fullKey = `businessDashboard.dashboard.pluginManager.config.telegram.${key}`;
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

  const [connectionStatus, setConnectionStatus] = useState<ConnectionStatus>({
    connected: false,
    plugin_enabled: false,
  });
  const [isLoading, setIsLoading] = useState(false);
  const [connectionUrl, setConnectionUrl] = useState("");
  const [connectionExpiresAt, setConnectionExpiresAt] = useState("");
  const [connectionExpiryCountdown, setConnectionExpiryCountdown] =
    useState("");
  const [testMessage, setTestMessage] = useState("");
  const [isSendingTest, setIsSendingTest] = useState(false);
  const [isConnecting, setIsConnecting] = useState(false);
  const [isGeneratingToken, setIsGeneratingToken] = useState(false);
  const [isRevokingToken, setIsRevokingToken] = useState(false);
  const connectionPollingRef = useRef<NodeJS.Timeout | null>(null);
  const connectionTimeoutRef = useRef<NodeJS.Timeout | null>(null);

  const {
    isOpen: isTestModalOpen,
    onOpen: onTestModalOpen,
    onClose: onTestModalClose,
  } = useDisclosure();
  const {
    isOpen: isDisconnectModalOpen,
    onOpen: onDisconnectModalOpen,
    onClose: onDisconnectModalClose,
  } = useDisclosure();

  // business_name is not an operator input — derive from saved config or the
  // loaded business profile so save is never blocked by a ghost field (L6-29).
  const derivedBusinessName = String(
    config.business_name || businessName || "",
  ).trim();

  const [formData, setFormData] = useState({
    business_name: derivedBusinessName,
    notification_settings: {
      order_notifications: true,
      payment_notifications: true,
      daily_summary: true,
      weekly_summary: false,
      monthly_summary: false,
      low_stock_alerts: false,
      new_customer_alerts: false,
      high_value_orders: false,
      ...config.notification_settings,
    } as NotificationSettings,
    // Preserve any existing primary `notifications` keys (e.g. order_created
    // seeded at connect time) and layer the workforce opt-ins on top.
    notifications: {
      ...config.notifications,
      schedule_published: config.notifications?.schedule_published ?? false,
      shift_reminder: config.notifications?.shift_reminder ?? false,
      coverage_decided: config.notifications?.coverage_decided ?? false,
    } as WorkforceNotifications,
  });
  const pluginEnabled = plugin.is_enabled || connectionStatus.plugin_enabled;

  const clearConnectionPolling = useCallback(() => {
    if (connectionPollingRef.current) {
      clearInterval(connectionPollingRef.current);
      connectionPollingRef.current = null;
    }
    if (connectionTimeoutRef.current) {
      clearTimeout(connectionTimeoutRef.current);
      connectionTimeoutRef.current = null;
    }
  }, []);

  useEffect(() => {
    return () => {
      clearConnectionPolling();
    };
  }, [clearConnectionPolling]);

  useEffect(() => {
    if (!connectionExpiresAt) {
      setConnectionExpiryCountdown("");
      return;
    }

    const updateCountdown = () => {
      const remainingMs = new Date(connectionExpiresAt).getTime() - Date.now();
      if (remainingMs <= 0) {
        setConnectionExpiryCountdown(t("expired"));
        return;
      }
      const totalSeconds = Math.ceil(remainingMs / 1000);
      const minutes = Math.floor(totalSeconds / 60);
      const seconds = totalSeconds % 60;
      setConnectionExpiryCountdown(`${minutes}m ${seconds}s`);
    };

    updateCountdown();
    const timer = setInterval(updateCountdown, 1000);
    return () => clearInterval(timer);
  }, [connectionExpiresAt, t]);

  // Keep derived name in form state if the business profile loads after mount.
  useEffect(() => {
    const next = String(config.business_name || businessName || "").trim();
    if (!next) return;
    setFormData((prev) =>
      prev.business_name === next ? prev : { ...prev, business_name: next },
    );
  }, [businessName, config.business_name]);

  useEffect(() => {
    // Update parent config when form data changes
    onConfigChange({
      ...formData,
      business_name:
        formData.business_name.trim() ||
        String(businessName || "").trim() ||
        "",
      chat_id: connectionStatus.chat_id || "",
      is_connected: connectionStatus.connected,
    });
  }, [formData, connectionStatus, onConfigChange, businessName]);

  const loadConnectionStatus = useCallback(async (
    showSpinner = true,
  ): Promise<ConnectionStatus | null> => {
    if (!businessId) {
      return null;
    }

    if (showSpinner) {
      setIsLoading(true);
    }
    try {
      const response = await axiosInstance.get(
        `/inside/businesses/${businessId}/plugins/telegram/status`,
      );
      setConnectionStatus(response.data);
      return response.data as ConnectionStatus;
    } catch (error) {
      console.error("Failed to load connection status:", error);
      const fallbackStatus = {
        connected: false,
        plugin_enabled: false,
        error: t("loadStatusError"),
      };
      setConnectionStatus(fallbackStatus);
      return fallbackStatus;
    } finally {
      if (showSpinner) {
        setIsLoading(false);
      }
    }
  }, [businessId, t]);

  const generateConnectionToken = useCallback(async () => {
    if (!businessId || !pluginEnabled) {
      setConnectionUrl("");
      setConnectionExpiresAt("");
      return;
    }

    setIsGeneratingToken(true);
    try {
      const response = await axiosInstance.post(
        `/inside/businesses/${businessId}/plugins/telegram/generate-token`,
      );
      setConnectionUrl(response.data.url || "");
      setConnectionExpiresAt(response.data.expires_at || "");
    } catch (error) {
      console.error("Failed to generate connection token:", error);
      toast.error(t("generateLinkFailed"));
    } finally {
      setIsGeneratingToken(false);
    }
  }, [businessId, pluginEnabled, t]);

  useEffect(() => {
    if (!businessId) {
      return;
    }

    void loadConnectionStatus();
    if (pluginEnabled) {
      void generateConnectionToken();
    } else {
      setConnectionUrl("");
      setConnectionExpiresAt("");
    }
  }, [
    businessId,
    generateConnectionToken,
    loadConnectionStatus,
    pluginEnabled,
  ]);

  const startConnectionPolling = () => {
    if (!businessId) {
      return;
    }

    clearConnectionPolling();
    setIsConnecting(true);

    connectionPollingRef.current = setInterval(async () => {
      try {
        const nextStatus = await loadConnectionStatus(false);

        // Check if connection was successful
        if (nextStatus?.connected) {
          clearConnectionPolling();
          setIsConnecting(false);
          toast.success(t("connectedToast"));
        } else if (
          connectionExpiresAt &&
          new Date(connectionExpiresAt).getTime() <= Date.now()
        ) {
          clearConnectionPolling();
          setIsConnecting(false);
          toast.error(t("telegramExpired"));
          await generateConnectionToken();
        }
      } catch (error) {
        console.error("Polling error:", error);
      }
    }, 3000); // Poll every 3 seconds

    // Stop polling after 5 minutes
    connectionTimeoutRef.current = setTimeout(() => {
      clearConnectionPolling();
      setIsConnecting(false);
    }, 300000); // 5 minutes
  };

  const handleConnectClick = () => {
    if (!pluginEnabled) {
      toast.error(t("enableBeforeConnectError"));
      return;
    }

    if (!businessId || !connectionUrl) {
      toast.error(t("startSetupError"));
      return;
    }

    // Open Telegram bot link
    window.open(connectionUrl, "_blank", "noopener,noreferrer");

    // Start polling for connection
    startConnectionPolling();
  };

  const stopConnectionPolling = () => {
    clearConnectionPolling();
    setIsConnecting(false);
  };

  const revokeConnectionToken = async () => {
    if (!businessId) {
      toast.error(t("businessMissing"));
      return;
    }

    setIsRevokingToken(true);
    try {
      await axiosInstance.post(
        `/inside/businesses/${businessId}/plugins/telegram/revoke-token`,
      );
      clearConnectionPolling();
      setIsConnecting(false);
      setConnectionUrl("");
      setConnectionExpiresAt("");
      await loadConnectionStatus(false);
      toast.success(t("connectionLinkRevoked"));
    } catch (error) {
      console.error("Failed to revoke Telegram connection token:", error);
      toast.error(t("connectionLinkRevokeFailed"));
    } finally {
      setIsRevokingToken(false);
    }
  };

  const handleNotificationChange = (
    setting: keyof NotificationSettings,
    value: boolean,
  ) => {
    const newSettings = {
      ...formData.notification_settings,
      [setting]: value,
    };
    setFormData({
      ...formData,
      notification_settings: newSettings,
    });
  };

  const handleWorkforceNotificationChange = (
    setting: keyof WorkforceNotifications,
    value: boolean,
  ) => {
    setFormData({
      ...formData,
      notifications: {
        ...formData.notifications,
        [setting]: value,
      },
    });
  };

  const copyToClipboard = async (text: string) => {
    try {
      await navigator.clipboard.writeText(text);
      toast.success(t("copySuccess"));
    } catch (error) {
      console.error("Failed to copy to clipboard:", error);
      toast.error(t("copyError"));
    }
  };

  const sendTestNotification = async () => {
    if (!testMessage.trim()) return;
    if (!businessId) {
      toast.error(t("businessMissing"));
      return;
    }

    setIsSendingTest(true);
    try {
      await axiosInstance.post(
        `/inside/businesses/${businessId}/plugins/telegram/test`,
        {
          message: testMessage,
        },
      );

      setTestMessage("");
      onTestModalClose();
      toast.success(t("testSentSuccess"));
    } catch (error) {
      console.error("Failed to send test notification:", error);
      toast.error(t("testSentError"));
    } finally {
      setIsSendingTest(false);
    }
  };

  const disconnectTelegram = async () => {
    if (!businessId) {
      toast.error(t("businessMissing"));
      return;
    }

    setIsLoading(true);
    try {
      await axiosInstance.post(
        `/inside/businesses/${businessId}/plugins/telegram/disconnect`,
      );
      await loadConnectionStatus();
      onDisconnectModalClose();
      toast.success(t("disconnectSuccess"));
    } catch (error) {
      console.error("Failed to disconnect Telegram:", error);
      toast.error(t("disconnectError"));
    } finally {
      setIsLoading(false);
    }
  };

  if (isLoading) {
    return (
      <div className="flex items-center justify-center p-8">
        <Spinner size="lg" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center gap-3">
        <div className={telegramIconTileClass}>
          <Image
            src={plugin.image || "/images/plugins/telegram.png"}
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
          <p className={telegramCopyClass}>{t("description")}</p>
        </div>
      </div>

      {/* Connection Status */}
      <div className={telegramPanelClass}>
        <div className="mb-4 flex w-full items-center justify-between">
          <h4 className={telegramSectionTitleClass}>{tCommon("connectionStatus")}</h4>
            {connectionStatus.connected ? (
              <Chip
                color={connectionStatus.health === "degraded" ? "warning" : "success"}
                variant="flat"
                startContent={<CircleCheck size={16} />}
              >
                {connectionStatus.health === "degraded"
                  ? t("degraded")
                  : tCommon("connected")}
              </Chip>
            ) : connectionStatus.health === "pending" ? (
              <Chip color="warning" variant="flat" startContent={<CircleAlert size={16} />}>
                {t("pending")}
              </Chip>
            ) : connectionStatus.health === "disabled" ? (
              <Chip color="default" variant="flat" startContent={<CircleAlert size={16} />}>
                {t("disabled")}
              </Chip>
            ) : (
              <Chip color="default" variant="flat" startContent={<CircleAlert size={16} />}>
                {tCommon("notConnected")}
              </Chip>
            )}
        </div>
        <div>
          {connectionStatus.connected ? (
            <div className="space-y-3">
              <p className={telegramCopyClass}>
                {t("telegramNotificationsActive")}
              </p>
              <p className={telegramCopyClass}>
                {tCommon("chatId")}:{" "}
                <Code size="sm">{connectionStatus.chat_id}</Code>
              </p>
              {connectionStatus.chat_title ? (
                <p className={telegramCopyClass}>
                  {t("chat")}: {connectionStatus.chat_title}
                </p>
              ) : null}
              {connectionStatus.connected_at && (
                <p className={telegramCopyClass}>
                  {tCommon("connectedAt")}:{" "}
                  {formatBusinessDateTime(
                    connectionStatus.connected_at,
                    locale,
                    businessTimezone,
                  )}
                </p>
              )}
              {connectionStatus.last_error ? (
                <div className="text-xs text-amber-700 bg-amber-50 p-3 rounded-lg border border-amber-200">
                  <p className="font-medium">{t("telegramDegraded")}</p>
                  <p>{connectionStatus.last_error}</p>
                  {connectionStatus.last_error_at ? (
                    <p className="mt-1">
                      {t("lastErrorAt")}:{" "}
                      {formatBusinessDateTime(
                        connectionStatus.last_error_at,
                        locale,
                        businessTimezone,
                      )}
                    </p>
                  ) : null}
                </div>
              ) : null}
              {connectionStatus.last_sent_at ? (
                <p className={telegramCopyClass}>
                  {t("lastSentAt")}:{" "}
                  {formatBusinessDateTime(
                    connectionStatus.last_sent_at,
                    locale,
                    businessTimezone,
                  )}
                </p>
              ) : null}
              {connectionStatus.last_test_sent_at ? (
                <p className={telegramCopyClass}>
                  {t("lastTestSentAt")}:{" "}
                  {formatBusinessDateTime(
                    connectionStatus.last_test_sent_at,
                    locale,
                    businessTimezone,
                  )}
                </p>
              ) : null}
              {typeof connectionStatus.delivery_queue_depth === "number" ? (
                <p className={telegramCopyClass}>
                  {t("deliveryQueueDepth")}:{" "}
                  {connectionStatus.delivery_queue_depth}
                </p>
              ) : null}
              <div className="flex gap-2">
                <Button
                  color="primary"
                  variant="flat"
                  size="sm"
                  onPress={onTestModalOpen}
                  startContent={<Send size={16} />}
                >
                  {t("sendTest")}
                </Button>
                <Button
                  color="danger"
                  variant="light"
                  size="sm"
                  onPress={onDisconnectModalOpen}
                >
                  {t("disconnect")}
                </Button>
                <Button
                  variant="light"
                  size="sm"
                  onPress={() => void loadConnectionStatus(false)}
                >
                  {t("refreshStatus")}
                </Button>
              </div>
            </div>
          ) : (
            <div className="space-y-4">
              <p className={telegramCopyClass}>
                {t("connectTelegramAccount")}
              </p>

              {connectionStatus.health === "pending" ? (
                <div className="space-y-2 text-xs text-amber-700 bg-amber-50 p-3 rounded-lg border border-amber-200">
                  <p>{t("telegramPending")}</p>
                  {connectionStatus.pending_token_expires_at ? (
                    <p>
                      {t("connectionExpiresAt")}:{" "}
                      {formatBusinessDateTime(
                        connectionStatus.pending_token_expires_at,
                        locale,
                        businessTimezone,
                      )}
                    </p>
                  ) : null}
                  {connectionExpiryCountdown ? (
                    <p>
                      {t("expiresIn")}: {connectionExpiryCountdown}
                    </p>
                  ) : null}
                  <Button
                    size="sm"
                    variant="light"
                    color="danger"
                    onPress={revokeConnectionToken}
                    isLoading={isRevokingToken}
                  >
                    {t("revokeConnectionLink")}
                  </Button>
                </div>
              ) : null}

              {!pluginEnabled ? (
                <div className="text-xs text-amber-700 bg-amber-50 p-3 rounded-lg border border-amber-200">
                  {t("enableBeforeConnectNotice")}
                </div>
              ) : null}

              {pluginEnabled && connectionUrl && (
                <div className="space-y-3">
                  <div className="flex items-center gap-2">
                    <Code size="sm" className="flex-1">
                      {connectionUrl}
                    </Code>
                    <Button
                      isIconOnly
                      size="sm"
                      variant="light"
                      aria-label={t("copyLink")}
                      onPress={() => copyToClipboard(connectionUrl)}
                    >
                      <Copy size={16} />
                    </Button>
                    <Button
                      size="sm"
                      variant="flat"
                      onPress={() => void generateConnectionToken()}
                      isLoading={isGeneratingToken}
                    >
                      {t("regenerateLink")}
                    </Button>
                    <Button
                      size="sm"
                      variant="light"
                      color="danger"
                      onPress={revokeConnectionToken}
                      isLoading={isRevokingToken}
                    >
                      {t("revoke")}
                    </Button>
                  </div>

                  <Button
                    color="primary"
                    onPress={handleConnectClick}
                    endContent={
                      isConnecting ? (
                        <Spinner size="sm" color="white" />
                      ) : (
                        <ExternalLink size={16} />
                      )
                    }
                    className="w-full"
                    isLoading={isConnecting}
                    isDisabled={isConnecting}
                  >
                    {isConnecting
                      ? t("waitingForConnection")
                      : t("connectToTelegramBot")}
                  </Button>

                  {isConnecting ? (
                    <div className={telegramBrandBoxClass}>
                      <p className="font-medium mb-1">
                        {t("waitingForTelegramConnection")}
                      </p>
                      <p>{t("pleaseSendMessageToBot")}</p>
                      {connectionExpiresAt ? (
                        <p className="mt-1">
                          {t("connectionExpiresAt")}:{" "}
                          {formatBusinessDateTime(
                            connectionExpiresAt,
                            locale,
                            businessTimezone,
                          )}
                        </p>
                      ) : null}
                      {connectionExpiryCountdown ? (
                        <p className="mt-1">
                          {t("expiresIn")}: {connectionExpiryCountdown}
                        </p>
                      ) : null}
                      <div className="mt-2">
                        <Button
                          size="sm"
                          variant="light"
                          color="danger"
                          onPress={stopConnectionPolling}
                        >
                          {tCommon("cancel")}
                        </Button>
                      </div>
                    </div>
                  ) : (
                    <div className={telegramHelpBoxClass}>
                      <p className="font-medium mb-1">{t("howToConnect")}</p>
                      <ol className="list-decimal list-inside space-y-1">
                        <li>{t("clickConnectToTelegramBot")}</li>
                        <li>{t("startConversationWithBot")}</li>
                        <li>{t("botWillAutomaticallyLinkAccount")}</li>
                        <li>
                          {t("returnHereToConfigureNotificationPreferences")}
                        </li>
                      </ol>
                    </div>
                  )}
                  <Button
                    variant="light"
                    size="sm"
                    onPress={() => void loadConnectionStatus(false)}
                  >
                    {t("refreshStatus")}
                  </Button>
                </div>
              )}
            </div>
          )}
        </div>
      </div>

      <div className="border-t border-warm-200/80" />

      {/* Notification Preferences */}
      <div className={telegramPanelClass}>
        <div className="mb-4">
          <div className="flex items-center gap-2">
            <Bell className="w-5 h-5" />
            <h4 className={telegramSectionTitleClass}>{t("notificationPreferences")}</h4>
          </div>
        </div>
        <div className="space-y-4">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div className="space-y-3">
              <h5 className="text-sm font-semibold text-ink-700">
                {t("orderAndPaymentNotifications")}
              </h5>

              <div className={telegramSwitchRowClass}>
                <div>
                  <p className="text-sm font-medium text-ink-950">
                    {t("orderNotifications")}
                  </p>
                  <p className={telegramMutedCopyClass}>
                    {t("getNotifiedForNewOrders")}
                  </p>
                </div>
                <Switch
                  isSelected={
                    formData.notification_settings.order_notifications
                  }
                  onValueChange={(value) =>
                    handleNotificationChange("order_notifications", value)
                  }
                />
              </div>

              <div className={telegramSwitchRowClass}>
                <div>
                  <p className="text-sm font-medium text-ink-950">
                    {t("paymentNotifications")}
                  </p>
                  <p className={telegramMutedCopyClass}>
                    {t("getNotifiedForPayments")}
                  </p>
                </div>
                <Switch
                  isSelected={
                    formData.notification_settings.payment_notifications
                  }
                  onValueChange={(value) =>
                    handleNotificationChange("payment_notifications", value)
                  }
                />
              </div>

              <div className={telegramSwitchRowClass}>
                <div>
                  <p className="text-sm font-medium text-ink-950">{t("highValueOrders")}</p>
                  <p className={telegramMutedCopyClass}>{t("highValueOrdersHint")}</p>
                </div>
                <Switch
                  isSelected={formData.notification_settings.high_value_orders}
                  onValueChange={(value) =>
                    handleNotificationChange("high_value_orders", value)
                  }
                />
              </div>
            </div>

            <div className="space-y-3">
              <h5 className="text-sm font-semibold text-ink-700">
                {t("businessSummaries")}
              </h5>

              <div className={telegramSwitchRowClass}>
                <div>
                  <p className="text-sm font-medium text-ink-950">{t("dailySummary")}</p>
                  <p className={telegramMutedCopyClass}>
                    {t("dailyBusinessOverview")}
                  </p>
                </div>
                <Switch
                  isSelected={formData.notification_settings.daily_summary}
                  onValueChange={(value) =>
                    handleNotificationChange("daily_summary", value)
                  }
                />
              </div>

              <div className={telegramSwitchRowClass}>
                <div>
                  <p className="text-sm font-medium text-ink-950">{t("weeklySummary")}</p>
                  <p className={telegramMutedCopyClass}>
                    {t("weeklyBusinessOverview")}
                  </p>
                </div>
                <Switch
                  isSelected={formData.notification_settings.weekly_summary}
                  onValueChange={(value) =>
                    handleNotificationChange("weekly_summary", value)
                  }
                />
              </div>

              <div className={telegramSwitchRowClass}>
                <div>
                  <p className="text-sm font-medium text-ink-950">{t("monthlySummary")}</p>
                  <p className={telegramMutedCopyClass}>
                    {t("monthlyBusinessOverview")}
                  </p>
                </div>
                <Switch
                  isSelected={formData.notification_settings.monthly_summary}
                  onValueChange={(value) =>
                    handleNotificationChange("monthly_summary", value)
                  }
                />
              </div>
            </div>
          </div>

          <div className="border-t border-warm-200/80" />

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div className="space-y-3">
              <h5 className="text-sm font-semibold text-ink-700">
                {t("inventoryAlerts")}
              </h5>

              <div className={telegramSwitchRowClass}>
                <div>
                  <p className="text-sm font-medium text-ink-950">{t("lowStockAlerts")}</p>
                  <p className={telegramMutedCopyClass}>
                    {t("getNotifiedForLowStock")}
                  </p>
                </div>
                <Switch
                  isSelected={formData.notification_settings.low_stock_alerts}
                  onValueChange={(value) =>
                    handleNotificationChange("low_stock_alerts", value)
                  }
                />
              </div>
            </div>

            <div className="space-y-3">
              <h5 className="text-sm font-semibold text-ink-700">
                {t("workforceAlerts")}
              </h5>

              <div className={telegramSwitchRowClass}>
                <div>
                  <p className="text-sm font-medium text-ink-950">
                    {t("schedulePublished")}
                  </p>
                  <p className={telegramMutedCopyClass}>
                    {t("getNotifiedForSchedulePublished")}
                  </p>
                </div>
                <Switch
                  isSelected={formData.notifications.schedule_published}
                  onValueChange={(value) =>
                    handleWorkforceNotificationChange("schedule_published", value)
                  }
                />
              </div>

              <div className={telegramSwitchRowClass}>
                <div>
                  <p className="text-sm font-medium text-ink-950">
                    {t("shiftReminders")}
                  </p>
                  <p className={telegramMutedCopyClass}>
                    {t("getNotifiedForShiftReminders")}
                  </p>
                </div>
                <Switch
                  isSelected={formData.notifications.shift_reminder}
                  onValueChange={(value) =>
                    handleWorkforceNotificationChange("shift_reminder", value)
                  }
                />
              </div>

              <div className={telegramSwitchRowClass}>
                <div>
                  <p className="text-sm font-medium text-ink-950">
                    {t("coverageDecisions")}
                  </p>
                  <p className={telegramMutedCopyClass}>
                    {t("getNotifiedForCoverageDecisions")}
                  </p>
                </div>
                <Switch
                  isSelected={formData.notifications.coverage_decided}
                  onValueChange={(value) =>
                    handleWorkforceNotificationChange("coverage_decided", value)
                  }
                />
              </div>
            </div>
          </div>
        </div>
      </div>

      <div className="border-t border-warm-200/80" />

      <PluginConfigFooter
        onSave={onSave}
        onCancel={onCancel}
        isEnabled={pluginEnabled}
        cancelLabel={tCommon("cancel")}
        saveLabel={tCommon("save")}
        enableLabel={tCommon("enable")}
        cancelClassName="rounded-xl px-4 py-2 text-sm font-medium text-ink-600 transition hover:bg-brand/5 hover:text-brand-700"
        buttonClassName="rounded-xl bg-brand px-4 py-2 text-sm font-semibold text-white shadow-sm transition hover:bg-brand-dark"
      />

      {/* Test Notification Modal */}
      <Modal isOpen={isTestModalOpen} onClose={onTestModalClose}>
        <ModalContent>
          <ModalHeader>{t("sendTestNotification")}</ModalHeader>
          <ModalBody>
            <Input
              label={t("testMessage")}
              placeholder={t("testMessagePlaceholder")}
              value={testMessage}
              onChange={(e) => setTestMessage(e.target.value)}
              description={t("testMessageDescription")}
            />
          </ModalBody>
          <ModalFooter>
            <Button variant="light" onPress={onTestModalClose}>
              {tCommon("cancel")}
            </Button>
            <Button
              color="primary"
              onPress={sendTestNotification}
              isLoading={isSendingTest}
              isDisabled={!testMessage.trim()}
            >
              {t("sendTest")}
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>

      <Modal
        isOpen={isDisconnectModalOpen}
        onClose={onDisconnectModalClose}
      >
        <ModalContent>
          <ModalHeader>{t("disconnect")}</ModalHeader>
          <ModalBody>
            <p className={telegramCopyClass}>
              {t("telegramDisconnectConfirm")}
            </p>
          </ModalBody>
          <ModalFooter>
            <Button variant="light" onPress={onDisconnectModalClose}>
              {tCommon("cancel")}
            </Button>
            <Button
              color="danger"
              onPress={disconnectTelegram}
              isLoading={isLoading}
            >
              {t("disconnect")}
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </div>
  );
}
