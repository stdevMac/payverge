"use client";

import React, { useCallback, useMemo, useState } from "react";
import type { Locale } from "@/i18n/localeRegistry";
import type { ManualLedgerEntry } from "@/api/accounting";
import { useVoidEntry } from "@/hooks/accounting/useAccountingQueries";
import {
  formatDay,
  formatMoney,
} from "@/components/business/accounting/format";
import { AnimatedNumberText } from "../premium";
import DetailDrawer from "../shared/DetailDrawer";
import ConfirmationModal from "../modals/ConfirmationModal";
import StatusBadge from "./StatusBadge";
import {
  accountingPrimaryButtonClass,
  accountingSecondaryButtonClass,
  formatCategoryLabel,
} from "./accountingShared";
import EntryAttachments from "./EntryAttachments";
import {
  isPeriodLockedError,
  periodLockedMessage,
} from "./periodLockErrors";

export type EntryDetailDrawerProps = {
  open: boolean;
  entry: ManualLedgerEntry | null;
  businessId: string;
  locale: Locale;
  currency: string;
  /** When false, hide Void / Void & duplicate. Duplicate remains available. */
  canWrite?: boolean;
  onClose: () => void;
  onDuplicate: (entry: ManualLedgerEntry) => void;
  t: (key: string, params?: Record<string, string | number>) => string;
  tWith?: (
    key: string,
    replacements: Record<string, string | number>,
  ) => string;
};

type PendingVoidAction = "void" | "voidAndDuplicate" | null;

function actorName(
  staff?: ManualLedgerEntry["created_by_staff"] | ManualLedgerEntry["voided_by_staff"],
  user?: ManualLedgerEntry["created_by_user"],
): string {
  const staffName = staff?.name?.trim();
  if (staffName) return staffName;
  const userName = user?.name?.trim() || user?.email?.trim();
  return userName || "—";
}

function DefinitionRow({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="grid grid-cols-[7.5rem_1fr] gap-x-3 gap-y-1 text-sm sm:grid-cols-[9rem_1fr]">
      <dt className="text-ink-500">{label}</dt>
      <dd className="min-w-0 text-ink-900">{children}</dd>
    </div>
  );
}

export default function EntryDetailDrawer({
  open,
  entry,
  businessId,
  locale,
  currency,
  canWrite = true,
  onClose,
  onDuplicate,
  t,
  tWith,
}: EntryDetailDrawerProps) {
  const voidMutation = useVoidEntry(businessId);
  const [pendingVoid, setPendingVoid] = useState<PendingVoidAction>(null);
  const [voidError, setVoidError] = useState<string | null>(null);

  const interpolate = useCallback(
    (key: string, replacements: Record<string, string | number>) => {
      if (tWith) return tWith(key, replacements);
      let value = t(key);
      Object.entries(replacements).forEach(([name, replacement]) => {
        value = value.replace(
          new RegExp(`\\{${name}\\}`, "g"),
          String(replacement),
        );
      });
      return value;
    },
    [t, tWith],
  );

  const isVoided = Boolean(entry?.voided_at);
  const showVoidActions = canWrite && !isVoided && !!entry;

  const amountClass = useMemo(() => {
    if (!entry) return "text-ink-900";
    return entry.entry_type === "income"
      ? "text-brand-700"
      : "text-warm-700";
  }, [entry]);

  const handleConfirmVoid = useCallback(() => {
    if (!entry || !pendingVoid) return;
    const action = pendingVoid;
    const target = entry;
    setPendingVoid(null);
    setVoidError(null);
    voidMutation.mutate(target.id, {
      onSuccess: () => {
        if (action === "voidAndDuplicate") {
          onDuplicate(target);
        }
        onClose();
      },
      onError: (err) => {
        setVoidError(
          isPeriodLockedError(err)
            ? periodLockedMessage(err, t)
            : t("errors.saveEntry"),
        );
      },
    });
  }, [entry, pendingVoid, voidMutation, onDuplicate, onClose, t]);

  const footer =
    entry == null ? null : (
      <div className="flex w-full flex-wrap items-center justify-end gap-2">
        <button
          type="button"
          className={accountingSecondaryButtonClass}
          onClick={() => onDuplicate(entry)}
        >
          {t("entryDetail.actions.duplicate")}
        </button>
        {showVoidActions ? (
          <>
            <button
              type="button"
              className="inline-flex h-9 items-center gap-1.5 rounded-xl border border-rose-200 bg-white px-3 text-sm font-medium text-rose-700 shadow-sm transition-all duration-200 hover:-translate-y-0.5 hover:border-rose-300 hover:bg-rose-50 disabled:cursor-not-allowed disabled:opacity-50"
              onClick={() => setPendingVoid("void")}
              disabled={voidMutation.isPending}
            >
              {t("entryDetail.actions.void")}
            </button>
            <button
              type="button"
              className={accountingPrimaryButtonClass}
              onClick={() => setPendingVoid("voidAndDuplicate")}
              disabled={voidMutation.isPending}
            >
              {t("entryDetail.actions.voidAndDuplicate")}
            </button>
          </>
        ) : null}
      </div>
    );

  return (
    <>
      <DetailDrawer
        open={open && !!entry}
        onClose={onClose}
        title={t("entryDetail.title")}
        subtitle={entry?.description}
        size="md"
        footer={footer}
      >
        {entry ? (
          <div className="space-y-5">
            {/* Amount hero */}
            <div className="rounded-2xl border border-warm-200/80 bg-white px-4 py-4 shadow-sm shadow-warm-900/5">
              <p className="text-xs font-medium uppercase tracking-wide text-ink-500">
                {t("entryDetail.amount")}
              </p>
              <AnimatedNumberText
                value={Number(entry.amount) || 0}
                format={(v) =>
                  formatMoney(v, entry.currency || currency, locale)
                }
                className={`mt-1 block font-title text-display-md tabular-nums ${amountClass}`}
              />
              <div className="mt-2">
                <StatusBadge
                  tone={isVoided ? "neutral" : entry.entry_type === "income" ? "success" : "neutral"}
                  label={
                    isVoided
                      ? t("entries.status.voided")
                      : entry.entry_type === "income"
                        ? t("entries.income")
                        : t("entries.expense")
                  }
                  size="sm"
                />
              </div>
            </div>

            {/* Definition list */}
            <dl className="space-y-3 rounded-2xl border border-warm-200/80 bg-white px-4 py-4 shadow-sm shadow-warm-900/5">
              <DefinitionRow label={t("entryDetail.category")}>
                {formatCategoryLabel(entry.category, t)}
              </DefinitionRow>
              <DefinitionRow label={t("entryDetail.type")}>
                {entry.entry_type === "income"
                  ? t("entries.income")
                  : t("entries.expense")}
              </DefinitionRow>
              <DefinitionRow label={t("entryDetail.occurredAt")}>
                {formatDay(entry.occurred_at, locale)}
              </DefinitionRow>
              {entry.notes?.trim() ? (
                <DefinitionRow label={t("entryDetail.notes")}>
                  <span className="whitespace-pre-wrap">{entry.notes}</span>
                </DefinitionRow>
              ) : null}
              {entry.reference?.trim() ? (
                <DefinitionRow label={t("entryDetail.reference")}>
                  {entry.reference}
                </DefinitionRow>
              ) : null}
            </dl>

            {/* Audit trail */}
            <div className="space-y-1 text-xs text-warm-500">
              {entry.created_at || entry.created_by_staff ? (
                <p>
                  {interpolate("entryDetail.audit.createdBy", {
                    name: actorName(entry.created_by_staff, entry.created_by_user),
                    date: entry.created_at
                      ? formatDay(entry.created_at, locale)
                      : "—",
                  })}
                </p>
              ) : null}
              {entry.voided_at ? (
                <p>
                  {interpolate("entryDetail.audit.voidedBy", {
                    name: actorName(entry.voided_by_staff),
                    date: formatDay(entry.voided_at, locale),
                  })}
                </p>
              ) : null}
            </div>

            <p className="rounded-xl border border-dashed border-warm-200 bg-warm-50/70 px-3 py-2 text-xs text-ink-500">
              {t("entryDetail.immutableHint")}
            </p>

            {voidError ? (
              <p role="alert" className="text-sm text-rose-700">
                {voidError}
              </p>
            ) : null}

            <EntryAttachments
              businessId={businessId}
              entryId={entry.id}
              canWrite={canWrite}
              t={t}
            />
          </div>
        ) : null}
      </DetailDrawer>

      <ConfirmationModal
        isOpen={!!pendingVoid && !!entry}
        onOpenChange={() => setPendingVoid(null)}
        title={t("entryDetail.confirmVoidTitle")}
        description={interpolate("entryDetail.confirmVoidBody", {
          description: entry?.description ?? "",
        })}
        confirmLabel={t("entries.void")}
        onConfirm={handleConfirmVoid}
        isDanger
      />
    </>
  );
}
