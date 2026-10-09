import type { LayoutDocument, LayoutTable } from "@/api/spaces";

export type ReviewApplyMode = "merge" | "replace";

/**
 * Decide how desktop review apply should write the draft.
 * - merge: true operator-owned draft (auto-apply refused → draft_apply_failed)
 * - replace: empty or scan-owned draft (filtered review set; rejected candidates drop)
 */
export function resolveReviewApplyMode(opts: {
  draftApplyFailed: boolean;
}): ReviewApplyMode {
  return opts.draftApplyFailed ? "merge" : "replace";
}

/**
 * Build the layout document to persist after scan review.
 * Drives the shipped desktop apply path — tests must call this helper.
 */
export function buildReviewApplyLayout(opts: {
  mode: ReviewApplyMode;
  existing: LayoutDocument | null;
  reviewLayout: LayoutDocument;
  acceptedTables: LayoutTable[];
}): LayoutDocument {
  const { mode, existing, reviewLayout, acceptedTables } = opts;

  if (mode === "replace" || !existing) {
    return {
      ...reviewLayout,
      tables: acceptedTables,
    };
  }

  // Merge: keep operator tables/elements/regions; append accepted scan candidates
  // that are not already present (by client_key or name+shape+position).
  const existingKeys = new Set(
    (existing.tables ?? [])
      .map((tbl) => tbl.client_key)
      .filter((k): k is string => Boolean(k)),
  );
  const existingNames = new Set(
    (existing.tables ?? []).map(
      (tbl) =>
        `${(tbl.name || "").toLowerCase()}|${tbl.shape}|${tbl.x_mm}|${tbl.y_mm}`,
    ),
  );
  const toAdd = acceptedTables.filter((tbl) => {
    if (tbl.client_key && existingKeys.has(tbl.client_key)) return false;
    const fp = `${(tbl.name || "").toLowerCase()}|${tbl.shape}|${tbl.x_mm}|${tbl.y_mm}`;
    return !existingNames.has(fp);
  });

  return {
    ...existing,
    schema_version: existing.schema_version || reviewLayout.schema_version || 1,
    width_mm: Math.max(existing.width_mm || 0, reviewLayout.width_mm || 0),
    height_mm: Math.max(existing.height_mm || 0, reviewLayout.height_mm || 0),
    tables: [...(existing.tables ?? []), ...toAdd],
  };
}
