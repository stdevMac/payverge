/**
 * @jest-environment jsdom
 *
 * #725 — Start scan in the layout editor was a dinner-setup no-op. The
 * editor no longer shows that control; scan stays on the overview cards.
 */
import React from "react";
import { render, screen } from "@testing-library/react";

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

jest.mock("@/hooks/useSSEEvents", () => ({
  useSSEEvents: () => ({ degraded: false, blocked: false, reconnect: jest.fn() }),
}));

jest.mock("qrcode", () => ({
  __esModule: true,
  default: { toDataURL: jest.fn().mockResolvedValue("data:image/png;base64,qr") },
}));

const SPACE = {
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
  created_at: "2026-07-01T00:00:00Z",
  updated_at: "2026-07-01T00:00:00Z",
};

const mockCreateScanSession = jest.fn();

jest.mock("@/api/spaces", () => {
  const actual = jest.requireActual("@/api/spaces");
  return {
    ...actual,
    spacesApi: {
      list: jest.fn().mockResolvedValue([
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
          created_at: "2026-07-01T00:00:00Z",
          updated_at: "2026-07-01T00:00:00Z",
        },
      ]),
      summary: jest.fn().mockResolvedValue({
        summary: {
          total_spaces: 1,
          draft_spaces: 1,
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
      validateLayout: jest.fn().mockResolvedValue({ valid: true }),
      assignTables: jest.fn(),
      create: jest.fn(),
      patch: jest.fn(),
      duplicate: jest.fn(),
      reorder: jest.fn(),
      archive: jest.fn(),
      remove: jest.fn(),
      createScanSession: (...a: unknown[]) => mockCreateScanSession(...a),
      getScanSession: jest.fn(),
      cancelScanSession: jest.fn(),
      retryProcessScanSession: jest.fn(),
    },
  };
});

jest.mock("@/api/business", () => ({
  getBusinessTables: jest.fn().mockResolvedValue({ tables: [] }),
  createTableWithQR: jest.fn(),
}));

import SpacesOverview from "../SpacesOverview";

describe("Spaces editor Start scan (#725)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("hides Start scan when the real parent opens the layout editor", async () => {
    render(<SpacesOverview businessId={1} initialSpaceId={SPACE.id} />);

    await screen.findByTestId("space-editor-page", undefined, { timeout: 20000 });
    expect(screen.queryByTestId("editor-start-scan")).not.toBeInTheDocument();
    expect(screen.queryByText("scan.start")).not.toBeInTheDocument();
    expect(mockCreateScanSession).not.toHaveBeenCalled();
  }, 40000);
});
