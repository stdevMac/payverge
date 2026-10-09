/** @jest-environment jsdom */

import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

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

jest.mock("../editor/SpaceEditorPage", () => ({
  __esModule: true,
  default: () => <div data-testid="space-editor-mock">editor</div>,
}));

const mockList = jest.fn();
const mockSummary = jest.fn();
const mockCreate = jest.fn();
const mockAssignTables = jest.fn();
const mockGetBusinessTables = jest.fn();

jest.mock("@/api/business", () => ({
  getBusinessTables: (...args: unknown[]) => mockGetBusinessTables(...args),
}));

jest.mock("@/api/spaces", () => {
  const actual = jest.requireActual("@/api/spaces");
  return {
    ...actual,
    spacesApi: {
      list: (...args: unknown[]) => mockList(...args),
      summary: (...args: unknown[]) => mockSummary(...args),
      create: (...args: unknown[]) => mockCreate(...args),
      assignTables: (...args: unknown[]) => mockAssignTables(...args),
      patch: jest.fn(),
      duplicate: jest.fn(),
      reorder: jest.fn(),
      archive: jest.fn(),
      remove: jest.fn(),
    },
  };
});

import SpacesOverview from "../SpacesOverview";

const emptySummary = {
  summary: {
    total_spaces: 0,
    draft_spaces: 0,
    published_spaces: 0,
    archived_spaces: 0,
    unassigned_tables: 0,
    assigned_tables: 0,
  },
  unassigned_tables: [],
};

describe("SpacesOverview", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockList.mockResolvedValue([]);
    mockSummary.mockResolvedValue(emptySummary);
    mockGetBusinessTables.mockResolvedValue({ tables: [] });
  });

  it("shows empty state with Scan and Draw CTAs", async () => {
    render(<SpacesOverview businessId={1} />);

    expect(await screen.findByTestId("spaces-empty")).toBeInTheDocument();
    expect(screen.getByTestId("spaces-empty-scan")).toBeInTheDocument();
    expect(screen.getByTestId("spaces-empty-draw")).toBeInTheDocument();
    expect(screen.getByText("spacesTables.empty.title")).toBeInTheDocument();
  });

  it("frames unassigned tables as waiting-to-place and offers place-existing", async () => {
    mockSummary.mockResolvedValue({
      summary: {
        total_spaces: 0,
        draft_spaces: 0,
        published_spaces: 0,
        archived_spaces: 0,
        unassigned_tables: 10,
        assigned_tables: 0,
      },
      unassigned_tables: Array.from({ length: 10 }, (_, i) => ({
        id: i + 1,
        business_id: 1,
        name: `Table ${i + 1}`,
        table_code: `T${i + 1}`,
        capacity: 4,
        is_active: true,
        space_id: null,
      })),
    });
    mockCreate.mockResolvedValue({
      id: 22,
      business_id: 1,
      name: "Main dining",
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
    });
    mockAssignTables.mockResolvedValue({ tables: [] });

    render(<SpacesOverview businessId={1} />);

    expect(await screen.findByTestId("spaces-aggregate-waiting")).toHaveTextContent(
      "10",
    );
    expect(screen.getByTestId("spaces-aggregate-seats")).toHaveTextContent("40");
    expect(screen.getByTestId("spaces-empty-place-existing")).toBeInTheDocument();

    fireEvent.click(screen.getByTestId("spaces-empty-place-existing"));
    await waitFor(() => {
      expect(mockCreate).toHaveBeenCalled();
      expect(mockAssignTables).toHaveBeenCalledWith(1, 22, {
        table_ids: [1, 2, 3, 4, 5, 6, 7, 8, 9, 10],
      });
    });
  });

  it("uses Live View tables when spaces summary reports none unassigned", async () => {
    mockGetBusinessTables.mockResolvedValue({
      tables: Array.from({ length: 10 }, (_, i) => ({
        id: i + 1,
        business_id: 1,
        name: `Table ${i + 1}`,
        table_code: `T${i + 1}`,
        capacity: 4,
        is_active: true,
      })),
    });
    mockCreate.mockResolvedValue({
      id: 22,
      business_id: 1,
      name: "Main dining",
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
    });
    mockAssignTables.mockResolvedValue({ tables: [] });

    render(<SpacesOverview businessId={1} />);

    expect(await screen.findByTestId("spaces-aggregate-waiting")).toHaveTextContent(
      "10",
    );
    expect(screen.getByTestId("spaces-aggregate-seats")).toHaveTextContent("40");
    expect(screen.getByTestId("spaces-empty-place-existing")).toBeInTheDocument();
    expect(screen.getByText("spacesTables.empty.existingTitle")).toBeInTheDocument();

    fireEvent.click(screen.getByTestId("spaces-empty-place-existing"));
    await waitFor(() => {
      expect(mockAssignTables).toHaveBeenCalledWith(1, 22, {
        table_ids: [1, 2, 3, 4, 5, 6, 7, 8, 9, 10],
      });
    });
  });

  it("accepts Live View tables from TableManager when the tables API fetch is empty", async () => {
    render(
      <SpacesOverview
        businessId={1}
        existingTables={Array.from({ length: 10 }, (_, i) => ({
          id: i + 1,
          business_id: 1,
          name: `Table ${i + 1}`,
          table_code: `T${i + 1}`,
          capacity: 4,
          is_active: true,
        }))}
      />,
    );

    expect(await screen.findByTestId("spaces-aggregate-waiting")).toHaveTextContent(
      "10",
    );
    expect(screen.getByTestId("spaces-aggregate-seats")).toHaveTextContent("40");
    expect(screen.getByTestId("spaces-empty-place-existing")).toBeInTheDocument();
  });

  it("runs create flow: open modal → details → path → create", async () => {
    mockCreate.mockResolvedValue({
      id: 11,
      business_id: 1,
      name: "Patio",
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
    });
    const onOpenSpace = jest.fn();

    render(<SpacesOverview businessId={1} onOpenSpace={onOpenSpace} />);
    await screen.findByTestId("spaces-empty");

    fireEvent.click(screen.getByTestId("spaces-create-button"));

    // NextUI Input: find by placeholder (label association is inconsistent in jsdom).
    const nameInput = await screen.findByPlaceholderText(
      "spacesTables.create.namePlaceholder",
    );
    fireEvent.change(nameInput, { target: { value: "Patio" } });

    fireEvent.click(screen.getByTestId("create-space-continue"));

    const draw = await screen.findByTestId("create-path-draw");
    fireEvent.click(draw);

    await waitFor(() => {
      expect(mockCreate).toHaveBeenCalledWith(
        1,
        expect.objectContaining({
          name: "Patio",
          space_type: "indoor",
          measurement_unit: "m",
        }),
      );
    });
    await waitFor(() => {
      expect(onOpenSpace).toHaveBeenCalledWith(11, "draw");
    });
  });

  it("renders space cards when list is non-empty", async () => {
    mockList.mockResolvedValue([
      {
        id: 3,
        business_id: 1,
        name: "Main Room",
        space_type: "indoor",
        floor_level: 0,
        sort_order: 0,
        measurement_unit: "m",
        status: "published",
        layout_schema_version: 1,
        draft_revision: 2,
        published_revision: 2,
        has_unpublished_changes: false,
        draft_layout_json: {
          schema_version: 1,
          width_mm: 10000,
          height_mm: 8000,
          tables: [
            {
              table_id: 1,
              x_mm: 100,
              y_mm: 100,
              width_mm: 1000,
              height_mm: 1000,
              shape: "round",
              max_capacity: 4,
            },
          ],
        },
        created_at: "2026-07-01T00:00:00Z",
        updated_at: "2026-07-01T00:00:00Z",
      },
    ]);
    mockSummary.mockResolvedValue({
      summary: {
        total_spaces: 1,
        draft_spaces: 0,
        published_spaces: 1,
        archived_spaces: 0,
        unassigned_tables: 2,
        assigned_tables: 1,
      },
      unassigned_tables: [],
    });

    render(<SpacesOverview businessId={1} />);

    expect(await screen.findByTestId("space-card-3")).toBeInTheDocument();
    expect(screen.getByText("Main Room")).toBeInTheDocument();
    expect(screen.queryByTestId("spaces-empty")).not.toBeInTheDocument();
  });

  it("card counts match assigned tables when layout JSON is empty (#725)", async () => {
    mockList.mockResolvedValue([
      {
        id: 7,
        business_id: 1,
        name: "Main dining",
        space_type: "indoor",
        floor_level: 0,
        sort_order: 0,
        measurement_unit: "m",
        status: "draft",
        layout_schema_version: 1,
        draft_revision: 1,
        published_revision: 0,
        has_unpublished_changes: false,
        draft_layout_json: {},
        published_layout_json: {},
        created_at: "2026-07-01T00:00:00Z",
        updated_at: "2026-07-01T00:00:00Z",
      },
    ]);
    mockSummary.mockResolvedValue({
      summary: {
        total_spaces: 1,
        draft_spaces: 1,
        published_spaces: 0,
        archived_spaces: 0,
        unassigned_tables: 0,
        assigned_tables: 10,
      },
      unassigned_tables: [],
    });
    mockGetBusinessTables.mockResolvedValue({
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

    render(<SpacesOverview businessId={1} />);

    const stats = await screen.findByTestId("space-card-stats-7");
    await waitFor(() => {
      expect(stats).toHaveAttribute("data-table-count", "10");
    });
    expect(stats).toHaveAttribute("data-seat-count", "40");
  });
});
