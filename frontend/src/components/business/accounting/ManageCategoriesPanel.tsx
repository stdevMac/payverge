"use client";

import React, { useCallback, useEffect, useState } from "react";
import { Input } from "@nextui-org/react";
import {
  accountingApi,
  type AccountingCategoryRow,
} from "@/api/accounting";
import {
  accountingPrimaryButtonClass,
  accountingSecondaryButtonClass,
} from "./accountingShared";

/** Manage business-defined EXTRA categories (defaults stay hardcoded). */
export default function ManageCategoriesPanel({
  businessId,
  t,
}: {
  businessId: string;
  t: (key: string, params?: Record<string, string | number>) => string;
}) {
  const [open, setOpen] = useState(false);
  const [custom, setCustom] = useState<AccountingCategoryRow[]>([]);
  const [key, setKey] = useState("");
  const [label, setLabel] = useState("");
  const [entryType, setEntryType] = useState<"expense" | "income">("expense");
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      // includeInactive so archived rows stay restorable in this panel.
      const res = await accountingApi.listAccountingCategories(
        businessId,
        undefined,
        { includeInactive: true },
      );
      setCustom(res.custom || []);
      setError(null);
    } catch {
      setError(t("categories.loadError"));
    }
  }, [businessId, t]);

  useEffect(() => {
    if (open) void load();
  }, [open, load]);

  const create = async () => {
    if (!key.trim() || !label.trim()) {
      setError(t("categories.invalid"));
      return;
    }
    try {
      await accountingApi.createAccountingCategory(businessId, {
        key: key.trim().toLowerCase().replace(/\s+/g, "_"),
        label: label.trim(),
        entry_type: entryType,
      });
      setKey("");
      setLabel("");
      await load();
    } catch {
      setError(t("categories.createError"));
    }
  };

  // Soft archive / restore via existing PATCH (no DELETE endpoint — L6-14).
  const setActive = async (row: AccountingCategoryRow, active: boolean) => {
    if (row.id == null) return;
    try {
      await accountingApi.updateAccountingCategory(businessId, row.id, {
        active,
      });
      await load();
    } catch {
      setError(t("categories.updateError"));
    }
  };

  return (
    <div className="rounded-xl border border-warm-200 bg-white p-3" data-testid="manage-categories">
      <div className="flex items-center justify-between gap-2">
        <h4 className="text-sm font-semibold text-ink-900">
          {t("categories.title")}
        </h4>
        <button
          type="button"
          className={accountingSecondaryButtonClass}
          onClick={() => setOpen((v) => !v)}
        >
          {open ? t("categories.hide") : t("categories.manage")}
        </button>
      </div>
      {open ? (
        <div className="mt-3 space-y-2">
          {error ? (
            <p role="alert" className="text-sm text-rose-700">
              {error}
            </p>
          ) : null}
          <div className="flex flex-wrap gap-2">
            <Input
              label={t("categories.key")}
              labelPlacement="outside"
              placeholder=" "
              value={key}
              onValueChange={setKey}
              variant="bordered"
              className="min-w-[8rem] flex-1"
            />
            <Input
              label={t("categories.label")}
              labelPlacement="outside"
              placeholder=" "
              value={label}
              onValueChange={setLabel}
              variant="bordered"
              className="min-w-[8rem] flex-1"
            />
            <select
              className="mt-6 h-10 rounded-lg border border-warm-200 px-2 text-sm"
              value={entryType}
              onChange={(e) =>
                setEntryType(e.target.value === "income" ? "income" : "expense")
              }
              aria-label={t("categories.type")}
            >
              <option value="expense">{t("entries.expense")}</option>
              <option value="income">{t("entries.income")}</option>
            </select>
            <button
              type="button"
              className={`${accountingPrimaryButtonClass} mt-6`}
              onClick={() => void create()}
              data-testid="category-create"
            >
              {t("categories.add")}
            </button>
          </div>
          {custom.length === 0 ? (
            <p className="text-sm text-ink-500">{t("categories.empty")}</p>
          ) : (
            <ul className="divide-y divide-warm-100 text-sm">
              {custom.map((c) => {
                const isActive = c.active !== false;
                return (
                  <li
                    key={`${c.entry_type}-${c.key}`}
                    className="flex flex-wrap items-center justify-between gap-2 py-1.5"
                  >
                    <span>
                      <span
                        className={`font-medium ${isActive ? "text-ink-900" : "text-ink-400 line-through"}`}
                      >
                        {c.label}
                      </span>
                      <span className="ml-2 text-ink-500">
                        {c.key} · {c.entry_type}
                        {!isActive ? ` · ${t("categories.archived")}` : ""}
                      </span>
                    </span>
                    {c.id != null ? (
                      <button
                        type="button"
                        className={accountingSecondaryButtonClass}
                        onClick={() => void setActive(c, !isActive)}
                        aria-label={
                          isActive
                            ? t("categories.archive")
                            : t("categories.restore")
                        }
                      >
                        {isActive
                          ? t("categories.archive")
                          : t("categories.restore")}
                      </button>
                    ) : null}
                  </li>
                );
              })}
            </ul>
          )}
        </div>
      ) : null}
    </div>
  );
}
