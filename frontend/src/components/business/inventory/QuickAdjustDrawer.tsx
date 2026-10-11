"use client";

import { getSafeApiErrorMessage } from "@/utils/apiError";

import React, { useEffect, useMemo, useState } from "react";
import { Button, Chip, Drawer, DrawerBody, DrawerContent, DrawerFooter, DrawerHeader, Textarea } from "@nextui-org/react";
import { ArrowDownCircle, ClipboardCheck, PackagePlus } from "lucide-react";

import { InventoryItem, InventoryMovement, inventoryApi } from "@/api/inventory";
import { tryParseLocaleDecimal } from "@/lib/parseLocaleDecimal";
import { DecimalInput } from "@/components/ui/DecimalInput";

export type AdjustVerb = "receive" | "waste" | "count";

interface QuickAdjustDrawerProps {
  isOpen: boolean;
  item: InventoryItem | null;
  businessId: number;
  onClose: () => void;
  /**
   * Called with the persisted item + movement so the parent can patch state in
   * place instead of refetching the whole inventory (audit §3.5 HIGH/C1).
   */
  onSaved: (result: {
    item: InventoryItem;
    movement: InventoryMovement;
  }) => void | Promise<void>;
  t: (key: string) => string;
}

const WASTE_REASONS = [
  "spoilage",
  "prep_waste",
  "server_error",
  "quality_reject",
  "other",
] as const;

export default function QuickAdjustDrawer({
  isOpen,
  item,
  businessId,
  onClose,
  onSaved,
  t,
}: QuickAdjustDrawerProps) {
  const [verb, setVerb] = useState<AdjustVerb>("receive");
  const [quantity, setQuantity] = useState("");
  const [counted, setCounted] = useState("");
  const [note, setNote] = useState("");
  const [reason, setReason] = useState<string>("spoilage");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Re-hydrate on every open so nothing leaks between items.
  useEffect(() => {
    if (isOpen) {
      setVerb("receive");
      setQuantity("");
      setCounted(item ? String(item.current_quantity) : "");
      setNote("");
      setReason("spoilage");
      setSaving(false);
      setError(null);
    }
  }, [isOpen, item]);

  const current = item?.current_quantity ?? 0;

  // The signed delta POSTed for the relative verbs (receive/waste). Count uses
  // an absolute target instead — see `target` below.
  // L5-39: tryParseLocaleDecimal so "2,5" is 2.5 (not NaN via bare Number).
  const delta = useMemo(() => {
    if (verb === "count") return 0;
    const q = tryParseLocaleDecimal(quantity);
    if (q === null || q <= 0) return 0;
    return verb === "waste" ? -q : q;
  }, [verb, quantity]);

  // The absolute on-hand value POSTed for a physical count. The backend
  // computes the delta against the LIVE locked row, so the operator's count
  // always wins even if stock moved since the drawer opened (INV-M1). A blank
  // field is "no input" (undefined), not 0 — so it can't accidentally zero the
  // stock. counted === current is still a valid count (it may differ from the
  // live value the backend will reconcile against).
  const target = useMemo(() => {
    if (verb !== "count") return undefined;
    if (!counted.trim()) return undefined;
    const c = tryParseLocaleDecimal(counted);
    if (c === null || c < 0) return undefined;
    return c;
  }, [verb, counted]);

  const canSave =
    !saving &&
    (verb === "count"
      ? target !== undefined
      : delta !== 0 && current + delta >= 0);

  // L5-39: error slot must never be empty while save is disabled for qty.
  // "2,5" used to hit Number → NaN → silent dead-end (no message, save off).
  const parsedQty =
    verb === "count"
      ? tryParseLocaleDecimal(counted)
      : tryParseLocaleDecimal(quantity);
  const quantityEmpty =
    verb !== "count" &&
    (!quantity.trim() || parsedQty === null || parsedQty <= 0);
  const overWaste =
    verb === "waste" &&
    quantity.trim() !== "" &&
    parsedQty !== null &&
    parsedQty > 0 &&
    current + delta < 0;
  const quantityUnparseable =
    verb !== "count" &&
    quantity.trim() !== "" &&
    parsedQty === null;
  const countedUnparseable =
    verb === "count" && counted.trim() !== "" && parsedQty === null;
  const quantityError = overWaste
    ? t("quickAdjust.overWaste").replace("{current}", String(current))
    : quantityEmpty && quantity.trim() === ""
      ? t("quickAdjust.quantityRequired")
      : quantityUnparseable || countedUnparseable
        ? t("quickAdjust.quantityInvalid") || t("quickAdjust.quantityRequired")
        : quantityEmpty
          ? t("quickAdjust.quantityRequired")
          : !canSave && !saving && verb !== "count"
            ? t("quickAdjust.quantityRequired")
            : undefined;

  const handleSave = async () => {
    if (!item || !canSave) return;
    try {
      setSaving(true);
      setError(null);
      const movementType =
        verb === "receive" ? "restock" : verb === "waste" ? "waste" : "correction";
      // Persist a STABLE enum key for the structured verbs (waste reason,
      // physical count), not the operator's translated label — otherwise the
      // ledger's reason column becomes locale-dependent and unstable. The
      // Activity list translates these keys back at render time. Free-text
      // notes on "receive" stay as-is. See R3-AC.
      const reasonText =
        verb === "waste"
          ? reason
          : verb === "count"
            ? "physical_count"
            : note.trim() || undefined;
      const result = await inventoryApi.createAdjustment(
        businessId,
        verb === "count"
          ? {
              inventory_item_id: item.id,
              movement_type: movementType,
              target_quantity: target,
              reason: reasonText,
            }
          : {
              inventory_item_id: item.id,
              movement_type: movementType,
              quantity_change: delta,
              reason: reasonText,
            },
      );
      await onSaved(result);
      onClose();
    } catch (err) {
      setError(getSafeApiErrorMessage(err, t("errors.saveAdjustment")));
    } finally {
      setSaving(false);
    }
  };

  const verbs: { key: AdjustVerb; icon: React.ReactNode }[] = [
    { key: "receive", icon: <PackagePlus className="h-4 w-4" /> },
    { key: "waste", icon: <ArrowDownCircle className="h-4 w-4" /> },
    { key: "count", icon: <ClipboardCheck className="h-4 w-4" /> },
  ];

  return (
    <Drawer
      isOpen={isOpen}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      placement="right"
      size="md"
    >
      <DrawerContent className="border-l border-warm-200 bg-white shadow-2xl shadow-warm-900/15">
        {() => (
          <>
            <DrawerHeader className="flex flex-col gap-1 border-b border-warm-200/80 bg-warm-50/70">
              <span className="text-lg font-semibold text-ink-950">
                {t("quickAdjust.title")}
              </span>
              {item && (
                <span className="text-sm text-ink-600">
                  {item.name} · {t("quickAdjust.currentOnHand")}: {current} {item.unit}
                </span>
              )}
            </DrawerHeader>

            <DrawerBody className="space-y-4">
              {error && (
                <div className="rounded-2xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm font-medium text-rose-700" role="alert">
                  {error}
                </div>
              )}

              {/* Verb segmented control */}
              <div className="grid grid-cols-3 gap-2">
                {verbs.map((v) => (
                  <button
                    key={v.key}
                    type="button"
                    onClick={() => setVerb(v.key)}
                    className={`flex flex-col items-center gap-1 rounded-xl border px-3 py-3 text-sm transition ${
                      verb === v.key
                        ? "border-brand bg-brand/5 text-brand"
                        : "border-warm-200 text-ink-700 hover:border-brand/30 hover:bg-warm-50"
                    }`}
                  >
                    {v.icon}
                    {t(`quickAdjust.verbs.${v.key}`)}
                  </button>
                ))}
              </div>
              <p className="text-sm text-ink-500">{t(`quickAdjust.hints.${verb}`)}</p>

              {verb === "count" ? (
                <>
                  <DecimalInput
                    label={t("quickAdjust.countedLabel")}
                    value={counted}
                    onValueChange={setCounted}
                    min={0}
                    data-testid="quick-adjust-counted"
                    isInvalid={countedUnparseable}
                    errorMessage={
                      countedUnparseable
                        ? t("quickAdjust.quantityInvalid") ||
                          t("quickAdjust.quantityRequired")
                        : undefined
                    }
                  />
                  {target !== undefined && (
                    <div className="rounded-2xl border border-warm-200 bg-warm-50/70 px-4 py-3 text-sm text-ink-700">
                      {t("quickAdjust.setOnHandPreview")
                        .replace("{counted}", String(target))
                        .replace("{unit}", item?.unit ?? "")}
                    </div>
                  )}
                </>
              ) : (
                <DecimalInput
                  label={t("quickAdjust.quantityLabel")}
                  value={quantity}
                  onValueChange={setQuantity}
                  min={0}
                  data-testid="quick-adjust-quantity"
                  isInvalid={!!quantityError}
                  errorMessage={quantityError}
                />
              )}

              {verb === "waste" && (
                <div>
                  <p className="mb-2 text-sm font-semibold text-ink-950">
                    {t("quickAdjust.reasonLabel")}
                  </p>
                  <div className="flex flex-wrap gap-2">
                    {WASTE_REASONS.map((r) => (
                      <Chip
                        key={r}
                        variant={reason === r ? "solid" : "flat"}
                        className={
                          reason === r
                            ? "cursor-pointer bg-brand text-white"
                            : "cursor-pointer border border-warm-200 bg-warm-100 text-ink-700"
                        }
                        onClick={() => setReason(r)}
                        role="button"
                        tabIndex={0}
                        onKeyDown={(e) => {
                          if (e.key === "Enter" || e.key === " ") {
                            e.preventDefault();
                            setReason(r);
                          }
                        }}
                      >
                        {t(`quickAdjust.reasons.${r}`)}
                      </Chip>
                    ))}
                  </div>
                </div>
              )}

              {verb === "receive" && (
                <Textarea
                  label={t("quickAdjust.noteLabel")}
                  value={note}
                  onValueChange={setNote}
                />
              )}
            </DrawerBody>

            <DrawerFooter className="border-t border-warm-200/80 bg-warm-50/70">
              <Button
                variant="flat"
                onPress={onClose}
                isDisabled={saving}
                className="font-semibold text-ink-700 hover:bg-warm-100"
              >
                {t("quickAdjust.cancel")}
              </Button>
              <Button
                onPress={() => void handleSave()}
                isDisabled={!canSave}
                isLoading={saving}
                data-testid="quick-adjust-save"
                className="bg-brand font-semibold text-white shadow-sm shadow-brand/20 hover:bg-brand-dark"
              >
                {t("quickAdjust.save")}
              </Button>
            </DrawerFooter>
          </>
        )}
      </DrawerContent>
    </Drawer>
  );
}
