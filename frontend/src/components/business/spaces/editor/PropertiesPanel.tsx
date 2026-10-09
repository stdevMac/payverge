"use client";

/* eslint-disable no-restricted-syntax -- region color input default uses brand hex */

import React, { useMemo } from "react";
import { Input, Switch, Select, SelectItem } from "@nextui-org/react";
import type { TableShape } from "@/api/spaces";
import type {
  EditorDocument,
  EditorElement,
  EditorRegion,
  EditorTable,
  SelectionRef,
} from "./types";
import { TABLE_SHAPES } from "./types";
import { fromMm, toMm } from "./geometry";

interface PropertiesPanelProps {
  t: (key: string, params?: Record<string, string | number>) => string;
  doc: EditorDocument;
  selection: SelectionRef[];
  onUpdateTable: (key: string, patch: Partial<EditorTable>) => void;
  onUpdateElement: (key: string, patch: Partial<EditorElement>) => void;
  onUpdateRegion: (key: string, patch: Partial<EditorRegion>) => void;
  tableQrLinks?: Record<number, string>;
}

function unitLabel(unit: string): "m" | "ft" | "mm" {
  if (unit === "ft") return "ft";
  if (unit === "mm") return "mm";
  return "m";
}

export function PropertiesPanel({
  t,
  doc,
  selection,
  onUpdateTable,
  onUpdateElement,
  onUpdateRegion,
  tableQrLinks = {},
}: PropertiesPanelProps) {
  const unit = unitLabel(String(doc.measurement_unit || "m"));

  const selected = useMemo(() => {
    if (selection.length !== 1) return null;
    const ref = selection[0];
    if (ref.kind === "table") {
      const table = doc.tables.find((x) => x.clientKey === ref.key);
      return table ? ({ kind: "table" as const, table }) : null;
    }
    if (ref.kind === "element") {
      const element = doc.elements.find((x) => x.clientKey === ref.key);
      return element ? ({ kind: "element" as const, element }) : null;
    }
    const region = doc.regions.find((x) => x.clientKey === ref.key);
    return region ? ({ kind: "region" as const, region }) : null;
  }, [selection, doc]);

  if (selection.length === 0) {
    return (
      <aside
        className="flex h-full w-full min-w-0 flex-col gap-3 overflow-y-auto bg-white p-3"
        data-testid="properties-panel"
        data-properties-state="empty"
      >
        <h3 className="text-xs font-semibold uppercase tracking-wider text-ink-500">
          {t("editor.properties.title")}
        </h3>
        <p className="text-sm text-ink-500" data-testid="properties-empty">
          {t("editor.properties.empty")}
        </p>
        <div className="mt-2 space-y-2 rounded-xl border border-warm-100 bg-warm-50 p-3 text-xs text-ink-600">
          <p>
            {t("editor.properties.roomSize")}:{" "}
            <span className="font-medium text-ink-800">
              {fromMm(doc.width_mm, unit).toFixed(unit === "mm" ? 0 : 2)}
              {" × "}
              {fromMm(doc.height_mm, unit).toFixed(unit === "mm" ? 0 : 2)}{" "}
              {unit}
            </span>
          </p>
          <p>
            {t("editor.properties.tableCount")}:{" "}
            <span className="font-medium text-ink-800">{doc.tables.length}</span>
          </p>
          <p>
            {t("editor.properties.regionCount")}:{" "}
            <span className="font-medium text-ink-800">{doc.regions.length}</span>
          </p>
        </div>
      </aside>
    );
  }

  if (selection.length > 1) {
    return (
      <aside
        className="flex h-full w-full min-w-0 flex-col gap-3 bg-white p-3"
        data-testid="properties-panel"
        data-properties-state="multi"
      >
        <h3 className="text-xs font-semibold uppercase tracking-wider text-ink-500">
          {t("editor.properties.title")}
        </h3>
        <p className="text-sm text-ink-600">
          {t("editor.properties.multiSelect", { count: selection.length })}
        </p>
      </aside>
    );
  }

  if (!selected) {
    return (
      <aside
        className="flex h-full w-full min-w-0 flex-col gap-3 overflow-y-auto bg-white p-3"
        data-testid="properties-panel"
        data-properties-state="empty"
      >
        <h3 className="text-xs font-semibold uppercase tracking-wider text-ink-500">
          {t("editor.properties.title")}
        </h3>
        <p className="text-sm text-ink-500" data-testid="properties-empty">
          {t("editor.properties.empty")}
        </p>
      </aside>
    );
  }

  if (selected.kind === "table") {
    const table = selected.table;
    const qr = table.table_id ? tableQrLinks[table.table_id] : undefined;
    return (
      <aside
        className="flex h-full w-full min-w-0 flex-col gap-3 overflow-y-auto bg-white p-3"
        data-testid="properties-panel"
        data-properties-state="table"
        data-selected-name={table.name ?? ""}
      >
        <h3 className="text-xs font-semibold uppercase tracking-wider text-ink-500">
          {t("editor.properties.table")}
        </h3>
        <Input
          size="sm"
          label={t("editor.properties.name")}
          value={table.name ?? ""}
          onValueChange={(v) => onUpdateTable(table.clientKey, { name: v })}
          variant="bordered"
          classNames={{ inputWrapper: "border-warm-200" }}
        />
        <Select
          size="sm"
          label={t("editor.properties.shape")}
          selectedKeys={[table.shape || "square"]}
          onSelectionChange={(keys) => {
            const v = Array.from(keys)[0] as TableShape;
            if (v) onUpdateTable(table.clientKey, { shape: v });
          }}
          variant="bordered"
          classNames={{ trigger: "border-warm-200" }}
        >
          {TABLE_SHAPES.map((s) => (
            <SelectItem key={s} value={s}>
              {t(`editor.shapes.${s}`)}
            </SelectItem>
          ))}
        </Select>
        <div className="grid grid-cols-2 gap-2">
          <Input
            size="sm"
            type="number"
            label={t("editor.properties.width")}
            value={String(
              Math.round(fromMm(table.width_mm, unit) * 100) / 100,
            )}
            onValueChange={(v) => {
              const n = Number(v);
              if (!Number.isFinite(n) || n <= 0) return;
              onUpdateTable(table.clientKey, { width_mm: Math.round(toMm(n, unit)) });
            }}
            variant="bordered"
            endContent={<span className="text-xs text-ink-400">{unit}</span>}
            classNames={{ inputWrapper: "border-warm-200" }}
          />
          <Input
            size="sm"
            type="number"
            label={t("editor.properties.height")}
            value={String(
              Math.round(fromMm(table.height_mm, unit) * 100) / 100,
            )}
            onValueChange={(v) => {
              const n = Number(v);
              if (!Number.isFinite(n) || n <= 0) return;
              onUpdateTable(table.clientKey, {
                height_mm: Math.round(toMm(n, unit)),
              });
            }}
            variant="bordered"
            endContent={<span className="text-xs text-ink-400">{unit}</span>}
            classNames={{ inputWrapper: "border-warm-200" }}
          />
        </div>
        <Input
          size="sm"
          type="number"
          label={t("editor.properties.rotation")}
          value={String(table.rotation_deg ?? 0)}
          onValueChange={(v) => {
            const n = Number(v);
            if (!Number.isFinite(n)) return;
            onUpdateTable(table.clientKey, { rotation_deg: n });
          }}
          variant="bordered"
          endContent={<span className="text-xs text-ink-400">°</span>}
          classNames={{ inputWrapper: "border-warm-200" }}
        />
        <div className="grid grid-cols-2 gap-2">
          <Input
            size="sm"
            type="number"
            min={1}
            max={99}
            label={t("editor.properties.minSeats")}
            value={String(table.min_capacity ?? 1)}
            onValueChange={(v) => {
              // L3-24: clamp 1..99 (0 was previously allowed).
              const n = Math.min(99, Math.max(1, Math.floor(Number(v) || 1)));
              const maxCap = table.max_capacity ?? 99;
              onUpdateTable(table.clientKey, {
                min_capacity: Math.min(n, maxCap),
              });
            }}
            isInvalid={
              (table.min_capacity ?? 1) < 1 ||
              (table.min_capacity ?? 1) > 99 ||
              (table.min_capacity ?? 1) > (table.max_capacity ?? 99)
            }
            errorMessage={
              (table.min_capacity ?? 1) > (table.max_capacity ?? 99)
                ? t("editor.properties.capacityMinMax")
                : (table.min_capacity ?? 1) < 1 ||
                    (table.min_capacity ?? 1) > 99
                  ? t("editor.properties.capacityRange")
                  : undefined
            }
            variant="bordered"
            classNames={{ inputWrapper: "border-warm-200" }}
            data-testid="props-min-capacity"
          />
          <Input
            size="sm"
            type="number"
            min={1}
            max={99}
            label={t("editor.properties.maxSeats")}
            value={String(table.max_capacity ?? 4)}
            onValueChange={(v) => {
              const n = Math.min(99, Math.max(1, Math.floor(Number(v) || 1)));
              const minCap = table.min_capacity ?? 1;
              onUpdateTable(table.clientKey, {
                max_capacity: Math.max(n, minCap),
                visible_seat_count: table.visible_seat_count ?? n,
              });
            }}
            isInvalid={
              (table.max_capacity ?? 4) < 1 || (table.max_capacity ?? 4) > 99
            }
            errorMessage={
              (table.max_capacity ?? 4) < 1 || (table.max_capacity ?? 4) > 99
                ? t("editor.properties.capacityRange")
                : undefined
            }
            variant="bordered"
            classNames={{ inputWrapper: "border-warm-200" }}
            data-testid="props-max-capacity"
          />
        </div>
        <Input
          size="sm"
          type="number"
          label={t("editor.properties.visibleSeats")}
          value={String(
            table.visible_seat_count ?? table.max_capacity ?? 4,
          )}
          onValueChange={(v) => {
            const n = Math.max(0, Math.floor(Number(v) || 0));
            onUpdateTable(table.clientKey, { visible_seat_count: n });
          }}
          variant="bordered"
          classNames={{ inputWrapper: "border-warm-200" }}
        />
        <Select
          size="sm"
          label={t("editor.properties.region")}
          selectedKeys={
            table.region_id != null ? [String(table.region_id)] : ["none"]
          }
          onSelectionChange={(keys) => {
            const v = Array.from(keys)[0] as string;
            if (v === "none" || v == null) {
              onUpdateTable(table.clientKey, { region_id: undefined });
            } else {
              onUpdateTable(table.clientKey, { region_id: Number(v) });
            }
          }}
          variant="bordered"
          classNames={{ trigger: "border-warm-200" }}
        >
          {[
            <SelectItem key="none" value="none">
              {t("editor.properties.noRegion")}
            </SelectItem>,
            ...doc.regions
              .filter((r) => r.id != null)
              .map((r) => (
                <SelectItem key={String(r.id)} value={String(r.id)}>
                  {r.name}
                </SelectItem>
              )),
          ]}
        </Select>
        <div className="space-y-2 pt-1">
          <Switch
            size="sm"
            isSelected={table.is_reservable !== false}
            onValueChange={(v) =>
              onUpdateTable(table.clientKey, { is_reservable: v })
            }
          >
            {t("editor.properties.reservable")}
          </Switch>
          <Switch
            size="sm"
            isSelected={Boolean(table.is_combinable)}
            onValueChange={(v) =>
              onUpdateTable(table.clientKey, { is_combinable: v })
            }
          >
            {t("editor.properties.combinable")}
          </Switch>
          <Switch
            size="sm"
            isSelected={Boolean(table.is_accessible)}
            onValueChange={(v) =>
              onUpdateTable(table.clientKey, { is_accessible: v })
            }
          >
            {t("editor.properties.accessible")}
          </Switch>
        </div>
        {table.table_id > 0 && (
          <div className="rounded-xl border border-warm-100 bg-warm-50 p-3 text-xs text-ink-600">
            <p className="font-medium text-ink-800">
              {t("editor.properties.qrLink")}
            </p>
            {qr ? (
              <a
                href={qr}
                target="_blank"
                rel="noreferrer"
                className="mt-1 block truncate text-brand underline"
              >
                {qr}
              </a>
            ) : (
              <p className="mt-1 text-ink-500">
                {t("editor.properties.qrReadOnly", {
                  id: table.table_id,
                })}
              </p>
            )}
          </div>
        )}
      </aside>
    );
  }

  if (selected.kind === "element") {
    const el = selected.element;
    return (
      <aside
        className="flex h-full w-full min-w-0 flex-col gap-3 overflow-y-auto bg-white p-3"
        data-testid="properties-panel"
        data-properties-state="element"
      >
        <h3 className="text-xs font-semibold uppercase tracking-wider text-ink-500">
          {t("editor.properties.element")}
        </h3>
        <p className="text-sm font-medium capitalize text-ink-800">
          {t(`editor.elements.${el.element_type}`)}
        </p>
        <Input
          size="sm"
          label={t("editor.properties.name")}
          value={el.name ?? ""}
          onValueChange={(v) => onUpdateElement(el.clientKey, { name: v })}
          variant="bordered"
          classNames={{ inputWrapper: "border-warm-200" }}
        />
        <div className="grid grid-cols-2 gap-2">
          <Input
            size="sm"
            type="number"
            label={t("editor.properties.width")}
            value={String(
              Math.round(fromMm(el.width_mm ?? 0, unit) * 100) / 100,
            )}
            onValueChange={(v) => {
              const n = Number(v);
              if (!Number.isFinite(n) || n <= 0) return;
              onUpdateElement(el.clientKey, {
                width_mm: Math.round(toMm(n, unit)),
              });
            }}
            variant="bordered"
            classNames={{ inputWrapper: "border-warm-200" }}
          />
          <Input
            size="sm"
            type="number"
            label={t("editor.properties.height")}
            value={String(
              Math.round(fromMm(el.height_mm ?? 0, unit) * 100) / 100,
            )}
            onValueChange={(v) => {
              const n = Number(v);
              if (!Number.isFinite(n) || n <= 0) return;
              onUpdateElement(el.clientKey, {
                height_mm: Math.round(toMm(n, unit)),
              });
            }}
            variant="bordered"
            classNames={{ inputWrapper: "border-warm-200" }}
          />
        </div>
        <Input
          size="sm"
          type="number"
          label={t("editor.properties.rotation")}
          value={String(el.rotation_deg ?? 0)}
          onValueChange={(v) => {
            const n = Number(v);
            if (!Number.isFinite(n)) return;
            onUpdateElement(el.clientKey, { rotation_deg: n });
          }}
          variant="bordered"
          classNames={{ inputWrapper: "border-warm-200" }}
        />
      </aside>
    );
  }

  const region = selected.region;
  return (
    <aside
      className="flex h-full w-full min-w-0 flex-col gap-3 overflow-y-auto bg-white p-3"
      data-testid="properties-panel"
      data-properties-state="region"
    >
      <h3 className="text-xs font-semibold uppercase tracking-wider text-ink-500">
        {t("editor.properties.region")}
      </h3>
      <Input
        size="sm"
        label={t("editor.properties.name")}
        value={region.name}
        onValueChange={(v) => onUpdateRegion(region.clientKey, { name: v })}
        variant="bordered"
        classNames={{ inputWrapper: "border-warm-200" }}
      />
      <Input
        size="sm"
        type="color"
        label={t("editor.properties.color")}
        value={(region.color || "#1a6b6a").slice(0, 7)}
        onValueChange={(v) =>
          onUpdateRegion(region.clientKey, { color: v + "55" })
        }
        variant="bordered"
        classNames={{ inputWrapper: "border-warm-200" }}
      />
      <Input
        size="sm"
        label={t("editor.properties.purpose")}
        value={region.purpose ?? ""}
        onValueChange={(v) => onUpdateRegion(region.clientKey, { purpose: v })}
        variant="bordered"
        classNames={{ inputWrapper: "border-warm-200" }}
      />
      <p className="text-xs text-ink-500">
        {t("editor.properties.regionPoints", {
          count: region.polygon_mm.length,
        })}
      </p>
    </aside>
  );
}
