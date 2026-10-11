/** @jest-environment node */

import {
  buildReviewApplyLayout,
  resolveReviewApplyMode,
} from "../reviewApplyLayout";
import type { LayoutDocument, LayoutTable } from "@/api/spaces";

const round = (partial: Partial<LayoutTable> & { name: string }): LayoutTable => ({
  table_id: 0,
  shape: "round",
  x_mm: 0,
  y_mm: 0,
  width_mm: 400,
  height_mm: 400,
  ...partial,
});

describe("reviewApplyLayout", () => {
  it("uses merge only when draft_apply_failed", () => {
    expect(resolveReviewApplyMode({ draftApplyFailed: true })).toBe("merge");
    expect(resolveReviewApplyMode({ draftApplyFailed: false })).toBe("replace");
  });

  it("replace drops rejected candidates (filtered review set)", () => {
    const existing: LayoutDocument = {
      schema_version: 1,
      width_mm: 8000,
      height_mm: 6000,
      tables: [
        round({ name: "A", client_key: "a", x_mm: 10, y_mm: 10 }),
        round({ name: "B", client_key: "b", x_mm: 100, y_mm: 100 }),
        round({ name: "C", client_key: "c", x_mm: 200, y_mm: 200 }),
      ],
    };
    const reviewLayout: LayoutDocument = {
      schema_version: 1,
      width_mm: 8000,
      height_mm: 6000,
      tables: existing.tables,
    };
    // Operator accepted only A.
    const accepted = [round({ name: "A", client_key: "a", x_mm: 10, y_mm: 10 })];
    const out = buildReviewApplyLayout({
      mode: "replace",
      existing,
      reviewLayout,
      acceptedTables: accepted,
    });
    expect(out.tables?.map((t) => t.name)).toEqual(["A"]);
    expect(out.tables).toHaveLength(1);
  });

  it("merge preserves operator tables/elements and appends new scan candidates", () => {
    const existing: LayoutDocument = {
      schema_version: 1,
      width_mm: 5000,
      height_mm: 4000,
      tables: [
        {
          table_id: 42,
          name: "OperatorT",
          shape: "rectangle",
          x_mm: 50,
          y_mm: 50,
          width_mm: 800,
          height_mm: 800,
        },
      ],
      elements: [
        { element_type: "wall", name: "W1", x_mm: 0, y_mm: 0, width_mm: 100, height_mm: 4000 },
      ],
    };
    const accepted = [
      round({ name: "ScanA", client_key: "s1", x_mm: 10, y_mm: 10 }),
    ];
    const reviewLayout: LayoutDocument = {
      schema_version: 1,
      width_mm: 8000,
      height_mm: 6000,
      tables: accepted,
    };
    const out = buildReviewApplyLayout({
      mode: "merge",
      existing,
      reviewLayout,
      acceptedTables: accepted,
    });
    expect(out.tables?.map((t) => t.name)).toEqual(["OperatorT", "ScanA"]);
    expect(out.elements?.[0]?.name).toBe("W1");
    expect(out.width_mm).toBe(8000);
  });

  it("merge does not duplicate existing client_key", () => {
    const existing: LayoutDocument = {
      schema_version: 1,
      tables: [round({ name: "ScanA", client_key: "s1", x_mm: 10, y_mm: 10 })],
    };
    const accepted = [
      round({ name: "ScanA", client_key: "s1", x_mm: 10, y_mm: 10 }),
      round({ name: "ScanB", client_key: "s2", x_mm: 20, y_mm: 20 }),
    ];
    const out = buildReviewApplyLayout({
      mode: "merge",
      existing,
      reviewLayout: { schema_version: 1, tables: accepted },
      acceptedTables: accepted,
    });
    expect(out.tables?.map((t) => t.client_key)).toEqual(["s1", "s2"]);
  });
});
