"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { Button, Chip, Select, SelectItem, useDisclosure } from "@nextui-org/react";
import {
  Printer as PrinterIcon,
  RefreshCw,
  RotateCcw,
  Send,
  XCircle,
} from "lucide-react";
import toast from "react-hot-toast";

import {
  cancelJob,
  fetchJobs,
  formatPrintJobStatus,
  isTerminalPrintJobStatus,
  reprintJob,
  rerouteJob,
} from "@/api/print";
import type { PrintJob, Printer } from "@/api/print";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import ConfirmationModal from "@/components/business/modals/ConfirmationModal";
import { EmptyState } from "@/components/ui/EmptyState";
import {
  humanizeDurationMinutes,
  elapsedUrgency,
  type UrgencyTone,
} from "@/utils/humanizeDuration";
import { intlLocaleFor } from "@/utils/intlLocale";
import {
  DATE_SHORT,
  formatBusinessDateTime,
} from "@/utils/businessTime";
import { localDateKey } from "@/lib/localDate";
import { formatEntityName } from "@/lib/tableLabel";
import { useBrowserPrintStation } from "./useBrowserPrintStation";

interface Props {
  businessId: number;
  printers: Printer[];
}

const PAGE_SIZE = 20;

/** L3-45: age via humanizeDurationMinutes; tone via elapsedUrgency. */
export function printJobAgeMinutes(iso: string, nowMs: number): number {
  const then = Date.parse(iso);
  if (Number.isNaN(then)) return 0;
  return Math.max(0, Math.floor((nowMs - then) / 60000));
}

export function printJobAgeLabel(iso: string, nowMs: number): string {
  const then = Date.parse(iso);
  if (Number.isNaN(then)) return "—";
  const sec = Math.max(0, Math.floor((nowMs - then) / 1000));
  if (sec < 60) return `${sec}s`;
  return humanizeDurationMinutes(printJobAgeMinutes(iso, nowMs));
}

export function printJobAgeChipColor(
  iso: string,
  nowMs: number,
  status?: PrintJob["status"] | null,
): "default" | "warning" | "danger" | "success" {
  // #833: five kitchen tickets sat `routed` for 21h-74h on the demo venue and
  // the three OLDEST rendered the calmest. Every age past `calmAfterMinutes`
  // de-escalates to the neutral "stale" tone, which is honest for a job that
  // already resolved - its age is history - and a lie for one that never
  // printed, where the age IS the problem. Only a settled job gets the calm
  // band; an unknown status is treated as in-flight so the failure mode is
  // loud, not quiet.
  const settled = isTerminalPrintJobStatus(status);
  const tone: UrgencyTone = elapsedUrgency(printJobAgeMinutes(iso, nowMs), {
    warnAfterMinutes: 5,
    criticalAfterMinutes: 15,
    calmAfterMinutes: settled ? 24 * 60 : Number.POSITIVE_INFINITY,
  });
  switch (tone) {
    case "warn":
      return "warning";
    case "critical":
      return "danger";
    case "stale":
      return "default";
    default:
      return "success";
  }
}

/**
 * #833: the row's status chip rendered flat and colourless for every status, so
 * "Printed" and "Failed" looked identical and a job that never came out of the
 * printer read as ordinary history. Colour the outcome: a failure is loud, a
 * successful print is quiet-green, and anything still in the queue stays
 * neutral so only real outcomes claim attention.
 */
export function printJobStatusChipColor(
  status: PrintJob["status"] | null | undefined,
): "default" | "warning" | "danger" | "success" {
  switch (status) {
    case "printed":
      return "success";
    case "failed":
    case "failed_permanent":
      return "danger";
    case "failed_retryable":
      return "warning";
    default:
      return "default";
  }
}

/**
 * #833: after this many minutes a queued/pending browser job is no longer
 * plain "Queued" — the row gets a stalled marker and, when no station is
 * armed on this browser, the list warns the operator. Matches the intent of
 * the backend BrowserStationHealth SLA (jobs sat 33m–1d+ silently in QA).
 */
const STALE_QUEUED_SLA_MINUTES = 10;

function isStaleQueuedJob(
  job: Pick<PrintJob, "status" | "created_at" | "printer_id">,
  browserPrinterIds: ReadonlySet<number>,
  nowMs: number,
): boolean {
  if (job.printer_id == null || !browserPrinterIds.has(job.printer_id)) {
    return false;
  }
  if (job.status !== "routed" && job.status !== "pending") return false;
  return printJobAgeMinutes(job.created_at, nowMs) >= STALE_QUEUED_SLA_MINUTES;
}

function canCancel(status: PrintJob["status"]): boolean {
  return !isTerminalPrintJobStatus(status);
}

function canReprint(job: PrintJob): boolean {
  // Match the backend recovery boundary: a job that can still print or retry
  // must not offer a second physical copy.
  return (
    (job.kind === "bill" || job.kind === "receipt") &&
    isTerminalPrintJobStatus(job.status)
  );
}

function isUnassignedJob(job: PrintJob): boolean {
  return (
    job.printer_id == null &&
    (job.status === "pending" || job.status === "failed_retryable")
  );
}

function dayKey(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return "unknown";
  return localDateKey(date);
}

export default function RecentPrintJobs({ businessId, printers }: Props) {
  const { locale } = useSimpleLocale();
  const t = useCallback(
    (key: string, params?: Record<string, string | number>): string => {
      const result = getTranslation(`printers.${key}`, locale, params);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [locale],
  );

  const printerNameById = useMemo(() => {
    const map = new Map<number, string>();
    for (const p of printers) map.set(p.id, p.name);
    return map;
  }, [printers]);

  // #833: which printers rely on a browser tab claiming their jobs.
  const browserPrinterIds = useMemo(() => {
    const ids = new Set<number>();
    for (const p of printers) {
      if (p.transport === "browser" && p.enabled) ids.add(p.id);
    }
    return ids;
  }, [printers]);

  const { printerId: stationPrinterId } = useBrowserPrintStation(businessId);

  const hasBillPrinter = printers.some(
    (printer) => printer.enabled && printer.role === "bill",
  );

  const [jobs, setJobs] = useState<PrintJob[] | null>(null);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(0);
  const [statusFilter, setStatusFilter] = useState<string>("all");
  const [kindFilter, setKindFilter] = useState<string>("all");
  const [error, setError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [busyId, setBusyId] = useState<number | null>(null);
  const [nowMs, setNowMs] = useState(() => Date.now());
  const [jobToCancel, setJobToCancel] = useState<PrintJob | null>(null);
  const {
    isOpen: isCancelOpen,
    onOpen: onCancelOpen,
    onOpenChange: onCancelOpenChange,
  } = useDisclosure();

  const load = useCallback(async () => {
    setError(null);
    try {
      const query: Parameters<typeof fetchJobs>[1] = {
        limit: PAGE_SIZE,
        offset: page * PAGE_SIZE,
      };
      if (statusFilter !== "all") query.status = statusFilter;
      if (kindFilter !== "all") query.kind = kindFilter;
      const { items, total: nextTotal } = await fetchJobs(businessId, query);
      setJobs(items);
      setTotal(typeof nextTotal === "number" ? nextTotal : items.length);
      setNowMs(Date.now());
    } catch (e) {
      setError((e as Error).message);
    }
  }, [businessId, kindFilter, page, statusFilter]);

  useEffect(() => {
    void load();
  }, [load]);

  useEffect(() => {
    setPage(0);
  }, [statusFilter, kindFilter]);

  const handleCancelRequest = useCallback(
    (job: PrintJob) => {
      setJobToCancel(job);
      onCancelOpen();
    },
    [onCancelOpen],
  );

  const confirmCancel = useCallback(async () => {
    if (!jobToCancel) return;
    setActionError(null);
    setBusyId(jobToCancel.id);
    try {
      await cancelJob(businessId, jobToCancel.id);
      await load();
      toast.success(t("jobs.cancelSuccess"));
    } catch (e) {
      setActionError((e as Error).message);
    } finally {
      setBusyId(null);
      setJobToCancel(null);
    }
  }, [businessId, jobToCancel, load, t]);

  const handleReprint = useCallback(
    async (job: PrintJob) => {
      setActionError(null);
      setBusyId(job.id);
      try {
        await reprintJob(businessId, job.id);
        await load();
        toast.success(t("jobs.reprintSuccess"));
      } catch (e) {
        setActionError((e as Error).message);
        toast.error(t("jobs.reprintFailed"));
      } finally {
        setBusyId(null);
      }
    },
    [businessId, load, t],
  );

  const handleReroute = useCallback(
    async (job: PrintJob) => {
      setActionError(null);
      setBusyId(job.id);
      try {
        await rerouteJob(businessId, job.id);
        await load();
        toast.success(t("jobs.rerouteSuccess"));
      } catch (e) {
        setActionError((e as Error).message);
        toast.error(t("jobs.rerouteFailed"));
      } finally {
        setBusyId(null);
      }
    },
    [businessId, load, t],
  );

  const jobLabel = useCallback(
    (job: PrintJob) => {
      const kindLabel = t(`kindLabels.${job.kind}`);
      const parts = [t("jobs.jobLabelShort", { id: job.id, kind: kindLabel })];
      if (job.table_name) {
        // #825: location names are stored WITH the type word ("Table 1",
        // "Counter 3") — pick the locale word matching the entity so a
        // counter never renders as "Mesa 3", and let formatEntityName dedupe
        // instead of composing "Table Table 1". Custom names ("Patio A")
        // keep their stored identity without a type-word prefix.
        const trimmed = job.table_name.trim();
        const seed = /^(table|counter)\s+\d+$/i.exec(trimmed);
        if (seed) {
          const word =
            seed[1].toLowerCase() === "counter"
              ? t("jobs.counterWord")
              : t("jobs.tableWord");
          parts.push(formatEntityName(word, trimmed));
        } else {
          parts.push(trimmed);
        }
      }
      if (job.bill_number) {
        parts.push(t("jobs.billLabel", { number: job.bill_number }));
      } else if (job.source_type === "bill" || job.source_type === "reprint") {
        parts.push(t("jobs.billIdLabel", { id: job.source_id }));
      }
      if (job.source_type === "order" || job.order_id) {
        parts.push(
          t("jobs.orderLabel", {
            id: job.order_id ?? job.source_id,
          }),
        );
      }
      return parts.join(" · ");
    },
    [t],
  );

  const groupedJobs = useMemo(() => {
    if (!jobs) return [];
    const groups: { key: string; label: string; jobs: PrintJob[] }[] = [];
    const byDay = new Map<string, PrintJob[]>();
    for (const job of jobs) {
      const key = dayKey(job.created_at);
      const list = byDay.get(key) ?? [];
      list.push(job);
      byDay.set(key, list);
    }
    for (const [key, dayJobs] of byDay) {
      const label =
        key === "unknown"
          ? t("jobs.unknownDay")
          : formatBusinessDateTime(
              new Date(`${key}T12:00:00.000Z`),
              intlLocaleFor(locale),
              null,
              DATE_SHORT,
            );
      groups.push({ key, label, jobs: dayJobs });
    }
    return groups;
  }, [jobs, locale, t]);

  const pageCount = Math.max(1, Math.ceil(total / PAGE_SIZE));

  // #833: browser printers with stale queued jobs whose station is NOT armed
  // on this browser. localStorage is per-browser, so another device may still
  // be the station — the copy says so instead of claiming the queue is dead.
  const staleUnarmedPrinterNames = useMemo(() => {
    if (!jobs) return [] as string[];
    const names = new Set<string>();
    for (const job of jobs) {
      if (!isStaleQueuedJob(job, browserPrinterIds, nowMs)) continue;
      if (job.printer_id === stationPrinterId) continue;
      names.add(
        printerNameById.get(job.printer_id as number) ??
          t("jobs.unknownPrinter"),
      );
    }
    return [...names];
  }, [browserPrinterIds, jobs, nowMs, printerNameById, stationPrinterId, t]);

  return (
    <section className="space-y-3" aria-labelledby="recent-print-jobs-heading">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h2
            id="recent-print-jobs-heading"
            className="text-base font-semibold text-ink-900"
          >
            {t("jobs.title")}
          </h2>
          <p className="text-sm text-ink-600">{t("jobs.subtitle")}</p>
        </div>
        <Button
          size="sm"
          variant="flat"
          startContent={
            <RefreshCw className="h-3.5 w-3.5" aria-hidden="true" />
          }
          onPress={() => void load()}
        >
          {t("jobs.refresh")}
        </Button>
      </div>

      <div className="grid gap-3 sm:grid-cols-2">
        <Select
          aria-label={t("jobs.filterStatus")}
          label={t("jobs.filterStatus")}
          size="sm"
          selectedKeys={new Set([statusFilter])}
          onSelectionChange={(keys) => {
            const value = Array.from(keys as Set<string>)[0];
            if (value) setStatusFilter(value);
          }}
        >
          <SelectItem key="all">{t("jobs.filters.allStatuses")}</SelectItem>
          <SelectItem key="unassigned">{t("jobs.filters.unassigned")}</SelectItem>
          <SelectItem key="pending">{t("status.pending")}</SelectItem>
          <SelectItem key="routed">{t("status.queued")}</SelectItem>
          <SelectItem key="printing">{t("status.printing")}</SelectItem>
          <SelectItem key="printed">{t("status.printed")}</SelectItem>
          <SelectItem key="failed">{t("status.failed")}</SelectItem>
          <SelectItem key="failed_retryable">
            {t("status.failedRetryable")}
          </SelectItem>
          {/* #833: the backend emits failed_permanent; without this option a
              permanently-failed job is reachable only under "All statuses". */}
          <SelectItem key="failed_permanent">
            {t("status.failedPermanent")}
          </SelectItem>
          <SelectItem key="cancelled">{t("status.cancelled")}</SelectItem>
        </Select>
        <Select
          aria-label={t("jobs.filterKind")}
          label={t("jobs.filterKind")}
          size="sm"
          selectedKeys={new Set([kindFilter])}
          onSelectionChange={(keys) => {
            const value = Array.from(keys as Set<string>)[0];
            if (value) setKindFilter(value);
          }}
        >
          <SelectItem key="all">{t("jobs.filters.allKinds")}</SelectItem>
          <SelectItem key="receipt">{t("kindLabels.receipt")}</SelectItem>
          <SelectItem key="bill">{t("kindLabels.bill")}</SelectItem>
          <SelectItem key="kitchen">{t("kindLabels.kitchen")}</SelectItem>
          <SelectItem key="bar">{t("kindLabels.bar")}</SelectItem>
        </Select>
      </div>

      {staleUnarmedPrinterNames.length > 0 && (
        <div
          role="alert"
          data-testid="print-station-offline-banner"
          className="rounded-lg border border-amber-300 bg-amber-50 px-4 py-3 text-sm text-amber-900"
        >
          {t("jobs.stationOfflineBanner", {
            printers: staleUnarmedPrinterNames.join(", "),
          })}
        </div>
      )}

      {(error || actionError) && (
        <div
          role="alert"
          className="rounded-lg border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-800"
        >
          {t("loadError", { error: error ?? actionError ?? "" })}
        </div>
      )}

      {jobs === null && !error ? (
        <div className="space-y-2 rounded-lg border border-warm-200 bg-white p-4">
          <div className="h-4 w-40 animate-pulse rounded bg-warm-200" />
          <div className="h-12 animate-pulse rounded-lg bg-warm-100" />
          <div className="h-12 animate-pulse rounded-lg bg-warm-100" />
          <span className="sr-only">{t("jobs.loading")}</span>
        </div>
      ) : jobs && jobs.length === 0 ? (
        <EmptyState
          icon={PrinterIcon}
          title={t("jobs.emptyTitle")}
          subtitle={t("jobs.emptyBody")}
          panel
          compact
        />
      ) : jobs ? (
        <div className="space-y-4">
          {groupedJobs.map((group) => (
            <div
              key={group.key}
              className="overflow-hidden rounded-lg border border-warm-200 bg-white"
            >
              <div className="border-b border-warm-200 bg-warm-50/70 px-4 py-2 text-xs font-semibold uppercase tracking-[0.14em] text-ink-600">
                {group.label}
              </div>
              <ul className="divide-y divide-warm-200">
                {group.jobs.map((job) => {
                  const statusKey = formatPrintJobStatus(job.status);
                  const unassigned = isUnassignedJob(job);
                  const staleQueued = isStaleQueuedJob(
                    job,
                    browserPrinterIds,
                    nowMs,
                  );
                  const printerLabel = unassigned
                    ? t("jobs.unassignedPrinter")
                    : job.printer_id != null
                      ? (printerNameById.get(job.printer_id) ??
                        t("jobs.unknownPrinter"))
                      : t("jobs.unassignedPrinter");
                  return (
                    <li
                      key={job.id}
                      className="flex flex-col gap-3 p-4 sm:flex-row sm:items-center sm:justify-between"
                    >
                      <div className="min-w-0 space-y-1">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="font-medium text-ink-900">
                            {jobLabel(job)}
                          </span>
                          <Chip
                            size="sm"
                            variant="flat"
                            color={printJobStatusChipColor(job.status)}
                            data-testid={`job-status-${job.id}`}
                          >
                            {t(`status.${statusKey}`)}
                          </Chip>
                          {staleQueued ? (
                            <Chip
                              size="sm"
                              variant="flat"
                              color="warning"
                              data-testid={`job-stalled-${job.id}`}
                            >
                              {t("jobs.stalledQueued")}
                            </Chip>
                          ) : null}
                          {unassigned ? (
                            <Chip size="sm" variant="flat" color="warning">
                              {t("jobs.needsPrinter")}
                            </Chip>
                          ) : null}
                        </div>
                        <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm text-ink-500">
                          <span>{printerLabel}</span>
                          <Chip
                            size="sm"
                            variant="flat"
                            color={printJobAgeChipColor(
                              job.created_at,
                              nowMs,
                              job.status,
                            )}
                            data-testid={`job-age-${job.id}`}
                          >
                            {printJobAgeLabel(job.created_at, nowMs)}
                          </Chip>
                        </div>
                        {unassigned ? (
                          <p className="text-sm text-amber-800">
                            {hasBillPrinter
                              ? t("jobs.unassignedRecoveryHint")
                              : t("jobs.unassignedAddPrinterHint")}
                          </p>
                        ) : null}
                        {job.last_error ? (
                          <p className="text-sm text-rose-700">
                            {job.last_error}
                          </p>
                        ) : null}
                      </div>
                      <div className="flex flex-wrap gap-2 sm:justify-end">
                        {unassigned && hasBillPrinter ? (
                          <Button
                            size="sm"
                            color="primary"
                            variant="flat"
                            startContent={
                              <Send className="h-3.5 w-3.5" aria-hidden="true" />
                            }
                            isLoading={busyId === job.id}
                            isDisabled={busyId !== null && busyId !== job.id}
                            onPress={() => void handleReroute(job)}
                          >
                            {t("jobs.assignPrinter")}
                          </Button>
                        ) : null}
                        {unassigned && !hasBillPrinter ? (
                          <Button
                            size="sm"
                            variant="bordered"
                            onPress={() => {
                              document
                                .querySelector<HTMLButtonElement>(
                                  "[data-testid='printers-add-button']",
                                )
                                ?.click();
                            }}
                          >
                            {t("jobs.addPrinterCta")}
                          </Button>
                        ) : null}
                        {canCancel(job.status) && (
                          <Button
                            size="sm"
                            variant="flat"
                            color="danger"
                            startContent={
                              <XCircle
                                className="h-3.5 w-3.5"
                                aria-hidden="true"
                              />
                            }
                            isLoading={busyId === job.id}
                            isDisabled={busyId !== null && busyId !== job.id}
                            onPress={() => handleCancelRequest(job)}
                          >
                            {t("jobs.cancel")}
                          </Button>
                        )}
                        {canReprint(job) && (
                          <Button
                            size="sm"
                            variant="flat"
                            startContent={
                              <RotateCcw
                                className="h-3.5 w-3.5"
                                aria-hidden="true"
                              />
                            }
                            isLoading={busyId === job.id}
                            isDisabled={busyId !== null && busyId !== job.id}
                            onPress={() => void handleReprint(job)}
                          >
                            {t("jobs.reprint")}
                          </Button>
                        )}
                      </div>
                    </li>
                  );
                })}
              </ul>
            </div>
          ))}

          {total > PAGE_SIZE ? (
            <div className="flex flex-wrap items-center justify-between gap-3">
              <span className="text-sm text-ink-600">
                {t("jobs.pageRange", {
                  from: page * PAGE_SIZE + 1,
                  to: page * PAGE_SIZE + jobs.length,
                  total,
                })}
              </span>
              <div className="flex items-center gap-2">
                <Button
                  size="sm"
                  variant="bordered"
                  isDisabled={page <= 0}
                  onPress={() => setPage((p) => Math.max(0, p - 1))}
                >
                  {t("jobs.previous")}
                </Button>
                <span className="text-sm text-ink-600">
                  {t("jobs.pageOf", { page: page + 1, pages: pageCount })}
                </span>
                <Button
                  size="sm"
                  variant="bordered"
                  isDisabled={(page + 1) * PAGE_SIZE >= total}
                  onPress={() => setPage((p) => p + 1)}
                >
                  {t("jobs.next")}
                </Button>
              </div>
            </div>
          ) : null}
        </div>
      ) : null}

      <ConfirmationModal
        isOpen={isCancelOpen}
        onOpenChange={onCancelOpenChange}
        title={t("jobs.cancelConfirmTitle")}
        description={t("jobs.cancelConfirmBody", {
          id: jobToCancel?.id ?? "",
        })}
        confirmLabel={t("jobs.cancel")}
        isDanger
        onConfirm={() => {
          void confirmCancel();
        }}
      />
    </section>
  );
}
