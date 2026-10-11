/**
 * @jest-environment jsdom
 */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

beforeAll(() => {
  global.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver;

  Element.prototype.setPointerCapture = (() =>
    undefined) as typeof Element.prototype.setPointerCapture;
  Element.prototype.releasePointerCapture = (() =>
    undefined) as typeof Element.prototype.releasePointerCapture;
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

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (
    key: string,
    _locale?: string,
    params?: Record<string, string | number>,
  ) => {
    if (params) {
      let out = key;
      for (const [k, v] of Object.entries(params)) {
        out = out.replace(`{${k}}`, String(v));
      }
      return out;
    }
    return key;
  },
}));

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { success: jest.fn(), error: jest.fn() },
}));

const mockGet = jest.fn();
const mockGetDraft = jest.fn();
const mockPutDraft = jest.fn();
const mockPublish = jest.fn();
const mockDiscard = jest.fn();
const mockAssign = jest.fn();
const mockCreateTable = jest.fn();

jest.mock("@/api/spaces", () => {
  const actual = jest.requireActual("@/api/spaces");
  return {
    ...actual,
    spacesApi: {
      get: (...a: unknown[]) => mockGet(...a),
      getLayoutDraft: (...a: unknown[]) => mockGetDraft(...a),
      putLayoutDraft: (...a: unknown[]) => mockPutDraft(...a),
      publishLayout: (...a: unknown[]) => mockPublish(...a),
      discardLayout: (...a: unknown[]) => mockDiscard(...a),
      assignTables: (...a: unknown[]) => mockAssign(...a),
      validateLayout: jest.fn().mockResolvedValue({ valid: true }),
    },
  };
});

jest.mock("@/api/business", () => ({
  createTableWithQR: (...a: unknown[]) => mockCreateTable(...a),
}));

// Simplify ConfirmationModal for publish confirm
jest.mock("../../../modals/ConfirmationModal", () => ({
  __esModule: true,
  default: ({
    isOpen,
    title,
    onConfirm,
    confirmLabel,
  }: {
    isOpen: boolean;
    title: string;
    onConfirm: () => void;
    confirmLabel: string;
  }) =>
    isOpen ? (
      <div data-testid="confirm-modal">
        <span>{title}</span>
        <button type="button" onClick={onConfirm}>
          {confirmLabel}
        </button>
      </div>
    ) : null,
}));

import SpaceEditorPage from "../SpaceEditorPage";
import { EDITOR_CANVAS_MIN_HEIGHT_PX, propertiesCoversCanvas } from "../editorLayout";
import toast from "react-hot-toast";

const sampleSpace = {
  id: 7,
  business_id: 1,
  name: "Main Dining",
  space_type: "indoor",
  floor_level: 0,
  sort_order: 0,
  measurement_unit: "m" as const,
  status: "draft" as const,
  width_mm: 10000,
  height_mm: 8000,
  layout_schema_version: 1,
  draft_revision: 2,
  published_revision: 0,
  has_unpublished_changes: true,
  created_at: "2026-07-01T00:00:00Z",
  updated_at: "2026-07-01T00:00:00Z",
};

describe("SpaceEditorPage", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockGet.mockResolvedValue({ space: sampleSpace, tables: [] });
    mockGetDraft.mockResolvedValue({
      space_id: 7,
      draft_revision: 2,
      has_unpublished_changes: true,
      layout: {
        schema_version: 1,
        width_mm: 10000,
        height_mm: 8000,
        tables: [
          {
            table_id: 9,
            name: "T1",
            x_mm: 1000,
            y_mm: 1000,
            width_mm: 900,
            height_mm: 900,
            shape: "square",
            max_capacity: 4,
            visible_seat_count: 4,
          },
        ],
        elements: [],
        regions: [],
      },
      measurement_unit: "m",
      width_mm: 10000,
      height_mm: 8000,
    });
    mockPutDraft.mockResolvedValue({
      space: sampleSpace,
      draft_revision: 3,
      has_unpublished_changes: true,
      validation: { valid: true },
    });
    mockPublish.mockResolvedValue({
      space: { ...sampleSpace, status: "published", draft_revision: 3, published_revision: 3 },
      validation: { valid: true },
    });
  });

  it("loads draft layout into the editor shell", async () => {
    render(
      <SpaceEditorPage businessId={1} spaceId={7} onClose={jest.fn()} />,
    );
    expect(await screen.findByTestId("space-editor-page")).toBeInTheDocument();
    expect(screen.getByTestId("editor-toolbar")).toBeInTheDocument();
    expect(screen.getByTestId("floor-canvas")).toBeInTheDocument();
    expect(mockGet).toHaveBeenCalledWith(1, 7);
    expect(mockGetDraft).toHaveBeenCalledWith(1, 7);
    expect(mockPutDraft).not.toHaveBeenCalled();
  });

  it("opens publish confirmation and publishes on confirm", async () => {
    render(
      <SpaceEditorPage businessId={1} spaceId={7} onClose={jest.fn()} />,
    );
    await screen.findByTestId("space-editor-page");

    fireEvent.click(screen.getByTestId("editor-publish"));
    const modal = await screen.findByTestId("confirm-modal");
    expect(modal).toBeInTheDocument();

    fireEvent.click(modal.querySelector("button")!);

    await waitFor(() => {
      expect(mockPutDraft).toHaveBeenCalled();
      expect(mockPublish).toHaveBeenCalledWith(1, 7, {
        expected_revision: 3,
      });
    });
    expect(toast.success).toHaveBeenCalled();
  });

  it("clicking a canvas table fills Properties (#725)", async () => {
    render(
      <SpaceEditorPage businessId={1} spaceId={7} onClose={jest.fn()} />,
    );
    await screen.findByTestId("space-editor-page");

    expect(screen.queryByTestId("editor-properties-column")).not.toBeInTheDocument();
    expect(screen.getByTestId("editor-workspace")).toHaveAttribute(
      "data-layout",
      "canvas-hero",
    );
    expect(screen.getByTestId("editor-canvas-column")).toBeInTheDocument();

    const tableNode = document.querySelector(
      "[data-testid^='canvas-table-']",
    );
    expect(tableNode).toBeTruthy();
    fireEvent.pointerDown(tableNode as Element, {
      button: 0,
      clientX: 120,
      clientY: 120,
      pointerId: 1,
    });

    await waitFor(() => {
      expect(screen.getByTestId("properties-panel")).toHaveAttribute(
        "data-properties-state",
        "table",
      );
    });
    expect(screen.queryByTestId("properties-empty")).not.toBeInTheDocument();
    expect(screen.getByTestId("properties-panel")).toHaveAttribute(
      "data-selected-name",
      "T1",
    );
    expect(screen.getByTestId("editor-properties-column")).toHaveAttribute(
      "data-properties-placement",
      "beside",
    );
  });

  function stubEmptyLayoutWithAssignedTables() {
    mockGet.mockResolvedValue({
      space: sampleSpace,
      tables: Array.from({ length: 10 }, (_, i) => ({
        id: i + 1,
        business_id: 1,
        name: `Table ${i + 1}`,
        table_code: `T${i + 1}`,
        capacity: 4,
        is_active: true,
        space_id: 7,
      })),
    });
    mockGetDraft.mockResolvedValue({
      space_id: 7,
      draft_revision: 2,
      has_unpublished_changes: true,
      layout: {
        schema_version: 1,
        width_mm: 10000,
        height_mm: 8000,
        tables: [],
        elements: [],
        regions: [],
      },
      measurement_unit: "m",
      width_mm: 10000,
      height_mm: 8000,
    });
  }

  it("auto-places assigned tables on open without writing a draft", async () => {
    stubEmptyLayoutWithAssignedTables();

    render(
      <SpaceEditorPage businessId={1} spaceId={7} onClose={jest.fn()} />,
    );
    await screen.findByTestId("space-editor-page");

    await waitFor(() => {
      expect(document.querySelectorAll("[data-testid^='canvas-table-']")).toHaveLength(
        10,
      );
    });
    expect(mockPutDraft).not.toHaveBeenCalled();
  });

  it("persists auto-placed tables only on explicit Save", async () => {
    stubEmptyLayoutWithAssignedTables();

    render(
      <SpaceEditorPage businessId={1} spaceId={7} onClose={jest.fn()} />,
    );
    await screen.findByTestId("space-editor-page");

    await waitFor(() => {
      expect(document.querySelectorAll("[data-testid^='canvas-table-']")).toHaveLength(
        10,
      );
    });
    expect(mockPutDraft).not.toHaveBeenCalled();

    fireEvent.click(screen.getByTestId("tool-save"));

    await waitFor(() => {
      expect(mockPutDraft).toHaveBeenCalledTimes(1);
    });
    const putArg = mockPutDraft.mock.calls[0][2] as {
      expected_revision: number;
      layout: { tables?: unknown[] };
    };
    expect(putArg.expected_revision).toBe(2);
    expect(putArg.layout.tables).toHaveLength(10);
  });

  it("keeps the canvas the hero and opens Properties beside it (#725)", async () => {
    render(
      <SpaceEditorPage businessId={1} spaceId={7} onClose={jest.fn()} />,
    );
    await screen.findByTestId("space-editor-page");

    const workspace = screen.getByTestId("editor-workspace");
    const canvas = screen.getByTestId("editor-canvas-column");
    const toolbar = screen.getByTestId("editor-toolbar");
    const palette = screen.getByTestId("element-palette");

    expect(workspace.style.display).toBe("flex");
    expect(workspace.style.flexDirection).toBe("row");
    expect(canvas.style.flexGrow).toBe("1");
    expect(canvas.style.minHeight).toBe(`${EDITOR_CANVAS_MIN_HEIGHT_PX}px`);
    expect(toolbar.style.flexWrap).toBe("nowrap");
    expect(screen.queryByTestId("editor-properties-column")).not.toBeInTheDocument();
    expect(
      propertiesCoversCanvas(workspace, canvas, null),
    ).toBe(false);

    const tableNode = document.querySelector("[data-testid^='canvas-table-']");
    expect(tableNode).toBeTruthy();
    fireEvent.pointerDown(tableNode as Element, {
      button: 0,
      clientX: 120,
      clientY: 120,
      pointerId: 1,
    });

    const properties = await screen.findByTestId("editor-properties-column");
    expect(properties).toHaveAttribute("data-properties-placement", "beside");
    expect(canvas.contains(properties)).toBe(false);
    expect(properties.style.position).toBe("relative");
    expect(properties.style.flexGrow).toBe("0");
    expect(propertiesCoversCanvas(workspace, canvas, properties)).toBe(false);

    expect(palette).toHaveAttribute("data-palette-density", "compact");
    expect(screen.queryByText("editor.palette.room")).not.toBeInTheDocument();
    expect(screen.queryByText("editor.palette.tables")).not.toBeInTheDocument();
    expect(screen.queryByText("editor.palette.elements")).not.toBeInTheDocument();
  });

  it("does not show Start scan in the layout editor (#725)", async () => {
    render(<SpaceEditorPage businessId={1} spaceId={7} onClose={jest.fn()} />);
    await screen.findByTestId("space-editor-page");
    expect(screen.queryByTestId("editor-start-scan")).not.toBeInTheDocument();
    expect(screen.queryByText("scan.start")).not.toBeInTheDocument();
  });
});
