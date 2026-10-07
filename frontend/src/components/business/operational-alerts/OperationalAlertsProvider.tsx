"use client";

import React, {
  createContext,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  claimOperationalAlert,
  fetchOperationalAlertSettings,
  fetchOperationalAlerts,
  OperationalAlertConflictError,
  resolveOperationalAlert,
  type OperationalAlert,
  type OperationalAlertPreferenceType,
  type OperationalAlertResourceType,
  type OperationalAlertSettings,
  type OperationalAlertStatus,
  type OperationalAlertType,
} from "@/api/operationalAlerts";
import { queryKeys } from "@/api/queryKeys";
import { useToast } from "@/contexts/ToastContext";
import { useSSEEvents, type SSEEvent } from "@/hooks/useSSEEvents";
import { useSoundLeadership } from "@/hooks/useSoundLeadership";
import {
  createOperationalAlertSoundEngine,
  getLocalAlertSoundOverrides,
} from "@/utils/operationalAlertSound";
import { getChatSoundPrefs } from "@/utils/chatSoundPrefs";
import { useSimpleLocale } from "@/i18n/SimpleTranslationProvider";
import { localizeAlert, localizeClaimConflict } from "./operationalAlertCopy";
import { isStaleServiceCall } from "./serviceCallTtl";
import type { Locale } from "@/i18n/localeRegistry";

const ACTIVE_ALERT_FILTERS = {
  status: ["open", "claimed"] as OperationalAlertStatus[],
};

const EMPTY_COUNTS = {
  bills: 0,
  kitchen: 0,
  reservations: 0,
  delivery: 0,
  tables: 0,
} as const;

const ALERT_COUNT_BUCKETS: Partial<
  Record<OperationalAlertType, keyof OperationalAlertCounts>
> = {
  order_new: "bills",
  bill_new: "bills",
  kitchen_order_ready: "kitchen",
  reservation_new: "reservations",
  reservation_approval: "reservations",
  delivery_new: "delivery",
  service_call: "tables",
};

type OperationalAlertCounts = Record<
  "bills" | "kitchen" | "reservations" | "delivery" | "tables",
  number
>;

export interface OperationalAlertsContextValue {
  alerts: OperationalAlert[];
  settings: OperationalAlertSettings | null;
  counts: OperationalAlertCounts;
  audioBlocked: boolean;
  enableSound: () => Promise<void>;
  claimAlert: (
    alertId: number,
    source: string,
    staffId?: number,
  ) => Promise<void>;
  resolveAlert: (alertId: number, reason: string) => Promise<void>;
  claimByResource: (
    resourceType: OperationalAlertResourceType,
    resourceId: number,
    source: string,
  ) => Promise<void>;
  getAlertForResource: (
    resourceType: OperationalAlertResourceType,
    resourceId: number,
  ) => OperationalAlert | null;
}

export const OperationalAlertsContext = createContext<
  OperationalAlertsContextValue | undefined
>(undefined);

interface OperationalAlertsProviderProps {
  businessId: number;
  enabled: boolean;
  children: React.ReactNode;
}

const isActiveStatus = (status: unknown): status is "open" | "claimed" =>
  status === "open" || status === "claimed";

const isTerminalStatus = (
  status: unknown,
): status is "resolved" | "dismissed" =>
  status === "resolved" || status === "dismissed";

const isSnoozed = (alert: OperationalAlert): boolean => {
  if (!alert.snoozed_until) return false;
  const until = Date.parse(alert.snoozed_until);
  return Number.isFinite(until) && until > Date.now();
};

const extractAlertFromEvent = (
  data: Record<string, unknown>,
): OperationalAlert | null => {
  if (typeof data.id === "number" && Number.isFinite(data.id)) {
    return data as unknown as OperationalAlert;
  }

  const nestedAlert = data.alert;
  if (
    nestedAlert &&
    typeof nestedAlert === "object" &&
    !Array.isArray(nestedAlert) &&
    typeof (nestedAlert as { id?: unknown }).id === "number"
  ) {
    return nestedAlert as OperationalAlert;
  }

  return null;
};

const alertTypeRepeats = (
  settings: OperationalAlertSettings,
  alertType: OperationalAlertType,
): boolean => {
  const alertTypeSetting =
    settings.event_settings?.[alertType as OperationalAlertPreferenceType];

  if (!alertTypeSetting?.enabled) return false;
  if (typeof alertTypeSetting.repeating === "boolean") {
    return alertTypeSetting.repeating;
  }
  return Number(alertTypeSetting.repeat_interval_seconds ?? 0) > 0;
};

/**
 * Whether visuals (toast) may show for this alert type. Decision (2026-07-05):
 * toasts respect the per-type `enabled` setting, matching sounds and browser
 * notifications — an operator who disabled a type in notification preferences
 * expects NO pings of any kind for it. `null` settings (still loading) fail
 * open so the first alert of a session is never dropped. The alerts list /
 * badges / counts still update regardless — `enabled` gates interruptions,
 * not data.
 */
const alertTypeAllowsToast = (
  settings: OperationalAlertSettings | null,
  alertType: OperationalAlertType,
): boolean => {
  if (!settings) return true;
  if (!settings.enabled) return false;
  return (
    settings.event_settings?.[alertType as OperationalAlertPreferenceType]
      ?.enabled !== false
  );
};

const alertTypeAllowsSound = (
  settings: OperationalAlertSettings,
  alertType: OperationalAlertType,
): boolean => {
  const alertTypeSetting =
    settings.event_settings?.[alertType as OperationalAlertPreferenceType];
  if (!alertTypeSetting?.enabled) return false;
  if (
    alertTypeSetting.sound_enabled === false ||
    alertTypeSetting.sound === false
  ) {
    return false;
  }
  return true;
};

const alertTypeAllowsBrowserNotification = (
  settings: OperationalAlertSettings,
  alertType: OperationalAlertType,
): boolean => {
  const alertTypeSetting =
    settings.event_settings?.[alertType as OperationalAlertPreferenceType];
  if (!alertTypeSetting?.enabled) return false;
  if (
    alertTypeSetting.browser_notifications_enabled === false ||
    alertTypeSetting.browser_notification === false
  ) {
    return false;
  }
  return true;
};

const showAlertBrowserNotification = (
  alert: OperationalAlert,
  settings: OperationalAlertSettings | null,
  locale: Locale,
): void => {
  if (
    !settings?.enabled ||
    !settings.browser_notifications_enabled ||
    !alertTypeAllowsBrowserNotification(settings, alert.alert_type) ||
    typeof window === "undefined" ||
    !("Notification" in window) ||
    Notification.permission !== "granted"
  ) {
    return;
  }

  const localized = localizeAlert(alert, locale);
  new Notification(localized.title, {
    body: localized.body || undefined,
    icon: "/favicon.ico",
    badge: "/favicon.ico",
  });
};

const requestBrowserNotificationPermission = async (): Promise<void> => {
  if (
    typeof window === "undefined" ||
    !("Notification" in window) ||
    Notification.permission !== "default"
  ) {
    return;
  }
  await Notification.requestPermission();
};

const uniqueRepeatingAlertTypes = (
  alerts: OperationalAlert[],
  settings: OperationalAlertSettings | null,
): OperationalAlertType[] => {
  if (!settings?.enabled || !settings.sound_enabled) return [];

  const alertTypes = new Set<OperationalAlertType>();
  for (const alert of alerts) {
    if (
      alert.status === "open" &&
      !isSnoozed(alert) &&
      alertTypeRepeats(settings, alert.alert_type)
    ) {
      alertTypes.add(alert.alert_type);
    }
  }

  return Array.from(alertTypes);
};

export function OperationalAlertsProvider({
  businessId,
  enabled,
  children,
}: OperationalAlertsProviderProps) {
  const queryClient = useQueryClient();
  const toast = useToast();
  const { locale } = useSimpleLocale();
  const [alerts, setAlerts] = useState<OperationalAlert[]>([]);
  const [soundEngine] = useState(() => createOperationalAlertSoundEngine());
  const [audioBlocked, setAudioBlocked] = useState(
    () => soundEngine.getState().blocked,
  );
  const shouldFetch = enabled && businessId > 0;
  // Bumped when THIS tab acquires sound leadership (e.g. the previous leader
  // tab closed mid-alarm). The repeating-sound effect below depends on it so
  // the surviving tab re-evaluates open repeating alerts and takes over the
  // alarm instead of staying silent.
  const [leaderEpoch, setLeaderEpoch] = useState(0);
  const handleLeadershipAcquired = useCallback(
    () => setLeaderEpoch((epoch) => epoch + 1),
    [],
  );
  const isSoundLeader = useSoundLeadership(
    businessId,
    handleLeadershipAcquired,
  );

  const activeAlertsQueryKey = useMemo(
    () => queryKeys.alerts.active(String(businessId), ACTIVE_ALERT_FILTERS),
    [businessId],
  );
  const settingsQueryKey = useMemo(
    () => queryKeys.alerts.settings(String(businessId)),
    [businessId],
  );
  const recentAlertsQueryKey = useMemo(
    () => queryKeys.alerts.recent(String(businessId)),
    [businessId],
  );

  const activeAlertsQuery = useQuery({
    queryKey: activeAlertsQueryKey,
    queryFn: () => fetchOperationalAlerts(businessId, ACTIVE_ALERT_FILTERS),
    enabled: shouldFetch,
  });

  const settingsQuery = useQuery({
    queryKey: settingsQueryKey,
    queryFn: () => fetchOperationalAlertSettings(businessId),
    enabled: shouldFetch,
  });

  useEffect(() => {
    if (!shouldFetch) {
      setAlerts([]);
      return;
    }

    if (activeAlertsQuery.data?.alerts) {
      setAlerts(
        activeAlertsQuery.data.alerts.filter(
          (alert) => !isStaleServiceCall(alert),
        ),
      );
    }
  }, [activeAlertsQuery.data?.alerts, shouldFetch]);

  const settings = shouldFetch ? (settingsQuery.data ?? null) : null;

  const patchAlert = useCallback(
    (alert: OperationalAlert, eventType?: string) => {
      queryClient.setQueryData<{ alerts: OperationalAlert[] }>(
        recentAlertsQueryKey,
        (current) => {
          if (!current) return current;
          const existingIndex = current.alerts.findIndex(
            (currentAlert) => currentAlert.id === alert.id,
          );
          if (existingIndex === -1) {
            return { alerts: [alert, ...current.alerts].slice(0, 50) };
          }
          const nextAlerts = [...current.alerts];
          nextAlerts[existingIndex] = {
            ...nextAlerts[existingIndex],
            ...alert,
          };
          return { ...current, alerts: nextAlerts };
        },
      );
      setAlerts((currentAlerts) => {
        const shouldRemove =
          eventType === "alert.resolved" ||
          eventType === "alert.dismissed" ||
          isTerminalStatus(alert.status);

        if (shouldRemove) {
          return currentAlerts.filter(
            (currentAlert) => currentAlert.id !== alert.id,
          );
        }
        if (!isActiveStatus(alert.status) || isStaleServiceCall(alert)) {
          return currentAlerts.filter(
            (currentAlert) => currentAlert.id !== alert.id,
          );
        }

        const existingIndex = currentAlerts.findIndex(
          (currentAlert) => currentAlert.id === alert.id,
        );
        if (existingIndex === -1) {
          return [...currentAlerts, alert];
        }

        const nextAlerts = [...currentAlerts];
        nextAlerts[existingIndex] = {
          ...nextAlerts[existingIndex],
          ...alert,
        };
        return nextAlerts;
      });
    },
    [queryClient, recentAlertsQueryKey],
  );

  const handleAlertEvent = useCallback(
    (event: SSEEvent) => {
      if (event.type === "chat.announcement" || event.type === "chat.message") {
        // Manager announcements get a discreet default-on sound; regular chat
        // traffic is badge-only unless the operator opts in on this device.
        // Neither is an alert.* frame, so this must never fall through to
        // patchAlert below.
        const prefs = getChatSoundPrefs();
        const wantsSound =
          event.type === "chat.announcement"
            ? prefs.announcementSound
            : prefs.messageSound;
        if (
          wantsSound &&
          isSoundLeader.current &&
          settings?.enabled &&
          settings.sound_enabled &&
          !getLocalAlertSoundOverrides().muted
        ) {
          void soundEngine
            .playOnce({ alertTypes: [], volume: settings.volume })
            .then(() => {
              setAudioBlocked(soundEngine.getState().blocked);
            });
        }
        return;
      }

      if (!event.type.startsWith("alert.")) return;

      const alert = extractAlertFromEvent(event.data);
      if (!alert) return;

      if (event.type === "alert.created") {
        const isUrgent =
          alert.priority === "urgent" || alert.priority === "high";
        // Localize at render time: backend stores English title/body but carries
        // structured metadata + alert_type; present the operator's locale. (R3-AI-8)
        // Toasts respect the per-type `enabled` preference (see
        // alertTypeAllowsToast) — disabled types stay in the list/badges only.
        if (alertTypeAllowsToast(settings, alert.alert_type)) {
          const localized = localizeAlert(alert, locale);
          if (isUrgent) {
            toast.showWarning?.(localized.title, localized.body || undefined);
          } else {
            toast.showInfo?.(localized.title, localized.body || undefined);
          }
        }
        // Visuals (toast, badges, counts) render in every tab; only the
        // sound-leader tab plays audio / posts browser notifications so
        // operators with multiple tabs open don't get a chorus of alerts.
        if (isSoundLeader.current) {
          showAlertBrowserNotification(alert, settings, locale);
        }
        const overrides = getLocalAlertSoundOverrides();
        if (
          isSoundLeader.current &&
          settings?.enabled &&
          settings.sound_enabled &&
          !overrides.muted &&
          alertTypeAllowsSound(settings, alert.alert_type) &&
          !alertTypeRepeats(settings, alert.alert_type)
        ) {
          void soundEngine
            .playOnce({
              alertTypes: [alert.alert_type],
              volume: settings.volume,
              emphasize: isUrgent,
            })
            .then(() => {
              setAudioBlocked(soundEngine.getState().blocked);
            });
        }
      }

      patchAlert(alert, event.type);

      // Keep the "what did I miss" popover honest: any alert.* frame
      // (created/updated/claimed/resolved/…) makes its cached 7-day feed
      // stale. Invalidation refetches only if the popover is open (the panel
      // unmounts when closed) and otherwise just marks it stale for next open.
      void queryClient.invalidateQueries({ queryKey: recentAlertsQueryKey });
    },
    [
      isSoundLeader,
      locale,
      patchAlert,
      queryClient,
      recentAlertsQueryKey,
      settings,
      soundEngine,
      toast,
    ],
  );

  const handleReconnect = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: activeAlertsQueryKey });
    void queryClient.invalidateQueries({ queryKey: settingsQueryKey });
  }, [activeAlertsQueryKey, queryClient, settingsQueryKey]);

  useSSEEvents({
    businessId,
    enabled: shouldFetch,
    onEvent: handleAlertEvent,
    onReconnect: handleReconnect,
  });

  const repeatingAlertTypes = useMemo(
    () => uniqueRepeatingAlertTypes(alerts, settings),
    [alerts, settings],
  );

  useEffect(() => {
    const overrides = getLocalAlertSoundOverrides();
    // Leadership lives in a ref (no re-render on change); `leaderEpoch` bumps
    // when THIS tab acquires the Web Lock, so a surviving tab that inherits
    // leadership mid-alarm re-runs this effect immediately and takes over the
    // repeating sound instead of staying silent until the next SSE event.
    const shouldRepeat =
      isSoundLeader.current &&
      settings?.enabled === true &&
      settings.sound_enabled &&
      !overrides.muted &&
      repeatingAlertTypes.length > 0;

    if (!shouldRepeat || !settings) {
      soundEngine.stopRepeating();
      setAudioBlocked(soundEngine.getState().blocked);
      return;
    }

    void Promise.resolve(
      soundEngine.startRepeating({
        alertTypes: repeatingAlertTypes,
        volume: settings.volume,
        repeatIntervalSeconds: settings.repeat_interval_seconds,
      }),
    ).then(() => {
      setAudioBlocked(soundEngine.getState().blocked);
    });

    return () => {
      soundEngine.stopRepeating();
    };
  }, [isSoundLeader, leaderEpoch, repeatingAlertTypes, settings, soundEngine]);

  const enableSound = useCallback(async () => {
    await soundEngine.unlock();
    if (settings?.browser_notifications_enabled) {
      await requestBrowserNotificationPermission();
    }
    setAudioBlocked(soundEngine.getState().blocked);
  }, [settings?.browser_notifications_enabled, soundEngine]);

  // Browsers block Web Audio until a user gesture. Unlock on the FIRST
  // pointer/key interaction so the operator never misses the first alert of a
  // session; the explicit EnableAlertSoundButton remains the manual fallback.
  useEffect(() => {
    if (!shouldFetch) return;

    const unlockOnce = () => {
      window.removeEventListener("pointerdown", unlockOnce);
      window.removeEventListener("keydown", unlockOnce);
      void soundEngine.unlock().then(() => {
        setAudioBlocked(soundEngine.getState().blocked);
      });
    };

    window.addEventListener("pointerdown", unlockOnce, { once: true });
    window.addEventListener("keydown", unlockOnce, { once: true });

    return () => {
      window.removeEventListener("pointerdown", unlockOnce);
      window.removeEventListener("keydown", unlockOnce);
    };
  }, [shouldFetch, soundEngine]);

  const claimAlert = useCallback(
    async (alertId: number, source: string, staffId?: number) => {
      if (!shouldFetch) return;

      try {
        const claimedAlert =
          staffId && staffId > 0
            ? await claimOperationalAlert(businessId, alertId, source, staffId)
            : await claimOperationalAlert(businessId, alertId, source);
        patchAlert(claimedAlert);
      } catch (error) {
        if (error instanceof OperationalAlertConflictError) {
          // Someone else got there first (HTTP 409). Not an error from the
          // operator's point of view: tell them who has it and resync.
          toast.showInfo?.(localizeClaimConflict(locale, error.claimedByName));
          await queryClient.invalidateQueries({
            queryKey: activeAlertsQueryKey,
          });
          await queryClient.invalidateQueries({
            queryKey: recentAlertsQueryKey,
          });
          return;
        }
        throw error;
      }
      await queryClient.invalidateQueries({ queryKey: activeAlertsQueryKey });
      await queryClient.invalidateQueries({ queryKey: recentAlertsQueryKey });
    },
    [
      activeAlertsQueryKey,
      businessId,
      locale,
      patchAlert,
      queryClient,
      recentAlertsQueryKey,
      shouldFetch,
      toast,
    ],
  );

  const resolveAlert = useCallback(
    async (alertId: number, reason: string) => {
      if (!shouldFetch) return;

      try {
        const resolvedAlert = await resolveOperationalAlert(
          businessId,
          alertId,
          reason,
        );
        patchAlert(resolvedAlert, "alert.resolved");
      } catch (error) {
        if (error instanceof OperationalAlertConflictError) {
          toast.showInfo?.(localizeClaimConflict(locale, error.claimedByName));
          await queryClient.invalidateQueries({
            queryKey: activeAlertsQueryKey,
          });
          await queryClient.invalidateQueries({
            queryKey: recentAlertsQueryKey,
          });
          return;
        }
        throw error;
      }

      await queryClient.invalidateQueries({ queryKey: activeAlertsQueryKey });
      await queryClient.invalidateQueries({ queryKey: recentAlertsQueryKey });
    },
    [
      activeAlertsQueryKey,
      businessId,
      locale,
      patchAlert,
      queryClient,
      recentAlertsQueryKey,
      shouldFetch,
      toast,
    ],
  );

  const getAlertForResource = useCallback(
    (
      resourceType: OperationalAlertResourceType,
      resourceId: number,
    ): OperationalAlert | null =>
      alerts.find(
        (alert) =>
          alert.resource_type === resourceType &&
          alert.resource_id === resourceId,
      ) ?? null,
    [alerts],
  );

  const claimByResource = useCallback(
    async (
      resourceType: OperationalAlertResourceType,
      resourceId: number,
      source: string,
    ) => {
      const alert = getAlertForResource(resourceType, resourceId);
      if (!alert || alert.status !== "open") return;
      await claimAlert(alert.id, source);
    },
    [claimAlert, getAlertForResource],
  );

  const counts = useMemo<OperationalAlertCounts>(() => {
    const nextCounts: OperationalAlertCounts = { ...EMPTY_COUNTS };
    for (const alert of alerts) {
      if (!isActiveStatus(alert.status)) continue;
      const bucket = ALERT_COUNT_BUCKETS[alert.alert_type];
      if (bucket) nextCounts[bucket] += 1;
    }
    return nextCounts;
  }, [alerts]);

  const value = useMemo<OperationalAlertsContextValue>(
    () => ({
      alerts,
      settings,
      counts,
      audioBlocked,
      enableSound,
      claimAlert,
      resolveAlert,
      claimByResource,
      getAlertForResource,
    }),
    [
      alerts,
      audioBlocked,
      claimAlert,
      resolveAlert,
      claimByResource,
      counts,
      enableSound,
      getAlertForResource,
      settings,
    ],
  );

  return (
    <OperationalAlertsContext.Provider value={value}>
      {children}
    </OperationalAlertsContext.Provider>
  );
}
