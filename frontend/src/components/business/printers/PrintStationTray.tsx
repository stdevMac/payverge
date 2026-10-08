"use client";

import { useCallback } from "react";

import type { PrintJob } from "@/api/print";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";

export interface PrintStationTrayProps {
  online: boolean;
  isLeader: boolean;
  pendingCount: number;
  currentJob: PrintJob | null;
  lastError: string | null;
  statusLabel: string;
  onDismissError: () => void;
  onReconnect: () => void;
  onStopStation: () => void;
}

/**
 * Compact operator-visible tray: station online/offline, pending count,
 * current job, last error, and setup guidance for browser printing.
 */
export function PrintStationTray({
  online,
  isLeader,
  pendingCount,
  currentJob,
  lastError,
  statusLabel,
  onDismissError,
  onReconnect,
  onStopStation,
}: PrintStationTrayProps) {
  const { locale } = useSimpleLocale();
  const t = useCallback(
    (key: string, params?: Record<string, string | number>): string => {
      const result = getTranslation(`printers.${key}`, locale, params);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [locale],
  );

  return (
    <div
      data-testid="print-station-tray"
      className="fixed bottom-4 right-4 z-40 max-w-xs rounded-xl border border-warm-200 bg-white/95 p-3 text-xs shadow-md backdrop-blur"
      role="status"
      aria-live="polite"
    >
      <div className="flex items-center gap-2 font-semibold text-ink-800">
        <span
          className={`inline-block h-2 w-2 rounded-full ${
            online ? "bg-emerald-500" : "bg-ink-300"
          }`}
          aria-hidden
        />
        {online ? t("agent.stationOnline") : t("agent.stationOffline")}
        {isLeader ? (
          <span className="rounded bg-brand-50 px-1.5 py-0.5 text-[10px] font-medium text-brand-800">
            {t("agent.leader")}
          </span>
        ) : (
          <span className="rounded bg-warm-100 px-1.5 py-0.5 text-[10px] font-medium text-ink-600">
            {t("agent.follower")}
          </span>
        )}
      </div>
      <p className="mt-1 text-ink-600">
        {t("agent.pendingCount", { count: pendingCount })} · {statusLabel}
      </p>
      {currentJob && (
        <p className="mt-0.5 text-ink-700">
          {t("agent.currentJob", { id: currentJob.id, kind: currentJob.kind })}
        </p>
      )}
      {lastError && (
        <p className="mt-1 text-rose-700" role="alert">
          {t("agent.lastError", { error: lastError })}
        </p>
      )}
      <p className="mt-2 text-[11px] leading-snug text-ink-500">
        {t("agent.setupHint")}
      </p>
      <div className="mt-2 flex flex-wrap gap-2">
        {lastError && (
          <button
            type="button"
            onClick={onDismissError}
            className="rounded-lg border border-warm-300 px-2.5 py-1.5 font-medium text-ink-700 hover:bg-warm-50"
          >
            {t("agent.dismissError")}
          </button>
        )}
        <button
          type="button"
          onClick={onReconnect}
          className="rounded-lg bg-brand px-2.5 py-1.5 font-semibold text-white hover:bg-brand-dark"
        >
          {t("agent.reconnect")}
        </button>
        <button
          type="button"
          onClick={onStopStation}
          className="rounded-lg border border-warm-300 px-2.5 py-1.5 font-medium text-ink-700 hover:bg-warm-50"
        >
          {t("stopStation")}
        </button>
      </div>
    </div>
  );
}
