"use client";

import { useEffect, useMemo, useState } from "react";
import { Button, Checkbox } from "@nextui-org/react";

import type { ReservationTableOption } from "@/api/reservations";
import { humanizeDurationMinutes } from "@/utils/humanizeDuration";

interface ReservationTablePickerLabels {
  available: string;
  occupied: string;
  staleOccupied: string;
  override: string;
  overrideRequired: string;
  reservationConflict: string;
  capacityConflict: string;
  /** Optional group heading for tables without a space. */
  unassignedSpace?: string;
}

interface ReservationTablePickerProps {
  options: ReservationTableOption[];
  selectedTableId?: number;
  overrideAcknowledged: boolean;
  labels: ReservationTablePickerLabels;
  onSelect: (tableId: number) => void;
  onOverrideChange: (acknowledged: boolean) => void;
}

type SpaceGroup = {
  key: string;
  label: string;
  options: ReservationTableOption[];
};

function groupBySpace(
  options: ReservationTableOption[],
  unassignedLabel: string,
): SpaceGroup[] | null {
  const anySpace = options.some(
    (o) => o.space_id != null && o.space_id > 0,
  );
  if (!anySpace) return null;

  const map = new Map<string, SpaceGroup>();
  for (const option of options) {
    const hasSpace = option.space_id != null && option.space_id > 0;
    const key = hasSpace ? `space-${option.space_id}` : "unassigned";
    const label = hasSpace
      ? option.space_name?.trim() || `Space ${option.space_id}`
      : unassignedLabel;
    let group = map.get(key);
    if (!group) {
      group = { key, label, options: [] };
      map.set(key, group);
    }
    group.options.push(option);
  }

  // Stable-ish: named spaces first (by label), unassigned last.
  return Array.from(map.values()).sort((a, b) => {
    if (a.key === "unassigned") return 1;
    if (b.key === "unassigned") return -1;
    return a.label.localeCompare(b.label);
  });
}

export function ReservationTablePicker({
  options,
  selectedTableId,
  overrideAcknowledged,
  labels,
  onSelect,
  onOverrideChange,
}: ReservationTablePickerProps) {
  const [pendingSelection, setPendingSelection] = useState(selectedTableId);
  useEffect(() => setPendingSelection(selectedTableId), [selectedTableId]);

  const sorted = useMemo(
    () => [...options].sort((a, b) => Number(b.recommended) - Number(a.recommended)),
    [options],
  );
  const groups = useMemo(
    () =>
      groupBySpace(
        sorted,
        labels.unassignedSpace || "Unassigned",
      ),
    [sorted, labels.unassignedSpace],
  );
  const selected = options.find((option) => option.id === pendingSelection);

  const occupancyLabel = (option: ReservationTableOption) => {
    if (option.conflict_reason === "capacity") return labels.capacityConflict;
    if (option.conflict_reason === "reservation_conflict") {
      return labels.reservationConflict;
    }
    if (option.occupancy_state === "stale_occupied") {
      return labels.staleOccupied;
    }
    if (option.occupancy_state === "occupied") return labels.occupied;
    return labels.available;
  };

  const renderButton = (option: ReservationTableOption) => {
    const status = occupancyLabel(option);
    return (
      <Button
        key={option.id}
        aria-label={`${option.name}, ${status}`}
        color={pendingSelection === option.id ? "primary" : "default"}
        variant={pendingSelection === option.id ? "solid" : "bordered"}
        isDisabled={
          !option.reservation_available ||
          option.conflict_reason === "capacity"
        }
        onPress={() => {
          setPendingSelection(option.id);
          if (option.id !== pendingSelection) onOverrideChange(false);
          onSelect(option.id);
        }}
      >
        {option.name} ({option.capacity}) · {status}
        {option.active_bill_age_minutes
          ? ` · ${humanizeDurationMinutes(option.active_bill_age_minutes)}`
          : ""}
      </Button>
    );
  };

  return (
    <div className="space-y-3">
      {groups ? (
        <div className="space-y-3" data-testid="reservation-table-picker-grouped">
          {groups.map((group) => (
            <div key={group.key} className="space-y-1.5">
              <p className="text-xs font-medium uppercase tracking-wider text-ink-500">
                {group.label}
              </p>
              <div className="flex flex-wrap gap-2">
                {group.options.map(renderButton)}
              </div>
            </div>
          ))}
        </div>
      ) : (
        <div className="flex flex-wrap gap-2" data-testid="reservation-table-picker-flat">
          {sorted.map(renderButton)}
        </div>
      )}
      {selected?.requires_occupancy_override ? (
        <div className="space-y-1 rounded-xl border border-amber-200 bg-amber-50 p-3">
          <Checkbox
            isSelected={overrideAcknowledged}
            onValueChange={onOverrideChange}
          >
            {labels.override}
          </Checkbox>
          {!overrideAcknowledged ? (
            <p className="text-sm text-amber-800">{labels.overrideRequired}</p>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}
