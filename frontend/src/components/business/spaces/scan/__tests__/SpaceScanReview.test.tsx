/** @jest-environment jsdom */

import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import SpaceScanReview from "../SpaceScanReview";

jest.mock("../../layoutPreviewSvg", () => ({
  LayoutPreviewSvg: () => <div data-testid="preview-mock" />,
}));

const t = (key: string, params?: Record<string, string | number>) => {
  if (!params) return key;
  let out = key;
  for (const [k, v] of Object.entries(params)) {
    out = out.replace(`{${k}}`, String(v));
  }
  return out;
};

describe("SpaceScanReview", () => {
  it("lists tables and applies accepted subset", () => {
    const onApply = jest.fn();
    const layout = {
      schema_version: 1,
      width_mm: 8000,
      height_mm: 6000,
      meta: { approximate: true, confidence: 0.4 },
      tables: [
        {
          table_id: 0,
          name: "A",
          x_mm: 0,
          y_mm: 0,
          width_mm: 1000,
          height_mm: 1000,
          shape: "rectangle" as const,
        },
        {
          table_id: 0,
          name: "B",
          x_mm: 2000,
          y_mm: 0,
          width_mm: 1000,
          height_mm: 1000,
          shape: "rectangle" as const,
        },
      ],
    };

    render(
      <SpaceScanReview
        layout={layout}
        t={t}
        onApply={onApply}
        onOpenEditor={jest.fn()}
      />,
    );

    expect(screen.getByTestId("space-scan-review")).toBeInTheDocument();
    expect(screen.getByTestId("space-scan-review-table-0")).toHaveAttribute(
      "data-low-confidence",
      "true",
    );
    // Uncheck first table via checkbox click
    const checkboxes = screen.getAllByRole("checkbox");
    fireEvent.click(checkboxes[0]);
    fireEvent.click(screen.getByTestId("space-scan-review-apply"));
    expect(onApply).toHaveBeenCalled();
    const accepted = onApply.mock.calls[0][0];
    expect(accepted).toHaveLength(1);
    expect(accepted[0].name).toBe("B");
  });
});
