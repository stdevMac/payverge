"use client";

import React from "react";
import { Button, Chip, Input, Select, SelectItem } from "@nextui-org/react";
import { Download, Search } from "lucide-react";

import {
  InventoryFilterState,
  InventorySort,
  InventoryStatusFilter,
} from "./useInventoryFilter";

interface InventoryToolbarProps {
  state: InventoryFilterState;
  onChange: (patch: Partial<InventoryFilterState>) => void;
  categories: string[];
  counts: { all: number; healthy: number; low: number; out: number };
  shown: number;
  total: number;
  onExport: () => void;
  t: (key: string) => string;
  tWith: (key: string, replacements: Record<string, string | number>) => string;
}

const STATUS_CHIPS: {
  key: InventoryStatusFilter;
  labelKey: string;
  countKey: keyof InventoryToolbarProps["counts"];
}[] = [
  { key: "all", labelKey: "toolbar.statusAll", countKey: "all" },
  { key: "healthy", labelKey: "toolbar.statusHealthy", countKey: "healthy" },
  { key: "low", labelKey: "toolbar.statusLow", countKey: "low" },
  { key: "out", labelKey: "toolbar.statusOut", countKey: "out" },
];

const SORTS: InventorySort[] = ["attention", "name", "value", "quantity", "category"];

export default function InventoryToolbar({
  state,
  onChange,
  categories,
  counts,
  shown,
  total,
  onExport,
  t,
  tWith,
}: InventoryToolbarProps) {
  const categoryItems = [
    { key: "", label: t("toolbar.allCategories") },
    ...categories.map((c) => ({ key: c, label: c })),
  ];

  return (
    <div className="space-y-3">
      {/* Stack through tablet (~1024): a single md:flex-row row crushed search
          to "S" and category to "All cate…". Horizontal controls start at lg. */}
      <div className="flex flex-col gap-3 lg:flex-row lg:items-center">
        <Input
          className="w-full min-w-0 flex-1 lg:min-w-[14rem] lg:max-w-sm"
          value={state.search}
          onValueChange={(v) => onChange({ search: v })}
          placeholder={t("toolbar.searchPlaceholder")}
          startContent={<Search className="h-4 w-4 text-ink-400" />}
          isClearable
          onClear={() => onChange({ search: "" })}
          aria-label={t("toolbar.searchPlaceholder")}
        />

        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:flex lg:min-w-0 lg:flex-1 lg:items-center lg:gap-3">
          <Select
            className="w-full min-w-0 lg:max-w-[220px]"
            selectedKeys={state.category ? [state.category] : [""]}
            onSelectionChange={(keys) =>
              onChange({ category: (Array.from(keys)[0] as string) || "" })
            }
            placeholder={t("toolbar.allCategories")}
            aria-label={t("toolbar.allCategories")}
          >
            {categoryItems.map((ci) => (
              <SelectItem key={ci.key}>{ci.label}</SelectItem>
            ))}
          </Select>

          <Select
            className="w-full min-w-0 lg:max-w-[220px]"
            selectedKeys={[state.sort]}
            onSelectionChange={(keys) =>
              onChange({ sort: (Array.from(keys)[0] as InventorySort) || "attention" })
            }
            aria-label={t("toolbar.sortLabel")}
            startContent={
              <span className="text-xs text-ink-500">{t("toolbar.sortLabel")}</span>
            }
          >
            {SORTS.map((s) => (
              <SelectItem key={s}>{t(`toolbar.sort.${s}`)}</SelectItem>
            ))}
          </Select>
        </div>

        <div className="flex flex-wrap items-center gap-2 lg:ml-auto lg:flex-nowrap">
          <Button
            variant="flat"
            startContent={<Download className="h-4 w-4" />}
            onPress={onExport}
            data-testid="inventory-export-csv"
            className="bg-brand/10 font-semibold text-brand-dark hover:bg-brand/15"
          >
            {t("toolbar.export")}
          </Button>
        </div>
      </div>

      <div
        className="flex flex-wrap items-center gap-2"
        data-testid="inventory-status-filter"
        role="group"
        aria-label={t("toolbar.statusFilter")}
      >
        {STATUS_CHIPS.map((chip) => (
          <Chip
            key={chip.key}
            variant={state.status === chip.key ? "solid" : "flat"}
            className={
              state.status === chip.key
                ? "cursor-pointer bg-brand text-white"
                : "cursor-pointer border border-warm-200 bg-warm-100 text-ink-700"
            }
            onClick={() => onChange({ status: chip.key })}
            role="button"
            tabIndex={0}
            onKeyDown={(e) => {
              if (e.key === "Enter" || e.key === " ") {
                e.preventDefault();
                onChange({ status: chip.key });
              }
            }}
            data-testid={`inventory-status-chip-${chip.key}`}
          >
            {t(chip.labelKey)} · {counts[chip.countKey]}
          </Chip>
        ))}
        <span className="ml-auto text-sm text-ink-500">
          {tWith("toolbar.resultCount", { shown, total })}
        </span>
      </div>
    </div>
  );
}
