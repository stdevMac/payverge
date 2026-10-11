"use client";

import React from "react";
import {
  Button,
  Chip,
  Drawer,
  DrawerBody,
  DrawerContent,
  DrawerFooter,
  DrawerHeader,
} from "@nextui-org/react";
import { Pencil, SlidersHorizontal, Trash2 } from "lucide-react";

import {
  InventoryItem,
  InventoryMenuItemStatus,
} from "@/api/inventory";
import { itemStatus, selectItemValue } from "./inventorySelectors";
import { useItemMovementHistory } from "./useInventoryMovements";

interface ItemDetailDrawerProps {
  isOpen: boolean;
  item: InventoryItem | null;
  businessId: number;
  usedBy: InventoryMenuItemStatus[];
  formatValue: (amount: number) => string;
  formatQty: (qty: number, unit: string) => string;
  statusLabel: (status: string) => string;
  statusTone: (status: string) => "primary" | "warning" | "danger" | "default";
  movementTypeLabel: (type: string) => string;
  onClose: () => void;
  onAdjust: (item: InventoryItem) => void;
  onEdit: (item: InventoryItem) => void;
  onDelete: (item: InventoryItem) => void;
  t: (key: string) => string;
}

export default function ItemDetailDrawer({
  isOpen,
  item,
  businessId,
  usedBy,
  formatValue,
  formatQty,
  statusLabel,
  statusTone,
  movementTypeLabel,
  onClose,
  onAdjust,
  onEdit,
  onDelete,
  t,
}: ItemDetailDrawerProps) {
  // Per-item movement history is fetched server-side (scoped to this item) with
  // load-more, so physical counts and adjustments are auditable past the old
  // 25-global-row window. Only fetch while the drawer is open on an item.
  const {
    movements,
    total,
    loading: movementsLoading,
    hasMore,
    loadMore,
  } = useItemMovementHistory(businessId, isOpen && item ? item.id : null);
  return (
    <Drawer
      isOpen={isOpen}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      placement="right"
      size="md"
    >
      <DrawerContent className="overflow-hidden border-l border-warm-200 bg-white shadow-2xl shadow-warm-900/15">
        {() =>
          item ? (
            <>
              <DrawerHeader className="flex flex-col gap-3 border-b border-warm-200/80 bg-warm-50/70 px-6 py-5">
                <span className="text-lg font-semibold tracking-normal text-ink-950">{item.name}</span>
                <div className="flex flex-wrap items-center gap-2">
                  <Chip size="sm" color={statusTone(itemStatus(item))} variant="flat">
                    {statusLabel(itemStatus(item))}
                  </Chip>
                  {item.category && (
                    <Chip
                      size="sm"
                      variant="flat"
                      className="border border-warm-200/80 bg-white text-ink-600"
                    >
                      {item.category}
                    </Chip>
                  )}
                </div>
              </DrawerHeader>

              <DrawerBody className="space-y-5 bg-white px-6 py-5">
                {/* Stat grid */}
                <div className="grid grid-cols-2 gap-3">
                  <div className="rounded-2xl border border-warm-200/80 bg-warm-50/60 px-4 py-3 shadow-sm shadow-warm-900/5">
                    <p className="text-xs font-medium text-ink-500">{t("valuation.onHand")}</p>
                    <p className="text-lg font-semibold text-ink-950">
                      {formatQty(item.current_quantity, item.unit)}
                    </p>
                  </div>
                  <div className="rounded-2xl border border-warm-200/80 bg-warm-50/60 px-4 py-3 shadow-sm shadow-warm-900/5">
                    <p className="text-xs font-medium text-ink-500">{t("items.tableColumns.threshold")}</p>
                    <p className="text-lg font-semibold text-ink-950">
                      {item.reorder_threshold > 0
                        ? formatQty(item.reorder_threshold, item.unit)
                        : "—"}
                    </p>
                  </div>
                  <div className="rounded-2xl border border-warm-200/80 bg-warm-50/60 px-4 py-3 shadow-sm shadow-warm-900/5">
                    <p className="text-xs font-medium text-ink-500">{t("valuation.unitCost")}</p>
                    <p className="text-lg font-semibold text-ink-950">{formatValue(item.cost_per_unit)}</p>
                  </div>
                  <div className="rounded-2xl border border-warm-200/80 bg-warm-50/60 px-4 py-3 shadow-sm shadow-warm-900/5">
                    <p className="text-xs font-medium text-ink-500">{t("valuation.valueColumn")}</p>
                    <p className="text-lg font-semibold text-ink-950">{formatValue(selectItemValue(item))}</p>
                  </div>
                </div>

                {/* Used by */}
                <div>
                  <h4 className="mb-2 text-sm font-semibold text-ink-950">{t("detail.usedBy")}</h4>
                  {usedBy.length === 0 ? (
                    <p className="rounded-2xl border border-dashed border-warm-200 bg-warm-50/60 px-3 py-3 text-sm text-ink-500">
                      {t("detail.usedByEmpty")}
                    </p>
                  ) : (
                    <div className="flex flex-wrap gap-2">
                      {usedBy.map((m) => (
                        <Chip
                          key={m.menu_item_id}
                          size="sm"
                          color={statusTone(m.status)}
                          variant="flat"
                        >
                          {m.menu_item_name}
                        </Chip>
                      ))}
                    </div>
                  )}
                </div>

                {/* Recent movements — server-scoped to this item, load-more */}
                <div>
                  <div className="mb-2 flex items-center justify-between">
                    <h4 className="text-sm font-semibold text-ink-950">
                      {t("detail.recentMovements")}
                    </h4>
                    {total > 0 && (
                      <span className="text-xs font-medium text-ink-500">
                        {total}
                      </span>
                    )}
                  </div>
                  {movementsLoading && movements.length === 0 ? (
                    <div className="space-y-2" aria-hidden>
                      {[0, 1, 2].map((i) => (
                        <div
                          key={i}
                          className="h-10 animate-pulse rounded-2xl border border-warm-200/60 bg-warm-100/60"
                        />
                      ))}
                    </div>
                  ) : movements.length === 0 ? (
                    <p className="rounded-2xl border border-dashed border-warm-200 bg-warm-50/60 px-3 py-3 text-sm text-ink-500">
                      {t("detail.noMovements")}
                    </p>
                  ) : (
                    <div className="space-y-2">
                      {movements.map((mv) => (
                        <div
                          key={mv.id}
                          className="flex items-center justify-between rounded-2xl border border-warm-200/80 bg-white px-3 py-2 text-sm shadow-sm shadow-warm-900/5"
                        >
                          <span className="font-medium text-ink-700">{movementTypeLabel(mv.movement_type)}</span>
                          <Chip
                            size="sm"
                            variant="flat"
                            className={
                              mv.quantity_delta < 0
                                ? "bg-rose-50 font-semibold text-rose-700"
                                : "bg-emerald-50 font-semibold text-emerald-700"
                            }
                          >
                            {mv.quantity_delta > 0 ? "+" : ""}
                            {mv.quantity_delta}
                          </Chip>
                        </div>
                      ))}
                      {hasMore && (
                        <Button
                          size="sm"
                          variant="flat"
                          fullWidth
                          isLoading={movementsLoading}
                          onPress={loadMore}
                          className="border border-warm-200 bg-white font-semibold text-ink-700 hover:bg-warm-100"
                        >
                          {t("detail.loadMoreMovements")}
                        </Button>
                      )}
                    </div>
                  )}
                </div>
              </DrawerBody>

              <DrawerFooter className="justify-between border-t border-warm-200/80 bg-warm-50/70 px-6 py-4">
                <Button
                  startContent={<SlidersHorizontal className="h-4 w-4" />}
                  onPress={() => onAdjust(item)}
                  className="bg-brand font-semibold text-white shadow-sm shadow-brand/20 transition-colors hover:bg-brand-dark"
                >
                  {t("detail.adjust")}
                </Button>
                <div className="flex gap-2">
                  <Button
                    variant="flat"
                    startContent={<Pencil className="h-4 w-4" />}
                    onPress={() => onEdit(item)}
                    className="border border-warm-200 bg-white font-semibold text-ink-700 transition-colors hover:bg-warm-100"
                  >
                    {t("detail.edit")}
                  </Button>
                  <Button
                    variant="flat"
                    startContent={<Trash2 className="h-4 w-4" />}
                    onPress={() => onDelete(item)}
                    className="border border-rose-200 bg-rose-50 font-semibold text-rose-700 transition-colors hover:bg-rose-100"
                  >
                    {t("detail.delete")}
                  </Button>
                </div>
              </DrawerFooter>
            </>
          ) : (
            <DrawerBody />
          )
        }
      </DrawerContent>
    </Drawer>
  );
}
