"use client";

import React, { useCallback, useEffect, useMemo, useState } from "react";
import { Button, Input, Select, SelectItem, Textarea } from "@nextui-org/react";
import { accountingApi, type ManualLedgerEntry } from "@/api/accounting";
import { useCreateEntry } from "@/hooks/accounting/useAccountingQueries";
import { asDollars } from "@/types/money";
import DetailDrawer from "../shared/DetailDrawer";
import SegmentedTabs from "../shared/SegmentedTabs";
import {
  accountingPrimaryButtonClass,
  accountingSecondaryButtonClass,
  formatCategoryLabel,
  formatDateInput,
  toIsoDate,
} from "./accountingShared";
import { expenseCategories, incomeCategories } from "./categories";
import { isPeriodLockedError, periodLockedMessage } from "./periodLockErrors";

export type EntryFormDrawerProps = {
  open: boolean;
  onClose: () => void;
  businessId: string;
  currency: string;
  /** Prefill for duplicate flow; occurred_at is always set to today. */
  initial?: ManualLedgerEntry | null;
  t: (key: string, params?: Record<string, string | number>) => string;
  /** Optional success side-effect (toast / banner). Called before onClose. */
  onSuccess?: () => void;
};

type EntryType = "income" | "expense";

type FieldErrors = {
  amount?: string;
  description?: string;
  category?: string;
};

function mutationErrorMessage(err: unknown, fallback: string): string {
  if (err && typeof err === "object") {
    const maybeAxios = err as {
      response?: { data?: { error?: string; message?: string } };
      message?: string;
    };
    const data = maybeAxios.response?.data;
    if (data?.error && typeof data.error === "string") return data.error;
    if (data?.message && typeof data.message === "string") return data.message;
    if (typeof maybeAxios.message === "string" && maybeAxios.message) {
      return maybeAxios.message;
    }
  }
  if (err instanceof Error && err.message) return err.message;
  return fallback;
}

// nextRecurringRunOn returns the first cadence occurrence strictly AFTER the
// given day (the entry itself already books that day).
function nextRecurringRunOn(
  day: string,
  cadence: "monthly" | "weekly",
  anchorDay: number,
): string {
  const base = new Date(day + "T12:00:00");
  if (cadence === "weekly") {
    base.setDate(base.getDate() + 7);
    return formatDateInput(base);
  }
  const anchor = Math.min(28, Math.max(1, anchorDay));
  return formatDateInput(
    new Date(base.getFullYear(), base.getMonth() + 1, anchor, 12),
  );
}

function emptyForm(currencyDate: string) {
  return {
    entryType: "expense" as EntryType,
    category: "",
    amount: "",
    occurredAt: currencyDate,
    description: "",
    notes: "",
    reference: "",
  };
}

export default function EntryFormDrawer({
  open,
  onClose,
  businessId,
  currency,
  initial = null,
  t,
  onSuccess,
}: EntryFormDrawerProps) {
  const createMutation = useCreateEntry(businessId);

  const today = useMemo(() => formatDateInput(new Date()), []);

  const [entryType, setEntryType] = useState<EntryType>("expense");
  const [category, setCategory] = useState("");
  const [amount, setAmount] = useState("");
  const [occurredAt, setOccurredAt] = useState(today);
  const [description, setDescription] = useState("");
  const [notes, setNotes] = useState("");
  const [reference, setReference] = useState("");
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({});
  const [serverError, setServerError] = useState<string | null>(null);
  const [makeRecurring, setMakeRecurring] = useState(false);
  const [recurringCadence, setRecurringCadence] = useState<
    "monthly" | "weekly"
  >("monthly");
  const [categoryOptions, setCategoryOptions] = useState<string[]>([]);

  // Load defaults + business custom categories for the picker.
  useEffect(() => {
    if (!open) return;
    let cancelled = false;
    void accountingApi
      .listAccountingCategories(businessId, entryType)
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
          entryType === "income"
            ? [...incomeCategories]
            : [...expenseCategories],
        );
      });
    return () => {
      cancelled = true;
    };
  }, [open, businessId, entryType]);

  // Reset / prefill whenever the drawer opens (or initial changes while open).
  useEffect(() => {
    if (!open) return;
    const day = formatDateInput(new Date());
    setMakeRecurring(false);
    setRecurringCadence("monthly");
    if (initial) {
      setEntryType(initial.entry_type);
      setCategory(initial.category || "");
      setAmount(
        initial.amount != null && initial.amount !== ("" as never)
          ? String(initial.amount)
          : "",
      );
      setDescription(initial.description || "");
      setNotes(initial.notes ?? "");
      setReference(initial.reference ?? "");
      setOccurredAt(day);
    } else {
      const blank = emptyForm(day);
      setEntryType(blank.entryType);
      setCategory(blank.category);
      setAmount(blank.amount);
      setOccurredAt(blank.occurredAt);
      setDescription(blank.description);
      setNotes(blank.notes);
      setReference(blank.reference);
    }
    setFieldErrors({});
    setServerError(null);
  }, [open, initial]);

  const categories = useMemo(() => {
    if (categoryOptions.length > 0) return categoryOptions;
    return entryType === "income"
      ? ([...incomeCategories] as string[])
      : ([...expenseCategories] as string[]);
  }, [categoryOptions, entryType]);

  const handleTypeChange = useCallback((key: string) => {
    const next = key === "income" ? "income" : "expense";
    setEntryType(next);
    setCategory("");
    setFieldErrors((prev) => ({ ...prev, category: undefined }));
  }, []);

  const validate = useCallback((): FieldErrors => {
    const errors: FieldErrors = {};
    const trimmedAmount = amount.trim();
    if (!trimmedAmount) {
      errors.amount = t("entryForm.errors.amountRequired");
    } else {
      const n = Number(trimmedAmount);
      if (!Number.isFinite(n) || n <= 0) {
        errors.amount = t("entryForm.errors.amountPositive");
      }
    }
    if (!description.trim()) {
      errors.description = t("entryForm.errors.descriptionRequired");
    }
    if (!category.trim()) {
      errors.category = t("entryForm.errors.categoryRequired");
    }
    return errors;
  }, [amount, description, category, t]);

  const handleSubmit = useCallback(() => {
    setServerError(null);
    const errors = validate();
    setFieldErrors(errors);
    if (Object.keys(errors).length > 0) return;

    const n = Number(amount.trim());
    createMutation.mutate(
      {
        entry_type: entryType,
        category: category.trim(),
        amount: asDollars(n),
        currency,
        occurred_at: toIsoDate(occurredAt || formatDateInput(new Date())),
        description: description.trim(),
        notes: notes.trim() || undefined,
        reference: reference.trim() || undefined,
      },
      {
        onSuccess: async () => {
          if (makeRecurring) {
            const day = occurredAt || formatDateInput(new Date());
            const anchor = Math.min(
              28,
              Math.max(1, Number(day.slice(8, 10)) || 1),
            );
            try {
              await accountingApi.createRecurringTemplate(businessId, {
                entry_type: entryType,
                category: category.trim(),
                amount: n,
                currency,
                description: description.trim(),
                notes: notes.trim() || undefined,
                reference: reference.trim() || undefined,
                cadence: recurringCadence,
                anchor_day: anchor,
                // The manual entry just created already covers `day`; the
                // template starts at the NEXT occurrence, otherwise the hourly
                // scheduler would generate a same-day duplicate within the hour.
                next_run_on: nextRecurringRunOn(day, recurringCadence, anchor),
                active: true,
              });
            } catch {
              // Entry was created; recurring failure is non-blocking for the form.
            }
          }
          onSuccess?.();
          onClose();
        },
        onError: (err) => {
          setServerError(
            isPeriodLockedError(err)
              ? periodLockedMessage(err, t)
              : mutationErrorMessage(err, t("errors.saveEntry")),
          );
        },
      },
    );
  }, [
    validate,
    amount,
    createMutation,
    entryType,
    category,
    currency,
    occurredAt,
    description,
    notes,
    reference,
    makeRecurring,
    recurringCadence,
    businessId,
    onSuccess,
    t,
    onClose,
  ]);

  const title = initial
    ? t("entryForm.titleDuplicate")
    : t("entryForm.titleNew");

  const selectClassNames = {
    trigger:
      "h-10 min-h-10 bg-white border-warm-200 data-[hover=true]:border-brand/40",
    value: "text-sm",
  };

  const footer = (
    <div className="flex w-full flex-wrap items-center justify-end gap-2">
      <button
        type="button"
        className={accountingSecondaryButtonClass}
        onClick={onClose}
        disabled={createMutation.isPending}
      >
        {t("entryForm.cancel")}
      </button>
      <Button
        type="button"
        className={accountingPrimaryButtonClass}
        onPress={handleSubmit}
        isLoading={createMutation.isPending}
        isDisabled={createMutation.isPending}
      >
        {t("entryForm.submit")}
      </Button>
    </div>
  );

  // L6-18: treat any non-blank field as dirty so Esc cannot silently discard.
  const dirty =
    category.trim() !== "" ||
    amount.trim() !== "" ||
    description.trim() !== "" ||
    notes.trim() !== "" ||
    reference.trim() !== "" ||
    makeRecurring;

  return (
    <DetailDrawer
      open={open}
      onClose={onClose}
      title={title}
      size="md"
      footer={footer}
      dirty={dirty}
      discardConfirm={{
        title: t("discardConfirm.entry.title"),
        description: t("discardConfirm.entry.description"),
        confirmLabel: t("discardConfirm.confirm"),
        cancelLabel: t("discardConfirm.cancel"),
      }}
    >
      <form
        // flex gap, not space-y: sibling margin-top would override NextUI's
        // outside-label headroom (data-[has-label=true]:mt-*) and clip labels.
        className="flex flex-col gap-4"
        onSubmit={(e) => {
          e.preventDefault();
          handleSubmit();
        }}
      >
        {serverError ? (
          <div
            role="alert"
            className="rounded-xl border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700"
          >
            {serverError}
          </div>
        ) : null}

        <div className="space-y-1.5">
          <span className="text-sm font-semibold text-ink-950">
            {t("entryForm.type")}
          </span>
          <SegmentedTabs
            size="sm"
            ariaLabel={t("entryForm.type")}
            activeKey={entryType}
            onChange={handleTypeChange}
            tabs={[
              { key: "expense", label: t("entryForm.typeExpense") },
              { key: "income", label: t("entryForm.typeIncome") },
            ]}
          />
        </div>

        <label className="flex items-center gap-2 text-sm text-ink-700">
          <input
            type="checkbox"
            checked={makeRecurring}
            onChange={(e) => setMakeRecurring(e.target.checked)}
            data-testid="make-recurring-toggle"
          />
          {t("entryForm.makeRecurring")}
        </label>
        {makeRecurring ? (
          <Select
            label={t("entryForm.recurringCadence")}
            aria-label={t("entryForm.recurringCadence")}
            labelPlacement="outside"
            placeholder=" "
            variant="bordered"
            selectedKeys={[recurringCadence]}
            onSelectionChange={(keys) => {
              const next = Array.from(keys)[0];
              if (next === "weekly" || next === "monthly") {
                setRecurringCadence(next);
              }
            }}
            classNames={selectClassNames}
          >
            <SelectItem key="monthly">
              {t("entryForm.cadenceMonthly")}
            </SelectItem>
            <SelectItem key="weekly">{t("entryForm.cadenceWeekly")}</SelectItem>
          </Select>
        ) : null}

        <Select
          label={t("entryForm.category")}
          aria-label={t("entryForm.category")}
          labelPlacement="outside"
          placeholder=" "
          variant="bordered"
          selectedKeys={category ? [category] : []}
          onSelectionChange={(keys) => {
            const next = Array.from(keys)[0];
            if (typeof next === "string") {
              setCategory(next);
              setFieldErrors((prev) => ({ ...prev, category: undefined }));
            }
          }}
          // Native change path for tests / a11y fallbacks.
          onChange={(e) => {
            const next = e.target.value;
            if (next) {
              setCategory(next);
              setFieldErrors((prev) => ({ ...prev, category: undefined }));
            }
          }}
          isInvalid={Boolean(fieldErrors.category)}
          errorMessage={fieldErrors.category}
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

        <Input
          type="number"
          step="0.01"
          min="0"
          label={t("entryForm.amount")}
          aria-label={t("entryForm.amount")}
          labelPlacement="outside"
          placeholder=" "
          variant="bordered"
          value={amount}
          onValueChange={(val) => {
            setAmount(val);
            setFieldErrors((prev) => ({ ...prev, amount: undefined }));
          }}
          // fireEvent.change path used by tests
          onChange={(e) => {
            setAmount(e.target.value);
            setFieldErrors((prev) => ({ ...prev, amount: undefined }));
          }}
          startContent={
            <span className="pointer-events-none text-sm text-ink-500">
              {currency}
            </span>
          }
          isInvalid={Boolean(fieldErrors.amount)}
          errorMessage={fieldErrors.amount}
          classNames={{
            inputWrapper:
              "bg-white border-warm-200 data-[hover=true]:border-brand/40",
          }}
        />

        <Input
          type="date"
          label={t("entryForm.occurredAt")}
          aria-label={t("entryForm.occurredAt")}
          labelPlacement="outside"
          placeholder=" "
          variant="bordered"
          value={occurredAt}
          onValueChange={setOccurredAt}
          onChange={(e) => setOccurredAt(e.target.value)}
          classNames={{
            inputWrapper:
              "bg-white border-warm-200 data-[hover=true]:border-brand/40",
          }}
        />

        <Input
          label={t("entryForm.description")}
          aria-label={t("entryForm.description")}
          labelPlacement="outside"
          placeholder=" "
          variant="bordered"
          value={description}
          onValueChange={(val) => {
            setDescription(val);
            setFieldErrors((prev) => ({ ...prev, description: undefined }));
          }}
          onChange={(e) => {
            setDescription(e.target.value);
            setFieldErrors((prev) => ({ ...prev, description: undefined }));
          }}
          isInvalid={Boolean(fieldErrors.description)}
          errorMessage={fieldErrors.description}
          classNames={{
            inputWrapper:
              "bg-white border-warm-200 data-[hover=true]:border-brand/40",
          }}
        />

        <Textarea
          label={t("entryForm.notes")}
          aria-label={t("entryForm.notes")}
          labelPlacement="outside"
          placeholder=" "
          variant="bordered"
          minRows={2}
          value={notes}
          onValueChange={setNotes}
          onChange={(e) => setNotes(e.target.value)}
          classNames={{
            inputWrapper:
              "bg-white border-warm-200 data-[hover=true]:border-brand/40",
          }}
        />

        <Input
          label={t("entryForm.reference")}
          aria-label={t("entryForm.reference")}
          labelPlacement="outside"
          placeholder=" "
          variant="bordered"
          value={reference}
          onValueChange={setReference}
          onChange={(e) => setReference(e.target.value)}
          classNames={{
            inputWrapper:
              "bg-white border-warm-200 data-[hover=true]:border-brand/40",
          }}
        />
      </form>
    </DetailDrawer>
  );
}
