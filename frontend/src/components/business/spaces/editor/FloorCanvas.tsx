"use client";

/* eslint-disable no-restricted-syntax -- SVG canvas stroke/fill must use absolute hex;
   Tailwind class tokens cannot style SVG attributes or freeform region colors. */

import React, {
  memo,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import type {
  EditorDocument,
  EditorElement,
  EditorRegion,
  EditorTable,
  EditorTool,
  PreviewMode,
  SelectionRef,
} from "./types";
import {
  DEFAULT_GRID_MM,
  hitTestRect,
  polygonPath,
  snapPosition,
  type AlignmentGuide,
  type OrientedRect,
  type Point,
} from "./geometry";
import {
  outwardSeatPositions,
  seatRenderRadius,
} from "./seatLayout";
import type { CanvasViewportApi } from "./hooks/useCanvasViewport";

interface FloorCanvasProps {
  doc: EditorDocument;
  selection: SelectionRef[];
  isSelected: (ref: SelectionRef) => boolean;
  onSelect: (refs: SelectionRef[], additive?: boolean) => void;
  tool: EditorTool;
  previewMode: PreviewMode;
  snapGrid: boolean;
  snapElements: boolean;
  viewport: CanvasViewportApi;
  onBeginDrag: () => void;
  onPreviewDoc: (updater: (prev: EditorDocument) => EditorDocument) => void;
  onCommitDrag: () => void;
  onCancelDrag: () => void;
  onPolygonComplete: (points: Point[]) => void;
  drawingPoints: Point[];
  onDrawingPoint: (p: Point) => void;
  onClearDrawing: () => void;
  t: (key: string) => string;
}

type DragMode =
  | { type: "pan"; lastX: number; lastY: number }
  | {
      type: "move";
      keys: string[];
      startWorld: Point;
      origins: Record<string, { x: number; y: number }>;
    }
  | {
      type: "resize";
      key: string;
      kind: "table" | "element";
      handle: string;
      start: OrientedRect;
      startWorld: Point;
    }
  | {
      type: "rotate";
      key: string;
      kind: "table" | "element";
      center: Point;
      startAngle: number;
      startRot: number;
    }
  | null;

function tableFill(mode: PreviewMode, selected: boolean): string {
  if (selected) return "rgba(26,107,106,0.45)";
  switch (mode) {
    case "guest":
      return "rgba(26,107,106,0.22)";
    case "staff":
      return "rgba(26,107,106,0.28)";
    case "live":
      return "rgba(16,185,129,0.25)";
    default:
      return "rgba(26,107,106,0.30)";
  }
}

const MemoTable = memo(function MemoTable({
  table,
  selected,
  previewMode,
  showSeats,
  onPointerDown,
}: {
  table: EditorTable;
  selected: boolean;
  previewMode: PreviewMode;
  showSeats: boolean;
  onPointerDown: (e: React.PointerEvent, key: string) => void;
}) {
  const cx = table.x_mm + table.width_mm / 2;
  const cy = table.y_mm + table.height_mm / 2;
  const rot = table.rotation_deg ?? 0;
  const isRound = table.shape === "round" || table.shape === "oval";
  const seats = showSeats
    ? outwardSeatPositions(
        table.shape || "square",
        table.visible_seat_count ?? table.max_capacity ?? 0,
        table.width_mm,
        table.height_mm,
      )
    : [];
  const r = seatRenderRadius(table.width_mm, table.height_mm);

  return (
    <g
      data-testid={`canvas-table-${table.clientKey}`}
      data-canvas-item="table"
      transform={rot ? `rotate(${rot} ${cx} ${cy})` : undefined}
      onPointerDown={(e) => onPointerDown(e, table.clientKey)}
      style={{ cursor: "move" }}
    >
      {isRound ? (
        <ellipse
          cx={cx}
          cy={cy}
          rx={table.width_mm / 2}
          ry={table.height_mm / 2}
          fill={tableFill(previewMode, selected)}
          stroke={selected ? "#1a6b6a" : "rgba(26,107,106,0.7)"}
          strokeWidth={selected ? 40 : 24}
        />
      ) : (
        <rect
          x={table.x_mm}
          y={table.y_mm}
          width={table.width_mm}
          height={table.height_mm}
          rx={Math.min(table.width_mm, table.height_mm) * 0.1}
          fill={tableFill(previewMode, selected)}
          stroke={selected ? "#1a6b6a" : "rgba(26,107,106,0.7)"}
          strokeWidth={selected ? 40 : 24}
        />
      )}
      {seats.map((s, i) => (
        <circle
          key={i}
          cx={table.x_mm + s.x}
          cy={table.y_mm + s.y}
          r={r}
          fill="rgba(250,249,246,0.95)"
          stroke="rgba(28,25,23,0.25)"
          strokeWidth={12}
          pointerEvents="none"
        />
      ))}
      {previewMode !== "guest" && table.name ? (
        <text
          x={cx}
          y={cy}
          textAnchor="middle"
          dominantBaseline="middle"
          fill="#1c1917"
          fontSize={Math.max(120, Math.min(table.width_mm, table.height_mm) * 0.18)}
          fontWeight={600}
          pointerEvents="none"
        >
          {table.name}
        </text>
      ) : null}
    </g>
  );
});

const MemoElement = memo(function MemoElement({
  element,
  selected,
  onPointerDown,
}: {
  element: EditorElement;
  selected: boolean;
  onPointerDown: (e: React.PointerEvent, key: string) => void;
}) {
  const x = element.x_mm ?? 0;
  const y = element.y_mm ?? 0;
  const w = element.width_mm ?? 400;
  const h = element.height_mm ?? 400;
  const cx = x + w / 2;
  const cy = y + h / 2;
  const rot = element.rotation_deg ?? 0;
  const type = element.element_type;

  let fill = "rgba(120,113,108,0.35)";
  let stroke = "rgba(68,64,60,0.7)";
  if (type === "door" || type === "entrance") {
    fill = "rgba(245,158,11,0.25)";
    stroke = "rgba(180,83,9,0.8)";
  } else if (type === "window") {
    fill = "rgba(56,189,248,0.2)";
    stroke = "rgba(14,116,144,0.7)";
  } else if (type === "bar" || type === "counter") {
    fill = "rgba(124,45,18,0.2)";
    stroke = "rgba(120,53,15,0.7)";
  } else if (type === "restroom" || type === "service_station") {
    fill = "rgba(99,102,241,0.15)";
    stroke = "rgba(67,56,202,0.6)";
  } else if (type === "label") {
    fill = "transparent";
    stroke = "rgba(28,25,23,0.4)";
  }

  return (
    <g
      data-testid={`canvas-el-${element.clientKey}`}
      data-canvas-item="element"
      transform={rot ? `rotate(${rot} ${cx} ${cy})` : undefined}
      onPointerDown={(e) => onPointerDown(e, element.clientKey)}
      style={{ cursor: "move" }}
    >
      <rect
        x={x}
        y={y}
        width={w}
        height={h}
        rx={type === "column" ? w / 2 : 40}
        fill={fill}
        stroke={selected ? "#1a6b6a" : stroke}
        strokeWidth={selected ? 36 : 20}
        strokeDasharray={type === "divider" ? "80 40" : undefined}
      />
      {(type === "label" || selected) && element.name ? (
        <text
          x={cx}
          y={cy}
          textAnchor="middle"
          dominantBaseline="middle"
          fill="#44403c"
          fontSize={Math.max(100, h * 0.35)}
          pointerEvents="none"
        >
          {element.name}
        </text>
      ) : null}
    </g>
  );
});

function SelectionHandles({
  rect,
  onResizeDown,
  onRotateDown,
}: {
  rect: OrientedRect;
  onResizeDown: (e: React.PointerEvent, handle: string) => void;
  onRotateDown: (e: React.PointerEvent) => void;
}) {
  const { x, y, w, h } = rect;
  const handles: { id: string; hx: number; hy: number }[] = [
    { id: "nw", hx: x, hy: y },
    { id: "n", hx: x + w / 2, hy: y },
    { id: "ne", hx: x + w, hy: y },
    { id: "e", hx: x + w, hy: y + h / 2 },
    { id: "se", hx: x + w, hy: y + h },
    { id: "s", hx: x + w / 2, hy: y + h },
    { id: "sw", hx: x, hy: y + h },
    { id: "w", hx: x, hy: y + h / 2 },
  ];
  const size = Math.max(60, Math.min(w, h) * 0.08);
  const cx = x + w / 2;
  const rotY = y - size * 2.5;

  return (
    <g pointerEvents="all">
      <rect
        x={x}
        y={y}
        width={w}
        height={h}
        fill="none"
        stroke="#1a6b6a"
        strokeWidth={24}
        strokeDasharray="80 40"
        pointerEvents="none"
      />
      <line
        x1={cx}
        y1={y}
        x2={cx}
        y2={rotY}
        stroke="#1a6b6a"
        strokeWidth={16}
        pointerEvents="none"
      />
      <circle
        cx={cx}
        cy={rotY}
        r={size * 0.7}
        fill="#fff"
        stroke="#1a6b6a"
        strokeWidth={20}
        style={{ cursor: "grab" }}
        onPointerDown={onRotateDown}
        data-testid="rotate-handle"
      />
      {handles.map((hnd) => (
        <rect
          key={hnd.id}
          x={hnd.hx - size / 2}
          y={hnd.hy - size / 2}
          width={size}
          height={size}
          fill="#fff"
          stroke="#1a6b6a"
          strokeWidth={16}
          style={{ cursor: "nwse-resize" }}
          onPointerDown={(e) => onResizeDown(e, hnd.id)}
          data-testid={`resize-${hnd.id}`}
        />
      ))}
    </g>
  );
}

export function FloorCanvas({
  doc,
  selection,
  isSelected,
  onSelect,
  tool,
  previewMode,
  snapGrid,
  snapElements,
  viewport,
  onBeginDrag,
  onPreviewDoc,
  onCommitDrag,
  onCancelDrag,
  onPolygonComplete,
  drawingPoints,
  onDrawingPoint,
  onClearDrawing,
  t,
}: FloorCanvasProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const svgRef = useRef<SVGSVGElement>(null);
  const dragRef = useRef<DragMode>(null);
  const [guides, setGuides] = useState<AlignmentGuide[]>([]);

  const roomW = doc.width_mm || 12000;
  const roomH = doc.height_mm || 8000;
  const didFitRef = useRef(false);
  useEffect(() => {
    didFitRef.current = false;
  }, [roomW, roomH]);

  useEffect(() => {
    const el = containerRef.current;
    if (!el || typeof ResizeObserver === "undefined") return;
    const ro = new ResizeObserver((entries) => {
      const cr = entries[0]?.contentRect;
      if (!cr || cr.width < 40 || cr.height < 40) return;
      if (!didFitRef.current && roomW > 0 && roomH > 0) {
        viewport.fitToView(roomW, roomH, cr.width, cr.height, 48);
        didFitRef.current = true;
      }
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, [roomW, roomH, viewport]);

  const clientToLocal = useCallback((e: { clientX: number; clientY: number }) => {
    const svg = svgRef.current;
    if (!svg) return { x: 0, y: 0 };
    const rect = svg.getBoundingClientRect();
    return { x: e.clientX - rect.left, y: e.clientY - rect.top };
  }, []);

  const clientToWorld = useCallback(
    (e: { clientX: number; clientY: number }) => {
      const local = clientToLocal(e);
      return viewport.screenToWorld(local.x, local.y);
    },
    [clientToLocal, viewport],
  );

  const sortedElements = useMemo(
    () =>
      [...doc.elements].sort(
        (a, b) => (a.z_index ?? 0) - (b.z_index ?? 0),
      ),
    [doc.elements],
  );

  const singleSelectionRect = useMemo((): {
    kind: "table" | "element";
    key: string;
    rect: OrientedRect;
  } | null => {
    if (selection.length !== 1 || previewMode !== "edit") return null;
    const ref = selection[0];
    if (ref.kind === "table") {
      const t = doc.tables.find((x) => x.clientKey === ref.key);
      if (!t) return null;
      return {
        kind: "table",
        key: t.clientKey,
        rect: {
          x: t.x_mm,
          y: t.y_mm,
          w: t.width_mm,
          h: t.height_mm,
          rotationDeg: t.rotation_deg ?? 0,
        },
      };
    }
    if (ref.kind === "element") {
      const el = doc.elements.find((x) => x.clientKey === ref.key);
      if (!el) return null;
      return {
        kind: "element",
        key: el.clientKey,
        rect: {
          x: el.x_mm ?? 0,
          y: el.y_mm ?? 0,
          w: el.width_mm ?? 400,
          h: el.height_mm ?? 400,
          rotationDeg: el.rotation_deg ?? 0,
        },
      };
    }
    return null;
  }, [selection, doc, previewMode]);

  const onWheel = useCallback(
    (e: React.WheelEvent) => {
      e.preventDefault();
      const local = clientToLocal(e);
      const factor = e.deltaY > 0 ? 1 / 1.1 : 1.1;
      viewport.zoomBy(factor, local);
    },
    [clientToLocal, viewport],
  );

  const hitTestAt = useCallback(
    (world: Point): SelectionRef | null => {
      // Tables on top
      for (let i = doc.tables.length - 1; i >= 0; i--) {
        const t = doc.tables[i];
        if (
          hitTestRect(world, {
            x: t.x_mm,
            y: t.y_mm,
            w: t.width_mm,
            h: t.height_mm,
            rotationDeg: t.rotation_deg ?? 0,
          })
        ) {
          return { kind: "table", key: t.clientKey };
        }
      }
      const els = [...doc.elements].sort(
        (a, b) => (b.z_index ?? 0) - (a.z_index ?? 0),
      );
      for (const el of els) {
        if (
          hitTestRect(world, {
            x: el.x_mm ?? 0,
            y: el.y_mm ?? 0,
            w: el.width_mm ?? 400,
            h: el.height_mm ?? 400,
            rotationDeg: el.rotation_deg ?? 0,
          })
        ) {
          return { kind: "element", key: el.clientKey };
        }
      }
      for (let i = doc.regions.length - 1; i >= 0; i--) {
        const r = doc.regions[i];
        // Simple bbox hit for regions
        if (r.polygon_mm.length < 3) continue;
        const xs = r.polygon_mm.map((p) => p.x);
        const ys = r.polygon_mm.map((p) => p.y);
        const minX = Math.min(...xs);
        const maxX = Math.max(...xs);
        const minY = Math.min(...ys);
        const maxY = Math.max(...ys);
        if (
          world.x >= minX &&
          world.x <= maxX &&
          world.y >= minY &&
          world.y <= maxY
        ) {
          return { kind: "region", key: r.clientKey };
        }
      }
      return null;
    },
    [doc],
  );

  const handlePointerDownBackground = (e: React.PointerEvent) => {
    // Primary button (0) or touch (-1 in some browsers); middle button pans.
    if (e.button > 0 && e.button !== 1) return;
    const fromItem = (e.target as Element | null)?.closest?.(
      "[data-canvas-item]",
    );
    // Table/element handlers own these hits. If the SVG handler also runs
    // (stopPropagation can fail on SVG in some browsers), do not clear.
    if (fromItem) return;
    (e.currentTarget as Element).setPointerCapture?.(e.pointerId);

    if (tool === "pan" || e.button === 1 || (tool === "select" && e.altKey)) {
      dragRef.current = {
        type: "pan",
        lastX: e.clientX,
        lastY: e.clientY,
      };
      return;
    }

    const world = clientToWorld(e);

    if (
      tool === "draw-polygon" ||
      tool === "draw-region" ||
      tool === "draw-rect-room"
    ) {
      onDrawingPoint(world);
      return;
    }

    if (tool !== "select" || previewMode !== "edit") {
      if (tool === "select") onSelect([]);
      return;
    }

    const hit = hitTestAt(world);
    if (!hit) {
      onSelect([]);
      return;
    }
    const additive = e.shiftKey;
    const already = isSelected(hit);
    if (!already) {
      onSelect([hit], additive);
    } else if (additive) {
      onSelect([hit], true);
    }

    // Start move for selected tables/elements
    if (hit.kind === "region") return;

    onBeginDrag();
    const keys =
      already && !additive
        ? selection
            .filter((s) => s.kind === hit.kind)
            .map((s) => s.key)
        : [hit.key];
    const selKeys = keys.length ? keys : [hit.key];
    const origins: Record<string, { x: number; y: number }> = {};
    if (hit.kind === "table") {
      for (const t of doc.tables) {
        if (selKeys.includes(t.clientKey)) {
          origins[`table:${t.clientKey}`] = { x: t.x_mm, y: t.y_mm };
        }
      }
    } else {
      for (const el of doc.elements) {
        if (selKeys.includes(el.clientKey)) {
          origins[`element:${el.clientKey}`] = {
            x: el.x_mm ?? 0,
            y: el.y_mm ?? 0,
          };
        }
      }
    }
    dragRef.current = {
      type: "move",
      keys: Object.keys(origins),
      startWorld: world,
      origins,
    };
  };

  const handleTablePointerDown = (e: React.PointerEvent, key: string) => {
    e.stopPropagation();
    if (previewMode !== "edit" || tool === "pan") {
      if (tool === "pan") {
        dragRef.current = {
          type: "pan",
          lastX: e.clientX,
          lastY: e.clientY,
        };
      }
      return;
    }
    (e.currentTarget as Element).setPointerCapture?.(e.pointerId);
    const ref: SelectionRef = { kind: "table", key };
    const additive = e.shiftKey;
    // Always write selection so Properties updates even when the same
    // table is clicked again (or a previous hit-test clear raced us).
    onSelect([ref], additive);
    if (tool !== "select") return;
    onBeginDrag();
    const world = clientToWorld(e);
    const selectedTables = (
      isSelected(ref) && !additive
        ? selection.filter((s) => s.kind === "table").map((s) => s.key)
        : [key]
    );
    const keys = selectedTables.includes(key)
      ? selectedTables
      : [key];
    const origins: Record<string, { x: number; y: number }> = {};
    for (const t of doc.tables) {
      if (keys.includes(t.clientKey)) {
        origins[`table:${t.clientKey}`] = { x: t.x_mm, y: t.y_mm };
      }
    }
    dragRef.current = {
      type: "move",
      keys: Object.keys(origins),
      startWorld: world,
      origins,
    };
  };

  const handleElementPointerDown = (e: React.PointerEvent, key: string) => {
    e.stopPropagation();
    if (previewMode !== "edit" || tool !== "select") return;
    (e.currentTarget as Element).setPointerCapture?.(e.pointerId);
    const ref: SelectionRef = { kind: "element", key };
    onSelect([ref], e.shiftKey);
    onBeginDrag();
    const world = clientToWorld(e);
    const origins: Record<string, { x: number; y: number }> = {};
    const el = doc.elements.find((x) => x.clientKey === key);
    if (el) {
      origins[`element:${key}`] = { x: el.x_mm ?? 0, y: el.y_mm ?? 0 };
    }
    dragRef.current = {
      type: "move",
      keys: Object.keys(origins),
      startWorld: world,
      origins,
    };
  };

  const handleResizeDown = (e: React.PointerEvent, handle: string) => {
    e.stopPropagation();
    if (!singleSelectionRect) return;
    (e.currentTarget as Element).setPointerCapture?.(e.pointerId);
    onBeginDrag();
    dragRef.current = {
      type: "resize",
      key: singleSelectionRect.key,
      kind: singleSelectionRect.kind,
      handle,
      start: { ...singleSelectionRect.rect },
      startWorld: clientToWorld(e),
    };
  };

  const handleRotateDown = (e: React.PointerEvent) => {
    e.stopPropagation();
    if (!singleSelectionRect) return;
    (e.currentTarget as Element).setPointerCapture?.(e.pointerId);
    onBeginDrag();
    const c = {
      x: singleSelectionRect.rect.x + singleSelectionRect.rect.w / 2,
      y: singleSelectionRect.rect.y + singleSelectionRect.rect.h / 2,
    };
    const world = clientToWorld(e);
    const startAngle = Math.atan2(world.y - c.y, world.x - c.x);
    dragRef.current = {
      type: "rotate",
      key: singleSelectionRect.key,
      kind: singleSelectionRect.kind,
      center: c,
      startAngle,
      startRot: singleSelectionRect.rect.rotationDeg,
    };
  };

  const handlePointerMove = (e: React.PointerEvent) => {
    const drag = dragRef.current;
    if (!drag) return;

    if (drag.type === "pan") {
      const dx = e.clientX - drag.lastX;
      const dy = e.clientY - drag.lastY;
      drag.lastX = e.clientX;
      drag.lastY = e.clientY;
      viewport.panBy(dx, dy);
      return;
    }

    const world = clientToWorld(e);

    if (drag.type === "move") {
      const dx = world.x - drag.startWorld.x;
      const dy = world.y - drag.startWorld.y;

      // Snap first key as primary
      const firstKey = drag.keys[0];
      const origin = drag.origins[firstKey];
      if (!origin) return;
      const primaryKind = firstKey.startsWith("table:") ? "table" : "element";
      const primaryId = firstKey.split(":")[1];
      let size = { w: 900, h: 900 };
      if (primaryKind === "table") {
        const t = doc.tables.find((x) => x.clientKey === primaryId);
        if (t) size = { w: t.width_mm, h: t.height_mm };
      } else {
        const el = doc.elements.find((x) => x.clientKey === primaryId);
        if (el) size = { w: el.width_mm ?? 400, h: el.height_mm ?? 400 };
      }

      const others: OrientedRect[] = [];
      for (const t of doc.tables) {
        if (!drag.keys.includes(`table:${t.clientKey}`)) {
          others.push({
            x: t.x_mm,
            y: t.y_mm,
            w: t.width_mm,
            h: t.height_mm,
            rotationDeg: t.rotation_deg ?? 0,
          });
        }
      }

      const raw = { x: origin.x + dx, y: origin.y + dy };
      const snapped = snapPosition(raw, size, others, {
        snapGrid,
        snapElements,
        gridMm: DEFAULT_GRID_MM,
      });
      setGuides(snapped.guides);
      const adx = snapped.point.x - origin.x;
      const ady = snapped.point.y - origin.y;

      onPreviewDoc((prev) => ({
        ...prev,
        tables: prev.tables.map((t) => {
          const o = drag.origins[`table:${t.clientKey}`];
          if (!o) return t;
          return { ...t, x_mm: Math.round(o.x + adx), y_mm: Math.round(o.y + ady) };
        }),
        elements: prev.elements.map((el) => {
          const o = drag.origins[`element:${el.clientKey}`];
          if (!o) return el;
          return {
            ...el,
            x_mm: Math.round(o.x + adx),
            y_mm: Math.round(o.y + ady),
          };
        }),
      }));
      return;
    }

    if (drag.type === "resize") {
      const s = drag.start;
      let { x, y, w, h } = s;
      const wx = world.x;
      const wy = world.y;
      const handle = drag.handle;
      if (handle.includes("e")) w = Math.max(200, wx - x);
      if (handle.includes("s")) h = Math.max(200, wy - y);
      if (handle.includes("w")) {
        const right = x + w;
        x = Math.min(wx, right - 200);
        w = right - x;
      }
      if (handle.includes("n")) {
        const bottom = y + h;
        y = Math.min(wy, bottom - 200);
        h = bottom - y;
      }
      if (snapGrid) {
        x = Math.round(x / DEFAULT_GRID_MM) * DEFAULT_GRID_MM;
        y = Math.round(y / DEFAULT_GRID_MM) * DEFAULT_GRID_MM;
        w = Math.max(DEFAULT_GRID_MM, Math.round(w / DEFAULT_GRID_MM) * DEFAULT_GRID_MM);
        h = Math.max(DEFAULT_GRID_MM, Math.round(h / DEFAULT_GRID_MM) * DEFAULT_GRID_MM);
      }
      onPreviewDoc((prev) => {
        if (drag.kind === "table") {
          return {
            ...prev,
            tables: prev.tables.map((t) =>
              t.clientKey === drag.key
                ? {
                    ...t,
                    x_mm: Math.round(x),
                    y_mm: Math.round(y),
                    width_mm: Math.round(w),
                    height_mm: Math.round(h),
                  }
                : t,
            ),
          };
        }
        return {
          ...prev,
          elements: prev.elements.map((el) =>
            el.clientKey === drag.key
              ? {
                  ...el,
                  x_mm: Math.round(x),
                  y_mm: Math.round(y),
                  width_mm: Math.round(w),
                  height_mm: Math.round(h),
                }
              : el,
          ),
        };
      });
      return;
    }

    if (drag.type === "rotate") {
      const angle = Math.atan2(
        world.y - drag.center.y,
        world.x - drag.center.x,
      );
      let deg =
        drag.startRot + ((angle - drag.startAngle) * 180) / Math.PI;
      // Snap to 15° when shift held
      if (e.shiftKey) deg = Math.round(deg / 15) * 15;
      onPreviewDoc((prev) => {
        if (drag.kind === "table") {
          return {
            ...prev,
            tables: prev.tables.map((t) =>
              t.clientKey === drag.key
                ? { ...t, rotation_deg: Math.round(deg * 10) / 10 }
                : t,
            ),
          };
        }
        return {
          ...prev,
          elements: prev.elements.map((el) =>
            el.clientKey === drag.key
              ? { ...el, rotation_deg: Math.round(deg * 10) / 10 }
              : el,
          ),
        };
      });
    }
  };

  const handlePointerUp = () => {
    if (dragRef.current) {
      if (dragRef.current.type !== "pan") {
        onCommitDrag();
      }
      dragRef.current = null;
      setGuides([]);
    }
  };

  const handleDoubleClick = (e: React.MouseEvent) => {
    if (
      tool === "draw-polygon" ||
      tool === "draw-region" ||
      tool === "draw-rect-room"
    ) {
      if (drawingPoints.length >= 3) {
        onPolygonComplete(drawingPoints);
      } else if (tool === "draw-rect-room" && drawingPoints.length === 2) {
        const [a, b] = drawingPoints;
        onPolygonComplete([
          { x: Math.min(a.x, b.x), y: Math.min(a.y, b.y) },
          { x: Math.max(a.x, b.x), y: Math.min(a.y, b.y) },
          { x: Math.max(a.x, b.x), y: Math.max(a.y, b.y) },
          { x: Math.min(a.x, b.x), y: Math.max(a.y, b.y) },
        ]);
      }
    }
  };

  // Complete rect room with 2 clicks
  useEffect(() => {
    if (tool === "draw-rect-room" && drawingPoints.length >= 2) {
      const [a, b] = drawingPoints;
      onPolygonComplete([
        { x: Math.min(a.x, b.x), y: Math.min(a.y, b.y) },
        { x: Math.max(a.x, b.x), y: Math.min(a.y, b.y) },
        { x: Math.max(a.x, b.x), y: Math.max(a.y, b.y) },
        { x: Math.min(a.x, b.x), y: Math.max(a.y, b.y) },
      ]);
    }
  }, [drawingPoints, tool, onPolygonComplete]);

  const boundaryPath = useMemo(() => {
    const pts = doc.boundary?.points_mm;
    if (pts && pts.length >= 3) return polygonPath(pts);
    return null;
  }, [doc.boundary]);

  const gridStep = DEFAULT_GRID_MM;
  const showGrid = previewMode === "edit";
  const { scale, panX, panY } = viewport.viewport;

  return (
    // Drawing surface: Escape cancels in-progress drag/draw. Keyboard shortcuts
    // also live in useKeyboardShortcuts on the page shell.
    // eslint-disable-next-line jsx-a11y/no-noninteractive-element-interactions -- SVG editor canvas
    <div
      ref={containerRef}
      role="application"
      tabIndex={0}
      aria-label={t("editor.canvasAria")}
      className="relative h-full min-h-0 w-full flex-1 overflow-hidden bg-[#f3f1ec] touch-none outline-none focus-visible:ring-2 focus-visible:ring-brand"
      data-testid="floor-canvas"
      onKeyDown={(e) => {
        if (e.key === "Escape") {
          onCancelDrag();
          onClearDrawing();
        }
      }}
    >
      <svg
        ref={svgRef}
        className="h-full w-full"
        onWheel={onWheel}
        onPointerDown={handlePointerDownBackground}
        onPointerMove={handlePointerMove}
        onPointerUp={handlePointerUp}
        onPointerCancel={handlePointerUp}
        onDoubleClick={handleDoubleClick}
        role="img"
        aria-label={t("editor.canvasAria")}
      >
        <defs>
          <pattern
            id="floor-grid"
            width={gridStep}
            height={gridStep}
            patternUnits="userSpaceOnUse"
          >
            <path
              d={`M ${gridStep} 0 L 0 0 0 ${gridStep}`}
              fill="none"
              stroke="rgba(28,25,23,0.08)"
              strokeWidth={8}
            />
          </pattern>
        </defs>
        <g transform={`translate(${panX},${panY}) scale(${scale})`}>
          {/* Room background */}
          <rect
            x={0}
            y={0}
            width={roomW}
            height={roomH}
            fill="#faf9f6"
            stroke="rgba(28,25,23,0.12)"
            strokeWidth={20}
          />
          {showGrid && (
            <rect
              x={0}
              y={0}
              width={roomW}
              height={roomH}
              fill="url(#floor-grid)"
            />
          )}
          {boundaryPath ? (
            <path
              d={boundaryPath}
              fill="rgba(26,107,106,0.04)"
              stroke="rgba(26,107,106,0.45)"
              strokeWidth={30}
            />
          ) : null}

          {/* Regions under everything */}
          {doc.regions.map((region: EditorRegion) => (
            <path
              key={region.clientKey}
              d={polygonPath(region.polygon_mm)}
              fill={region.color || "rgba(26,107,106,0.12)"}
              stroke={
                isSelected({ kind: "region", key: region.clientKey })
                  ? "#1a6b6a"
                  : "rgba(26,107,106,0.35)"
              }
              strokeWidth={
                isSelected({ kind: "region", key: region.clientKey })
                  ? 36
                  : 16
              }
              onPointerDown={(e) => {
                e.stopPropagation();
                onSelect(
                  [{ kind: "region", key: region.clientKey }],
                  e.shiftKey,
                );
              }}
              data-canvas-item="region"
              data-testid={`canvas-region-${region.clientKey}`}
            />
          ))}

          {/* Structural elements under tables */}
          {sortedElements.map((el) => (
            <MemoElement
              key={el.clientKey}
              element={el}
              selected={isSelected({ kind: "element", key: el.clientKey })}
              onPointerDown={handleElementPointerDown}
            />
          ))}

          {/* Tables */}
          {doc.tables.map((table) => (
            <MemoTable
              key={table.clientKey}
              table={table}
              selected={isSelected({ kind: "table", key: table.clientKey })}
              previewMode={previewMode}
              showSeats={previewMode !== "guest"}
              onPointerDown={handleTablePointerDown}
            />
          ))}

          {/* Selection handles */}
          {singleSelectionRect && Math.abs(singleSelectionRect.rect.rotationDeg) < 0.01 && (
            <SelectionHandles
              rect={singleSelectionRect.rect}
              onResizeDown={handleResizeDown}
              onRotateDown={handleRotateDown}
            />
          )}
          {singleSelectionRect && Math.abs(singleSelectionRect.rect.rotationDeg) >= 0.01 && (
            <g
              transform={`rotate(${singleSelectionRect.rect.rotationDeg} ${
                singleSelectionRect.rect.x + singleSelectionRect.rect.w / 2
              } ${
                singleSelectionRect.rect.y + singleSelectionRect.rect.h / 2
              })`}
            >
              <SelectionHandles
                rect={singleSelectionRect.rect}
                onResizeDown={handleResizeDown}
                onRotateDown={handleRotateDown}
              />
            </g>
          )}

          {/* Alignment guides */}
          {guides.map((g, i) =>
            g.orientation === "v" ? (
              <line
                key={`g-${i}`}
                x1={g.position}
                y1={0}
                x2={g.position}
                y2={roomH}
                stroke="#f59e0b"
                strokeWidth={12}
                strokeDasharray="60 40"
                pointerEvents="none"
              />
            ) : (
              <line
                key={`g-${i}`}
                x1={0}
                y1={g.position}
                x2={roomW}
                y2={g.position}
                stroke="#f59e0b"
                strokeWidth={12}
                strokeDasharray="60 40"
                pointerEvents="none"
              />
            ),
          )}

          {/* Drawing preview */}
          {drawingPoints.length > 0 && (
            <g pointerEvents="none">
              {drawingPoints.map((p, i) => (
                <circle
                  key={i}
                  cx={p.x}
                  cy={p.y}
                  r={80}
                  fill="#1a6b6a"
                />
              ))}
              {drawingPoints.length >= 2 && (
                <polyline
                  points={drawingPoints.map((p) => `${p.x},${p.y}`).join(" ")}
                  fill="none"
                  stroke="#1a6b6a"
                  strokeWidth={24}
                  strokeDasharray="80 40"
                />
              )}
            </g>
          )}
        </g>
      </svg>

      {(tool === "draw-polygon" ||
        tool === "draw-region" ||
        tool === "draw-rect-room") && (
        <div className="pointer-events-none absolute bottom-3 left-1/2 -translate-x-1/2 rounded-full bg-ink-900/85 px-3 py-1.5 text-xs text-white">
          {tool === "draw-rect-room"
            ? t("editor.drawHintRect")
            : t("editor.drawHintPoly")}
        </div>
      )}
    </div>
  );
}
