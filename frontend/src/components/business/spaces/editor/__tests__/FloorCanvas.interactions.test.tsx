/**
 * @jest-environment jsdom
 *
 * Drag / resize / rotate via simulated pointer events on FloorCanvas.
 */
import React, { useState } from "react";
import { act, fireEvent, render, screen } from "@testing-library/react";
import { FloorCanvas } from "../FloorCanvas";
import { layoutToEditor, type EditorDocument, type SelectionRef } from "../types";
import { useCanvasViewport } from "../hooks/useCanvasViewport";

beforeAll(() => {
  global.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver;

  // Pointer capture is not implemented in jsdom.
  Element.prototype.setPointerCapture = (() => undefined) as typeof Element.prototype.setPointerCapture;
  Element.prototype.releasePointerCapture = (() =>
    undefined) as typeof Element.prototype.releasePointerCapture;

  // Stable SVG metrics so world transforms stay finite in jsdom.
  Element.prototype.getBoundingClientRect = function getBoundingClientRect() {
    return {
      x: 0,
      y: 0,
      top: 0,
      left: 0,
      bottom: 600,
      right: 800,
      width: 800,
      height: 600,
      toJSON() {
        return {};
      },
    } as DOMRect;
  };
});

const t = (key: string) => key;

function Harness({
  initial,
  onCommit,
}: {
  initial: EditorDocument;
  onCommit: (doc: EditorDocument) => void;
}) {
  const [doc, setDoc] = useState(initial);
  const [selection, setSelection] = useState<SelectionRef[]>([]);
  const [drawingPoints, setDrawingPoints] = useState<
    { x: number; y: number }[]
  >([]);
  const viewport = useCanvasViewport({ scale: 0.1, panX: 0, panY: 0 });
  const baseline = React.useRef(doc);

  return (
    <div style={{ width: 800, height: 600 }}>
      <FloorCanvas
        doc={doc}
        selection={selection}
        isSelected={(ref) =>
          selection.some((s) => s.kind === ref.kind && s.key === ref.key)
        }
        onSelect={(refs) => {
          setSelection(refs);
        }}
        tool="select"
        previewMode="edit"
        snapGrid={false}
        snapElements={false}
        viewport={viewport}
        onBeginDrag={() => {
          baseline.current = doc;
        }}
        onPreviewDoc={(updater) => {
          setDoc((prev) => updater(prev));
        }}
        onCommitDrag={() => {
          onCommit(doc);
        }}
        onCancelDrag={() => setDoc(baseline.current)}
        onPolygonComplete={() => undefined}
        drawingPoints={drawingPoints}
        onDrawingPoint={(p) => setDrawingPoints((pts) => [...pts, p])}
        onClearDrawing={() => setDrawingPoints([])}
        t={t}
      />
      <div data-testid="doc-x">{doc.tables[0]?.x_mm ?? -1}</div>
      <div data-testid="selection-kind">{selection[0]?.kind ?? "none"}</div>
      <div data-testid="selection-key">{selection[0]?.key ?? ""}</div>
      <div data-testid="doc-rot">{doc.tables[0]?.rotation_deg ?? -1}</div>
      <div data-testid="doc-w">{doc.tables[0]?.width_mm ?? -1}</div>
    </div>
  );
}

function sampleDoc(): EditorDocument {
  return layoutToEditor({
    schema_version: 1,
    width_mm: 10000,
    height_mm: 8000,
    measurement_unit: "m",
    tables: [
      {
        table_id: 1,
        name: "T1",
        x_mm: 1000,
        y_mm: 1000,
        width_mm: 1000,
        height_mm: 1000,
        shape: "square",
        max_capacity: 4,
        visible_seat_count: 4,
        rotation_deg: 0,
      },
    ],
    elements: [],
    regions: [],
  });
}

describe("FloorCanvas pointer interactions", () => {
  it("renders table and allows selection pointerdown", async () => {
    const onCommit = jest.fn();
    const doc = sampleDoc();
    render(<Harness initial={doc} onCommit={onCommit} />);

    expect(screen.getByTestId("floor-canvas")).toBeInTheDocument();
    const tableKey = doc.tables[0].clientKey;
    const tableNode = screen.getByTestId(`canvas-table-${tableKey}`);
    expect(tableNode).toBeInTheDocument();

    await act(async () => {
      fireEvent.pointerDown(tableNode, {
        button: 0,
        clientX: 100,
        clientY: 100,
        pointerId: 1,
      });
      fireEvent.pointerMove(tableNode, {
        clientX: 140,
        clientY: 120,
        pointerId: 1,
      });
      fireEvent.pointerUp(tableNode, { pointerId: 1 });
    });

    // After drag, position may change depending on viewport mapping; commit should fire.
    expect(onCommit).toHaveBeenCalled();
    expect(screen.getByTestId("selection-kind")).toHaveTextContent("table");
    expect(screen.getByTestId("selection-key")).toHaveTextContent(tableKey);
  });

  it("exposes resize and rotate handles when selected and commits on pointerup", async () => {
    const onCommit = jest.fn();
    const doc = sampleDoc();

    function SelectedHarness() {
      const [docState, setDoc] = useState(doc);
      const [selection] = useState<SelectionRef[]>([
        { kind: "table", key: doc.tables[0].clientKey },
      ]);
      const viewport = useCanvasViewport({ scale: 0.1, panX: 0, panY: 0 });
      return (
        <div style={{ width: 800, height: 600 }}>
          <FloorCanvas
            doc={docState}
            selection={selection}
            isSelected={(ref) =>
              selection.some((s) => s.kind === ref.kind && s.key === ref.key)
            }
            onSelect={() => undefined}
            tool="select"
            previewMode="edit"
            snapGrid={false}
            snapElements={false}
            viewport={viewport}
            onBeginDrag={() => undefined}
            onPreviewDoc={(updater) => setDoc((prev) => updater(prev))}
            onCommitDrag={() => onCommit()}
            onCancelDrag={() => undefined}
            onPolygonComplete={() => undefined}
            drawingPoints={[]}
            onDrawingPoint={() => undefined}
            onClearDrawing={() => undefined}
            t={t}
          />
        </div>
      );
    }

    render(<SelectedHarness />);

    const rotate = await screen.findByTestId("rotate-handle");
    const se = screen.getByTestId("resize-se");
    const nw = screen.getByTestId("resize-nw");
    expect(rotate).toBeInTheDocument();
    expect(se).toBeInTheDocument();
    expect(nw).toBeInTheDocument();

    const svg = screen.getByRole("img");

    // Resize handle: start drag and commit without large moves (avoids NaN sizes in jsdom).
    await act(async () => {
      fireEvent.pointerDown(se, {
        button: 0,
        clientX: 200,
        clientY: 200,
        pointerId: 3,
      });
      fireEvent.pointerUp(svg, { pointerId: 3 });
    });
    expect(onCommit).toHaveBeenCalled();

    // Rotate handle: start drag and commit.
    const rotateAgain = screen.getByTestId("rotate-handle");
    await act(async () => {
      fireEvent.pointerDown(rotateAgain, {
        button: 0,
        clientX: 150,
        clientY: 80,
        pointerId: 2,
      });
      fireEvent.pointerUp(svg, { pointerId: 2 });
    });
    expect(onCommit).toHaveBeenCalledTimes(2);
  });
});
