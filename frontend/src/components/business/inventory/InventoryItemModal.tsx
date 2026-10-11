"use client";

import { getSafeApiErrorMessage } from "@/utils/apiError";

import React, { useCallback, useEffect, useState } from "react";
import {
  Autocomplete,
  AutocompleteItem,
  Button,
  Input,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
  Switch,
} from "@nextui-org/react";
import { Coins, PackagePlus, Save } from "lucide-react";

import { InventoryItem, inventoryApi } from "@/api/inventory";
import { tryParseLocaleDecimal } from "@/lib/parseLocaleDecimal";
import { isWhitespaceOnly } from "@/lib/fieldValidation";
import { DecimalInput } from "@/components/ui/DecimalInput";

/**
 * Form state for the inventory item modal. Fields stay as strings so the
 * `Input type="number"` components can render freely while the user is
 * typing (including transient empty state). Validation / coercion happens
 * at submit time.
 */
interface InventoryItemFormState {
  name: string;
  sku: string;
  category: string;
  unit: string;
  currentQuantity: string;
  reorderThreshold: string;
  costPerUnit: string;
  isActive: boolean;
}

function defaultItemForm(unitDefault: string): InventoryItemFormState {
  return {
    name: "",
    sku: "",
    category: "",
    unit: unitDefault,
    currentQuantity: "0",
    reorderThreshold: "0",
    costPerUnit: "0",
    isActive: true,
  };
}

function formStateFrom(
  initial: InventoryItem | null,
  unitDefault: string,
): InventoryItemFormState {
  if (!initial) return defaultItemForm(unitDefault);
  return {
    name: initial.name,
    sku: initial.sku || "",
    category: initial.category || "",
    unit: initial.unit || unitDefault,
    currentQuantity: String(initial.current_quantity ?? 0),
    reorderThreshold: String(initial.reorder_threshold ?? 0),
    costPerUnit: String(initial.cost_per_unit ?? 0),
    isActive: initial.is_active,
  };
}

/**
 * Locale-tolerant optional number for inventory money/qty fields.
 * Empty → undefined (omit from payload); invalid → NaN (caller rejects via
 * !Number.isFinite). Uses tryParseLocaleDecimal so "12,34" is 12.34 not NaN
 * or a digit-stripped 1234 (L5-36).
 */
function parseOptionalNumber(value: string): number | undefined {
  const trimmed = value.trim();
  if (!trimmed) return undefined;
  const n = tryParseLocaleDecimal(trimmed);
  return n === null ? Number.NaN : n;
}

interface InventoryItemModalTranslator {
  (key: string): string;
}

export interface InventoryItemModalProps {
  isOpen: boolean;
  onClose: () => void;
  mode: "create" | "edit";
  initial: InventoryItem | null;
  businessId: number;
  /** Existing categories to suggest in the autocomplete (dedupes drift). */
  categories: string[];
  /**
   * Called after a successful save with the persisted item and the save mode,
   * so the parent can patch it into local state instead of refetching the whole
   * inventory (audit §3.5 HIGH/C1).
   */
  onSaved: (result: { item: InventoryItem; mode: "create" | "edit" }) => void | Promise<void>;
  /** Surfaces non-validation errors up to the parent's error banner. */
  onError?: (message: string) => void;
  /** Local i18n scoped to `businessDashboard.inventoryManager.*`. */
  t: InventoryItemModalTranslator;
}

/**
 * Modal wrapper around the add/edit ingredient form. Replaces the previous
 * inline side-panel that stole ~380px of vertical space while the operator
 * was browsing the inventory list. Follows the TableModals.tsx /
 * AddItemModal.tsx pattern — Header/Body/Footer split with NextUI Modal.
 *
 * Form state is owned by this component. On every open, the form is
 * re-hydrated from `initial` (edit) or cleared (create), so reopening Add
 * after an edit never shows the last-edited values — the prior inline
 * implementation had exactly that bug.
 */
export default function InventoryItemModal({
  isOpen,
  onClose,
  mode,
  initial,
  businessId,
  categories,
  onSaved,
  onError,
  t,
}: InventoryItemModalProps) {
  // L5-37: localized unit default/placeholder (not English "unit" literal).
  const unitDefault = t("itemForm.unitDefault") || t("itemForm.fields.unit") || "unit";
  const [form, setForm] = useState<InventoryItemFormState>(() =>
    formStateFrom(initial, unitDefault),
  );
  const [saving, setSaving] = useState(false);
  const [fieldError, setFieldError] = useState<string | null>(null);
  // Tracks whether the operator actually changed the quantity field during this
  // edit session. When false, current_quantity is omitted from the update
  // payload so that concurrent order-deductions made after the modal was opened
  // are not silently overwritten.
  const [quantityDirty, setQuantityDirty] = useState(false);

  // Re-hydrate every time the modal opens, so edit hydrates from `initial`
  // and create always shows a blank form — no leak of the last-edited row.
  useEffect(() => {
    if (isOpen) {
      setForm(formStateFrom(initial, unitDefault));
      setFieldError(null);
      setSaving(false);
      setQuantityDirty(false);
    }
  }, [isOpen, initial, unitDefault]);

  const handleClose = useCallback(() => {
    // Never reset *before* close — the exit animation still renders the
    // body, and clearing fields mid-animation looks jarring. We rely on
    // the mount-time `useEffect` above to re-initialize on the next open.
    onClose();
  }, [onClose]);

  const handleSubmit = useCallback(async () => {
    if (saving) return; // double-submit guard; the audit flagged this pattern.

    setFieldError(null);

    const currentQuantity = parseOptionalNumber(form.currentQuantity);
    const reorderThreshold = parseOptionalNumber(form.reorderThreshold);
    const costPerUnit = parseOptionalNumber(form.costPerUnit);

    if (
      (currentQuantity !== undefined && !Number.isFinite(currentQuantity)) ||
      (reorderThreshold !== undefined && !Number.isFinite(reorderThreshold)) ||
      (costPerUnit !== undefined && !Number.isFinite(costPerUnit))
    ) {
      setFieldError(t("errors.invalidNumber"));
      return;
    }
    if (currentQuantity !== undefined && currentQuantity < 0) {
      setFieldError(t("errors.invalidCurrentQuantity"));
      return;
    }
    if (reorderThreshold !== undefined && reorderThreshold < 0) {
      setFieldError(t("errors.invalidReorderThreshold"));
      return;
    }
    if (costPerUnit !== undefined && costPerUnit < 0) {
      setFieldError(t("errors.invalidCostPerUnit"));
      return;
    }

    const name = form.name.trim();
    if (!name) {
      // Distinguish whitespace-only (L5-38) from empty for clearer copy.
      setFieldError(
        isWhitespaceOnly(form.name)
          ? t("errors.itemNameWhitespace")
          : t("errors.itemNameRequired"),
      );
      return;
    }

    // In edit mode, omit current_quantity unless the operator explicitly
    // changed the field (quantityDirty). Sending the loaded value
    // unconditionally would overwrite concurrent server-side deductions
    // (e.g. an order processed between the operator opening the modal and
    // clicking Save). In create mode the field is always included.
    const includeQuantity = mode === "create" || quantityDirty;

    const payload = {
      name,
      sku: form.sku.trim() || undefined,
      category: form.category.trim() || undefined,
      unit: form.unit.trim() || undefined,
      current_quantity: includeQuantity ? currentQuantity : undefined,
      reorder_threshold: reorderThreshold,
      cost_per_unit: costPerUnit,
      is_active: form.isActive,
    };

    try {
      setSaving(true);
      const saved =
        mode === "edit" && initial?.id
          ? await inventoryApi.updateItem(businessId, initial.id, payload)
          : await inventoryApi.createItem(businessId, payload);
      await onSaved({ item: saved, mode });
      handleClose();
    } catch (err) {
      console.error("Failed to save inventory item:", err);
      const message = getSafeApiErrorMessage(err, t("errors.saveItem"));
      setFieldError(message);
      onError?.(message);
    } finally {
      setSaving(false);
    }
  }, [
    saving,
    form,
    mode,
    initial,
    businessId,
    onSaved,
    handleClose,
    onError,
    t,
    quantityDirty,
  ]);

  return (
    <Modal
      isOpen={isOpen}
      onOpenChange={(open) => {
        if (!open) handleClose();
      }}
      size="2xl"
      scrollBehavior="inside"
      isDismissable={!saving}
      isKeyboardDismissDisabled={saving}
      data-testid="inventory-item-modal"
    >
      <ModalContent className="overflow-hidden rounded-3xl border border-warm-200 bg-white shadow-2xl shadow-warm-900/15">
        {() => (
          <>
            <ModalHeader className="flex flex-col gap-1 border-b border-warm-200/80 bg-warm-50/70 px-6 py-5">
              <div className="flex items-center gap-2 text-lg font-semibold text-ink-950">
                {mode === "edit" ? (
                  <Save className="w-4 h-4" />
                ) : (
                  <PackagePlus className="w-4 h-4" />
                )}
                {mode === "edit"
                  ? t("itemForm.editTitle")
                  : t("itemForm.addTitle")}
              </div>
              <p className="text-sm text-ink-600">
                {mode === "edit"
                  ? t("itemForm.editSubtitle") || t("itemForm.subtitle")
                  : t("itemForm.subtitle")}
              </p>
            </ModalHeader>

            <ModalBody className="space-y-4 py-5">
              {fieldError && (
                <div
                  className="rounded-2xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm font-medium text-rose-700"
                  role="alert"
                >
                  {fieldError}
                </div>
              )}

              <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                <Input
                  label={t("itemForm.fields.name")}
                  value={form.name}
                  onValueChange={(v) => setForm((c) => ({ ...c, name: v }))}
                  isRequired
                  autoFocus
                  isInvalid={isWhitespaceOnly(form.name)}
                  errorMessage={
                    isWhitespaceOnly(form.name)
                      ? t("errors.itemNameWhitespace")
                      : undefined
                  }
                  data-testid="inventory-item-name"
                />
                <Input
                  label={t("itemForm.fields.sku")}
                  value={form.sku}
                  onValueChange={(v) => setForm((c) => ({ ...c, sku: v }))}
                />
                <Autocomplete
                  label={t("itemForm.fields.category")}
                  placeholder={t("categoryAutocomplete.placeholder")}
                  defaultItems={categories.map((c) => ({ key: c, label: c }))}
                  inputValue={form.category}
                  onInputChange={(v) => setForm((c) => ({ ...c, category: v }))}
                  allowsCustomValue
                >
                  {(entry) => (
                    <AutocompleteItem key={(entry as { key: string }).key}>
                      {(entry as { label: string }).label}
                    </AutocompleteItem>
                  )}
                </Autocomplete>
                <Input
                  label={t("itemForm.fields.unit")}
                  value={form.unit}
                  onValueChange={(v) => setForm((c) => ({ ...c, unit: v }))}
                  placeholder={unitDefault}
                  data-testid="inventory-item-unit"
                />
                <DecimalInput
                  // A3b: DecimalInput blur-clamp + live invalid for negatives /
                  // garbage (was raw text Input with submit-only validation).
                  label={t("itemForm.fields.currentQuantity")}
                  value={form.currentQuantity}
                  onValueChange={(v) => {
                    setQuantityDirty(true);
                    setForm((c) => ({ ...c, currentQuantity: v }));
                  }}
                  min={0}
                  isInvalid={
                    form.currentQuantity.trim() !== "" &&
                    (() => {
                      const n = tryParseLocaleDecimal(form.currentQuantity);
                      return n === null || n < 0;
                    })()
                  }
                  errorMessage={
                    form.currentQuantity.trim() !== "" &&
                    (() => {
                      const n = tryParseLocaleDecimal(form.currentQuantity);
                      return n === null || n < 0;
                    })()
                      ? t("errors.invalidCurrentQuantity")
                      : undefined
                  }
                  data-testid="inventory-current-quantity"
                />
                <DecimalInput
                  label={t("itemForm.fields.reorderThreshold")}
                  value={form.reorderThreshold}
                  onValueChange={(v) =>
                    setForm((c) => ({ ...c, reorderThreshold: v }))
                  }
                  min={0}
                  isInvalid={
                    form.reorderThreshold.trim() !== "" &&
                    (() => {
                      const n = tryParseLocaleDecimal(form.reorderThreshold);
                      return n === null || n < 0;
                    })()
                  }
                  errorMessage={
                    form.reorderThreshold.trim() !== "" &&
                    (() => {
                      const n = tryParseLocaleDecimal(form.reorderThreshold);
                      return n === null || n < 0;
                    })()
                      ? t("errors.invalidReorderThreshold")
                      : undefined
                  }
                  data-testid="inventory-reorder-threshold"
                />
                <DecimalInput
                  // type=text path via DecimalInput: native type=number strips
                  // commas (L5-36 silent 100×). min=0 clamps negatives on blur.
                  label={t("itemForm.fields.costPerUnit")}
                  value={form.costPerUnit}
                  onValueChange={(v) =>
                    setForm((c) => ({ ...c, costPerUnit: v }))
                  }
                  min={0}
                  isInvalid={
                    form.costPerUnit.trim() !== "" &&
                    (() => {
                      const n = tryParseLocaleDecimal(form.costPerUnit);
                      return n === null || n < 0;
                    })()
                  }
                  errorMessage={
                    form.costPerUnit.trim() !== "" &&
                    (() => {
                      const n = tryParseLocaleDecimal(form.costPerUnit);
                      return n === null || n < 0;
                    })()
                      ? t("errors.invalidCostPerUnit")
                      : undefined
                  }
                  startContent={
                    <Coins className="h-4 w-4 text-ink-400" aria-hidden="true" />
                  }
                  data-testid="inventory-cost-per-unit"
                />
                <div className="flex items-center justify-between rounded-2xl border border-warm-200 bg-warm-50/70 px-4 py-3">
                  <div>
                    <p className="font-semibold text-ink-950">
                      {t("itemForm.active.title")}
                    </p>
                    <p className="text-sm text-ink-600">
                      {t("itemForm.active.description")}
                    </p>
                  </div>
                  <Switch
                    isSelected={form.isActive}
                    onValueChange={(v) =>
                      setForm((c) => ({ ...c, isActive: v }))
                    }
                    classNames={{
                      wrapper: "group-data-[selected=true]:bg-brand",
                    }}
                  />
                </div>
              </div>
            </ModalBody>

            <ModalFooter className="border-t border-warm-200/80 bg-warm-50/70 px-6 py-4">
              <Button
                variant="flat"
                onPress={handleClose}
                isDisabled={saving}
                className="font-semibold text-ink-700 hover:bg-warm-100"
              >
                {t("itemForm.cancel")}
              </Button>
              <Button
                startContent={
                  mode === "edit" ? (
                    <Save className="h-4 w-4" />
                  ) : (
                    <PackagePlus className="h-4 w-4" />
                  )
                }
                onPress={() => void handleSubmit()}
                isLoading={saving}
                // L5-38: keep enabled when name is whitespace so press can
                // surface the localized name error (was silent disable).
                isDisabled={saving}
                className="bg-brand font-semibold text-white shadow-sm shadow-brand/20 hover:bg-brand-dark"
              >
                {mode === "edit"
                  ? t("itemForm.updateButton")
                  : t("itemForm.createButton")}
              </Button>
            </ModalFooter>
          </>
        )}
      </ModalContent>
    </Modal>
  );
}
