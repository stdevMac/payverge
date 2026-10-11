"use client";

import React from "react";
import { Select, SelectItem } from "@nextui-org/react";
import type { Position } from "@/api/positions";
import { translatePositionName } from "@/components/business/schedule/positionLabel";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";

export interface PositionSelectProps {
  label: string;
  positions: Position[];
  /** Selected position id, or null when nothing is chosen. */
  value: number | null;
  onChange: (positionId: number | null) => void;
  placeholder?: string;
  isRequired?: boolean;
  isDisabled?: boolean;
  errorMessage?: string;
  isInvalid?: boolean;
  className?: string;
}

function Dot({ color }: { color: string }) {
  // A real per-role hex drives the swatch inline; when a position has no color
  // set we fall back to a neutral token class instead of a hardcoded hex.
  return (
    <span
      aria-hidden
      className={`inline-block h-2.5 w-2.5 shrink-0 rounded-full ring-1 ring-black/5 ${color ? "" : "bg-ink-300"}`}
      style={color ? { backgroundColor: color } : undefined}
    />
  );
}

/**
 * Premium position picker — NextUI Select themed to the app, with each role's
 * color swatch shown both in the list and in the trigger. Replaces the raw
 * native `<select>` the schedule surfaces used to ship.
 */
export function PositionSelect({
  label,
  positions,
  value,
  onChange,
  placeholder,
  isRequired = false,
  isDisabled = false,
  errorMessage,
  isInvalid = false,
  className,
}: PositionSelectProps) {
  const { locale } = useSimpleLocale();
  // L5-29: same bilingual defaults as the schedule grid (seeded Manager/Server/…).
  const displayName = (name: string) =>
    translatePositionName(name, (key) => {
      const full = `dashboardSchedule.${key}`;
      const v = getTranslation(full, locale);
      return Array.isArray(v) ? v[0] || name : (v as string) || name;
    });
  const selected = positions.find((p) => p.id === value) ?? null;

  return (
    <Select
      label={label}
      placeholder={placeholder ?? "—"}
      selectedKeys={value != null ? [String(value)] : []}
      onChange={(e) => onChange(e.target.value ? Number(e.target.value) : null)}
      isRequired={isRequired}
      isDisabled={isDisabled}
      isInvalid={isInvalid}
      errorMessage={errorMessage}
      labelPlacement="outside"
      variant="bordered"
      radius="lg"
      className={className}
      classNames={{ label: "text-sm font-medium text-ink-700" }}
      renderValue={() =>
        selected ? (
          <span className="flex items-center gap-2">
            <Dot color={selected.color_hex} />
            <span className="truncate">{displayName(selected.name)}</span>
          </span>
        ) : null
      }
    >
      {positions.map((p) => (
        <SelectItem
          key={String(p.id)}
          textValue={displayName(p.name)}
          startContent={<Dot color={p.color_hex} />}
        >
          {displayName(p.name)}
        </SelectItem>
      ))}
    </Select>
  );
}
