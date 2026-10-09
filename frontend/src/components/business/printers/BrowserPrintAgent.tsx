"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import {
  agentCancelPrintJob,
  claimPrintJob,
  confirmPrinted,
  formatPrintJobStatus,
  markPresented,
  renewPrintLease,
  retryPrintJob,
  type PrintJob,
} from "@/api/print";
import { useStaffPermissionsContext } from "@/contexts/StaffPermissionsContext";
import { getApiErrorStatus } from "@/utils/apiError";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";

import { useIframePrint } from "./useIframePrint";
import { getOrCreatePrintClientId, usePrintLeader } from "./usePrintLeader";
import { PrintStationTray } from "./PrintStationTray";
import { useSSEEvents, type SSEEvent } from "@/hooks/useSSEEvents";

const IDLE_RECOVERY_POLL_MS = 30_000;
const HIDDEN_RECOVERY_POLL_MS = 60_000;
const MAX_BACKOFF_MS = 120_000;
const RENEW_MS = 20_000;

export interface BrowserPrintAgentProps {
  businessId: number;
  /** Explicit browser printer assigned to this workstation. */
  printerId: number | null;
  /** When false, agent is idle (logout / no access). */
  enabled?: boolean;
  /** Owner bypasses staff permission checks. */
  isOwner?: boolean;
  /** Removes the explicit station assignment for this workstation. */
  onStopStation?: () => void;
}

/**
 * Single-leader browser print consumer. Mount once in the operator shell when
 * the user can print. Elects one tab per business, claims jobs, presents via
 * useIframePrint, then shows a non-blocking confirmation panel — never
 * auto-marks printed.
 */
export default function BrowserPrintAgent({
  businessId,
  printerId,
  enabled = true,
  isOwner = false,
  onStopStation,
}: BrowserPrintAgentProps) {
  const { locale } = useSimpleLocale();
  const t = useCallback(
    (key: string, params?: Record<string, string | number>): string => {
      const result = getTranslation(`printers.${key}`, locale, params);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [locale],
  );

  // formatPrintJobStatus returns a stable status KEY (not English); translate it
  // through printers.status.*. Unknown/future backend statuses have no key, so
  // getTranslation falls back to a sentence-cased leaf — still safe, never raw enum.
  const statusLabel = useCallback(
    (status: PrintJob["status"] | null | undefined): string =>
      t(`status.${formatPrintJobStatus(status)}`),
    [t],
  );

  const { permissions, isLoading: permsLoading } = useStaffPermissionsContext();
  const canPrint =
    isOwner ||
    permissions.some((permission) =>
      ["print:bill", "print:receipt", "orders:kitchen"].includes(permission),
    );

  const stationPrinterId = printerId ?? 0;
  const active = Boolean(
    enabled &&
    businessId > 0 &&
    stationPrinterId > 0 &&
    canPrint &&
    !permsLoading,
  );
  const [leaderEpoch, setLeaderEpoch] = useState(0);
  const isLeader = usePrintLeader(active ? businessId : 0, () => {
    setLeaderEpoch((n) => n + 1);
  });
  const { print } = useIframePrint();

  const [clientId] = useState(() => getOrCreatePrintClientId());
  const [currentJob, setCurrentJob] = useState<PrintJob | null>(null);
  const [awaitingConfirm, setAwaitingConfirm] = useState(false);
  const [lastError, setLastError] = useState<string | null>(null);
  const [pendingCount, setPendingCount] = useState(0);
  const [online, setOnline] = useState(
    typeof navigator !== "undefined" ? navigator.onLine : true,
  );
  const [busy, setBusy] = useState(false);
  const [wakeEpoch, setWakeEpoch] = useState(0);
  const wake = useCallback(() => setWakeEpoch((value) => value + 1), []);
  const onPrintEvent = useCallback(
    (event: SSEEvent) => {
      if (!event.type.startsWith("print.")) return;
      if (Number(event.data.printer_id) !== stationPrinterId) return;
      wake();
    },
    [stationPrinterId, wake],
  );
  useSSEEvents({
    businessId,
    enabled: active,
    onEvent: onPrintEvent,
    onReconnect: wake,
  });
  const showTray =
    !online || pendingCount > 0 || currentJob !== null || lastError !== null;
  // leaderEpoch forces tray re-render when Web Lock leadership is acquired.
  void leaderEpoch;

  const abortRef = useRef<AbortController | null>(null);
  const backoffRef = useRef(IDLE_RECOVERY_POLL_MS);
  const processingRef = useRef(false);

  // Cleanup on unmount / business switch / logout.
  useEffect(() => {
    abortRef.current?.abort();
    abortRef.current = new AbortController();
    return () => {
      abortRef.current?.abort();
      abortRef.current = null;
      setCurrentJob(null);
      setAwaitingConfirm(false);
    };
  }, [businessId, active]);

  useEffect(() => {
    const onOnline = () => setOnline(true);
    const onOffline = () => setOnline(false);
    window.addEventListener("online", onOnline);
    window.addEventListener("offline", onOffline);
    return () => {
      window.removeEventListener("online", onOnline);
      window.removeEventListener("offline", onOffline);
    };
  }, []);

  const signal = () => abortRef.current?.signal;

  const presentJob = useCallback(
    async (job: PrintJob) => {
      if (!job.payload_html) {
        setLastError(t("agent.missingHtml"));
        return;
      }
      processingRef.current = true;
      setCurrentJob(job);
      setAwaitingConfirm(false);
      try {
        await print(job.payload_html);
        await markPresented(businessId, job.id, clientId, { signal: signal() });
        setAwaitingConfirm(true);
        setLastError(null);
      } catch (e) {
        if ((e as Error).name === "AbortError") return;
        setLastError((e as Error).message || t("agent.presentError"));
      } finally {
        processingRef.current = false;
      }
    },
    [businessId, clientId, print, t],
  );

  // Leader poll loop.
  useEffect(() => {
    if (!active || !clientId) return;

    let cancelled = false;
    let timer: ReturnType<typeof setTimeout> | null = null;

    const schedule = (ms: number) => {
      if (cancelled) return;
      timer = setTimeout(() => {
        void tick();
      }, ms);
    };

    const tick = async () => {
      if (cancelled) return;
      if (!isLeader.current) {
        schedule(IDLE_RECOVERY_POLL_MS);
        return;
      }
      if (!online || document.visibilityState === "hidden") {
        schedule(
          document.visibilityState === "hidden"
            ? HIDDEN_RECOVERY_POLL_MS
            : MAX_BACKOFF_MS,
        );
        return;
      }
      // Hold claiming while the operator confirms the current job. The lease
      // renew loop keeps it live; the next claim refreshes server queue depth.
      if (awaitingConfirm || processingRef.current || currentJob) {
        schedule(RENEW_MS);
        return;
      }

      try {
        const { job, pending_count } = await claimPrintJob(
          businessId,
          clientId,
          stationPrinterId,
          { signal: signal() },
        );
        if (cancelled) return;
        setPendingCount(pending_count);
        setLastError(null);
        backoffRef.current = IDLE_RECOVERY_POLL_MS;
        if (job) {
          await presentJob(job);
        }
        // The hidden branch returned above, so this successful claim path is
        // necessarily visible. Keeping the branch here confused TypeScript's
        // control-flow narrowing and made production typecheck fail.
        schedule(IDLE_RECOVERY_POLL_MS);
      } catch (e) {
        if ((e as Error).name === "AbortError") return;
        const msg = (e as Error).message || "";
        // 429 empty-poll throttle — back off quietly. The printers client
        // carries the status; the message is the backend's own wording.
        if (getApiErrorStatus(e) === 429 || msg.includes("429")) {
          backoffRef.current = Math.min(
            MAX_BACKOFF_MS,
            backoffRef.current * 1.5,
          );
        } else {
          setLastError(msg || t("agent.claimError"));
          backoffRef.current = Math.min(MAX_BACKOFF_MS, backoffRef.current * 2);
        }
        schedule(backoffRef.current);
      }
    };

    schedule(0);
    return () => {
      cancelled = true;
      if (timer) clearTimeout(timer);
    };
  }, [
    active,
    businessId,
    clientId,
    online,
    awaitingConfirm,
    currentJob,
    isLeader,
    presentJob,
    stationPrinterId,
    t,
    wakeEpoch,
  ]);

  // Lease renew while holding a job.
  useEffect(() => {
    if (!active || !currentJob || !clientId) return;
    const id = setInterval(() => {
      if (!isLeader.current) return;
      void renewPrintLease(businessId, currentJob.id, clientId, {
        signal: signal(),
      }).catch(() => {
        /* lease may have expired — next claim recovers */
      });
    }, RENEW_MS);
    return () => clearInterval(id);
  }, [active, businessId, clientId, currentJob, isLeader]);

  const onConfirm = async () => {
    if (!currentJob) return;
    setBusy(true);
    try {
      await confirmPrinted(businessId, currentJob.id, clientId, {
        signal: signal(),
      });
      setCurrentJob(null);
      setAwaitingConfirm(false);
      setLastError(null);
    } catch (e) {
      setLastError((e as Error).message || t("agent.confirmError"));
    } finally {
      setBusy(false);
    }
  };

  const onRetry = async () => {
    if (!currentJob) return;
    setBusy(true);
    try {
      await retryPrintJob(businessId, currentJob.id, clientId, {
        signal: signal(),
      });
      setCurrentJob(null);
      setAwaitingConfirm(false);
    } catch (e) {
      setLastError((e as Error).message || t("agent.retryError"));
    } finally {
      setBusy(false);
    }
  };

  const onCancel = async () => {
    if (!currentJob) return;
    setBusy(true);
    try {
      await agentCancelPrintJob(
        businessId,
        currentJob.id,
        clientId,
        "operator cancel",
        {
          signal: signal(),
        },
      );
      setCurrentJob(null);
      setAwaitingConfirm(false);
    } catch (e) {
      setLastError((e as Error).message || t("agent.cancelError"));
    } finally {
      setBusy(false);
    }
  };

  if (!active) return null;

  return (
    <>
      {showTray && (
        <PrintStationTray
          online={online && isLeader.current}
          isLeader={isLeader.current}
          pendingCount={pendingCount}
          currentJob={currentJob}
          lastError={lastError}
          statusLabel={
            currentJob ? statusLabel(currentJob.status) : t("agent.idle")
          }
          onDismissError={() => setLastError(null)}
          onReconnect={() => {
            setLastError(null);
            wake();
          }}
          onStopStation={() => onStopStation?.()}
        />
      )}

      {awaitingConfirm && currentJob && (
        <div
          role="dialog"
          aria-modal="false"
          aria-labelledby="print-confirm-title"
          className="fixed bottom-4 right-4 z-50 w-full max-w-sm rounded-2xl border border-warm-200 bg-white p-4 shadow-lg"
        >
          <h2
            id="print-confirm-title"
            className="font-serif text-lg text-ink-900"
          >
            {t("agent.confirmTitle")}
          </h2>
          <p className="mt-1 text-sm text-ink-600">{t("agent.confirmBody")}</p>
          <p className="mt-2 text-xs text-ink-500">
            {t("agent.jobMeta", {
              id: currentJob.id,
              kind: currentJob.kind,
              status: statusLabel(currentJob.status),
            })}
          </p>
          <div className="mt-4 flex flex-wrap gap-2">
            <button
              type="button"
              disabled={busy}
              onClick={() => void onConfirm()}
              className="rounded-xl bg-brand px-3 py-2 text-sm font-semibold text-white hover:bg-brand-dark disabled:opacity-50"
            >
              {t("agent.printed")}
            </button>
            <button
              type="button"
              disabled={busy}
              onClick={() => void onRetry()}
              className="rounded-xl border border-warm-300 bg-warm-50 px-3 py-2 text-sm font-semibold text-ink-800 hover:bg-warm-100 disabled:opacity-50"
            >
              {t("agent.retry")}
            </button>
            <button
              type="button"
              disabled={busy}
              onClick={() => void onCancel()}
              className="rounded-xl px-3 py-2 text-sm font-semibold text-rose-700 hover:bg-rose-50 disabled:opacity-50"
            >
              {t("agent.cancel")}
            </button>
          </div>
        </div>
      )}
    </>
  );
}
