"use client";

import React from "react";
import { createPortal } from "react-dom";
import {
  Button,
  Popover,
  PopoverContent,
  PopoverTrigger,
  Spinner,
} from "@nextui-org/react";
import { Bell, History } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import EnableAlertSoundButton from "./EnableAlertSoundButton";
import { useOptionalOperationalAlerts } from "./useOperationalAlerts";
import {
  fetchRecentOperationalAlerts,
  type OperationalAlert,
  type OperationalAlertStatus,
  type OperationalAlertType,
} from "@/api/operationalAlerts";
import { queryKeys } from "@/api/queryKeys";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";
import { StatusChip, type StatusTone } from "@/components/ui/StatusChip";
import { EmptyState } from "@/components/ui/EmptyState";
import { localizeAlert } from "./operationalAlertCopy";
import { alertTabSpec } from "./alertNavigation";

interface RecentAlertsPopoverProps {
  businessId: number;
  /** Where the popover panel opens relative to the trigger. */
  placement?: "bottom-end" | "top-start";
  className?: string;
  /** Embed the alert-sound toggle at the top of the panel and surface an
      "audio blocked" nudge dot on the trigger. Consolidates the old separate
      bell (sound) + clock (history) icons into one Alerts control. */
  showSoundControl?: boolean;
  /** Drop the native `title` tooltip when a parent already provides one
      (e.g. the sidebar wraps the trigger in its own styled tooltip). */
  hideNativeTitle?: boolean;
  /** Wave 4: navigate to the alert's home tab (handleSetActiveTab). */
  onNavigate?: (spec: string) => void;
}

// open/claimed still need attention (or just got it) = warning tone;
// resolved = neutral; dismissed = neutral, further muted at the row level.
const STATUS_TONES: Record<OperationalAlertStatus, StatusTone> = {
  open: "warn",
  claimed: "warn",
  resolved: "neutral",
  dismissed: "neutral",
};

type Translator = (
  key: string,
  params?: Record<string, string | number>,
) => string;

// Same relative-time vocabulary the tables dashboard uses
// (businessDashboard.dashboard.tableManager.timeAgo) — reused, not duplicated.
function timeAgo(iso: string, t: Translator): string {
  const base = "dashboard.tableManager.timeAgo";
  const ms = Date.now() - new Date(iso).getTime();
  const m = Math.floor(ms / 60000);
  if (m < 1) return t(`${base}.justNow`);
  if (m < 60) return t(`${base}.minutesAgo`, { n: m });
  const h = Math.floor(m / 60);
  if (h < 24) return t(`${base}.hoursAgo`, { n: h });
  const d = Math.floor(h / 24);
  return t(`${base}.daysAgo`, { n: d });
}

const FILTER_CHIP_BASE =
  "shrink-0 rounded-full px-2.5 py-1 text-[11px] font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand";
const FILTER_CHIP_ACTIVE = "bg-brand/10 text-brand";
const FILTER_CHIP_IDLE = "bg-warm-100 text-ink-600 hover:bg-warm-200";

/**
 * Panel body — owns the query. Kept as a separate component mounted only while
 * the popover is open, so the closed trigger never touches React Query (host
 * surfaces and their tests don't need a QueryClient until the user opens it;
 * it also guarantees "fetch only while open" alongside `staleTime`).
 */
function RecentAlertsPanel({
  businessId,
  t,
  showSoundControl = false,
  onNavigate,
}: {
  businessId: number;
  t: Translator;
  showSoundControl?: boolean;
  onNavigate?: (spec: string) => void;
}) {
  const { locale } = useSimpleLocale();
  const alertsCtx = useOptionalOperationalAlerts();
  const soundAvailable =
    showSoundControl &&
    !!alertsCtx &&
    alertsCtx.settings?.sound_enabled !== false;
  const [typeFilter, setTypeFilter] = React.useState<
    OperationalAlertType | "all"
  >("all");
  const [busyAlertId, setBusyAlertId] = React.useState<number | null>(null);
  const [actionErrorId, setActionErrorId] = React.useState<number | null>(null);

  const typeLabel = (type: OperationalAlertType) =>
    String(
      getTranslation(
        `businessSettings.notifications.operationalTypes.${type}`,
        locale,
      ),
    );

  const { data, isLoading } = useQuery({
    queryKey: queryKeys.alerts.recent(String(businessId)),
    queryFn: () => fetchRecentOperationalAlerts(businessId),
    staleTime: 30_000,
  });

  const alerts: OperationalAlert[] = React.useMemo(
    () => data?.alerts ?? [],
    [data],
  );

  // Only offer filter chips for the types actually present in the window.
  const presentTypes = React.useMemo(() => {
    const seen = new Set<OperationalAlertType>();
    alerts.forEach((a) => seen.add(a.alert_type));
    return Array.from(seen);
  }, [alerts]);

  const activeFilter =
    typeFilter !== "all" && !presentTypes.includes(typeFilter)
      ? "all"
      : typeFilter;
  const visible =
    activeFilter === "all"
      ? alerts
      : alerts.filter((a) => a.alert_type === activeFilter);

  return (
    <div id="recent-alerts-panel" className="w-full">
      <div className="border-b border-warm-100 px-4 py-3">
        <h3 className="text-sm font-semibold text-ink-900">
          {t("recentAlerts.title")}
        </h3>
      </div>

      {soundAvailable && (
        <div className="flex items-center justify-between gap-3 border-b border-warm-100 px-4 py-2.5">
          <span className="text-xs font-medium text-ink-600">
            {t("recentAlerts.soundLabel")}
          </span>
          <EnableAlertSoundButton />
        </div>
      )}

      {presentTypes.length > 1 && (
        <div
          role="group"
          aria-label={t("recentAlerts.filterLabel")}
          className="flex items-center gap-1.5 overflow-x-auto border-b border-warm-100 px-4 py-2"
        >
          <button
            type="button"
            onClick={() => setTypeFilter("all")}
            aria-pressed={activeFilter === "all"}
            className={`${FILTER_CHIP_BASE} ${
              activeFilter === "all" ? FILTER_CHIP_ACTIVE : FILTER_CHIP_IDLE
            }`}
          >
            {t("recentAlerts.filterAll")}
          </button>
          {presentTypes.map((type) => (
            <button
              key={type}
              type="button"
              onClick={() => setTypeFilter(type)}
              aria-pressed={activeFilter === type}
              className={`${FILTER_CHIP_BASE} ${
                activeFilter === type ? FILTER_CHIP_ACTIVE : FILTER_CHIP_IDLE
              }`}
            >
              {typeLabel(type)}
            </button>
          ))}
        </div>
      )}

      {isLoading ? (
        <div className="flex justify-center py-8">
          <Spinner size="sm" aria-label={t("recentAlerts.title")} />
        </div>
      ) : visible.length === 0 ? (
        <EmptyState
          compact
          icon={History}
          title={t("recentAlerts.empty")}
          subtitle={t("recentAlerts.emptyHint")}
          className="px-4"
        />
      ) : (
        <ul className="max-h-80 divide-y divide-warm-100 overflow-y-auto">
          {visible.map((alert) => {
            const spec = onNavigate ? alertTabSpec(alert) : null;
            const localized = localizeAlert(alert, locale);
            const contextBody =
              localized.body && localized.body !== localized.title
                ? localized.body
                : alert.body && alert.body !== localized.title
                  ? alert.body
                  : localized.body;
            const serviceCallReason =
              alert.alert_type === "service_call" &&
              typeof alert.metadata?.reason === "string"
                ? t(`recentAlerts.serviceCallReason.${alert.metadata.reason}`)
                : null;
            const actionLabel =
              alert.alert_type === "service_call" && alert.status === "open"
                ? t("recentAlerts.acknowledge")
                : alert.alert_type === "service_call" &&
                    alert.status === "claimed"
                  ? t("recentAlerts.markHandled")
                  : null;
            const runServiceCallAction = async () => {
              if (!alertsCtx || !actionLabel || busyAlertId !== null) return;
              setBusyAlertId(alert.id);
              setActionErrorId(null);
              try {
                if (alert.status === "open") {
                  await alertsCtx.claimAlert(alert.id, "recent_alerts");
                } else {
                  await alertsCtx.resolveAlert(alert.id, "handled");
                }
              } catch {
                setActionErrorId(alert.id);
              } finally {
                setBusyAlertId(null);
              }
            };
            const rowInner = (
              <>
                <p className="truncate text-sm font-medium text-ink-900">
                  {localized.title}
                </p>
                {(serviceCallReason || contextBody) && (
                  <p className="mt-0.5 text-xs leading-5 text-ink-600">
                    {serviceCallReason || contextBody}
                  </p>
                )}
                <div className="mt-1 flex items-center gap-2">
                  <StatusChip
                    tone={STATUS_TONES[alert.status]}
                    label={t(`recentAlerts.status.${alert.status}`)}
                  />
                  <span className="text-xs text-ink-500">
                    {timeAgo(alert.created_at, t)}
                  </span>
                </div>
              </>
            );
            return (
              <li
                key={alert.id}
                className={`px-4 py-2.5 ${
                  alert.status === "dismissed" ? "opacity-60" : ""
                }`}
              >
                {spec ? (
                  <button
                    type="button"
                    onClick={() => onNavigate!(spec)}
                    className="-mx-1 w-full rounded-lg px-1 text-left transition-colors hover:bg-warm-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
                  >
                    {rowInner}
                  </button>
                ) : (
                  <div>{rowInner}</div>
                )}
                {actionLabel && alertsCtx && (
                  <button
                    type="button"
                    disabled={busyAlertId !== null}
                    onClick={() => void runServiceCallAction()}
                    className="mt-2 inline-flex min-h-8 items-center rounded-lg bg-brand px-3 text-xs font-semibold text-white transition-colors hover:bg-brand-600 disabled:cursor-wait disabled:opacity-60 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
                  >
                    {busyAlertId === alert.id
                      ? t("recentAlerts.updating")
                      : actionLabel}
                  </button>
                )}
                {actionErrorId === alert.id && (
                  <p role="alert" className="mt-1.5 text-xs text-rose-700">
                    {t("recentAlerts.updateFailed")}
                  </p>
                )}
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}

/**
 * "What did I miss" popover for the dashboard header/sidebar: the last 7 days
 * of operational alerts (including resolved/dismissed history), fetched only
 * while open, with a client-side type filter over the ≤100 returned rows.
 */
export default function RecentAlertsPopover({
  businessId,
  placement = "bottom-end",
  className = "",
  showSoundControl = false,
  hideNativeTitle = true,
  onNavigate,
}: RecentAlertsPopoverProps) {
  const { locale } = useSimpleLocale();
  const [open, setOpen] = React.useState(false);
  const alerts = useOptionalOperationalAlerts();

  const t = React.useCallback<Translator>(
    (key, params) =>
      String(getTranslation(`businessDashboard.${key}`, locale, params)),
    [locale],
  );

  const title = t("recentAlerts.title");
  // Nudge dot: sound is on in settings but the browser is still blocking audio
  // (needs a user gesture). Opening the popover and clicking Enable unblocks it.
  const audioNudge =
    showSoundControl &&
    !!alerts?.audioBlocked &&
    alerts?.settings?.sound_enabled !== false;

  return (
    <>
      {open &&
        createPortal(
          <div
            data-testid="recent-alerts-backdrop"
            aria-hidden="true"
            className="fixed inset-0 z-[60] bg-ink-950/40"
            onClick={() => setOpen(false)}
          />,
          document.body,
        )}
      <Popover
        isOpen={open}
        onOpenChange={setOpen}
        placement={placement}
        shouldBlockScroll={false}
      >
        <PopoverTrigger>
          <Button
            isIconOnly
            size="sm"
            variant="light"
            data-testid="recent-alerts-trigger"
            aria-label={title}
            aria-expanded={open}
            aria-controls="recent-alerts-panel"
            data-open={open ? "true" : "false"}
            title={hideNativeTitle ? undefined : title}
            onKeyDown={(event) => {
              if (event.key === "Escape") setOpen(false);
            }}
            className={`relative ${className}`}
          >
            <Bell className="w-4 h-4" aria-hidden="true" />
            {audioNudge && (
              <span
                aria-hidden="true"
                className="absolute right-1 top-1 h-1.5 w-1.5 rounded-full bg-amber-500 ring-2 ring-white"
              />
            )}
          </Button>
        </PopoverTrigger>
        <PopoverContent className="z-[70] w-[22rem] max-w-[calc(100vw-1.5rem)] overflow-hidden rounded-xl border border-warm-200 bg-white p-0 shadow-lg shadow-ink-950/15">
          <div
            data-testid="recent-alerts-chrome"
            className="w-full overflow-hidden rounded-xl bg-white"
          >
            <RecentAlertsPanel
              businessId={businessId}
              t={t}
              showSoundControl={showSoundControl}
              onNavigate={
                onNavigate
                  ? (spec) => {
                      setOpen(false);
                      onNavigate(spec);
                    }
                  : undefined
              }
            />
          </div>
        </PopoverContent>
      </Popover>
    </>
  );
}
