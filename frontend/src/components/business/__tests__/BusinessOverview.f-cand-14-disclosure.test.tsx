/**
 * F-cand-14 — overview "more metrics" disclosure must expose button semantics
 * and aria-expanded (native <summary> often surfaces as DisclosureTriangle /
 * generic in AX trees with no aria-expanded).
 *
 * Source-structure gate: the overview-more-metrics control is a <button>
 * with aria-expanded, not a bare <summary>.
 *
 * Full BusinessOverview RTL mount is heavy; this freezes the shipped pattern.
 */

import fs from "fs";
import path from "path";

const SRC = path.resolve(
  __dirname,
  "../BusinessOverview.tsx",
);

describe("BusinessOverview — F-cand-14 more-metrics disclosure", () => {
  const src = fs.readFileSync(SRC, "utf8");

  it("renders overview-more-metrics as a button with aria-expanded, not summary-only", () => {
    expect(src).toMatch(/data-testid=["']overview-more-metrics["']/);
    // Must not use native details/summary for this control (AX tree hole).
    const block = src.slice(
      src.indexOf('data-testid="overview-more-metrics"') - 80,
      src.indexOf('data-testid="overview-more-metrics"') + 400,
    );
    expect(block).not.toMatch(/<details\b/);
    expect(block).not.toMatch(/<summary\b/);
    expect(block).toMatch(/<button\b/);
    expect(block).toMatch(/aria-expanded=/);
    expect(block).toMatch(/type=["']button["']/);
  });

  it("toggles label between additionalMetrics and additionalMetricsHide with a rotating chevron when extras collapse", () => {
    expect(src).toMatch(/additionalMetricsHide/);
    expect(src).toMatch(/ChevronDown/);
    expect(src).toMatch(/rotate-180/);
  });

  it("surfaces two-or-fewer extras inline instead of a nested disclosure (#70 / #229)", () => {
    expect(src).toMatch(/overview-more-metrics-inline/);
    expect(src).toMatch(/OVERVIEW_EXTRAS_DISCLOSURE_MIN/);
  });
});
