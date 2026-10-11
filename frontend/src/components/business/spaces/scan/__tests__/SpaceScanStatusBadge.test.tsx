/** @jest-environment jsdom */

import React from "react";
import { render, screen } from "@testing-library/react";
import SpaceScanStatusBadge from "../SpaceScanStatusBadge";
import { SCAN_STATUSES } from "../scanStatus";

describe("SpaceScanStatusBadge", () => {
  it.each(SCAN_STATUSES)("renders status %s", (status) => {
    render(
      <SpaceScanStatusBadge
        status={status}
        label={status}
        progressPct={status === "processing" ? 55 : 0}
      />,
    );
    const root = screen.getByTestId("space-scan-status");
    expect(root).toHaveAttribute("data-status", status);
    expect(screen.getByTestId("space-scan-status-badge")).toHaveTextContent(
      status,
    );
    if (status === "processing") {
      expect(screen.getByTestId("space-scan-progress")).toHaveTextContent("55%");
    }
  });
});
