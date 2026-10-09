"use client";

import React, { useCallback, useEffect, useState } from "react";
import { Input, Select, SelectItem } from "@nextui-org/react";
import {
  accountingApi,
  type RecurringTemplate,
  type RecurringTemplateInput,
} from "@/api/accounting";
import {
  accountingPrimaryButtonClass,
  accountingSecondaryButtonClass,
  formatDateInput,
} from "./accountingShared";
import LedgerEntryFields, {
  type LedgerEntryFieldValues,
} from "./LedgerEntryFields";

const blankFields = (): LedgerEntryFieldValues => ({
  entryType: "expense",
  category: "",
  amount: "",
  description: "",
  notes: "",
  reference: "",
});

export default function RecurringTemplatesPanel({
  businessId,
  t,
  currency = "USD",
}: {
  businessId: string;
  t: (key: string, params?: Record<string, string | number>) => string;
  currency?: string;
}) {
  const [rows, setRows] = useState<RecurringTemplate[]>([]);
  const [open, setOpen] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [fields, setFields] = useState<LedgerEntryFieldValues>(blankFields);
  const [cadence, setCadence] = useState<"monthly" | "weekly">("monthly");
  const [nextRunOn, setNextRunOn] = useState(formatDateInput(new Date()));
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      const data = await accountingApi.listRecurringTemplates(businessId);
      setRows(data || []);
      setError(null);
    } catch {
      setError(t("recurring.loadError"));
    }
  }, [businessId, t]);

  useEffect(() => {
    if (open) void load();
  }, [open, load]);

  const create = async () => {
    const n = Number(fields.amount);
    if (
      !fields.description.trim() ||
      !fields.category.trim() ||
      !Number.isFinite(n) ||
      n <= 0
    ) {
      setError(t("recurring.createInvalid"));
      return;
    }
    setBusy(true);
    setError(null);
    try {
      const anchor =
        cadence === "monthly"
          ? Math.min(28, Math.max(1, Number(nextRunOn.slice(8, 10)) || 1))
          : new Date(nextRunOn + "T12:00:00").getDay();
      const payload: RecurringTemplateInput = {
        entry_type: fields.entryType,
        category: fields.category.trim(),
        amount: n,
        currency,
        description: fields.description.trim(),
        notes: fields.notes.trim() || undefined,
        reference: fields.reference.trim() || undefined,
        cadence,
        anchor_day: anchor,
        next_run_on: nextRunOn,
        active: true,
      };
      await accountingApi.createRecurringTemplate(businessId, payload);
      setShowCreate(false);
      setFields(blankFields());
      await load();
    } catch {
      setError(t("recurring.createError"));
    } finally {
      setBusy(false);
    }
  };

  const toggleActive = async (row: RecurringTemplate) => {
    try {
      await accountingApi.updateRecurringTemplate(businessId, row.id, {
        entry_type: row.entry_type,
        category: row.category,
        amount: Number(row.amount),
        currency: row.currency,
        description: row.description,
        notes: row.notes,
        reference: row.reference,
        cadence: row.cadence,
        anchor_day: row.anchor_day,
        next_run_on: row.next_run_on,
        active: !row.active,
      });
      await load();
    } catch {
      setError(t("recurring.updateError"));
    }
  };

  return (
    <div
      className="rounded-xl border border-warm-200 bg-white p-3"
      data-testid="recurring-panel"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h4 className="text-sm font-semibold text-ink-900">
          {t("recurring.title")}
        </h4>
        <div className="flex gap-2">
          {open ? (
            <button
              type="button"
              className={accountingSecondaryButtonClass}
              onClick={() => setShowCreate((v) => !v)}
              data-testid="recurring-create-toggle"
            >
              {showCreate ? t("recurring.cancelCreate") : t("recurring.create")}
            </button>
          ) : null}
          <button
            type="button"
            className={accountingSecondaryButtonClass}
            onClick={() => setOpen((v) => !v)}
          >
            {open ? t("recurring.hide") : t("recurring.manage")}
          </button>
        </div>
      </div>
      {open ? (
        <div className="mt-3 space-y-3">
          {error ? (
            <p role="alert" className="text-sm text-rose-700">
              {error}
            </p>
          ) : null}
          {showCreate ? (
            <div
              className="flex flex-col gap-2 rounded-lg border border-warm-200 bg-warm-50/50 p-3"
              data-testid="recurring-create-form"
            >
              <LedgerEntryFields
                businessId={businessId}
                currency={currency}
                t={t}
                values={fields}
                onChange={(patch) =>
                  setFields((prev) => ({ ...prev, ...patch }))
                }
              />
              <Select
                label={t("recurring.fields.cadence")}
                labelPlacement="outside"
                placeholder=" "
                selectedKeys={[cadence]}
                onSelectionChange={(keys) => {
                  const v = Array.from(keys)[0];
                  if (v === "monthly" || v === "weekly") setCadence(v);
                }}
                variant="bordered"
              >
                <SelectItem key="monthly">
                  {t("entryForm.cadenceMonthly")}
                </SelectItem>
                <SelectItem key="weekly">
                  {t("entryForm.cadenceWeekly")}
                </SelectItem>
              </Select>
              <Input
                type="date"
                label={t("recurring.fields.nextRun")}
                labelPlacement="outside"
                placeholder=" "
                value={nextRunOn}
                onValueChange={setNextRunOn}
                variant="bordered"
              />
              <button
                type="button"
                className={accountingPrimaryButtonClass}
                disabled={busy}
                onClick={() => void create()}
                data-testid="recurring-create-submit"
              >
                {t("recurring.save")}
              </button>
            </div>
          ) : null}
          {rows.length === 0 ? (
            <p className="text-sm text-ink-500">{t("recurring.empty")}</p>
          ) : (
            <ul className="divide-y divide-warm-100">
              {rows.map((r) => (
                <li
                  key={r.id}
                  className="flex flex-wrap items-center justify-between gap-2 py-2 text-sm"
                  data-testid={`recurring-row-${r.id}`}
                >
                  <div>
                    <span className="font-medium text-ink-900">
                      {r.description}
                    </span>
                    <span className="ml-2 text-ink-500">
                      {r.cadence} · next {r.next_run_on}
                    </span>
                    {r.needs_attention ? (
                      <span className="ml-2 text-amber-700">
                        {t("recurring.needsAttention")}
                      </span>
                    ) : null}
                    {!r.active ? (
                      <span className="ml-2 text-ink-400">
                        {t("recurring.paused")}
                      </span>
                    ) : null}
                  </div>
                  <div className="flex gap-2">
                    <button
                      type="button"
                      className="text-xs text-brand-700 underline"
                      onClick={() => void toggleActive(r)}
                      data-testid={`recurring-pause-${r.id}`}
                    >
                      {r.active ? t("recurring.pause") : t("recurring.resume")}
                    </button>
                    <button
                      type="button"
                      className="text-xs text-rose-700 underline"
                      onClick={async () => {
                        await accountingApi.deleteRecurringTemplate(
                          businessId,
                          r.id,
                        );
                        void load();
                      }}
                    >
                      {t("recurring.delete")}
                    </button>
                  </div>
                </li>
              ))}
            </ul>
          )}
        </div>
      ) : null}
    </div>
  );
}
