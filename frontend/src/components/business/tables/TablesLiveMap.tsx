"use client";

/**
 * Read-only published floor-plan canvas for Tables → Live View (Map mode).
 * Table statuses come from the existing /tables/status feed (via parent).
 * Clicking a table opens the parent-owned TableDetailModal/drawer — no
 * parallel operational system.
 */

/* eslint-disable no-restricted-syntax -- SVG map stroke/fill and canvas ground use absolute hex */

import React, { useCallback, useEffect, useMemo, useState } from "react";
import { Select, SelectItem } from "@nextui-org/react";
import {
  parseLayoutDocument,
  spacesApi,
  type LayoutDocument,
  type LayoutRegion,
  type LayoutTable,
  type Space,
} from "@/api/spaces";
import type { TableWithStatus } from "@/api/business";
import type { TableStatus } from "./TableRow";
import { liveMapNodeAffordance } from "./liveMapNode";
import { findUnplacedActiveTables } from "./unplacedTables";

type LiveMapStatusFilter = TableStatus | "all";

export interface TablesLiveMapProps {
  businessId: number;
  /** Live table rows from /tables/status (same source as list view). */
  tables: TableWithStatus[];
  statusFilter: LiveMapStatusFilter;
  onStatusFilterChange: (next: LiveMapStatusFilter) => void;
  onSelectTable: (tableId: number) => void;
  selectedTableId?: number | null;
  t: (key: string, params?: Record<string, string | number>) => string;
  /** Table status labels (available / occupied / reserved). */
  statusLabel: (status: TableStatus) => string;
}

function statusFill(status: TableStatus | "unknown"): string {
  switch (status) {
    case "occupied":
      return "rgba(244,63,94,0.35)"; // rose
    case "reserved":
      return "rgba(245,158,11,0.40)"; // amber
    case "available":
      return "rgba(16,185,129,0.30)"; // emerald
    default:
      return "rgba(120,113,108,0.25)";
  }
}

function statusStroke(status: TableStatus | "unknown", selected: boolean): string {
  if (selected) return "#1a6b6a";
  switch (status) {
    case "occupied":
      return "rgba(225,29,72,0.85)";
    case "reserved":
      return "rgba(180,83,9,0.85)";
    case "available":
      return "rgba(5,150,105,0.85)";
    default:
      return "rgba(120,113,108,0.55)";
  }
}

function publishedSpaces(list: Space[]): Space[] {
  return list
    .filter(
      (s) =>
        s.status === "published" &&
        parseLayoutDocument(s.published_layout_json) != null,
    )
    .sort((a, b) => a.sort_order - b.sort_order || a.id - b.id);
}

export default function TablesLiveMap({
  businessId,
  tables,
  statusFilter,
  onStatusFilterChange,
  onSelectTable,
  selectedTableId = null,
  t,
  statusLabel,
}: TablesLiveMapProps) {
  const [spaces, setSpaces] = useState<Space[]>([]);
  const [loading, setLoading] = useState(true);
  const [activeSpaceId, setActiveSpaceId] = useState<number | null>(null);
  const [regionFilter, setRegionFilter] = useState<string>("all");
  // L3-26: explicit label for layout nodes without live status feed data.
  const noLiveDataLabel = t("liveMap.noLiveData") || "No live data";

  const statusById = useMemo(() => {
    const map = new Map<number, { status: TableStatus; name: string }>();
    for (const tws of tables) {
      if (!tws.table.is_active) continue;
      map.set(tws.table.id, {
        status: (tws.status || "available") as TableStatus,
        name: tws.table.name,
      });
    }
    return map;
  }, [tables]);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    spacesApi
      .list(businessId)
      .then((list) => {
        if (cancelled) return;
        const pub = publishedSpaces(list);
        setSpaces(pub);
        setActiveSpaceId((prev) => {
          if (prev && pub.some((s) => s.id === prev)) return prev;
          return pub[0]?.id ?? null;
        });
      })
      .catch(() => {
        if (!cancelled) {
          setSpaces([]);
          setActiveSpaceId(null);
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [businessId]);

  const activeSpace = useMemo(
    () => spaces.find((s) => s.id === activeSpaceId) ?? null,
    [spaces, activeSpaceId],
  );

  const layout: LayoutDocument | null = useMemo(() => {
    if (!activeSpace) return null;
    return parseLayoutDocument(activeSpace.published_layout_json);
  }, [activeSpace]);

  const regions: LayoutRegion[] = layout?.regions ?? [];

  // Reset region filter when switching spaces.
  useEffect(() => {
    setRegionFilter("all");
  }, [activeSpaceId]);

  const layoutTableIds = useMemo(() => {
    const ids = new Set<number>();
    for (const lt of layout?.tables ?? []) {
      ids.add(lt.table_id);
    }
    return ids;
  }, [layout?.tables]);

  const visibleTables = useMemo(() => {
    const layoutTables: LayoutTable[] = layout?.tables ?? [];
    return layoutTables.filter((lt) => {
      const live = statusById.get(lt.table_id);
      const status = live?.status ?? "available";
      if (statusFilter !== "all" && status !== statusFilter) return false;
      if (regionFilter !== "all") {
        const rid = lt.region_id;
        if (regionFilter === "none") {
          if (rid != null) return false;
        } else if (String(rid ?? "") !== regionFilter) {
          return false;
        }
      }
      return true;
    });
  }, [layout?.tables, statusById, statusFilter, regionFilter]);

  // L3-26 residual: active tables missing from the published layout must not
  // vanish — list them so operators can still open detail (incl. occupied).
  const unplacedActiveTables = useMemo(() => {
    return findUnplacedActiveTables(tables, layoutTableIds).filter((tws) => {
      const status = (tws.status || "available") as TableStatus;
      if (statusFilter !== "all" && status !== statusFilter) return false;
      return true;
    });
  }, [tables, layoutTableIds, statusFilter]);

  const { viewW, viewH, boundaryPath } = useMemo(() => {
    const w =
      layout?.width_mm ||
      activeSpace?.width_mm ||
      10000;
    const h =
      layout?.height_mm ||
      activeSpace?.height_mm ||
      8000;
    let boundaryPath = "";
    const pts = layout?.boundary?.points_mm;
    if (pts && pts.length >= 3) {
      boundaryPath =
        pts.map((p, i) => `${i === 0 ? "M" : "L"} ${p.x} ${p.y}`).join(" ") +
        " Z";
    }
    return {
      viewW: Math.max(w, 1),
      viewH: Math.max(h, 1),
      boundaryPath,
    };
  }, [layout, activeSpace]);

  const handleTableClick = useCallback(
    (tableId: number) => {
      // Only open modal for tables that exist in the status feed (real IDs).
      if (!statusById.has(tableId)) return;
      onSelectTable(tableId);
    },
    [onSelectTable, statusById],
  );

  if (loading) {
    return (
      <div
        className="flex min-h-[280px] items-center justify-center rounded-2xl border border-warm-200 bg-warm-50"
        role="status"
        data-testid="live-map-loading"
      >
        <p className="text-sm text-ink-500">{t("liveMap.loading")}</p>
      </div>
    );
  }

  if (spaces.length === 0) {
    return null;
  }

  return (
    <div className="space-y-3" data-testid="tables-live-map">
      <div className="flex flex-wrap items-center gap-2">
        {spaces.length > 1 && (
          <div
            className="inline-flex flex-wrap gap-1 rounded-full border border-warm-200 bg-white p-0.5"
            role="tablist"
            aria-label={t("liveMap.spaceSwitcherAria")}
          >
            {spaces.map((s) => {
              const active = s.id === activeSpaceId;
              return (
                <button
                  key={s.id}
                  type="button"
                  role="tab"
                  aria-selected={active}
                  data-testid={`live-map-space-${s.id}`}
                  onClick={() => setActiveSpaceId(s.id)}
                  className={`rounded-full px-3 py-1 text-xs font-medium transition-colors ${
                    active
                      ? "bg-brand text-white shadow-sm"
                      : "text-ink-600 hover:bg-warm-50 hover:text-ink-900"
                  }`}
                >
                  {s.name}
                </button>
              );
            })}
          </div>
        )}

        <Select
          aria-label={t("liveMap.statusFilterAria")}
          selectedKeys={[statusFilter]}
          onSelectionChange={(keys) => {
            const next = Array.from(keys)[0] as LiveMapStatusFilter | undefined;
            if (next) onStatusFilterChange(next);
          }}
          size="sm"
          variant="bordered"
          className="w-40"
          classNames={{
            trigger:
              "h-9 min-h-9 bg-white border-warm-200 data-[hover=true]:border-brand/40",
          }}
          items={[
            { key: "all", label: t("liveMap.filterAll") },
            { key: "available", label: statusLabel("available") },
            { key: "occupied", label: statusLabel("occupied") },
            { key: "reserved", label: statusLabel("reserved") },
          ]}
        >
          {(item) => <SelectItem key={item.key}>{item.label}</SelectItem>}
        </Select>

        {regions.length > 0 && (
          <Select
            aria-label={t("liveMap.regionFilterAria")}
            selectedKeys={[regionFilter]}
            onSelectionChange={(keys) => {
              const next = Array.from(keys)[0] as string | undefined;
              if (next) setRegionFilter(next);
            }}
            size="sm"
            variant="bordered"
            className="w-44"
            classNames={{
              trigger:
                "h-9 min-h-9 bg-white border-warm-200 data-[hover=true]:border-brand/40",
            }}
            items={[
              { key: "all", label: t("liveMap.regionAll") },
              { key: "none", label: t("liveMap.regionNone") },
              ...regions.map((r, i) => ({
                key: String(r.id ?? i),
                label: r.name || t("liveMap.regionFallback", { n: i + 1 }),
              })),
            ]}
          >
            {(item) => <SelectItem key={item.key}>{item.label}</SelectItem>}
          </Select>
        )}

        <div className="ml-auto flex flex-wrap items-center gap-3 text-[11px] text-ink-500">
          {(
            [
              ["available", statusLabel("available")],
              ["occupied", statusLabel("occupied")],
              ["reserved", statusLabel("reserved")],
            ] as const
          ).map(([key, label]) => (
            <span key={key} className="inline-flex items-center gap-1.5">
              <span
                className="inline-block h-2.5 w-2.5 rounded-sm border"
                style={{
                  backgroundColor: statusFill(key),
                  borderColor: statusStroke(key, false),
                }}
                aria-hidden
              />
              {label}
            </span>
          ))}
        </div>
      </div>

      <div className="overflow-hidden rounded-2xl border border-warm-200 bg-[#f3f1ec]">
        <svg
          viewBox={`0 0 ${viewW} ${viewH}`}
          className="h-auto max-h-[min(70vh,560px)] w-full"
          preserveAspectRatio="xMidYMid meet"
          role="img"
          aria-label={
            activeSpace
              ? t("liveMap.canvasAria", { name: activeSpace.name })
              : t("liveMap.canvasAriaGeneric")
          }
          data-testid="live-map-canvas"
        >
          <rect
            x={0}
            y={0}
            width={viewW}
            height={viewH}
            className="fill-warm-50"
          />
          {boundaryPath ? (
            <path
              d={boundaryPath}
              className="fill-brand/5 stroke-brand/40"
              strokeWidth={Math.max(viewW, viewH) * 0.004}
            />
          ) : (
            <rect
              x={viewW * 0.04}
              y={viewH * 0.04}
              width={viewW * 0.92}
              height={viewH * 0.92}
              rx={Math.min(viewW, viewH) * 0.02}
              className="fill-none stroke-warm-200"
              strokeWidth={Math.max(viewW, viewH) * 0.003}
            />
          )}

          {regions.map((region, i) => {
            const pts = region.polygon_mm;
            if (!pts || pts.length < 3) return null;
            const d =
              pts.map((p, j) => `${j === 0 ? "M" : "L"} ${p.x} ${p.y}`).join(" ") +
              " Z";
            return (
              <path
                key={region.id ?? `region-${i}`}
                d={d}
                fill={region.color || "rgba(26,107,106,0.10)"}
                stroke="rgba(26,107,106,0.25)"
                strokeWidth={Math.max(viewW, viewH) * 0.002}
                pointerEvents="none"
              />
            );
          })}

          {visibleTables.map((lt) => {
            const live = statusById.get(lt.table_id);
            const status: TableStatus | "unknown" = live?.status ?? "unknown";
            const selected = selectedTableId === lt.table_id;
            const label = live?.name || lt.name || String(lt.table_id);
            const cx = lt.x_mm + lt.width_mm / 2;
            const cy = lt.y_mm + lt.height_mm / 2;
            const isRound = lt.shape === "round" || lt.shape === "oval";
            const fill = statusFill(status);
            const stroke = statusStroke(status, selected);
            const strokeW = selected
              ? Math.max(viewW, viewH) * 0.004
              : Math.max(viewW, viewH) * 0.0025;
            const affordance = liveMapNodeAffordance(statusById.has(lt.table_id));
            const clickable = affordance.clickable;
            const common = {
              fill,
              stroke,
              strokeWidth: strokeW,
              style: { cursor: clickable ? "pointer" : "default" } as const,
              onClick: () => handleTableClick(lt.table_id),
              "data-testid": `live-map-table-${lt.table_id}`,
            };
            return (
              <g key={lt.table_id}>
                {isRound ? (
                  <ellipse
                    cx={cx}
                    cy={cy}
                    rx={lt.width_mm / 2}
                    ry={lt.height_mm / 2}
                    transform={
                      lt.rotation_deg
                        ? `rotate(${lt.rotation_deg} ${cx} ${cy})`
                        : undefined
                    }
                    {...common}
                    opacity={affordance.opacity}
                  />
                ) : (
                  <rect
                    x={lt.x_mm}
                    y={lt.y_mm}
                    width={lt.width_mm}
                    height={lt.height_mm}
                    rx={Math.min(lt.width_mm, lt.height_mm) * 0.12}
                    transform={
                      lt.rotation_deg
                        ? `rotate(${lt.rotation_deg} ${cx} ${cy})`
                        : undefined
                    }
                    {...common}
                    opacity={affordance.opacity}
                  />
                )}
                <text
                  x={cx}
                  y={cy - (clickable ? 0 : Math.min(lt.width_mm, lt.height_mm) * 0.08)}
                  textAnchor="middle"
                  dominantBaseline="middle"
                  fill="#1c1917"
                  fontSize={Math.max(
                    120,
                    Math.min(lt.width_mm, lt.height_mm) * 0.18,
                  )}
                  fontWeight={600}
                  pointerEvents="none"
                >
                  {label}
                </text>
                {/* L3-26: layout-only nodes without live status get an explicit
                    "no live data" label so they are not silent inert shapes. */}
                {affordance.showNoLiveDataLabel && (
                  <text
                    x={cx}
                    y={cy + Math.min(lt.width_mm, lt.height_mm) * 0.22}
                    textAnchor="middle"
                    dominantBaseline="middle"
                    fill="#78716c"
                    fontSize={Math.max(
                      80,
                      Math.min(lt.width_mm, lt.height_mm) * 0.1,
                    )}
                    fontWeight={500}
                    pointerEvents="none"
                    data-testid={`live-map-no-data-${lt.table_id}`}
                  >
                    {noLiveDataLabel}
                  </text>
                )}
              </g>
            );
          })}
        </svg>
      </div>

      {visibleTables.length === 0 && unplacedActiveTables.length === 0 && (
        <p className="text-center text-sm text-ink-500" data-testid="live-map-empty-filter">
          {t("liveMap.noTablesMatch")}
        </p>
      )}

      {/* L3-26 residual: active tables absent from the layout are still reachable. */}
      {unplacedActiveTables.length > 0 && (
        <div
          className="rounded-2xl border border-amber-200 bg-amber-50/60 p-3"
          data-testid="live-map-unplaced"
        >
          <p className="text-sm font-semibold text-ink-900">
            {t("liveMap.unplacedTitle")}
          </p>
          <p className="mt-0.5 text-xs text-ink-600">
            {t("liveMap.unplacedBody")}
          </p>
          <ul className="mt-2 flex flex-wrap gap-2">
            {unplacedActiveTables.map((tws) => {
              const status = (tws.status || "available") as TableStatus;
              const selected = selectedTableId === tws.table.id;
              return (
                <li key={tws.table.id}>
                  <button
                    type="button"
                    data-testid={`live-map-unplaced-${tws.table.id}`}
                    onClick={() => onSelectTable(tws.table.id)}
                    className={`inline-flex items-center gap-2 rounded-full border px-3 py-1.5 text-xs font-medium transition-colors ${
                      selected
                        ? "border-brand bg-brand text-white"
                        : "border-warm-200 bg-white text-ink-800 hover:border-brand/40"
                    }`}
                  >
                    <span
                      className="h-2 w-2 rounded-full"
                      style={{ backgroundColor: statusFill(status) }}
                      aria-hidden
                    />
                    {tws.table.name}
                    <span className="text-[10px] opacity-80">
                      {statusLabel(status)}
                    </span>
                  </button>
                </li>
              );
            })}
          </ul>
        </div>
      )}
    </div>
  );
}
