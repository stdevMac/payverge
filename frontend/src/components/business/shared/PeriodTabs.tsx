"use client";

import React from "react";
import { Tabs, Tab } from "@nextui-org/react";

interface PeriodTabsOption<K extends string> {
  key: K;
  label: string;
}

export interface PeriodTabsProps<K extends string> {
  options: PeriodTabsOption<K>[];
  value: K;
  onChange: (key: K) => void;
  ariaLabel: string;
  className?: string;
}

/**
 * Compact period selector shared by the accounting analysis cards. Extracted
 * from three near-identical inline NextUI Tabs blocks:
 *   - FoodCostKpiCard  (accounting/accountingShared.tsx via CostAnalyticsSection)
 *   - LaborCostCard    (accounting/LaborCostCard.tsx)
 *   - WasteVarianceCard (accounting/WasteVarianceCard.tsx)
 *
 * The shadow-variant cursor (`shadow-sm shadow-warm-900/5`) is canonical here —
 * it was used by two of the three donors.
 *
 * Generic over the option-key union so consumers get a typed `onChange`
 * without hand-written guards. Every rendered tab key comes from `options`,
 * so the single `as K` cast below (NextUI's onSelectionChange hands back a
 * widened `Key`) is safe and lives here instead of in every consumer.
 */
export default function PeriodTabs<K extends string>({
  options,
  value,
  onChange,
  ariaLabel,
  className = "shrink-0",
}: PeriodTabsProps<K>) {
  // Sentinel / empty value (L6-9 custom range) must not match a real option —
  // NextUI Tabs with selectedKey="" auto-picks the first tab and fires
  // onSelectionChange, which would clear Personalizado via setPeriod.
  const known = options.some((o) => o.key === value);
  const selectedKey = known ? value : null;
  return (
    <Tabs
      aria-label={ariaLabel}
      selectedKey={selectedKey}
      onSelectionChange={(key) => {
        if (key == null || key === "") return;
        const next = String(key) as K;
        if (!options.some((o) => o.key === next)) return;
        onChange(next);
      }}
      size="sm"
      radius="full"
      variant="solid"
      className={className}
      classNames={{
        tabList: "h-7 gap-0 bg-warm-100 p-0.5 max-w-full overflow-x-auto",
        tab: "h-6 px-2 text-[11px]",
        cursor: "bg-white shadow-sm shadow-warm-900/5",
      }}
    >
      {options.map((opt) => (
        <Tab key={opt.key} title={opt.label} />
      ))}
    </Tabs>
  );
}
