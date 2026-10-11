"use client";

/**
 * PV-LIVE-20260720-009 — Open service-call queue on the Tables tab.
 *
 * Alert navigation already lands on ?tab=tables. This panel lists active
 * service_call alerts with table, reason, age, and claim/resolve actions so
 * floor staff can act without digging into Recent Alerts.
 */

import React, { useCallback, useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { Bell, Hand } from "lucide-react";
import type { OperationalAlert } from "@/api/operationalAlerts";
import { getBusinessStaff, type StaffMember } from "@/api/staff";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import type { Locale } from "@/i18n/localeRegistry";
import {
  localizeAlert,
  serviceCallReasonLabel,
} from "../operational-alerts/operationalAlertCopy";
import { useOptionalOperationalAlerts } from "../operational-alerts/useOperationalAlerts";
import { isStaleServiceCall } from "../operational-alerts/serviceCallTtl";
import { humanizeDurationMinutes } from "@/utils/humanizeDuration";

/**
 * Live-queue ages route through humanizeDurationMinutes. Day-old calls are
 * filtered by the seating SLA (#729); this formatter still covers sub-SLA
 * ages and any leftover history views.
 */
export function serviceCallTimeAgo(
  iso: string,
  t: (key: string, p?: Record<string, string | number>) => string,
  nowMs: number = Date.now(),
): string {
  const ms = Date.parse(iso);
  if (!Number.isFinite(ms)) return "";
  const minutes = Math.max(0, Math.floor((nowMs - ms) / 60000));
  if (minutes < 1) return t("serviceCalls.justNow");
  const duration = humanizeDurationMinutes(minutes);
  return t("serviceCalls.elapsedAgo", { duration });
}

/** Amber ≥5m, rose ≥15m — escalate visually before the guest gives up. */
export function serviceCallAgeTone(iso: string, nowMs = Date.now()): string {
  const ms = Date.parse(iso);
  if (!Number.isFinite(ms)) return "text-ink-600";
  const minutes = Math.max(0, Math.floor((nowMs - ms) / 60000));
  if (minutes >= 15) return "text-rose-700 font-semibold";
  if (minutes >= 5) return "text-amber-800 font-medium";
  return "text-ink-600";
}

export default function ServiceCallsQueue({
  businessId,
}: {
  businessId: number;
}) {
  const alertsCtx = useOptionalOperationalAlerts();
  const { locale } = useSimpleLocale();
  const [busyId, setBusyId] = useState<number | null>(null);
  const [errorId, setErrorId] = useState<number | null>(null);
  const [assignTo, setAssignTo] = useState<Record<number, number>>({});
  const [staff, setStaff] = useState<StaffMember[]>([]);

  useEffect(() => {
    let cancelled = false;
    void getBusinessStaff(String(businessId))
      .then((res) => {
        if (cancelled) return;
        setStaff(
          (res.staff || []).filter(
            (member) =>
              member.is_active &&
              (member.role === "server" ||
                member.role === "host" ||
                member.role === "manager"),
          ),
        );
      })
      .catch(() => {
        if (!cancelled) setStaff([]);
      });
    return () => {
      cancelled = true;
    };
  }, [businessId]);

  const t = useCallback(
    (key: string, params?: Record<string, string | number>): string => {
      const fullKey = `businessDashboard.dashboard.tableManager.${key}`;
      const result = getTranslation(fullKey, locale as Locale, params);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [locale],
  );

  const serviceCalls = useMemo(() => {
    if (!alertsCtx?.alerts) return [] as OperationalAlert[];
    return alertsCtx.alerts
      .filter(
        (a) =>
          a.alert_type === "service_call" &&
          (a.status === "open" || a.status === "claimed") &&
          !isStaleServiceCall(a),
      )
      .slice()
      .sort(
        (a, b) =>
          Date.parse(a.created_at) - Date.parse(b.created_at) || a.id - b.id,
      );
  }, [alertsCtx?.alerts]);

  if (!alertsCtx || serviceCalls.length === 0) {
    return null;
  }

  const reasonLocale = locale === "es" || locale === "es-AR" ? "es" : "en";

  return (
    <section
      className="mb-4 overflow-hidden rounded-2xl border border-amber-200 bg-amber-50/80"
      data-testid="service-calls-queue"
      aria-label={t("serviceCalls.title")}
    >
      <header className="flex flex-wrap items-center gap-2 border-b border-amber-200/80 px-4 py-3">
        <Hand className="h-4 w-4 text-amber-800" strokeWidth={1.75} />
        <h3 className="text-sm font-semibold text-ink-900">
          {t("serviceCalls.title")}
        </h3>
        <span className="rounded-full bg-amber-200/80 px-2 py-0.5 text-xs font-semibold text-amber-900">
          {serviceCalls.length}
        </span>
        <p className="w-full text-xs text-ink-600 sm:ml-auto sm:w-auto">
          {t("serviceCalls.ackVsResolveHint")}
        </p>
      </header>
      <ul className="divide-y divide-amber-100">
        {serviceCalls.map((alert) => {
          const localized = localizeAlert(alert, locale as Locale);
          const tableName =
            typeof alert.metadata?.table_name === "string"
              ? alert.metadata.table_name
              : null;
          const reasonRaw =
            typeof alert.metadata?.reason === "string"
              ? alert.metadata.reason
              : null;
          const reason =
            reasonRaw && serviceCallReasonLabel(reasonRaw, reasonLocale);
          const actionLabel =
            alert.status === "open"
              ? t("serviceCalls.acknowledge")
              : t("serviceCalls.markHandled");
          const busy = busyId === alert.id;
          const tableIdRaw = alert.metadata?.table_id;
          const tableId =
            typeof tableIdRaw === "number"
              ? tableIdRaw
              : typeof tableIdRaw === "string" && /^\d+$/.test(tableIdRaw)
                ? Number(tableIdRaw)
                : null;
          const ageTone = serviceCallAgeTone(alert.created_at);
          const escalated =
            ageTone.includes("rose") || ageTone.includes("amber");

          return (
            <li
              key={alert.id}
              className="flex flex-wrap items-center justify-between gap-3 px-4 py-3"
              data-testid={`service-call-row-${alert.id}`}
            >
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-semibold text-ink-900">
                  {tableName || localized.title}
                  {reason ? (
                    <span className="font-medium text-ink-700">
                      {" "}
                      · {reason}
                    </span>
                  ) : null}
                </p>
                <div className="mt-0.5 flex flex-wrap items-center gap-2 text-xs text-ink-600">
                  <span
                    className={`rounded-full px-2 py-0.5 font-medium ${
                      alert.status === "open"
                        ? "bg-amber-200 text-amber-900"
                        : "bg-brand/10 text-brand-dark"
                    }`}
                  >
                    {t(`serviceCalls.status.${alert.status}`)}
                  </span>
                  {escalated ? (
                    <span className="rounded-full bg-rose-100 px-2 py-0.5 font-medium text-rose-800">
                      {t("serviceCalls.needsAttention")}
                    </span>
                  ) : null}
                  <span
                    className={ageTone}
                    data-testid={`service-call-age-${alert.id}`}
                  >
                    {serviceCallTimeAgo(alert.created_at, t)}
                  </span>
                  {alert.status === "claimed" && alert.claimed_by_name ? (
                    <span className="text-ink-600">
                      {t("serviceCalls.claimedBy", {
                        name: alert.claimed_by_name,
                      })}
                    </span>
                  ) : null}
                  {tableId != null ? (
                    <Link
                      href={`?tab=tables&tableId=${tableId}`}
                      className="font-medium text-brand hover:text-brand-dark"
                    >
                      {t("serviceCalls.openTable")}
                    </Link>
                  ) : null}
                  {errorId === alert.id && (
                    <span className="text-rose-700">
                      {t("serviceCalls.updateFailed")}
                    </span>
                  )}
                </div>
              </div>
              <div className="flex flex-wrap items-center justify-end gap-2">
                {alert.status === "open" && staff.length > 0 ? (
                  <label
                    className="sr-only"
                    htmlFor={`service-call-assign-${alert.id}`}
                  >
                    {t("serviceCalls.assignTo")}
                  </label>
                ) : null}
                {alert.status === "open" && staff.length > 0 ? (
                  <select
                    id={`service-call-assign-${alert.id}`}
                    data-testid={`service-call-assign-${alert.id}`}
                    value={assignTo[alert.id] ?? ""}
                    disabled={busy}
                    onChange={(event) => {
                      const next = Number(event.target.value);
                      setAssignTo((prev) => ({
                        ...prev,
                        [alert.id]: Number.isFinite(next) ? next : 0,
                      }));
                    }}
                    className="max-w-[10rem] rounded-full border border-warm-200 bg-white px-2 py-1 text-xs text-ink-800"
                  >
                    <option value="">{t("serviceCalls.assignMe")}</option>
                    {staff.map((member) => (
                      <option key={member.id} value={member.id}>
                        {member.name || member.email}
                      </option>
                    ))}
                  </select>
                ) : null}
                <button
                  type="button"
                  disabled={busy}
                  className="inline-flex items-center gap-1.5 rounded-full bg-brand px-3 py-1.5 text-xs font-semibold text-white transition-colors hover:bg-brand-dark disabled:opacity-60"
                  onClick={async () => {
                    setBusyId(alert.id);
                    setErrorId(null);
                    try {
                      if (alert.status === "open") {
                        const routed = assignTo[alert.id];
                        if (routed && routed > 0) {
                          await alertsCtx.claimAlert(
                            alert.id,
                            "tables_queue",
                            routed,
                          );
                        } else {
                          await alertsCtx.claimAlert(alert.id, "tables_queue");
                        }
                      } else {
                        await alertsCtx.resolveAlert(alert.id, "handled");
                      }
                    } catch {
                      setErrorId(alert.id);
                    } finally {
                      setBusyId(null);
                    }
                  }}
                >
                  <Bell className="h-3.5 w-3.5" strokeWidth={1.75} />
                  {busy
                    ? t("serviceCalls.updating")
                    : alert.status === "open" && (assignTo[alert.id] ?? 0) > 0
                      ? t("serviceCalls.assign")
                      : actionLabel}
                </button>
              </div>
            </li>
          );
        })}
      </ul>
    </section>
  );
}
