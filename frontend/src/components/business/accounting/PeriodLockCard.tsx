"use client";

import React, { useCallback, useEffect, useState } from "react";
import { Input, useDisclosure } from "@nextui-org/react";
import { Lock } from "lucide-react";
import { accountingApi } from "@/api/accounting";
import ConfirmationModal from "@/components/business/modals/ConfirmationModal";
import {
  accountingPrimaryButtonClass,
  accountingSecondaryButtonClass,
  formatDateInput,
} from "./accountingShared";

/** Earliest start used when counting entries frozen by a close-through date. */
const BOOKS_ORIGIN = "2000-01-01";

export default function PeriodLockCard({
  businessId,
  canOwn,
  t,
}: {
  businessId: string;
  /** Owner-only POST. */
  canOwn: boolean;
  t: (key: string, params?: Record<string, string | number>) => string;
}) {
  const [lockedThrough, setLockedThrough] = useState<string | null>(null);
  const [closeDate, setCloseDate] = useState(formatDateInput(new Date()));
  const [note, setNote] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [freezeCount, setFreezeCount] = useState<number | null>(null);
  const [freezeLoading, setFreezeLoading] = useState(false);
  const {
    isOpen: isCloseConfirmOpen,
    onOpen: onCloseConfirmOpen,
    onOpenChange: onCloseConfirmOpenChange,
  } = useDisclosure();

  const load = useCallback(async () => {
    try {
      const data = await accountingApi.getPeriodLock(businessId);
      setLockedThrough(data.locked_through);
      setError(null);
    } catch {
      setError(t("periodLock.loadError"));
    }
  }, [businessId, t]);

  useEffect(() => {
    void load();
  }, [load]);

  // Freeze preview: count active ledger entries that will be locked through closeDate.
  useEffect(() => {
    if (!canOwn || !closeDate) {
      setFreezeCount(null);
      return;
    }
    let cancelled = false;
    setFreezeLoading(true);
    void accountingApi
      .listEntries(businessId, {
        start: BOOKS_ORIGIN,
        end: closeDate,
        status: "active",
        page: 1,
        page_size: 1,
      })
      .then((page) => {
        if (!cancelled) setFreezeCount(page.total ?? 0);
      })
      .catch(() => {
        if (!cancelled) setFreezeCount(null);
      })
      .finally(() => {
        if (!cancelled) setFreezeLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [businessId, canOwn, closeDate]);

  const noteReady = note.trim().length > 0;

  const submit = async (through: string | null) => {
    // Backstop: keep trim() validation even if the button is force-enabled.
    if (!note.trim()) {
      setError(t("periodLock.noteRequired"));
      return;
    }
    setBusy(true);
    setError(null);
    try {
      const data = await accountingApi.postPeriodLock(businessId, {
        locked_through: through,
        note: note.trim(),
      });
      setLockedThrough(data.locked_through);
      setNote("");
    } catch {
      setError(t("periodLock.saveError"));
    } finally {
      setBusy(false);
    }
  };

  // Close books: never pass empty date (that would reopen). Confirm first (L6-24).
  const requestCloseBooks = () => {
    if (!note.trim()) {
      setError(t("periodLock.noteRequired"));
      return;
    }
    if (!closeDate.trim()) {
      setError(t("periodLock.dateRequired"));
      return;
    }
    setError(null);
    onCloseConfirmOpen();
  };

  const confirmCloseBooks = () => submit(closeDate.trim());

  return (
    <>
    <div
      className="rounded-2xl border border-warm-200/80 bg-white p-4 shadow-sm"
      data-testid="period-lock-card"
    >
      <div className="flex items-start gap-3">
        <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-warm-100 text-ink-700">
          <Lock className="h-4 w-4" aria-hidden />
        </span>
        <div className="min-w-0 flex-1 space-y-2">
          <h3 className="text-sm font-semibold text-ink-900">
            {t("periodLock.title")}
          </h3>
          <p className="text-sm text-ink-600">
            {lockedThrough
              ? t("periodLock.closedThrough", { date: lockedThrough })
              : t("periodLock.open")}
          </p>
          {error ? (
            <p role="alert" className="text-sm text-rose-700">
              {error}
            </p>
          ) : null}
          {canOwn ? (
            <div
              // flex gap, not space-y: sibling margin-top would override the
              // NextUI outside-label headroom and clip the labels.
              className="flex flex-col gap-2 pt-1"
            >
              <Input
                type="date"
                label={t("periodLock.closeDate")}
                labelPlacement="outside"
                placeholder=" "
                value={closeDate}
                onValueChange={setCloseDate}
                variant="bordered"
                classNames={{
                  inputWrapper: "h-10 min-h-10 border-warm-200 bg-white",
                  label:
                    "text-xs font-semibold uppercase tracking-wide text-ink-500",
                }}
              />
              <Input
                type="text"
                label={t("periodLock.note")}
                labelPlacement="outside"
                placeholder=" "
                value={note}
                onValueChange={setNote}
                isRequired
                variant="bordered"
                classNames={{
                  inputWrapper: "h-10 min-h-10 border-warm-200 bg-white",
                  label:
                    "text-xs font-semibold uppercase tracking-wide text-ink-500",
                }}
              />
              {/* Freeze preview — informed consent before irreversible close. */}
              <p
                data-testid="period-lock-freeze-preview"
                className="rounded-xl border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-950"
              >
                {freezeLoading
                  ? t("periodLock.freezePreviewLoading")
                  : t("periodLock.freezePreview", {
                      count: freezeCount ?? 0,
                      end: closeDate || "—",
                      start: BOOKS_ORIGIN,
                    })}
              </p>
              <div className="flex flex-wrap gap-2">
                <button
                  type="button"
                  className={accountingPrimaryButtonClass}
                  disabled={busy || !noteReady}
                  onClick={requestCloseBooks}
                  data-testid="period-lock-close"
                >
                  {t("periodLock.closeBooks")}
                </button>
                {lockedThrough ? (
                  <button
                    type="button"
                    className={accountingSecondaryButtonClass}
                    disabled={busy || !noteReady}
                    onClick={() => void submit(null)}
                    data-testid="period-lock-reopen"
                  >
                    {t("periodLock.reopen")}
                  </button>
                ) : null}
              </div>
            </div>
          ) : null}
        </div>
      </div>
    </div>

    <ConfirmationModal
      isOpen={isCloseConfirmOpen}
      onOpenChange={onCloseConfirmOpenChange}
      isDanger
      title={t("periodLock.confirmCloseTitle")}
      description={t("periodLock.freezePreview", {
        count: freezeCount ?? 0,
        end: closeDate || "—",
        start: BOOKS_ORIGIN,
      })}
      confirmLabel={t("periodLock.confirmCloseAction")}
      onConfirm={() => confirmCloseBooks()}
    />
    </>
  );
}
