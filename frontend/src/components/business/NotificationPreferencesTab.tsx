"use client";

import React, { useState, useEffect, useCallback, useRef } from "react";
import { Switch, Slider, Divider, Button } from "@nextui-org/react";
import { BellRing, Mail, Send, Volume2 } from "lucide-react";
import { axiosInstance } from "@/api/tools/instance";
import {
  fetchOperationalAlertSettings,
  updateOperationalAlertSettings,
  type AlertTypeSetting,
  type OperationalAlertPreferenceType,
  type OperationalAlertSettings,
} from "@/api/operationalAlerts";
import { pluginAPI } from "@/api/plugins";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { useToast } from "@/contexts/ToastContext";
import { createOperationalAlertSoundEngine } from "@/utils/operationalAlertSound";
import {
  getChatSoundPrefs,
  saveChatSoundPrefs,
  type ChatSoundPrefs,
} from "@/utils/chatSoundPrefs";

// Module scope: one shared engine instance for previewing alert sounds from
// the settings UI. Mirrors the pattern used by the live alert engine — a
// single AudioContext is reused across preview clicks instead of a new one
// per button press.
const previewSoundEngine = createOperationalAlertSoundEngine();

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface EmailNotificationPreferences {
  email_enabled?: boolean;
  transactional_enabled: boolean;
  reports_enabled: boolean;
  news_enabled: boolean;
  updates_enabled: boolean;
  security_enabled: boolean;
  statistics_enabled?: boolean;
}

interface NotificationPreferencesTabProps {
  businessId: number;
  telegramConnected?: boolean;
}

const OPERATIONAL_ALERT_TYPES: OperationalAlertPreferenceType[] = [
  "order_new",
  "kitchen_order_ready",
  "reservation_new",
  "reservation_approval",
  "delivery_new",
  "bill_new",
  "payment_requested",
  "payment_received",
  "payment_refund_review",
  "service_call",
  "ai_takeover",
];

const DEFAULT_OPERATIONAL_EVENT_SETTINGS: Record<
  OperationalAlertPreferenceType,
  AlertTypeSetting
> = {
  order_new: { enabled: true, repeating: true, priority: "urgent" },
  kitchen_order_ready: { enabled: true, repeating: true, priority: "urgent" },
  reservation_new: { enabled: true, repeating: false, priority: "normal" },
  reservation_approval: { enabled: true, repeating: true, priority: "urgent" },
  delivery_new: { enabled: true, repeating: true, priority: "urgent" },
  bill_new: { enabled: true, repeating: false, priority: "normal" },
  payment_requested: { enabled: true, repeating: false, priority: "normal" },
  payment_received: { enabled: true, repeating: false, priority: "normal" },
  payment_refund_review: { enabled: true, repeating: false, priority: "high" },
  service_call: { enabled: true, repeating: true, priority: "urgent" },
  ai_takeover: { enabled: true, repeating: true, priority: "urgent" },
};

const scalarSliderValue = (value: number | number[]): number =>
  Array.isArray(value) ? value[0] : value;

// Dinner-safe fallback when the API omits repeat_interval_seconds (#267).
// Keep in sync with backend Min/DefaultBusinessAlertRepeatIntervalSeconds.
const DEFAULT_REPEAT_INTERVAL_SECONDS = 30;
const MIN_REPEAT_INTERVAL_SECONDS = 30;
const MAX_REPEAT_INTERVAL_SECONDS = 60;

const clampRepeatIntervalSeconds = (value: unknown): number =>
  Math.min(
    MAX_REPEAT_INTERVAL_SECONDS,
    Math.max(
      MIN_REPEAT_INTERVAL_SECONDS,
      Number(value) || DEFAULT_REPEAT_INTERVAL_SECONDS,
    ),
  );

const normalizeOperationalSettings = (
  settings: OperationalAlertSettings,
): OperationalAlertSettings => ({
  ...settings,
  volume: Math.min(1, Math.max(0, Number(settings.volume) || 0)),
  repeat_interval_seconds: clampRepeatIntervalSeconds(
    settings.repeat_interval_seconds,
  ),
  event_settings: OPERATIONAL_ALERT_TYPES.reduce(
    (acc, alertType) => ({
      ...acc,
      [alertType]: {
        ...DEFAULT_OPERATIONAL_EVENT_SETTINGS[alertType],
        ...(settings.event_settings?.[alertType] ?? {}),
      },
    }),
    {} as Record<OperationalAlertPreferenceType, AlertTypeSetting>,
  ),
});

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

// Stable default shapes at module scope: identity is constant across renders, so
// they can seed state/refs and back per-field reverts without churn or pulling
// into effect dependency lists.
const DEFAULT_EMAIL_PREFS: EmailNotificationPreferences = {
  transactional_enabled: true,
  reports_enabled: false,
  news_enabled: false,
  updates_enabled: true,
  security_enabled: true,
};

const DEFAULT_TELEGRAM_TOGGLES = {
  order_created: true,
  payment_received: true,
  reservation_created: true,
  reservation_status_changed: false,
  inventory_low_stock: false,
};

export default function NotificationPreferencesTab({
  businessId,
  telegramConnected = false,
}: NotificationPreferencesTabProps) {
  const { locale } = useSimpleLocale();
  const { showSuccess, showError } = useToast();

  const tString = useCallback(
    (key: string): string => {
      const fullKey = `businessSettings.${key}`;
      const result = getTranslation(fullKey, locale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [locale],
  );

  // -------------------------------------------------------------------------
  // Section 1 — Email Notifications
  // -------------------------------------------------------------------------

  const [emailPrefs, setEmailPrefs] =
    useState<EmailNotificationPreferences>(DEFAULT_EMAIL_PREFS);
  // Authoritative latest-intended prefs, mirrored synchronously so that
  // concurrent toggles always build their payload from current state rather
  // than a stale render-time closure (F9). Mirrors `operationalSavedRef`.
  const emailPrefsRef =
    useRef<EmailNotificationPreferences>(DEFAULT_EMAIL_PREFS);
  const [emailLoading, setEmailLoading] = useState(true);
  const [emailSaving, setEmailSaving] = useState(false);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const res = await axiosInstance.get<{
          address: string;
          preferences: EmailNotificationPreferences;
        }>("/inside/settings/notifications");
        if (!cancelled) {
          emailPrefsRef.current = res.data.preferences;
          setEmailPrefs(res.data.preferences);
        }
      } catch {
        // Non-fatal — keep defaults
      } finally {
        if (!cancelled) setEmailLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  const handleEmailToggle = async (
    field: keyof EmailNotificationPreferences,
    value: boolean,
  ) => {
    // Build the payload from the authoritative ref (always current), flip only
    // `field`, and POST that object. A captured `emailPrefs` snapshot would let
    // a concurrent toggle on another field get clobbered on revert (F9).
    const posted = { ...emailPrefsRef.current, [field]: value };
    emailPrefsRef.current = posted;
    setEmailPrefs(posted);
    setEmailSaving(true);
    try {
      await axiosInstance.put("/inside/settings/notifications", {
        preferences: posted,
      });
      showSuccess(
        tString("messages.updateSuccess"),
        tString("messages.updateSuccessDescription"),
        2000,
      );
    } catch {
      // Revert only the field that failed; leave any concurrent toggles intact.
      emailPrefsRef.current = { ...emailPrefsRef.current, [field]: !value };
      setEmailPrefs(emailPrefsRef.current);
      showError(
        tString("messages.updateFailedTitle"),
        tString("messages.updateFailed"),
        4000,
      );
    } finally {
      setEmailSaving(false);
    }
  };

  // -------------------------------------------------------------------------
  // Section 2 — Operational Alerts
  // -------------------------------------------------------------------------

  const [operationalSettings, setOperationalSettings] =
    useState<OperationalAlertSettings | null>(null);
  const operationalSavedRef = useRef<OperationalAlertSettings | null>(null);
  const [operationalLoading, setOperationalLoading] = useState(true);
  const [operationalSaving, setOperationalSaving] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setOperationalLoading(true);
    (async () => {
      try {
        const settings = normalizeOperationalSettings(
          await fetchOperationalAlertSettings(businessId),
        );
        if (cancelled) return;
        operationalSavedRef.current = settings;
        setOperationalSettings(settings);
      } catch {
        if (!cancelled) {
          showError(
            tString("messages.updateFailedTitle"),
            tString("messages.updateFailed"),
            4000,
          );
        }
      } finally {
        if (!cancelled) setOperationalLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [businessId, showError, tString]);

  const persistOperationalSettings = async (
    next: OperationalAlertSettings,
    previous: OperationalAlertSettings | null = operationalSavedRef.current,
  ) => {
    const normalized = normalizeOperationalSettings(next);
    setOperationalSettings(normalized);
    setOperationalSaving(true);
    try {
      const saved = normalizeOperationalSettings(
        await updateOperationalAlertSettings(businessId, normalized),
      );
      operationalSavedRef.current = saved;
      setOperationalSettings(saved);
      showSuccess(
        tString("messages.updateSuccess"),
        tString("messages.updateSuccessDescription"),
        2000,
      );
    } catch {
      if (previous) {
        setOperationalSettings(previous);
      }
      showError(
        tString("messages.updateFailedTitle"),
        tString("messages.updateFailed"),
        4000,
      );
    } finally {
      setOperationalSaving(false);
    }
  };

  const handleOperationalToggle = (
    field: "enabled" | "sound_enabled" | "browser_notifications_enabled",
    value: boolean,
  ) => {
    if (!operationalSettings) return;
    void persistOperationalSettings({
      ...operationalSettings,
      [field]: value,
    });
  };

  const handleOperationalNumberDraft = (
    field: "volume" | "repeat_interval_seconds",
    value: number | number[],
  ) => {
    const raw = scalarSliderValue(value);
    const nextValue =
      field === "repeat_interval_seconds"
        ? clampRepeatIntervalSeconds(raw)
        : raw;
    setOperationalSettings((current) =>
      current
        ? {
            ...current,
            [field]: nextValue,
          }
        : current,
    );
  };

  const handleOperationalNumberCommit = (
    field: "volume" | "repeat_interval_seconds",
    value: number | number[],
  ) => {
    if (!operationalSettings) return;
    const raw = scalarSliderValue(value);
    const nextValue =
      field === "repeat_interval_seconds"
        ? clampRepeatIntervalSeconds(raw)
        : raw;
    void persistOperationalSettings({
      ...operationalSettings,
      [field]: nextValue,
    });
  };

  const handleOperationalEventToggle = (
    alertType: OperationalAlertPreferenceType,
    field: "enabled" | "repeating",
    value: boolean,
  ) => {
    if (!operationalSettings) return;
    const current = operationalSettings.event_settings[alertType];
    void persistOperationalSettings({
      ...operationalSettings,
      event_settings: {
        ...operationalSettings.event_settings,
        [alertType]: {
          ...current,
          [field]: value,
        },
      },
    });
  };

  // -------------------------------------------------------------------------
  // Section 2b — Chat sounds (this device). Client-side localStorage only —
  // no backend settings surface for v1. Announcements default ON (rare and
  // important); regular channel/DM traffic defaults OFF (noisy during
  // service) and is a per-device opt-in.
  // -------------------------------------------------------------------------

  const [chatSoundPrefs, setChatSoundPrefs] = useState<ChatSoundPrefs>(() =>
    getChatSoundPrefs(),
  );

  const handleChatSoundToggle = (
    field: keyof ChatSoundPrefs,
    value: boolean,
  ) => {
    saveChatSoundPrefs({ [field]: value });
    setChatSoundPrefs(getChatSoundPrefs());
  };

  // -------------------------------------------------------------------------
  // Section 3 — Telegram Notifications (persisted to plugin config blob)
  // -------------------------------------------------------------------------

  const [telegramPluginId, setTelegramPluginId] = useState<number | null>(null);
  const [telegramToggles, setTelegramToggles] = useState(
    DEFAULT_TELEGRAM_TOGGLES,
  );
  // Authoritative latest-intended toggles, mirrored synchronously so concurrent
  // flips build their payload from current state, not a stale closure (F9).
  const telegramTogglesRef = useRef<typeof DEFAULT_TELEGRAM_TOGGLES>(
    DEFAULT_TELEGRAM_TOGGLES,
  );
  const [telegramConfig, setTelegramConfig] = useState<Record<
    string,
    any
  > | null>(null);
  const telegramConfigRef = useRef<Record<string, any> | null>(null);
  const [telegramSaving, setTelegramSaving] = useState(false);

  useEffect(() => {
    if (!telegramConnected) return;
    let cancelled = false;
    (async () => {
      try {
        const bpRes = await pluginAPI.business.getBusinessPlugins(
          String(businessId),
        );
        const bp = (bpRes.plugins || []).find(
          (p) => p.plugin?.name?.toLowerCase() === "telegram" && p.is_enabled,
        );
        if (!bp || cancelled) return;
        setTelegramPluginId(bp.plugin_id);
        const configRes = await pluginAPI.business.getPluginConfig(
          String(businessId),
          bp.plugin_id,
        );
        if (cancelled) return;
        const cfg = configRes.config || {};
        telegramConfigRef.current = cfg;
        setTelegramConfig(cfg);
        const notifs = (cfg.notifications || {}) as Record<string, boolean>;
        const loaded = {
          order_created:
            notifs.order_created ?? DEFAULT_TELEGRAM_TOGGLES.order_created,
          payment_received:
            notifs.payment_received ??
            DEFAULT_TELEGRAM_TOGGLES.payment_received,
          reservation_created:
            notifs.reservation_created ??
            DEFAULT_TELEGRAM_TOGGLES.reservation_created,
          reservation_status_changed:
            notifs.reservation_status_changed ??
            DEFAULT_TELEGRAM_TOGGLES.reservation_status_changed,
          inventory_low_stock:
            notifs.inventory_low_stock ??
            DEFAULT_TELEGRAM_TOGGLES.inventory_low_stock,
        };
        telegramTogglesRef.current = loaded;
        setTelegramToggles(loaded);
      } catch {
        // Non-fatal — keep defaults
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [telegramConnected, businessId]);

  const handleTelegramToggle = async (
    field: keyof typeof telegramToggles,
    value: boolean,
  ) => {
    // Flip only `field`, derived from the authoritative ref (always current),
    // and persist that object. A captured snapshot would clobber a concurrent
    // toggle on another field when reverting on error (F9).
    const updated = { ...telegramTogglesRef.current, [field]: value };
    telegramTogglesRef.current = updated;
    setTelegramToggles(updated);
    if (telegramPluginId === null || telegramConfigRef.current === null) return;
    setTelegramSaving(true);
    try {
      const newConfig = {
        ...telegramConfigRef.current,
        notifications: updated,
      };
      await pluginAPI.business.updatePluginConfig(
        String(businessId),
        telegramPluginId,
        { config: newConfig },
      );
      telegramConfigRef.current = newConfig;
      setTelegramConfig(newConfig);
    } catch {
      // Revert only the field that failed; leave any concurrent toggles intact.
      telegramTogglesRef.current = {
        ...telegramTogglesRef.current,
        [field]: !value,
      };
      setTelegramToggles(telegramTogglesRef.current);
      showError(
        tString("messages.updateFailedTitle"),
        tString("messages.updateFailed"),
        4000,
      );
    } finally {
      setTelegramSaving(false);
    }
  };

  // -------------------------------------------------------------------------
  // Render helpers
  // -------------------------------------------------------------------------

  const sectionIconClass =
    "p-2 rounded-xl bg-brand/10 border border-brand/20 shadow-sm shadow-brand/10";
  const sectionTitleClass = "text-lg font-semibold text-ink-950";
  const sectionDescriptionClass = "text-sm text-ink-600";
  const rowClass =
    "flex items-center justify-between gap-4 rounded-2xl border border-warm-200/70 bg-white/75 px-3 py-3 shadow-sm shadow-warm-900/5";

  const ScopeCaption = ({
    scope,
  }: {
    scope: "account" | "device" | "business" | "browser";
  }) => (
    <div
      className="mt-2 flex flex-wrap items-start gap-2"
      data-testid={`notif-scope-${scope}`}
    >
      <span
        className="inline-flex items-center rounded-md border border-brand/25 bg-brand/10 px-2 py-0.5 text-[11px] font-semibold uppercase tracking-[0.12em] text-brand"
        data-testid={`notif-scope-badge-${scope}`}
      >
        {tString(`notifications.scope.${scope}.label`)}
      </span>
      <p className="min-w-0 flex-1 text-xs leading-5 text-ink-500">
        {tString(`notifications.scope.${scope}.helper`)}
      </p>
    </div>
  );

  // -------------------------------------------------------------------------
  // Render
  // -------------------------------------------------------------------------

  return (
    <div className="space-y-6">
      {/* #268: surface that this screen mixes persistence scopes up front */}
      <div
        className="rounded-2xl border border-warm-200 bg-warm-50/80 px-4 py-3"
        data-testid="notif-scope-legend"
      >
        <p className="text-sm font-semibold text-ink-950">
          {tString("notifications.scopeLegendTitle")}
        </p>
        <p className="mt-1 text-xs leading-5 text-ink-600">
          {tString("notifications.scopeLegendDescription")}
        </p>
        <p
          className="mt-2 text-xs leading-5 text-ink-500"
          data-testid="notif-save-model-hint"
        >
          {tString("notifications.saveModelHint")}
        </p>
      </div>

      {/* ================================================================== */}
      {/* Section 1 — Email Notifications (account scope)                     */}
      {/* ================================================================== */}
      <section
        className="rounded-3xl border border-warm-200/80 bg-gradient-to-br from-white via-warm-50/75 to-brand/5 p-5 shadow-sm shadow-warm-900/5"
        data-storage-scope="account"
      >
        <div className="flex items-center gap-3 mb-4">
          <div className={sectionIconClass}>
            <Mail className="w-5 h-5 text-brand" />
          </div>
          <div>
            <h3 className={sectionTitleClass}>
              {tString("notifications.emailSection")}
            </h3>
            <p className={sectionDescriptionClass}>
              {tString("notifications.emailDescription")}
            </p>
            <ScopeCaption scope="account" />
          </div>
        </div>

        {emailLoading ? (
          <div className="space-y-4 animate-pulse">
            {[...Array(2)].map((_, i) => (
              <div key={i} className="h-12 rounded-2xl bg-warm-100/80" />
            ))}
          </div>
        ) : (
          <div className="space-y-4">
            {/* news/updates/security/statistics are stored but never read by the backend, so they are not offered. */}
            {(
              [
                {
                  field: "transactional_enabled" as const,
                  label: tString("notifications.ordersAndPayments"),
                  desc: tString("notifications.ordersAndPaymentsDesc"),
                },
                {
                  field: "reports_enabled" as const,
                  label: tString("notifications.reportsAndAnalytics"),
                  desc: tString("notifications.reportsAndAnalyticsDesc"),
                },
              ] as {
                field: keyof EmailNotificationPreferences;
                label: string;
                desc: string;
              }[]
            ).map(({ field, label, desc }) => (
              <div key={field} className={rowClass}>
                <div className="min-w-0">
                  <p className="text-sm font-semibold text-ink-950">{label}</p>
                  <p className="text-xs text-warm-600">{desc}</p>
                </div>
                <Switch
                  isSelected={Boolean(emailPrefs[field])}
                  isDisabled={emailSaving}
                  onValueChange={(val) => handleEmailToggle(field, val)}
                  size="sm"
                  aria-label={label}
                />
              </div>
            ))}
          </div>
        )}
      </section>

      <Divider className="bg-warm-200/70" />

      {/* ================================================================== */}
      {/* Section 2 — Operational Alerts (business scope)                     */}
      {/* ================================================================== */}
      <section
        className="rounded-3xl border border-warm-200/80 bg-white/80 p-5 shadow-sm shadow-warm-900/5"
        data-storage-scope="business"
      >
        <div className="flex items-center gap-3 mb-4">
          <div className={sectionIconClass}>
            <BellRing className="w-5 h-5 text-brand" />
          </div>
          <div>
            <h3 className={sectionTitleClass}>
              {tString("notifications.operationalSection")}
            </h3>
            <p className={sectionDescriptionClass}>
              {tString("notifications.operationalDescription")}
            </p>
            <ScopeCaption scope="business" />
          </div>
        </div>

        {operationalLoading ? (
          <div className="space-y-4 animate-pulse">
            {[...Array(4)].map((_, i) => (
              <div key={i} className="h-12 rounded-2xl bg-warm-100/80" />
            ))}
          </div>
        ) : operationalSettings ? (
          <div className="space-y-6">
            <div className="grid gap-4 md:grid-cols-3">
              {(
                [
                  {
                    field: "enabled" as const,
                    label: tString("notifications.operationalEnabled"),
                    desc: tString("notifications.operationalEnabledDesc"),
                  },
                  {
                    field: "sound_enabled" as const,
                    label: tString("notifications.operationalSoundEnabled"),
                    desc: tString("notifications.operationalSoundEnabledDesc"),
                  },
                  {
                    field: "browser_notifications_enabled" as const,
                    label: tString("notifications.operationalBrowserEnabled"),
                    desc: tString(
                      "notifications.operationalBrowserEnabledDesc",
                    ),
                  },
                ] as {
                  field:
                    | "enabled"
                    | "sound_enabled"
                    | "browser_notifications_enabled";
                  label: string;
                  desc: string;
                }[]
              ).map(({ field, label, desc }) => (
                <div
                  key={field}
                  className="flex flex-col gap-2 rounded-2xl border border-warm-200/70 bg-warm-50/80 px-3 py-3 shadow-sm shadow-warm-900/5"
                  data-storage-scope={
                    field === "browser_notifications_enabled"
                      ? "browser"
                      : "business"
                  }
                >
                  <div className="flex items-center justify-between gap-3">
                    <div className="min-w-0">
                      <p className="text-sm font-semibold text-ink-950">
                        {label}
                      </p>
                      <p className="text-xs text-warm-600">{desc}</p>
                    </div>
                    <Switch
                      isSelected={Boolean(operationalSettings[field])}
                      isDisabled={operationalSaving}
                      onValueChange={(val) =>
                        handleOperationalToggle(field, val)
                      }
                      size="sm"
                      aria-label={label}
                    />
                  </div>
                  {field === "browser_notifications_enabled" ? (
                    <ScopeCaption scope="browser" />
                  ) : null}
                  {field === "sound_enabled" ? (
                    <p
                      className="text-xs text-ink-500"
                      data-testid="notif-sound-opt-in-hint"
                    >
                      {tString("notifications.operationalSoundOptInHint")}
                    </p>
                  ) : null}
                </div>
              ))}
            </div>

            <div className="grid gap-6 md:grid-cols-2">
              <div className="space-y-2">
                <div className="flex items-center justify-between gap-3">
                  <p className="text-sm font-semibold text-ink-950">
                    {tString("notifications.operationalVolume")}
                  </p>
                  <p className="text-xs font-medium text-warm-600">
                    {Math.round(operationalSettings.volume * 100)}%
                  </p>
                </div>
                <Slider
                  aria-label={tString("notifications.operationalVolume")}
                  minValue={0}
                  maxValue={1}
                  step={0.05}
                  value={operationalSettings.volume}
                  onChange={(val) =>
                    handleOperationalNumberDraft("volume", val)
                  }
                  onChangeEnd={(val) =>
                    handleOperationalNumberCommit("volume", val)
                  }
                  isDisabled={
                    operationalSaving ||
                    !operationalSettings.enabled ||
                    !operationalSettings.sound_enabled
                  }
                  className="max-w-md"
                  size="sm"
                />
              </div>

              <div className="space-y-2">
                <div className="flex items-center justify-between gap-3">
                  <p className="text-sm font-semibold text-ink-950">
                    {tString("notifications.operationalRepeatInterval")}
                  </p>
                  <p
                    className="text-xs font-medium text-warm-600"
                    data-testid="notif-repeat-interval-value"
                    data-seconds={String(
                      operationalSettings.repeat_interval_seconds,
                    )}
                  >
                    {tString(
                      "notifications.operationalRepeatIntervalValue",
                    ).replace(
                      "{{seconds}}",
                      String(operationalSettings.repeat_interval_seconds),
                    )}
                  </p>
                </div>
                <Slider
                  aria-label={tString(
                    "notifications.operationalRepeatInterval",
                  )}
                  minValue={MIN_REPEAT_INTERVAL_SECONDS}
                  maxValue={MAX_REPEAT_INTERVAL_SECONDS}
                  step={1}
                  value={operationalSettings.repeat_interval_seconds}
                  onChange={(val) =>
                    handleOperationalNumberDraft("repeat_interval_seconds", val)
                  }
                  onChangeEnd={(val) =>
                    handleOperationalNumberCommit(
                      "repeat_interval_seconds",
                      val,
                    )
                  }
                  isDisabled={
                    operationalSaving ||
                    !operationalSettings.enabled ||
                    !operationalSettings.sound_enabled
                  }
                  className="max-w-md"
                  size="sm"
                />
                <p
                  className="text-xs text-ink-500"
                  data-testid="notif-repeat-interval-floor-hint"
                >
                  {tString("notifications.operationalRepeatIntervalFloorHint")}
                </p>
              </div>
            </div>

            <div className="space-y-3">
              <div>
                <p className="text-sm font-semibold text-ink-950">
                  {tString("notifications.operationalEventsTitle")}
                </p>
                <p className="text-xs text-warm-600">
                  {tString("notifications.operationalEventsDescription")}
                </p>
              </div>

              <div className="space-y-2">
                {OPERATIONAL_ALERT_TYPES.map((alertType) => {
                  const eventSetting =
                    operationalSettings.event_settings[alertType];
                  const label = tString(
                    `notifications.operationalTypes.${alertType}`,
                  );
                  const desc = tString(
                    `notifications.operationalTypeDescriptions.${alertType}`,
                  );

                  return (
                    <div
                      key={alertType}
                      className="flex flex-col gap-3 rounded-2xl border border-warm-200/70 bg-white/75 px-3 py-3 shadow-sm shadow-warm-900/5 sm:flex-row sm:items-center sm:justify-between"
                    >
                      <div className="min-w-0">
                        <p className="text-sm font-semibold text-ink-950">
                          {label}
                        </p>
                        <p className="text-xs text-warm-600">{desc}</p>
                      </div>
                      <div className="flex items-center gap-5">
                        <Button
                          size="sm"
                          variant="light"
                          isIconOnly
                          aria-label={tString(
                            "notifications.operationalPreviewSound",
                          ).replace("{{type}}", label)}
                          onPress={() => {
                            void previewSoundEngine.playOnce({
                              alertTypes: [alertType],
                              volume: operationalSettings.volume,
                            });
                          }}
                        >
                          <Volume2 className="h-4 w-4 text-ink-600" />
                        </Button>
                        <label className="flex items-center gap-2 text-xs font-medium text-ink-700">
                          <span>
                            {tString("notifications.operationalEventEnabled")}
                          </span>
                          <Switch
                            isSelected={eventSetting.enabled}
                            isDisabled={
                              operationalSaving || !operationalSettings.enabled
                            }
                            onValueChange={(val) =>
                              handleOperationalEventToggle(
                                alertType,
                                "enabled",
                                val,
                              )
                            }
                            size="sm"
                            aria-label={`${label} ${tString("notifications.operationalEventEnabled")}`}
                          />
                        </label>
                        <label className="flex items-center gap-2 text-xs font-medium text-ink-700">
                          <span>
                            {tString("notifications.operationalEventRepeats")}
                          </span>
                          <Switch
                            isSelected={eventSetting.repeating}
                            isDisabled={
                              operationalSaving ||
                              !operationalSettings.enabled ||
                              !eventSetting.enabled
                            }
                            onValueChange={(val) =>
                              handleOperationalEventToggle(
                                alertType,
                                "repeating",
                                val,
                              )
                            }
                            size="sm"
                            aria-label={`${label} ${tString("notifications.operationalEventRepeats")}`}
                          />
                        </label>
                      </div>
                    </div>
                  );
                })}
              </div>
            </div>

          </div>
        ) : null}
      </section>

      <Divider className="bg-warm-200/70" />

      {/* ================================================================== */}
      {/* Section 2b — Chat sounds (device / localStorage scope)               */}
      {/* ================================================================== */}
      <section
        className="rounded-3xl border border-warm-200/80 bg-white/80 p-5 shadow-sm shadow-warm-900/5"
        data-storage-scope="device"
      >
        <div className="flex items-center gap-3 mb-4">
          <div className={sectionIconClass}>
            <Volume2 className="w-5 h-5 text-brand" />
          </div>
          <div>
            <h3 className={sectionTitleClass}>
              {tString("notifications.chatSoundsTitle")}
            </h3>
            <p className={sectionDescriptionClass}>
              {tString("notifications.chatSoundsDescription")}
            </p>
            <ScopeCaption scope="device" />
          </div>
        </div>
        <div className="space-y-2">
          {(
            [
              {
                field: "announcementSound" as const,
                label: tString("notifications.chatAnnouncementSound"),
                desc: tString("notifications.chatAnnouncementSoundDesc"),
              },
              {
                field: "messageSound" as const,
                label: tString("notifications.chatMessageSound"),
                desc: tString("notifications.chatMessageSoundDesc"),
              },
            ] as {
              field: keyof ChatSoundPrefs;
              label: string;
              desc: string;
            }[]
          ).map(({ field, label, desc }) => (
            <div key={field} className={rowClass}>
              <div className="min-w-0">
                <p className="text-sm font-semibold text-ink-950">{label}</p>
                <p className="text-xs text-warm-600">{desc}</p>
              </div>
              <Switch
                isSelected={chatSoundPrefs[field]}
                onValueChange={(val) => handleChatSoundToggle(field, val)}
                size="sm"
                aria-label={label}
              />
            </div>
          ))}
        </div>
      </section>

      <Divider className="bg-warm-200/70" />

      {/* ================================================================== */}
      {/* Section 3 — Telegram Notifications (business plugin scope)          */}
      {/* ================================================================== */}
      <section
        className="rounded-3xl border border-warm-200/80 bg-gradient-to-br from-white via-warm-50/75 to-emerald-50/40 p-5 shadow-sm shadow-warm-900/5"
        data-storage-scope="business"
      >
        <div className="flex items-center gap-3 mb-4">
          <div className={sectionIconClass}>
            <Send className="w-5 h-5 text-brand" />
          </div>
          <div>
            <h3 className={sectionTitleClass}>
              {tString("notifications.telegramSection")}
            </h3>
            <p className={sectionDescriptionClass}>
              {tString("notifications.telegramDescription")}
            </p>
            <ScopeCaption scope="business" />
          </div>
        </div>

        {!telegramConnected ? (
          <p className="rounded-2xl border border-dashed border-warm-300 bg-warm-50/70 p-4 text-sm text-warm-600 italic">
            {tString("notifications.telegramNotConnected")}
          </p>
        ) : (
          <div className="space-y-4">
            {(
              [
                {
                  field: "order_created" as const,
                  label: tString("notifications.orderCreated"),
                },
                {
                  field: "payment_received" as const,
                  label: tString("notifications.paymentReceived"),
                },
                {
                  field: "reservation_created" as const,
                  label: tString("notifications.reservationCreated"),
                },
                {
                  field: "reservation_status_changed" as const,
                  label: tString("notifications.reservationStatusChanged"),
                },
                {
                  field: "inventory_low_stock" as const,
                  label: tString("notifications.inventoryLowStock"),
                },
              ] as {
                field: keyof typeof telegramToggles;
                label: string;
              }[]
            ).map(({ field, label }) => (
              <div key={field} className={rowClass}>
                <p className="text-sm font-medium text-ink-800">{label}</p>
                <Switch
                  isSelected={telegramToggles[field]}
                  isDisabled={telegramSaving}
                  onValueChange={(val) => handleTelegramToggle(field, val)}
                  size="sm"
                  aria-label={label}
                />
              </div>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}
