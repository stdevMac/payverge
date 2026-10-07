"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { Button, Chip, useDisclosure } from "@nextui-org/react";
import { Pencil, Plus, Power, PowerOff, Printer as PrinterIcon } from "lucide-react";

import { deletePrinter, fetchPrinters, testPrint, updatePrinter } from "@/api/print";
import type { Printer } from "@/api/print";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { resolveNumericBusinessId } from "@/utils/resolveBusinessId";
import DashboardTabShell from "@/components/business/shared/DashboardTabShell";
import ConfirmationModal from "@/components/business/modals/ConfirmationModal";
import { EmptyState } from "@/components/ui/EmptyState";

import AddPrinterWizard from "./AddPrinterWizard";
import EditPrinterModal from "./EditPrinterModal";
import RecentPrintJobs from "./RecentPrintJobs";
import { useIframePrint } from "./useIframePrint";
import { useBrowserPrintStation } from "./useBrowserPrintStation";

interface Props {
  /** Numeric business id, or null when only a slug is known (resolve client-side). */
  businessId: number | null;
  /** Raw route param — resolved to a numeric id when `businessId` is null. */
  slug?: string;
}

export default function PrintersSettings({ businessId, slug }: Props) {
  const { locale } = useSimpleLocale();

  // The route param under /business/[businessId] can be a slug. Resolve it to a
  // numeric id before any fetch so we never call /businesses/NaN/printers.
  const [resolvedId, setResolvedId] = useState<number | null>(businessId);
  // Distinguish "still resolving the slug" from "resolved to nothing". Without
  // this, a failed slug lookup (resolvedId stays null) would show an eternal
  // loading spinner instead of a clear "business not found" state.
  const [resolutionFailed, setResolutionFailed] = useState(false);

  useEffect(() => {
    if (resolvedId !== null || !slug) return;
    let cancelled = false;
    void resolveNumericBusinessId(slug).then((id) => {
      if (cancelled) return;
      if (id === null) setResolutionFailed(true);
      else setResolvedId(id);
    });
    return () => {
      cancelled = true;
    };
  }, [resolvedId, slug]);
  const t = useCallback(
    (key: string, params?: Record<string, string | number>): string => {
      const result = getTranslation(`printers.${key}`, locale, params);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [locale],
  );

  const { print } = useIframePrint();
  const { printerId: stationPrinterId, selectPrinter, stopStation } =
    useBrowserPrintStation(resolvedId);

  const [printers, setPrinters] = useState<Printer[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [wizardOpen, setWizardOpen] = useState(false);
  const [printerToEdit, setPrinterToEdit] = useState<Printer | null>(null);
  // Id of the printer whose Test Print round-trip is in flight. Disabling the
  // button while pending stops a slow round-trip from being double-tapped into
  // duplicate test jobs.
  const [testingId, setTestingId] = useState<number | null>(null);
  // Id of the printer whose Enable PATCH is in flight. Recovery is one-click
  // (not confirm) so this guard stops a double-tap from firing two updates.
  const [enablingId, setEnablingId] = useState<number | null>(null);

  // Soft-disable (backend deletePrinter sets enabled=false). Confirm first —
  // reversible, but it stops new jobs immediately.
  const [printerToDisable, setPrinterToDisable] = useState<Printer | null>(null);
  const {
    isOpen: isDisableOpen,
    onOpen: onDisableOpen,
    onOpenChange: onDisableOpenChange,
  } = useDisclosure();

  const shellHeader = {
    title: t("navTitle"),
    subtitle: t("subtitle"),
    actions: (
      <Button
        color="primary"
        data-testid="printers-add-button"
        startContent={<Plus className="h-4 w-4" aria-hidden="true" />}
        onPress={() => setWizardOpen(true)}
        isDisabled={resolvedId === null}
      >
        {t("addPrinter")}
      </Button>
    ),
  };

  const load = useCallback(async () => {
    if (resolvedId === null) return;
    setError(null);
    setActionError(null);
    try {
      const { items } = await fetchPrinters(resolvedId);
      setPrinters(items);
    } catch (e) {
      setError((e as Error).message);
    }
  }, [resolvedId]);

  useEffect(() => {
    void load();
  }, [load]);

  useEffect(() => {
    if (
      stationPrinterId !== null &&
      printers !== null &&
      !printers.some(
        (printer) =>
          printer.id === stationPrinterId &&
          printer.transport === "browser" &&
          printer.enabled,
      )
    ) {
      stopStation();
    }
  }, [printers, stationPrinterId, stopStation]);

  const handleTestPrint = useCallback(
    async (printer: Printer) => {
      if (resolvedId === null) return;
      // Guard against a double-tap while the previous round-trip is still open.
      if (testingId !== null) return;
      setActionError(null);
      setTestingId(printer.id);
      try {
        const job = await testPrint(resolvedId, printer.id);
        if (printer.transport === "browser" && job.payload_html) {
          await print(job.payload_html);
        }
        await load();
      } catch (e) {
        setActionError((e as Error).message);
      } finally {
        setTestingId(null);
      }
    },
    [load, print, resolvedId, testingId],
  );

  const handleDisableRequest = useCallback(
    (printer: Printer) => {
      if (!printer.enabled) return;
      setPrinterToDisable(printer);
      onDisableOpen();
    },
    [onDisableOpen],
  );

  const handleEnable = useCallback(
    async (printer: Printer) => {
      if (resolvedId === null || printer.enabled) return;
      if (enablingId !== null) return;
      setActionError(null);
      setEnablingId(printer.id);
      try {
        await updatePrinter(resolvedId, printer.id, { enabled: true });
        await load();
      } catch (e) {
        setActionError((e as Error).message);
      } finally {
        setEnablingId(null);
      }
    },
    [enablingId, load, resolvedId],
  );

  const confirmDisable = useCallback(async () => {
    if (resolvedId === null || printerToDisable === null) return;
    setActionError(null);
    try {
      if (stationPrinterId === printerToDisable.id) stopStation();
      await deletePrinter(resolvedId, printerToDisable.id);
      await load();
    } catch (e) {
      setActionError((e as Error).message);
    } finally {
      setPrinterToDisable(null);
    }
  }, [load, printerToDisable, resolvedId, stationPrinterId, stopStation]);

  if (error || resolutionFailed) {
    return (
      <DashboardTabShell header={shellHeader}>
        <div
          role="alert"
          className="rounded-2xl border border-rose-200 bg-rose-50 p-6"
        >
          <h2 className="font-semibold text-rose-900">
            {resolutionFailed ? t("businessNotFound") : t("errorTitle")}
          </h2>
          <p className="mt-1 text-sm text-rose-800">
            {resolutionFailed
              ? t("businessNotFoundDetail")
              : t("loadError", { error: error ?? "" })}
          </p>
          <div className="mt-4 flex flex-wrap gap-3">
            {!resolutionFailed && (
              <Button color="danger" onPress={() => void load()}>
                {t("retry")}
              </Button>
            )}
            <Button
              as={Link}
              href="/dashboard"
              variant="light"
              className="text-rose-700"
            >
              {t("backToDashboard")}
            </Button>
          </div>
        </div>
      </DashboardTabShell>
    );
  }
  if (printers === null) {
    return (
      <DashboardTabShell header={shellHeader}>
        <div className="rounded-lg border border-warm-200 bg-white p-5">
          <div className="h-4 w-36 animate-pulse rounded bg-warm-200" />
          <div className="mt-4 space-y-3">
            <div className="h-14 animate-pulse rounded-lg bg-warm-100" />
            <div className="h-14 animate-pulse rounded-lg bg-warm-100" />
            <div className="h-14 animate-pulse rounded-lg bg-warm-100" />
          </div>
          <span className="sr-only">{t("loading")}</span>
        </div>
      </DashboardTabShell>
    );
  }
  if (printers.length === 0) {
    return (
      <DashboardTabShell header={shellHeader}>
        <EmptyState
          icon={PrinterIcon}
          title={t("empty")}
          subtitle={t("subtitle")}
          panel
          action={
            <Button
              color="primary"
              startContent={<Plus className="h-4 w-4" aria-hidden="true" />}
              onPress={() => setWizardOpen(true)}
            >
              {t("addAPrinter")}
            </Button>
          }
        />
        <AddPrinterWizard
          businessId={resolvedId ?? 0}
          open={wizardOpen}
          onClose={() => setWizardOpen(false)}
          onCreated={() => {
            setWizardOpen(false);
            void load();
          }}
        />
      </DashboardTabShell>
    );
  }

  return (
    <DashboardTabShell header={shellHeader}>
      {actionError && (
        <div
          role="alert"
          className="rounded-lg border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-800"
        >
          {t("loadError", { error: actionError })}
        </div>
      )}
      <div className="rounded-lg border border-brand-200 bg-brand-50 px-4 py-3">
        <p className="text-sm font-semibold text-ink-900">
          {t("stationTitle")}
        </p>
        <p className="mt-1 text-sm text-ink-600">{t("stationHint")}</p>
      </div>
      <div className="overflow-hidden rounded-lg border border-warm-200 bg-white">
        {printers.map((p, index) => (
          <div
            key={p.id}
            className={`flex flex-col gap-4 p-4 sm:flex-row sm:items-center sm:justify-between ${
              index > 0 ? "border-t border-warm-200" : ""
            }`}
          >
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <div className="truncate font-semibold text-ink-900">{p.name}</div>
                {!p.enabled && (
                  <Chip size="sm" variant="flat" color="warning">
                    {t("disabledBadge")}
                  </Chip>
                )}
                {p.transport === "browser" &&
                  p.enabled &&
                  stationPrinterId === p.id && (
                    <Chip
                      size="sm"
                      variant="flat"
                      color="success"
                      data-testid={`printer-active-${p.id}`}
                    >
                      {t("activeOnThisBrowser")}
                    </Chip>
                  )}
              </div>
              <div className="mt-2 flex flex-wrap items-center gap-2 text-sm text-ink-500">
                <Chip size="sm" variant="flat">
                  {t(`roleLabels.${p.role}`)}
                </Chip>
                <Chip size="sm" variant="flat">
                  {t(`transportLabels.${p.transport}`)}
                </Chip>
                <span>{p.paper_width_mm}mm</span>
              </div>
            </div>
            <div className="flex flex-wrap gap-2 sm:justify-end">
              {p.transport === "browser" && p.enabled && (
                stationPrinterId === p.id ? (
                  <Button
                    size="sm"
                    variant="bordered"
                    color="danger"
                    onPress={stopStation}
                  >
                    {t("stopStation")}
                  </Button>
                ) : (
                  <Button
                    size="sm"
                    variant="bordered"
                    onPress={() => selectPrinter(p.id)}
                  >
                    {t("useOnThisBrowser")}
                  </Button>
                )
              )}
              <Button
                size="sm"
                variant="flat"
                onPress={() => void handleTestPrint(p)}
                isLoading={testingId === p.id}
                isDisabled={
                  !p.enabled || (testingId !== null && testingId !== p.id)
                }
              >
                {t("testPrint")}
              </Button>
              <Button
                isIconOnly
                aria-label={`${t("edit.action")} ${p.name}`}
                size="sm"
                variant="light"
                onPress={() => setPrinterToEdit(p)}
              >
                <Pencil className="h-4 w-4" aria-hidden="true" />
              </Button>
              {p.enabled ? (
                <Button
                  size="sm"
                  variant="bordered"
                  color="danger"
                  aria-label={`${t("disable")} ${p.name}`}
                  startContent={
                    <PowerOff className="h-3.5 w-3.5" aria-hidden="true" />
                  }
                  onPress={() => handleDisableRequest(p)}
                >
                  {t("disable")}
                </Button>
              ) : (
                <Button
                  size="sm"
                  variant="bordered"
                  aria-label={`${t("enable")} ${p.name}`}
                  startContent={
                    <Power className="h-3.5 w-3.5" aria-hidden="true" />
                  }
                  onPress={() => void handleEnable(p)}
                  isLoading={enablingId === p.id}
                  isDisabled={enablingId !== null && enablingId !== p.id}
                >
                  {t("enable")}
                </Button>
              )}
            </div>
          </div>
        ))}
      </div>
      {resolvedId !== null && (
        <RecentPrintJobs businessId={resolvedId} printers={printers} />
      )}
      <AddPrinterWizard
        businessId={resolvedId ?? 0}
        open={wizardOpen}
        onClose={() => setWizardOpen(false)}
        onCreated={() => {
          setWizardOpen(false);
          void load();
        }}
      />
      <EditPrinterModal
        businessId={resolvedId ?? 0}
        printer={printerToEdit}
        open={printerToEdit !== null}
        onClose={() => setPrinterToEdit(null)}
        onSaved={() => {
          setPrinterToEdit(null);
          void load();
        }}
      />
      <ConfirmationModal
        isOpen={isDisableOpen}
        onOpenChange={onDisableOpenChange}
        title={t("disableConfirmTitle")}
        description={t("disableConfirmBody", {
          name: printerToDisable?.name ?? "",
        })}
        confirmLabel={t("disable")}
        isDanger
        onConfirm={() => {
          void confirmDisable();
        }}
      />
    </DashboardTabShell>
  );
}
