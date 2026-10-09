/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import { LaborPreviewFooter, type LaborPreviewLabels } from "./LaborPreviewFooter";
import type { LaborPreview } from "@/api/schedule";

const labels: LaborPreviewLabels = {
  title: "Labor preview",
  scheduledHours: "Scheduled hours",
  laborCost: "Labor cost",
  laborCostPct: "Labor %",
  salesTarget: "Sales target",
  salesTargetPlaceholder: "Enter a sales target",
  salesTargetHint: "Defaults from last week's sales — edit anytime",
  hoursOnly: "Hours shown; pay is owner-only",
  warnings: {
    overtime: "Overtime: {name} is over {hours}h this week",
    postedLate:
      "Published late — the team got less than {days} days' notice. Tap to adjust lead time in Schedule settings.",
    minorLate: "A minor is scheduled past {time}",
  },
};

const financialPreview: LaborPreview = {
  schedule_id: 9,
  can_see_dollars: true,
  total_hours: 7.5,
  labor_cost: 150,
  sales_target: 1000,
  labor_cost_pct: 0.15,
  positions: [{ position_id: 3, shift_count: 1, hours: 7.5, labor_cost: 150 }],
  warnings: [],
};

const nonFinancialPreview: LaborPreview = {
  schedule_id: 9,
  can_see_dollars: false,
  total_hours: 7.5,
  positions: [{ position_id: 3, shift_count: 1, hours: 7.5 }],
  warnings: [],
};

describe("LaborPreviewFooter", () => {
  it("renders labor-% + hours + $ + target input for a financial caller", () => {
    render(
      <LaborPreviewFooter
        labels={labels}
        canViewFinancials
        currency="USD"
        locale="en"
        salesTarget={1000}
        onSalesTargetChange={() => {}}
        preview={financialPreview}
      />,
    );
    expect(screen.getByText("Scheduled hours")).toBeInTheDocument();
    expect(screen.getByText("Labor %")).toBeInTheDocument();
    expect(screen.getByText("Labor cost")).toBeInTheDocument();
    // labor-$ rendered (currency-aware), labor-% rendered.
    expect(document.body.textContent).toMatch(/\$\s*150/);
    expect(document.body.textContent).toMatch(/15(\.0)?%/);
    // editable sales-target input present, with the explainer under it.
    expect(screen.getByPlaceholderText("Enter a sales target")).toBeInTheDocument();
    expect(
      screen.getByText("Defaults from last week's sales — edit anytime"),
    ).toBeInTheDocument();
  });

  it("NEVER renders a dollar figure for a non-financial caller", () => {
    render(
      <LaborPreviewFooter
        labels={labels}
        canViewFinancials={false}
        salesTarget={undefined}
        onSalesTargetChange={() => {}}
        preview={nonFinancialPreview}
      />,
    );
    expect(screen.getByText("Scheduled hours")).toBeInTheDocument();
    // Labor-% is pay/sales, so it is financial too: no label, no percentage.
    expect(screen.queryByText("Labor %")).toBeNull();
    expect(document.body.textContent).not.toMatch(/\d%/);
    // money isolation in the DOM: no $ amount, no "Labor cost" label, no target input.
    expect(document.body.textContent).not.toMatch(/\$\s*\d/);
    expect(document.body.textContent).not.toMatch(/Labor cost/);
    expect(screen.queryByPlaceholderText("Enter a sales target")).toBeNull();
    expect(screen.getByText("Hours shown; pay is owner-only")).toBeInTheDocument();
  });

  it("hides $ even when can_see_dollars is true if the UI gate is off (defense-in-depth)", () => {
    render(
      <LaborPreviewFooter
        labels={labels}
        canViewFinancials={false}
        salesTarget={1000}
        onSalesTargetChange={() => {}}
        preview={financialPreview}
      />,
    );
    expect(document.body.textContent).not.toMatch(/\$\s*\d/);
    expect(document.body.textContent).not.toMatch(/Labor cost/);
  });

  it("renders a localized warning chip per code, naming the person", () => {
    render(
      <LaborPreviewFooter
        labels={labels}
        canViewFinancials
        currency="USD"
        locale="en"
        salesTarget={1000}
        onSalesTargetChange={() => {}}
        staffNames={new Map([[7, "Dana"]])}
        preview={{
          ...financialPreview,
          total_hours: 80,
          labor_cost: 2000,
          labor_cost_pct: 2,
          warnings: [
            { code: "overtime", staff_id: 7, detail: 2520 },
            { code: "overtime", staff_id: 9, detail: 2460 },
            { code: "posted_late", detail: 7 },
            { code: "minor_late", staff_id: 4, detail: 1290 },
          ],
        }}
      />,
    );
    // overtime: 2520 min → 42h; posted_late: 7d; minor_late: 1290 min → 21:30.
    expect(screen.getByText(/Overtime: Dana is over 42h/)).toBeInTheDocument();
    // Unknown staff ids fall back to #id rather than an empty name.
    expect(screen.getByText(/Overtime: #9 is over 41h/)).toBeInTheDocument();
    expect(
      screen.getByText(/Published late — the team got less than 7 days/),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/A minor is scheduled past 21:30/),
    ).toBeInTheDocument();
  });

  it("is not referenced by the staff dashboard shell (operator-only)", () => {
    const fs = require("fs");
    const path = require("path");
    const staffDir = path.join(__dirname, "../../staff");
    const hits: string[] = [];
    const walk = (d: string) =>
      fs.readdirSync(d, { withFileTypes: true }).forEach((e: { name: string; isDirectory: () => boolean }) => {
        const p = path.join(d, e.name);
        if (e.isDirectory()) walk(p);
        else if (/\.(t|j)sx?$/.test(e.name) && fs.readFileSync(p, "utf8").includes("LaborPreviewFooter"))
          hits.push(p);
      });
    if (fs.existsSync(staffDir)) walk(staffDir);
    expect(hits).toEqual([]);
  });
});

// Live-review regression: with no sales target there is no denominator —
// "Labor % 0.0%" reads as a computed zero. Render an em dash instead.
describe("LaborPreviewFooter — no sales target", () => {
  it("renders — for labor % when a financial caller has no target set", () => {
    render(
      <LaborPreviewFooter
        labels={labels}
        canViewFinancials
        currency="USD"
        locale="en"
        salesTarget={undefined}
        onSalesTargetChange={() => {}}
        preview={{ ...financialPreview, sales_target: 0, labor_cost_pct: 0 }}
      />,
    );
    expect(screen.getByText("Labor %")).toBeInTheDocument();
    expect(screen.getByText("—")).toBeInTheDocument();
    expect(document.body.textContent).not.toMatch(/0\.0%/);
  });

  // LABOR-PCT-ORACLE: the safe DTO no longer carries labor_cost_pct, so a
  // non-financial caller must never see a percentage (it would leak payroll
  // via pct × target).
  it("renders no labor pct for non-financial callers", () => {
    render(
      <LaborPreviewFooter
        labels={labels}
        canViewFinancials={false}
        onSalesTargetChange={() => {}}
        preview={nonFinancialPreview}
      />,
    );
    expect(screen.queryByText("Labor %")).not.toBeInTheDocument();
    expect(document.body.textContent).not.toMatch(/\d%/);
  });
});
