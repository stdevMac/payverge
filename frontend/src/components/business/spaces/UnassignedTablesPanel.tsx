"use client";

/**
 * Assign legacy (QR-bearing) tables to a space without recreating them.
 * Calls POST .../spaces/:id/tables/assign — preserves table IDs and QR codes.
 */

import React, { useCallback, useMemo, useState } from "react";
import {
  Button,
  Checkbox,
  Select,
  SelectItem,
} from "@nextui-org/react";
import { Link2 } from "lucide-react";
import toast from "react-hot-toast";
import {
  spacesApi,
  type Space,
  type SpaceTableRef,
} from "@/api/spaces";
import { getSafeApiErrorMessage } from "@/utils/apiError";
import { PremiumPanel } from "../premium";

export interface UnassignedTablesPanelProps {
  businessId: number;
  spaces: Space[];
  unassigned: SpaceTableRef[];
  t: (key: string, params?: Record<string, string | number>) => string;
  onAssigned: () => void;
  /** Open the space editor after assign so the operator can place tables. */
  onOpenSpace?: (spaceId: number) => void;
}

export default function UnassignedTablesPanel({
  businessId,
  spaces,
  unassigned,
  t,
  onAssigned,
  onOpenSpace,
}: UnassignedTablesPanelProps) {
  const activeSpaces = useMemo(
    () => spaces.filter((s) => s.status !== "archived"),
    [spaces],
  );
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set());
  const [targetSpaceId, setTargetSpaceId] = useState<string>(
    activeSpaces[0] ? String(activeSpaces[0].id) : "",
  );
  const [busy, setBusy] = useState(false);

  // Keep target space valid when the list changes.
  React.useEffect(() => {
    if (
      targetSpaceId &&
      activeSpaces.some((s) => String(s.id) === targetSpaceId)
    ) {
      return;
    }
    setTargetSpaceId(activeSpaces[0] ? String(activeSpaces[0].id) : "");
  }, [activeSpaces, targetSpaceId]);

  const toggle = useCallback((id: number) => {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  const selectAll = () => {
    setSelectedIds(new Set(unassigned.map((u) => u.id)));
  };

  const clearSelection = () => setSelectedIds(new Set());

  const assign = async (tableIds: number[] | null) => {
    const spaceId = Number.parseInt(targetSpaceId, 10);
    if (!Number.isFinite(spaceId) || spaceId <= 0) {
      toast.error(t("assign.selectSpacePlaceholder"));
      return;
    }
    setBusy(true);
    try {
      await spacesApi.assignTables(
        businessId,
        spaceId,
        tableIds ? { table_ids: tableIds } : {},
      );
      toast.success(t("assign.success"));
      setSelectedIds(new Set());
      onAssigned();
      onOpenSpace?.(spaceId);
    } catch (err) {
      toast.error(getSafeApiErrorMessage(err, t("assign.error")));
    } finally {
      setBusy(false);
    }
  };

  if (unassigned.length === 0 || activeSpaces.length === 0) {
    return null;
  }

  return (
    <PremiumPanel
      className="p-4"
      withTexture={false}
      data-testid="unassigned-tables-panel"
    >
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h3 className="flex items-center gap-2 text-sm font-semibold text-ink-900">
            <Link2 className="h-4 w-4 text-brand" aria-hidden />
            {t("assign.title")}
          </h3>
          <p className="mt-1 text-xs leading-5 text-ink-600">{t("assign.body")}</p>
          <p className="mt-1 text-xs font-medium text-ink-500">
            {t(
              unassigned.length === 1 ? "assign.count_one" : "assign.count_other",
              { count: unassigned.length },
            )}
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Select
            aria-label={t("assign.selectSpace")}
            selectedKeys={targetSpaceId ? [targetSpaceId] : []}
            onSelectionChange={(keys) => {
              const next = Array.from(keys)[0];
              if (typeof next === "string") setTargetSpaceId(next);
            }}
            size="sm"
            variant="bordered"
            className="w-48"
            placeholder={t("assign.selectSpacePlaceholder")}
            classNames={{
              trigger:
                "h-9 min-h-9 bg-white border-warm-200 data-[hover=true]:border-brand/40",
            }}
            items={activeSpaces.map((s) => ({
              key: String(s.id),
              label: s.name,
            }))}
          >
            {(item) => <SelectItem key={item.key}>{item.label}</SelectItem>}
          </Select>
          <Button
            size="sm"
            radius="full"
            variant="bordered"
            className="border-warm-200"
            isDisabled={busy || selectedIds.size === 0}
            isLoading={busy && selectedIds.size > 0}
            onPress={() => void assign(Array.from(selectedIds))}
            data-testid="assign-selected"
          >
            {t("assign.assignSelected")}
          </Button>
          <Button
            size="sm"
            radius="full"
            className="bg-brand text-white font-medium"
            isDisabled={busy}
            isLoading={busy && selectedIds.size === 0}
            onPress={() => void assign(null)}
            data-testid="assign-all"
          >
            {t("assign.assignAll")}
          </Button>
        </div>
      </div>

      <div className="mt-3 flex flex-wrap gap-2">
        <button
          type="button"
          className="text-xs font-medium text-brand hover:text-brand-dark"
          onClick={selectAll}
        >
          {t("assign.selectAll")}
        </button>
        {selectedIds.size > 0 && (
          <button
            type="button"
            className="text-xs font-medium text-ink-500 hover:text-ink-700"
            onClick={clearSelection}
          >
            {t("assign.clearSelection")}
          </button>
        )}
      </div>

      <ul className="mt-3 grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
        {unassigned.map((table) => (
          <li
            key={table.id}
            className="flex items-center gap-2 rounded-xl border border-warm-200 bg-white px-3 py-2"
          >
            <Checkbox
              isSelected={selectedIds.has(table.id)}
              onValueChange={() => toggle(table.id)}
              aria-label={table.name}
              size="sm"
            />
            <div className="min-w-0">
              <p className="truncate text-sm font-medium text-ink-900">
                {table.name}
              </p>
              <p className="truncate text-xs text-ink-500">
                {table.table_code}
                {table.capacity ? ` · ${table.capacity}` : ""}
              </p>
            </div>
          </li>
        ))}
      </ul>
    </PremiumPanel>
  );
}
