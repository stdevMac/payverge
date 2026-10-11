/**
 * @jest-environment jsdom
 */
import { act, renderHook } from "@testing-library/react";
import { useEditorState, computeSoftWarnings } from "../hooks/useEditorState";
import { layoutToEditor, type EditorDocument } from "../types";

function baseDoc(overrides?: Partial<EditorDocument>): EditorDocument {
  const doc = layoutToEditor({
    schema_version: 1,
    width_mm: 10000,
    height_mm: 8000,
    measurement_unit: "m",
    tables: [],
    elements: [],
    regions: [],
  });
  return { ...doc, ...overrides };
}

describe("useEditorState", () => {
  it("supports undo/redo for table add", () => {
    const { result } = renderHook(() =>
      useEditorState({ initial: baseDoc() }),
    );

    act(() => {
      result.current.addTable({
        table_id: 11,
        name: "A1",
        shape: "square",
        x_mm: 1000,
        y_mm: 1000,
      });
    });
    expect(result.current.doc.tables).toHaveLength(1);
    expect(result.current.canUndo).toBe(true);

    act(() => {
      result.current.undo();
    });
    expect(result.current.doc.tables).toHaveLength(0);
    expect(result.current.canRedo).toBe(true);

    act(() => {
      result.current.redo();
    });
    expect(result.current.doc.tables).toHaveLength(1);
    expect(result.current.doc.tables[0].name).toBe("A1");
  });

  it("adds a region polygon without deleting tables", () => {
    const { result } = renderHook(() =>
      useEditorState({
        initial: baseDoc({
          tables: [
            {
              clientKey: "t1",
              table_id: 1,
              name: "T1",
              x_mm: 500,
              y_mm: 500,
              width_mm: 800,
              height_mm: 800,
              shape: "square",
              max_capacity: 4,
              visible_seat_count: 4,
            },
          ],
        }),
      }),
    );

    act(() => {
      result.current.addRegion(
        [
          { x: 0, y: 0 },
          { x: 3000, y: 0 },
          { x: 3000, y: 3000 },
          { x: 0, y: 3000 },
        ],
        "Patio",
        "#aabbcc55",
      );
    });

    expect(result.current.doc.regions).toHaveLength(1);
    expect(result.current.doc.regions[0].name).toBe("Patio");
    expect(result.current.doc.tables).toHaveLength(1);

    // Remove region only
    act(() => {
      result.current.select([
        { kind: "region", key: result.current.doc.regions[0].clientKey },
      ]);
    });
    act(() => {
      result.current.removeSelection();
    });
    expect(result.current.doc.regions).toHaveLength(0);
    expect(result.current.doc.tables).toHaveLength(1);
  });

  it("nudges selected tables", () => {
    const { result } = renderHook(() =>
      useEditorState({
        initial: baseDoc({
          tables: [
            {
              clientKey: "t1",
              table_id: 1,
              name: "T1",
              x_mm: 100,
              y_mm: 200,
              width_mm: 800,
              height_mm: 800,
              shape: "round",
              max_capacity: 4,
            },
          ],
        }),
      }),
    );

    act(() => {
      result.current.select([{ kind: "table", key: "t1" }]);
    });
    act(() => {
      result.current.nudgeSelection(50, -20);
    });
    expect(result.current.doc.tables[0].x_mm).toBe(150);
    expect(result.current.doc.tables[0].y_mm).toBe(180);
  });

  it("updates table properties (edit name, size, rotation)", () => {
    const { result } = renderHook(() =>
      useEditorState({
        initial: baseDoc({
          tables: [
            {
              clientKey: "t1",
              table_id: 1,
              name: "Old",
              x_mm: 100,
              y_mm: 200,
              width_mm: 800,
              height_mm: 800,
              shape: "square",
              max_capacity: 4,
              rotation_deg: 0,
            },
          ],
        }),
      }),
    );

    act(() => {
      result.current.updateTable("t1", {
        name: "Window",
        width_mm: 1200,
        height_mm: 900,
        rotation_deg: 45,
        max_capacity: 6,
      });
    });

    const t = result.current.doc.tables[0];
    expect(t.name).toBe("Window");
    expect(t.width_mm).toBe(1200);
    expect(t.height_mm).toBe(900);
    expect(t.rotation_deg).toBe(45);
    expect(t.max_capacity).toBe(6);
    expect(result.current.canUndo).toBe(true);

    act(() => {
      result.current.undo();
    });
    expect(result.current.doc.tables[0].name).toBe("Old");
  });

  it("simulates drag move via beginDrag/preview/commit", () => {
    const { result } = renderHook(() =>
      useEditorState({
        initial: baseDoc({
          tables: [
            {
              clientKey: "t1",
              table_id: 1,
              name: "T1",
              x_mm: 500,
              y_mm: 500,
              width_mm: 800,
              height_mm: 800,
              shape: "round",
              max_capacity: 4,
            },
          ],
        }),
      }),
    );

    act(() => {
      result.current.beginDrag();
      result.current.previewDoc((prev) => ({
        ...prev,
        tables: prev.tables.map((t) =>
          t.clientKey === "t1"
            ? { ...t, x_mm: t.x_mm + 250, y_mm: t.y_mm + 100 }
            : t,
        ),
      }));
    });
    expect(result.current.doc.tables[0].x_mm).toBe(750);
    expect(result.current.doc.tables[0].y_mm).toBe(600);

    act(() => {
      result.current.commitDrag();
    });
    expect(result.current.canUndo).toBe(true);

    act(() => {
      result.current.undo();
    });
    expect(result.current.doc.tables[0].x_mm).toBe(500);
  });

  it("simulates resize and rotate via previewDoc", () => {
    const { result } = renderHook(() =>
      useEditorState({
        initial: baseDoc({
          tables: [
            {
              clientKey: "t1",
              table_id: 1,
              name: "T1",
              x_mm: 0,
              y_mm: 0,
              width_mm: 1000,
              height_mm: 1000,
              shape: "square",
              max_capacity: 4,
              rotation_deg: 0,
            },
          ],
        }),
      }),
    );

    act(() => {
      result.current.beginDrag();
      result.current.previewDoc((prev) => ({
        ...prev,
        tables: prev.tables.map((t) =>
          t.clientKey === "t1"
            ? { ...t, width_mm: 1400, height_mm: 900 }
            : t,
        ),
      }));
      result.current.commitDrag();
    });
    expect(result.current.doc.tables[0].width_mm).toBe(1400);
    expect(result.current.doc.tables[0].height_mm).toBe(900);

    act(() => {
      result.current.beginDrag();
      result.current.previewDoc((prev) => ({
        ...prev,
        tables: prev.tables.map((t) =>
          t.clientKey === "t1" ? { ...t, rotation_deg: 30 } : t,
        ),
      }));
      result.current.commitDrag();
    });
    expect(result.current.doc.tables[0].rotation_deg).toBe(30);
  });

  it("cancels an in-progress drag", () => {
    const { result } = renderHook(() =>
      useEditorState({
        initial: baseDoc({
          tables: [
            {
              clientKey: "t1",
              table_id: 1,
              name: "T1",
              x_mm: 100,
              y_mm: 100,
              width_mm: 500,
              height_mm: 500,
              shape: "square",
              max_capacity: 2,
            },
          ],
        }),
      }),
    );

    act(() => {
      result.current.beginDrag();
      result.current.previewDoc((prev) => ({
        ...prev,
        tables: prev.tables.map((t) =>
          t.clientKey === "t1" ? { ...t, x_mm: 999 } : t,
        ),
      }));
      result.current.cancelDrag();
    });
    expect(result.current.doc.tables[0].x_mm).toBe(100);
  });

  it("sets room size for manual drawing boundary", () => {
    const { result } = renderHook(() =>
      useEditorState({ initial: baseDoc() }),
    );

    act(() => {
      result.current.setRoomSize(6000, 4500);
    });
    expect(result.current.doc.width_mm).toBe(6000);
    expect(result.current.doc.height_mm).toBe(4500);
    expect(result.current.doc.boundary?.points_mm).toHaveLength(4);
  });

  it("adds multiple tables (multi-space layout building)", () => {
    const { result } = renderHook(() =>
      useEditorState({ initial: baseDoc() }),
    );

    act(() => {
      result.current.addTable({ table_id: 1, name: "A1", shape: "round" });
      result.current.addTable({ table_id: 2, name: "A2", shape: "bar" });
      result.current.addTable({ table_id: 3, name: "A3", shape: "oval" });
    });
    expect(result.current.doc.tables).toHaveLength(3);
    expect(result.current.doc.tables.map((t) => t.name)).toEqual([
      "A1",
      "A2",
      "A3",
    ]);
  });
});

describe("computeSoftWarnings", () => {
  it("flags empty layout and overlaps", () => {
    const empty = baseDoc();
    expect(
      computeSoftWarnings(empty).some((w) => w.code === "emptyLayout"),
    ).toBe(true);

    const overlap = baseDoc({
      tables: [
        {
          clientKey: "a",
          table_id: 1,
          name: "A",
          x_mm: 0,
          y_mm: 0,
          width_mm: 1000,
          height_mm: 1000,
          shape: "square",
          max_capacity: 4,
        },
        {
          clientKey: "b",
          table_id: 2,
          name: "B",
          x_mm: 500,
          y_mm: 500,
          width_mm: 1000,
          height_mm: 1000,
          shape: "square",
          max_capacity: 4,
        },
      ],
    });
    expect(
      computeSoftWarnings(overlap).some((w) => w.code === "overlap"),
    ).toBe(true);
  });

  it("flags duplicate labels", () => {
    const doc = baseDoc({
      tables: [
        {
          clientKey: "a",
          table_id: 1,
          name: "T1",
          x_mm: 0,
          y_mm: 0,
          width_mm: 500,
          height_mm: 500,
          shape: "square",
          max_capacity: 2,
        },
        {
          clientKey: "b",
          table_id: 2,
          name: "T1",
          x_mm: 2000,
          y_mm: 0,
          width_mm: 500,
          height_mm: 500,
          shape: "square",
          max_capacity: 2,
        },
      ],
    });
    expect(
      computeSoftWarnings(doc).some((w) => w.code === "duplicateLabels"),
    ).toBe(true);
  });
});
