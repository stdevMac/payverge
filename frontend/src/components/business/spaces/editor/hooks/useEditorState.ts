"use client";

/* eslint-disable no-restricted-syntax -- default region fill is brand teal with alpha */

import { useCallback, useMemo, useRef, useState } from "react";
import type { LayoutElementType, TableShape } from "@/api/spaces";
import {
  DEFAULT_ELEMENT_SIZE,
  DEFAULT_TABLE_SIZE,
  cloneDoc,
  nextClientKey,
  type EditorDocument,
  type EditorElement,
  type EditorRegion,
  type EditorTable,
  type SelectionRef,
  type SoftWarning,
} from "../types";
import {
  tableIntersects,
  type OrientedRect,
  type Point,
} from "../geometry";

const MAX_HISTORY = 80;

export interface ClipboardPayload {
  tables: EditorTable[];
  elements: EditorElement[];
  regions: EditorRegion[];
}

function selectionKey(ref: SelectionRef): string {
  return `${ref.kind}:${ref.key}`;
}

function collectOriented(tables: EditorTable[]): OrientedRect[] {
  return tables.map((t) => ({
    x: t.x_mm,
    y: t.y_mm,
    w: t.width_mm,
    h: t.height_mm,
    rotationDeg: t.rotation_deg ?? 0,
  }));
}

export function computeSoftWarnings(doc: EditorDocument): SoftWarning[] {
  const warnings: SoftWarning[] = [];
  if (doc.tables.length === 0) {
    warnings.push({
      code: "emptyLayout",
      messageKey: "warnings.emptyLayout",
    });
  }

  const rects = collectOriented(doc.tables);
  for (let i = 0; i < rects.length; i++) {
    for (let j = i + 1; j < rects.length; j++) {
      if (tableIntersects(rects[i], rects[j])) {
        warnings.push({
          code: "overlap",
          messageKey: "warnings.overlap",
          path: `tables[${i}]/tables[${j}]`,
        });
      }
    }
  }

  if (doc.width_mm > 0 && doc.height_mm > 0) {
    for (let i = 0; i < doc.tables.length; i++) {
      const t = doc.tables[i];
      const rot = t.rotation_deg ?? 0;
      if (
        rot === 0 &&
        (t.x_mm < 0 ||
          t.y_mm < 0 ||
          t.x_mm + t.width_mm > doc.width_mm ||
          t.y_mm + t.height_mm > doc.height_mm)
      ) {
        warnings.push({
          code: "outOfBounds",
          messageKey: "warnings.outOfBounds",
          path: `tables[${i}]`,
        });
      }
    }
  }

  const labels = new Map<string, number>();
  for (const t of doc.tables) {
    const name = (t.name || "").trim().toLowerCase();
    if (!name) continue;
    labels.set(name, (labels.get(name) ?? 0) + 1);
  }
  for (const [name, count] of labels) {
    if (count > 1) {
      warnings.push({
        code: "duplicateLabels",
        messageKey: "warnings.duplicateLabels",
        path: name,
      });
    }
  }

  for (let i = 0; i < doc.tables.length; i++) {
    const t = doc.tables[i];
    const max = t.max_capacity ?? t.visible_seat_count ?? t.min_capacity;
    if (max == null || max <= 0) {
      warnings.push({
        code: "missingCapacity",
        messageKey: "warnings.missingCapacity",
        path: `tables[${i}]`,
      });
    }
  }

  return warnings;
}

export interface UseEditorStateOptions {
  initial: EditorDocument;
}

export function useEditorState({ initial }: UseEditorStateOptions) {
  const [doc, setDoc] = useState<EditorDocument>(() => cloneDoc(initial));
  const [selection, setSelection] = useState<SelectionRef[]>([]);
  const [revision, setRevisionLocal] = useState(0);
  const [canUndo, setCanUndo] = useState(false);
  const [canRedo, setCanRedo] = useState(false);
  const undoStack = useRef<EditorDocument[]>([]);
  const redoStack = useRef<EditorDocument[]>([]);
  const clipboard = useRef<ClipboardPayload | null>(null);
  const docRef = useRef(doc);
  docRef.current = doc;

  const syncHistoryFlags = useCallback(() => {
    setCanUndo(undoStack.current.length > 0);
    setCanRedo(redoStack.current.length > 0);
  }, []);

  const pushHistory = useCallback((next: EditorDocument) => {
    undoStack.current.push(cloneDoc(docRef.current));
    if (undoStack.current.length > MAX_HISTORY) {
      undoStack.current.shift();
    }
    redoStack.current = [];
    docRef.current = next;
    setDoc(next);
    setRevisionLocal((r) => r + 1);
    setCanUndo(true);
    setCanRedo(false);
  }, []);

  /** Replace document without history (load / discard / remote reload). */
  const replaceDocument = useCallback((next: EditorDocument) => {
    undoStack.current = [];
    redoStack.current = [];
    docRef.current = next;
    setDoc(cloneDoc(next));
    setSelection([]);
    // Do not bump historyRevision — avoids accidental autosave on load.
    setCanUndo(false);
    setCanRedo(false);
  }, []);

  /** Commit a mutation from current doc via updater (with history). */
  const commit = useCallback(
    (updater: (prev: EditorDocument) => EditorDocument) => {
      const next = updater(cloneDoc(docRef.current));
      pushHistory(next);
    },
    [pushHistory],
  );

  /**
   * Live preview mutation without history (pointer drag). Call commitDrag
   * once on pointerup to snapshot history from the pre-drag baseline.
   */
  const dragBaseline = useRef<EditorDocument | null>(null);

  const beginDrag = useCallback(() => {
    dragBaseline.current = cloneDoc(docRef.current);
  }, []);

  const previewDoc = useCallback((updater: (prev: EditorDocument) => EditorDocument) => {
    const next = updater(cloneDoc(docRef.current));
    docRef.current = next;
    setDoc(next);
  }, []);

  const commitDrag = useCallback(() => {
    if (dragBaseline.current) {
      undoStack.current.push(dragBaseline.current);
      if (undoStack.current.length > MAX_HISTORY) {
        undoStack.current.shift();
      }
      redoStack.current = [];
      dragBaseline.current = null;
      setRevisionLocal((r) => r + 1);
      setCanUndo(true);
      setCanRedo(false);
    }
  }, []);

  const cancelDrag = useCallback(() => {
    if (dragBaseline.current) {
      docRef.current = dragBaseline.current;
      setDoc(cloneDoc(dragBaseline.current));
      dragBaseline.current = null;
    }
  }, []);

  const undo = useCallback(() => {
    const prev = undoStack.current.pop();
    if (!prev) return;
    redoStack.current.push(cloneDoc(docRef.current));
    docRef.current = prev;
    setDoc(cloneDoc(prev));
    setRevisionLocal((r) => r + 1);
    syncHistoryFlags();
  }, [syncHistoryFlags]);

  const redo = useCallback(() => {
    const next = redoStack.current.pop();
    if (!next) return;
    undoStack.current.push(cloneDoc(docRef.current));
    docRef.current = next;
    setDoc(cloneDoc(next));
    setRevisionLocal((r) => r + 1);
    syncHistoryFlags();
  }, [syncHistoryFlags]);

  const select = useCallback((refs: SelectionRef[], additive = false) => {
    setSelection((prev) => {
      if (!additive) return refs;
      const map = new Map(prev.map((r) => [selectionKey(r), r]));
      for (const r of refs) {
        const k = selectionKey(r);
        if (map.has(k)) map.delete(k);
        else map.set(k, r);
      }
      return Array.from(map.values());
    });
  }, []);

  const clearSelection = useCallback(() => setSelection([]), []);

  const selectedKeys = useMemo(
    () => new Set(selection.map(selectionKey)),
    [selection],
  );

  const isSelected = useCallback(
    (ref: SelectionRef) => selectedKeys.has(selectionKey(ref)),
    [selectedKeys],
  );

  const addTable = useCallback(
    (partial: {
      table_id: number;
      name?: string;
      shape?: TableShape | string;
      x_mm?: number;
      y_mm?: number;
      width_mm?: number;
      height_mm?: number;
      min_capacity?: number;
      max_capacity?: number;
      visible_seat_count?: number;
    }) => {
      const shape = (partial.shape || "square") as TableShape;
      const size = DEFAULT_TABLE_SIZE[shape] || DEFAULT_TABLE_SIZE.square;
      const table: EditorTable = {
        clientKey: nextClientKey("tbl"),
        table_id: partial.table_id,
        name: partial.name ?? `T${docRef.current.tables.length + 1}`,
        x_mm: partial.x_mm ?? 500,
        y_mm: partial.y_mm ?? 500,
        width_mm: partial.width_mm ?? size.width_mm,
        height_mm: partial.height_mm ?? size.height_mm,
        rotation_deg: 0,
        shape,
        min_capacity: partial.min_capacity ?? 2,
        max_capacity: partial.max_capacity ?? 4,
        visible_seat_count: partial.visible_seat_count ?? 4,
        is_reservable: true,
        is_combinable: false,
        is_accessible: false,
      };
      commit((prev) => ({ ...prev, tables: [...prev.tables, table] }));
      setSelection([{ kind: "table", key: table.clientKey }]);
      return table;
    },
    [commit],
  );

  const addElement = useCallback(
    (elementType: LayoutElementType, at?: Point) => {
      const size =
        DEFAULT_ELEMENT_SIZE[elementType] || DEFAULT_ELEMENT_SIZE.obstacle;
      const el: EditorElement = {
        clientKey: nextClientKey("el"),
        element_type: elementType,
        name: elementType.replace(/_/g, " "),
        x_mm: at?.x ?? 400,
        y_mm: at?.y ?? 400,
        width_mm: size.width_mm,
        height_mm: size.height_mm,
        rotation_deg: 0,
        z_index: 0,
      };
      commit((prev) => ({
        ...prev,
        elements: [...prev.elements, el],
      }));
      setSelection([{ kind: "element", key: el.clientKey }]);
      return el;
    },
    [commit],
  );

  const addRegion = useCallback(
    (polygon: Point[], name?: string, color?: string) => {
      const region: EditorRegion = {
        clientKey: nextClientKey("region"),
        name: name ?? `Region ${docRef.current.regions.length + 1}`,
        color: color ?? "#1a6b6a33",
        polygon_mm: polygon,
      };
      commit((prev) => ({
        ...prev,
        regions: [...prev.regions, region],
      }));
      setSelection([{ kind: "region", key: region.clientKey }]);
      return region;
    },
    [commit],
  );

  const updateTable = useCallback(
    (key: string, patch: Partial<EditorTable>) => {
      commit((prev) => ({
        ...prev,
        tables: prev.tables.map((t) =>
          t.clientKey === key ? { ...t, ...patch, clientKey: t.clientKey } : t,
        ),
      }));
    },
    [commit],
  );

  const updateElement = useCallback(
    (key: string, patch: Partial<EditorElement>) => {
      commit((prev) => ({
        ...prev,
        elements: prev.elements.map((e) =>
          e.clientKey === key ? { ...e, ...patch, clientKey: e.clientKey } : e,
        ),
      }));
    },
    [commit],
  );

  const updateRegion = useCallback(
    (key: string, patch: Partial<EditorRegion>) => {
      commit((prev) => ({
        ...prev,
        regions: prev.regions.map((r) =>
          r.clientKey === key ? { ...r, ...patch, clientKey: r.clientKey } : r,
        ),
      }));
    },
    [commit],
  );

  const setRoomSize = useCallback(
    (width_mm: number, height_mm: number, boundary?: EditorDocument["boundary"]) => {
      commit((prev) => ({
        ...prev,
        width_mm,
        height_mm,
        boundary:
          boundary ??
          prev.boundary ?? {
            points_mm: [
              { x: 0, y: 0 },
              { x: width_mm, y: 0 },
              { x: width_mm, y: height_mm },
              { x: 0, y: height_mm },
            ],
            closed: true,
          },
      }));
    },
    [commit],
  );

  const removeSelection = useCallback(() => {
    if (selection.length === 0) return;
    const keys = new Set(selection.map(selectionKey));
    commit((prev) => {
      const removedRegionIds = new Set(
        prev.regions
          .filter((r) => keys.has(`region:${r.clientKey}`) && r.id != null)
          .map((r) => r.id as number),
      );
      return {
        ...prev,
        tables: prev.tables
          .filter((t) => !keys.has(`table:${t.clientKey}`))
          .map((t) =>
            t.region_id != null && removedRegionIds.has(t.region_id)
              ? { ...t, region_id: undefined }
              : t,
          ),
        elements: prev.elements.filter(
          (e) => !keys.has(`element:${e.clientKey}`),
        ),
        regions: prev.regions.filter(
          (r) => !keys.has(`region:${r.clientKey}`),
        ),
      };
    });
    setSelection([]);
  }, [commit, selection]);

  const duplicateSelection = useCallback(() => {
    if (selection.length === 0) return;
    const keys = new Set(selection.map(selectionKey));
    const offset = 300;
    commit((prev) => {
      const newTables: EditorTable[] = [];
      const newElements: EditorElement[] = [];
      const newRegions: EditorRegion[] = [];
      const newSel: SelectionRef[] = [];

      for (const t of prev.tables) {
        if (!keys.has(`table:${t.clientKey}`)) continue;
        // Duplicating creates a layout clone with same table_id is invalid —
        // caller should create a new table via API. For pure layout duplicate
        // of shape/position we still clone with a temp negative id; SpaceEditorPage
        // intercepts and creates real tables. Here we clone with table_id=0 flag.
        const copy: EditorTable = {
          ...t,
          clientKey: nextClientKey("tbl"),
          table_id: 0,
          name: t.name ? `${t.name} copy` : undefined,
          x_mm: t.x_mm + offset,
          y_mm: t.y_mm + offset,
        };
        newTables.push(copy);
        newSel.push({ kind: "table", key: copy.clientKey });
      }
      for (const e of prev.elements) {
        if (!keys.has(`element:${e.clientKey}`)) continue;
        const copy: EditorElement = {
          ...e,
          clientKey: nextClientKey("el"),
          x_mm: (e.x_mm ?? 0) + offset,
          y_mm: (e.y_mm ?? 0) + offset,
        };
        newElements.push(copy);
        newSel.push({ kind: "element", key: copy.clientKey });
      }
      for (const r of prev.regions) {
        if (!keys.has(`region:${r.clientKey}`)) continue;
        const copy: EditorRegion = {
          ...r,
          clientKey: nextClientKey("region"),
          id: undefined,
          name: r.name ? `${r.name} copy` : r.name,
          polygon_mm: r.polygon_mm.map((p) => ({
            x: p.x + offset,
            y: p.y + offset,
          })),
        };
        newRegions.push(copy);
        newSel.push({ kind: "region", key: copy.clientKey });
      }
      // Store pending selection after commit
      queueMicrotask(() => setSelection(newSel));
      return {
        ...prev,
        tables: [...prev.tables, ...newTables],
        elements: [...prev.elements, ...newElements],
        regions: [...prev.regions, ...newRegions],
      };
    });
  }, [commit, selection]);

  const copySelection = useCallback(() => {
    if (selection.length === 0) return;
    const keys = new Set(selection.map(selectionKey));
    clipboard.current = {
      tables: docRef.current.tables.filter((t) =>
        keys.has(`table:${t.clientKey}`),
      ),
      elements: docRef.current.elements.filter((e) =>
        keys.has(`element:${e.clientKey}`),
      ),
      regions: docRef.current.regions.filter((r) =>
        keys.has(`region:${r.clientKey}`),
      ),
    };
  }, [selection]);

  const pasteClipboard = useCallback(() => {
    const clip = clipboard.current;
    if (!clip) return;
    const offset = 300;
    commit((prev) => {
      const newSel: SelectionRef[] = [];
      const tables = clip.tables.map((t) => {
        const copy: EditorTable = {
          ...t,
          clientKey: nextClientKey("tbl"),
          table_id: 0,
          name: t.name ? `${t.name} copy` : undefined,
          x_mm: t.x_mm + offset,
          y_mm: t.y_mm + offset,
        };
        newSel.push({ kind: "table", key: copy.clientKey });
        return copy;
      });
      const elements = clip.elements.map((e) => {
        const copy: EditorElement = {
          ...e,
          clientKey: nextClientKey("el"),
          x_mm: (e.x_mm ?? 0) + offset,
          y_mm: (e.y_mm ?? 0) + offset,
        };
        newSel.push({ kind: "element", key: copy.clientKey });
        return copy;
      });
      const regions = clip.regions.map((r) => {
        const copy: EditorRegion = {
          ...r,
          clientKey: nextClientKey("region"),
          id: undefined,
          polygon_mm: r.polygon_mm.map((p) => ({
            x: p.x + offset,
            y: p.y + offset,
          })),
        };
        newSel.push({ kind: "region", key: copy.clientKey });
        return copy;
      });
      queueMicrotask(() => setSelection(newSel));
      return {
        ...prev,
        tables: [...prev.tables, ...tables],
        elements: [...prev.elements, ...elements],
        regions: [...prev.regions, ...regions],
      };
    });
  }, [commit]);

  const nudgeSelection = useCallback(
    (dx: number, dy: number) => {
      if (selection.length === 0) return;
      const keys = new Set(selection.map(selectionKey));
      commit((prev) => ({
        ...prev,
        tables: prev.tables.map((t) =>
          keys.has(`table:${t.clientKey}`)
            ? { ...t, x_mm: t.x_mm + dx, y_mm: t.y_mm + dy }
            : t,
        ),
        elements: prev.elements.map((e) =>
          keys.has(`element:${e.clientKey}`)
            ? {
                ...e,
                x_mm: (e.x_mm ?? 0) + dx,
                y_mm: (e.y_mm ?? 0) + dy,
              }
            : e,
        ),
        regions: prev.regions.map((r) =>
          keys.has(`region:${r.clientKey}`)
            ? {
                ...r,
                polygon_mm: r.polygon_mm.map((p) => ({
                  x: p.x + dx,
                  y: p.y + dy,
                })),
              }
            : r,
        ),
      }));
    },
    [commit, selection],
  );

  const bringForward = useCallback(() => {
    // Elements under tables: higher z_index draws later among elements.
    if (selection.length === 0) return;
    const keys = new Set(
      selection.filter((s) => s.kind === "element").map((s) => s.key),
    );
    if (keys.size === 0) return;
    commit((prev) => ({
      ...prev,
      elements: prev.elements.map((e) =>
        keys.has(e.clientKey)
          ? { ...e, z_index: (e.z_index ?? 0) + 1 }
          : e,
      ),
    }));
  }, [commit, selection]);

  const sendBackward = useCallback(() => {
    if (selection.length === 0) return;
    const keys = new Set(
      selection.filter((s) => s.kind === "element").map((s) => s.key),
    );
    if (keys.size === 0) return;
    commit((prev) => ({
      ...prev,
      elements: prev.elements.map((e) =>
        keys.has(e.clientKey)
          ? { ...e, z_index: (e.z_index ?? 0) - 1 }
          : e,
      ),
    }));
  }, [commit, selection]);

  const softWarnings = useMemo(() => computeSoftWarnings(doc), [doc]);

  return {
    doc,
    docRef,
    selection,
    select,
    clearSelection,
    isSelected,
    selectedKeys,
    commit,
    replaceDocument,
    beginDrag,
    previewDoc,
    commitDrag,
    cancelDrag,
    undo,
    redo,
    canUndo,
    canRedo,
    historyRevision: revision,
    addTable,
    addElement,
    addRegion,
    updateTable,
    updateElement,
    updateRegion,
    setRoomSize,
    removeSelection,
    duplicateSelection,
    copySelection,
    pasteClipboard,
    nudgeSelection,
    bringForward,
    sendBackward,
    softWarnings,
    clipboardRef: clipboard,
  };
}
