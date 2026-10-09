"use client";

import React, { useMemo } from "react";
import { Select, SelectItem } from "@nextui-org/react";

// Shared audience picker for announcements / polls / documents. Replaces the
// per-composer native <select> with a NextUI Select (a11y: labelled trigger +
// option roles NextUI provides). The value is the server audience token
// ("all" | "role:<role>" | "dept:<dept>"). The collection is assembled as ONE
// items array (never a static SelectItem sibling next to a mapped array — that
// trips NextUI's TS2322), grouped by a leading disabled header row so roles and
// departments read as sections without depending on SelectSection typing.

const ROLE_KEYS = ["manager", "server", "host", "kitchen"] as const;

export interface AudienceSelectProps {
  label: string;
  value: string;
  onChange: (value: string) => void;
  departments: string[];
  /** Localizer for the audience.* keys (allLabel, roleGroup, deptGroup, roles.*). */
  t: (key: string) => string;
  /** aria-label fallback when the visible label is elsewhere. */
  ariaLabel?: string;
  isDisabled?: boolean;
  className?: string;
}

interface AudienceOption {
  key: string;
  label: string;
  /** A non-selectable section header row. */
  isHeader?: boolean;
}

export function AudienceSelect({
  label,
  value,
  onChange,
  departments,
  t,
  ariaLabel,
  isDisabled = false,
  className,
}: AudienceSelectProps) {
  const options = useMemo<AudienceOption[]>(() => {
    const out: AudienceOption[] = [{ key: "all", label: t("all") }];
    out.push({ key: "__role_header__", label: t("roleGroup"), isHeader: true });
    ROLE_KEYS.forEach((r) => out.push({ key: `role:${r}`, label: t(`roles.${r}`) }));
    if (departments.length > 0) {
      out.push({ key: "__dept_header__", label: t("deptGroup"), isHeader: true });
      departments.forEach((d) => out.push({ key: `dept:${d}`, label: d }));
    }
    return out;
  }, [departments, t]);

  const disabledKeys = useMemo(
    () => options.filter((o) => o.isHeader).map((o) => o.key),
    [options],
  );

  return (
    <Select
      label={label}
      aria-label={ariaLabel ?? label}
      selectedKeys={[value]}
      disabledKeys={disabledKeys}
      onChange={(e) => {
        if (e.target.value) onChange(e.target.value);
      }}
      isDisabled={isDisabled}
      labelPlacement="outside"
      variant="bordered"
      radius="lg"
      className={className}
      classNames={{ label: "text-sm font-semibold text-ink-700" }}
    >
      {options.map((o) =>
        o.isHeader ? (
          <SelectItem
            key={o.key}
            textValue={o.label}
            isReadOnly
            className="pointer-events-none text-xs font-semibold uppercase tracking-wide text-ink-400"
          >
            {o.label}
          </SelectItem>
        ) : (
          <SelectItem key={o.key} textValue={o.label}>
            {o.label}
          </SelectItem>
        ),
      )}
    </Select>
  );
}
