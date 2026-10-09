/**
 * D1 / L3-23: aggregate "Mesas colocadas" / totalTables must show assigned
 * tables only — not assigned + unassigned (the audit "ESPACIOS 0 / MESAS
 * COLOCADAS 10" inversion when unassigned were summed in).
 *
 * Pure placedTablesCount unit tests pass if SpacesOverview never calls it.
 * This mounts SpacesOverview with 3 assigned + 7 unassigned and asserts the
 * totalTables chip value is 3.
 *
 * Revert-proof: tables = assigned + unassigned → chip shows 10.
 *
 * @jest-environment jsdom
 */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";

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
      create: jest.fn(),
      patch: jest.fn(),
      duplicate: jest.fn(),
      reorder: jest.fn(),
      archive: jest.fn(),
      remove: jest.fn(),
    },
  };
});

import SpacesOverview from "../SpacesOverview";

describe("SpacesOverview L3-23 placed tables count (DOM)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    // One draft space so the aggregate strip renders (not empty-state only).
    mockList.mockResolvedValue([
      {
        id: 1,
        business_id: 1,
        name: "Main Floor",
        space_type: "indoor",
        floor_level: 0,
        sort_order: 0,
        measurement_unit: "m",
        status: "draft",
        layout_schema_version: 1,
        draft_revision: 1,
        published_revision: 0,
        has_unpublished_changes: false,
        draft_layout_json: null,
        published_layout_json: null,
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
        assigned_tables: 3,
        unassigned_tables: 7,
      },
      unassigned_tables: Array.from({ length: 7 }, (_, i) => ({
        id: 100 + i,
        name: `Unassigned ${i}`,
      })),
    });
  });

  it("totalTables chip is 3 (assigned), not 10 (assigned+unassigned)", async () => {
    render(<SpacesOverview businessId={1} />);

    const label = await screen.findByText("spacesTables.aggregate.totalTables");
    const panel = label.closest("div") || label.parentElement;
    // Value is the sibling/child number under the same premium panel.
    await waitFor(() => {
      const valueEl = panel?.querySelector(".tabular-nums") || panel;
      expect(valueEl?.textContent?.trim()).toBe("3");
    });

    // Unassigned has its own chip with 7 — prove we did not merge them.
    const unassignedLabel = screen.getByText(
      "spacesTables.aggregate.unassigned",
    );
    const unassignedPanel =
      unassignedLabel.closest("div") || unassignedLabel.parentElement;
    const unassignedVal =
      unassignedPanel?.querySelector(".tabular-nums") || unassignedPanel;
    expect(unassignedVal?.textContent?.trim()).toBe("7");
  });
});
