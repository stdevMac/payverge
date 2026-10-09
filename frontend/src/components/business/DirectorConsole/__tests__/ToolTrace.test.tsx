/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import ToolTrace from "../ToolTrace";

describe("ToolTrace", () => {
  it("renders one calm chip per tool call with its human label + summary", () => {
    render(
      <ToolTrace
        toolCalls={[
          { id: "1", name: "get_revenue_summary", human_label: "Reading revenue", args: {}, state: "running" },
          { id: "2", name: "get_menu_top_items", human_label: "Top items", args: { limit: 5 }, state: "done", summary: "5 items", duration_ms: 280 },
        ]}
      />,
    );
    expect(screen.getByText(/Reading revenue/)).toBeInTheDocument();
    expect(screen.getByText(/Top items/)).toBeInTheDocument();
    expect(screen.getByText(/5 items/)).toBeInTheDocument();
  });

  it("never surfaces engineering telemetry — no millisecond timings to the owner", () => {
    render(
      <ToolTrace
        toolCalls={[
          { id: "2", name: "get_menu_top_items", human_label: "Top items", args: { limit: 5 }, state: "done", summary: "5 items", duration_ms: 280 },
        ]}
      />,
    );
    // The owner sees "what Sage checked", never "280ms".
    expect(screen.queryByText(/280ms/)).not.toBeInTheDocument();
    // No "<number>ms" timing anywhere (the regex avoids matching words like "items").
    expect(screen.queryByText(/\d+\s*ms\b/)).not.toBeInTheDocument();
  });

  it("never dumps raw JSON args (pills are read-only status, not a debugger)", () => {
    render(
      <ToolTrace
        toolCalls={[
          { id: "1", name: "get_revenue_summary", human_label: "Reading revenue", args: { period: "week" }, state: "done", summary: "...", duration_ms: 120 },
        ]}
      />,
    );
    // The chips are not buttons, and no JSON drawer exists to open.
    expect(screen.queryByRole("button", { name: /reading revenue/i })).not.toBeInTheDocument();
    expect(screen.queryByText(/"period": "week"/)).not.toBeInTheDocument();
  });
});
