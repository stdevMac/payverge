"use client";

/**
 * Shared ledger field set used by EntryFormDrawer and RecurringTemplatesPanel
 * (L6-13). Category is a fetched Select (defaults + custom), not free-text.
 */

import React, { useEffect, useMemo, useState } from "react";
import { Input, Select, SelectItem, Textarea } from "@nextui-org/react";
import { accountingApi } from "@/api/accounting";
import { DecimalInput } from "@/components/ui/DecimalInput";
import SegmentedTabs from "../shared/SegmentedTabs";
import { formatCategoryLabel } from "./accountingShared";
import { expenseCategories, incomeCategories } from "./categories";

type LedgerEntryType = "income" | "expense";

export type LedgerEntryFieldValues = {
  entryType: LedgerEntryType;
  category: string;
  amount: string;
  description: string;
  notes: string;
  reference: string;
};

export type LedgerEntryFieldErrors = {
  amount?: string;
  description?: string;
  category?: string;
};

const selectClassNames = {
  trigger:
    "h-10 min-h-10 bg-white border-warm-200 data-[hover=true]:border-brand/40",
  value: "text-sm",
};

const inputWrapperClass =
  "bg-white border-warm-200 data-[hover=true]:border-brand/40";

export default function LedgerEntryFields({
  businessId,
  currency,
  values,
  errors,
  onChange,
  t,
  showType = true,
}: {
  businessId: string;
  currency: string;
  values: LedgerEntryFieldValues;
  errors?: LedgerEntryFieldErrors;
  onChange: (patch: Partial<LedgerEntryFieldValues>) => void;
  t: (key: string, params?: Record<string, string | number>) => string;
  showType?: boolean;
}) {
  const [categoryOptions, setCategoryOptions] = useState<string[]>([]);

  useEffect(() => {
    let cancelled = false;
    void accountingApi
      .listAccountingCategories(businessId, values.entryType)
      .then((res) => {
        if (cancelled) return;
        const keys = [
          ...res.defaults.map((d) => d.key),
          ...res.custom.map((c) => c.key),
        ];
        setCategoryOptions(Array.from(new Set(keys)));
      })
      .catch(() => {
        if (cancelled) return;
        setCategoryOptions(
          values.entryType === "income"
            ? [...incomeCategories]
            : [...expenseCategories],
        );
      });
    return () => {
      cancelled = true;
    };
  }, [businessId, values.entryType]);

  const categories = useMemo(() => {
    if (categoryOptions.length > 0) return categoryOptions;
    return values.entryType === "income"
      ? ([...incomeCategories] as string[])
      : ([...expenseCategories] as string[]);
  }, [categoryOptions, values.entryType]);

  return (
    <div className="flex flex-col gap-4" data-testid="ledger-entry-fields">
      {showType ? (
        <div className="space-y-1.5">
          <span className="text-sm font-semibold text-ink-950">
            {t("entryForm.type")}
          </span>
          <SegmentedTabs
            size="sm"
            ariaLabel={t("entryForm.type")}
            activeKey={values.entryType}
            onChange={(key) => {
              const next = key === "income" ? "income" : "expense";
              onChange({ entryType: next, category: "" });
            }}
            tabs={[
              { key: "expense", label: t("entryForm.typeExpense") },
              { key: "income", label: t("entryForm.typeIncome") },
            ]}
          />
        </div>
      ) : null}

      <Select
        label={t("entryForm.category")}
        aria-label={t("entryForm.category")}
        labelPlacement="outside"
        placeholder=" "
        variant="bordered"
        selectedKeys={values.category ? [values.category] : []}
        onSelectionChange={(keys) => {
          const next = Array.from(keys)[0];
          if (typeof next === "string") {
            onChange({ category: next });
          }
        }}
        onChange={(e) => {
          const next = e.target.value;
          if (next) onChange({ category: next });
        }}
        isInvalid={Boolean(errors?.category)}
        errorMessage={errors?.category}
        classNames={selectClassNames}
        items={categories.map((cat) => ({
          key: cat,
          label: formatCategoryLabel(cat, t),
        }))}
      >
        {(item) => (
          <SelectItem key={item.key} textValue={item.label}>
            {item.label}
          </SelectItem>
        )}
      </Select>

      <DecimalInput
        min={0}
        step={0.01}
        maxFractionDigits={2}
        label={t("entryForm.amount")}
        aria-label={t("entryForm.amount")}
        labelPlacement="outside"
        placeholder=" "
        variant="bordered"
        value={values.amount}
        onValueChange={(val) => onChange({ amount: val })}
        startContent={
          <span className="pointer-events-none text-sm text-ink-500">
            {currency}
          </span>
        }
        isInvalid={Boolean(errors?.amount)}
        errorMessage={errors?.amount}
        classNames={{ inputWrapper: inputWrapperClass }}
        data-testid="ledger-entry-amount"
      />

      <Input
        label={t("entryForm.description")}
        aria-label={t("entryForm.description")}
        labelPlacement="outside"
        placeholder=" "
        variant="bordered"
        value={values.description}
        onValueChange={(val) => onChange({ description: val })}
        onChange={(e) => onChange({ description: e.target.value })}
        isInvalid={Boolean(errors?.description)}
        errorMessage={errors?.description}
        classNames={{ inputWrapper: inputWrapperClass }}
      />

      <Textarea
        label={t("entryForm.notes")}
        aria-label={t("entryForm.notes")}
        labelPlacement="outside"
        placeholder=" "
        variant="bordered"
        minRows={2}
        value={values.notes}
        onValueChange={(val) => onChange({ notes: val })}
        onChange={(e) => onChange({ notes: e.target.value })}
        classNames={{ inputWrapper: inputWrapperClass }}
      />

      <Input
        label={t("entryForm.reference")}
        aria-label={t("entryForm.reference")}
        labelPlacement="outside"
        placeholder=" "
        variant="bordered"
        value={values.reference}
        onValueChange={(val) => onChange({ reference: val })}
        onChange={(e) => onChange({ reference: e.target.value })}
        classNames={{ inputWrapper: inputWrapperClass }}
      />
    </div>
  );
}
