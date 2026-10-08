/**
 * @jest-environment jsdom
 *
 * Responsive smoke: editor shell and overview render under narrow viewports.
 */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";

beforeAll(() => {
  global.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver;
});

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => key,
}));

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { success: jest.fn(), error: jest.fn() },
}));

jest.mock("@/api/spaces", () => {
  const actual = jest.requireActual("@/api/spaces");
  return {
    ...actual,
    spacesApi: {
      list: jest.fn().mockResolvedValue([]),
      summary: jest.fn().mockResolvedValue({
        summary: {
          total_spaces: 0,
          draft_spaces: 0,
          published_spaces: 0,
          archived_spaces: 0,
          unassigned_tables: 0,
          assigned_tables: 0,
        },
        unassigned_tables: [],
      }),
      get: jest.fn().mockResolvedValue({
        space: {
          id: 7,
          business_id: 1,
          name: "Main",
          space_type: "indoor",
          floor_level: 0,
          sort_order: 0,
          measurement_unit: "m",
          status: "draft",
          layout_schema_version: 1,
          draft_revision: 1,
          published_revision: 0,
          has_unpublished_changes: false,
          created_at: "2026-07-01T00:00:00Z",
          updated_at: "2026-07-01T00:00:00Z",
        },
        tables: [],
      }),
      getLayoutDraft: jest.fn().mockResolvedValue({
        space_id: 7,
        draft_revision: 1,
        has_unpublished_changes: false,
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
      }),
      putLayoutDraft: jest.fn(),
      publishLayout: jest.fn(),
      discardLayout: jest.fn(),
      assignTables: jest.fn(),
      validateLayout: jest.fn().mockResolvedValue({ valid: true }),
      create: jest.fn(),
      patch: jest.fn(),
      duplicate: jest.fn(),
      reorder: jest.fn(),
      archive: jest.fn(),
      remove: jest.fn(),
    },
  };
});

jest.mock("@/api/business", () => ({
  createTableWithQR: jest.fn(),
  getBusinessTables: jest.fn(async () => ({ tables: [] })),
}));

jest.mock("../../../modals/ConfirmationModal", () => ({
  __esModule: true,
  default: () => null,
}));

import SpacesOverview from "../../SpacesOverview";
import SpaceEditorPage from "../SpaceEditorPage";

function setViewport(width: number, height: number) {
  Object.defineProperty(window, "innerWidth", {
    configurable: true,
    value: width,
  });
  Object.defineProperty(window, "innerHeight", {
    configurable: true,
    value: height,
  });
  window.dispatchEvent(new Event("resize"));
}

describe("Spaces responsive smoke", () => {
  afterEach(() => {
    setViewport(1024, 768);
  });

  it("renders empty overview at mobile 390px", async () => {
    setViewport(390, 844);
    render(<SpacesOverview businessId={1} />);
    expect(await screen.findByTestId("spaces-empty")).toBeInTheDocument();
    expect(screen.getByTestId("spaces-empty-scan")).toBeInTheDocument();
    expect(screen.getByTestId("spaces-empty-draw")).toBeInTheDocument();
  });

  it("renders editor shell at tablet 768px", async () => {
    setViewport(768, 1024);
    render(
      <SpaceEditorPage businessId={1} spaceId={7} onClose={jest.fn()} />,
    );
    await waitFor(() => {
      expect(screen.getByTestId("space-editor-page")).toBeInTheDocument();
    });
    expect(screen.getByTestId("editor-toolbar")).toBeInTheDocument();
    expect(screen.getByTestId("floor-canvas")).toBeInTheDocument();
    expect(screen.getByTestId("editor-workspace")).toHaveAttribute(
      "data-layout",
      "canvas-hero",
    );
    expect(screen.getByTestId("element-palette")).toHaveAttribute(
      "data-palette-density",
      "compact",
    );
    expect(screen.queryByTestId("editor-properties-column")).not.toBeInTheDocument();
    expect(screen.queryByTestId("editor-start-scan")).not.toBeInTheDocument();
    const workspace = screen.getByTestId("editor-workspace");
    const canvas = screen.getByTestId("editor-canvas-column");
    expect(workspace.style.display).toBe("flex");
    expect(workspace.style.flexDirection).toBe("row");
    expect(canvas.style.flexGrow).toBe("1");
  });
});
