"use client";

import React from "react";
import { Autocomplete, AutocompleteItem } from "@nextui-org/react";
import type { StaffMember } from "@/api/staff";

const UNASSIGNED_KEY = "__unassigned__";

export interface StaffSelectProps {
  label: string;
  staff: StaffMember[];
  /** Selected staff id, or null for unassigned / open shift. */
  value: number | null;
  onChange: (staffId: number | null) => void;
  /** When set, prepends a sentinel option (e.g. "— Open shift —") mapping to null. */
  unassignedLabel?: string;
  placeholder?: string;
  isDisabled?: boolean;
  className?: string;
}

function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  if (parts.length === 0) return "?";
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase();
  return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase();
}

function Avatar({ name }: { name: string }) {
  return (
    <span
      aria-hidden
      className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-brand/10 text-[10px] font-semibold text-brand"
    >
      {initials(name)}
    </span>
  );
}

/**
 * Premium, searchable staff picker — NextUI Autocomplete with initial-avatars,
 * so a long roster is type-to-filter instead of an endless native `<select>`.
 * An optional unassigned sentinel maps to `null` (open shift).
 */
export function StaffSelect({
  label,
  staff,
  value,
  onChange,
  unassignedLabel,
  placeholder,
  isDisabled = false,
  className,
}: StaffSelectProps) {
  const selectedKey =
    value != null ? String(value) : unassignedLabel ? UNASSIGNED_KEY : null;

  return (
    <Autocomplete
      label={label}
      placeholder={placeholder}
      aria-label={label}
      selectedKey={selectedKey}
      onSelectionChange={(key) => {
        if (key == null || key === UNASSIGNED_KEY) {
          onChange(null);
          return;
        }
        onChange(Number(key));
      }}
      isDisabled={isDisabled}
      labelPlacement="outside"
      variant="bordered"
      radius="lg"
      menuTrigger="input"
      className={className}
      inputProps={{ classNames: { label: "text-sm font-medium text-ink-700" } }}
    >
      {[
        ...(unassignedLabel
          ? [
              <AutocompleteItem key={UNASSIGNED_KEY} textValue={unassignedLabel}>
                <span className="text-ink-500">{unassignedLabel}</span>
              </AutocompleteItem>,
            ]
          : []),
        ...staff.map((s) => (
          <AutocompleteItem key={String(s.id)} textValue={s.name} startContent={<Avatar name={s.name} />}>
            {s.name}
          </AutocompleteItem>
        )),
      ]}
    </Autocomplete>
  );
}
