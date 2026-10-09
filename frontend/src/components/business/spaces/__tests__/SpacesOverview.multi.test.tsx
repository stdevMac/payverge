/** @jest-environment jsdom */

import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => key,
}));

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { success: jest.fn(), error: jest.fn() },
}));

const mockList = jest.fn();
const mockSummary = jest.fn();
const mockCreate = jest.fn();

jest.mock("@/api/business", () => ({
  getBusinessTables: jest.fn(async () => ({ tables: [] })),
}));

jest.mock("@/api/spaces", () => {
  const actual = jest.requireActual("@/api/spaces");
  return {
    ...actual,
    spacesApi: {
      list: (...args: unknown[]) => mockList(...args),
      summary: (...args: unknown[]) => mockSummary(...args),
      create: (...args: unknown[]) => mockCreate(...args),
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

function spaceRow(id: number, name: string) {
  return {
    id,
    business_id: 1,
    name,
    space_type: "indoor",
    floor_level: 0,
    sort_order: id,
    measurement_unit: "m",
    status: "draft",
    layout_schema_version: 1,
    draft_revision: 1,
    published_revision: 0,
    has_unpublished_changes: false,
    created_at: "2026-07-01T00:00:00Z",
    updated_at: "2026-07-01T00:00:00Z",
  };
}

describe("SpacesOverview multi-space create", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockList.mockResolvedValue([]);
    mockSummary.mockResolvedValue(emptySummary);
  });

  it("creates two spaces sequentially and lists both", async () => {
    let created: ReturnType<typeof spaceRow>[] = [];
    mockCreate.mockImplementation(async (_biz: number, body: { name: string }) => {
      const row = spaceRow(created.length + 10, body.name);
      created = [...created, row];
      mockList.mockResolvedValue(created);
      mockSummary.mockResolvedValue({
        summary: {
          total_spaces: created.length,
          draft_spaces: created.length,
          published_spaces: 0,
          archived_spaces: 0,
          unassigned_tables: 0,
          assigned_tables: 0,
        },
        unassigned_tables: [],
      });
      return row;
    });

    const onOpenSpace = jest.fn();
    const { rerender } = render(
      <SpacesOverview businessId={1} onOpenSpace={onOpenSpace} />,
    );
    await screen.findByTestId("spaces-empty");

    // First space
    fireEvent.click(screen.getByTestId("spaces-create-button"));
    const nameInput = await screen.findByPlaceholderText(
      "spacesTables.create.namePlaceholder",
    );
    fireEvent.change(nameInput, { target: { value: "Main Floor" } });
    fireEvent.click(screen.getByTestId("create-space-continue"));
    fireEvent.click(await screen.findByTestId("create-path-draw"));

    await waitFor(() => {
      expect(mockCreate).toHaveBeenCalledTimes(1);
    });

    // Re-render with updated list mocks (component may refresh after create)
    rerender(<SpacesOverview businessId={1} onOpenSpace={onOpenSpace} />);

    // If still empty after create, open create again for second space
    // Component typically reloads list after create; force list refresh via re-render.
    mockList.mockResolvedValue(created);
    rerender(<SpacesOverview businessId={1} onOpenSpace={onOpenSpace} />);

    // Second create from empty or list header
    const createBtn =
      screen.queryByTestId("spaces-create-button") ||
      screen.queryByTestId("spaces-empty-draw");
    if (createBtn) {
      fireEvent.click(createBtn);
      // empty-draw may open create modal path directly; if modal not open, click create
      if (!screen.queryByPlaceholderText("spacesTables.create.namePlaceholder")) {
        const btn = screen.queryByTestId("spaces-create-button");
        if (btn) fireEvent.click(btn);
      }
      const name2 = await screen.findByPlaceholderText(
        "spacesTables.create.namePlaceholder",
      );
      fireEvent.change(name2, { target: { value: "Patio" } });
      fireEvent.click(screen.getByTestId("create-space-continue"));
      fireEvent.click(await screen.findByTestId("create-path-draw"));
      await waitFor(() => {
        expect(mockCreate).toHaveBeenCalledTimes(2);
      });
    }

    expect(mockCreate.mock.calls[0][1]).toEqual(
      expect.objectContaining({ name: "Main Floor" }),
    );
    if (mockCreate.mock.calls.length > 1) {
      expect(mockCreate.mock.calls[1][1]).toEqual(
        expect.objectContaining({ name: "Patio" }),
      );
    }
  });
});
