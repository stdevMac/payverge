// src/components/dashboard/charts/analyticsTheme.test.ts
import {
  ANALYTICS_TEAL, ANALYTICS_BAR_HUE, DELTA_UP, DELTA_DOWN,
  chartBaseOptions, trendLineOptions,
} from "./analyticsTheme";

describe("analyticsTheme", () => {
  it("anchors on brand teal, not the chart.js rainbow", () => {
    expect(ANALYTICS_TEAL).toBe("#1a6b6a");
    expect(ANALYTICS_BAR_HUE).toBe("#1a6b6a");
  });

  it("uses distinct restrained delta colors", () => {
    expect(DELTA_UP).not.toBe(DELTA_DOWN);
  });

  it("strips chartjunk in the base options", () => {
    expect(chartBaseOptions.plugins?.legend?.display).toBe(false);
    expect(chartBaseOptions.maintainAspectRatio).toBe(false);
  });

  it("trend lines start at zero with no curve", () => {
    const opts = trendLineOptions((n) => `$${n}`);
    // `scales.y` is a union of chart.js scale types; only the linear scale
    // carries `beginAtZero`, so narrow before asserting (jest skips type
    // checks, but `tsc --noEmit` would otherwise flag the union access).
    const yScale = opts.scales?.y as { beginAtZero?: boolean } | undefined;
    expect(yScale?.beginAtZero).toBe(true);
    expect(opts.elements?.line?.tension).toBe(0);
  });
});
