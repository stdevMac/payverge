/** @jest-environment jsdom */
/**
 * Metric primitive — no green zeros, no fake axes.
 *
 * Findings 14/15/18: $0.00 / 0% must never paint success-green; empty/all-zero
 * series must not fabricate an axis. Tone is derived from state + sign.
 */
import React from "react";
import { render, screen } from "@testing-library/react";
import { Metric, deriveMetricTone, isAllZeroSeries } from "../Metric";

describe("Metric", () => {
  describe("empty / zero honesty", () => {
    it('value={0} with state="empty" renders neutral (text-ink-500), never emerald/green', () => {
      const { container } = render(
        <Metric value={0} state="empty" format="money" data-testid="m" />,
      );
      const el = screen.getByTestId("m");
      expect(el.className).toContain("text-ink-500");
      expect(el.className).not.toMatch(/emerald|green/);
      expect(container.innerHTML).not.toMatch(/emerald|green/);
      // No proud $0.00 — empty shows a placeholder, not a success number.
      expect(el.textContent).not.toMatch(/\$0\.00/);
    });

    it("zero with state=ok is neutral, never success-green", () => {
      render(
        <Metric value={0} state="ok" format="money" data-testid="m" />,
      );
      const el = screen.getByTestId("m");
      expect(el.getAttribute("data-tone")).toBe("neutral");
      expect(el.className).not.toMatch(/emerald|green/);
      expect(el.className).toMatch(/text-ink-500|text-ink-900/);
    });
  });

  describe("insufficient / implausible", () => {
    it('state="insufficient" renders the reason string, not a number', () => {
      render(
        <Metric
          state="insufficient"
          reason="Not enough payroll data"
          value={0.12}
          format="percent"
          data-testid="m"
        />,
      );
      const el = screen.getByTestId("m");
      expect(el).toHaveTextContent("Not enough payroll data");
      expect(el.textContent).not.toMatch(/12%|0\.12|\$/);
    });

    it('state="implausible" surfaces the impossible value in rose, not success-green', () => {
      render(
        <Metric
          state="implausible"
          reason="Ratio exceeds 100%"
          value={3.29}
          format="percent"
          data-testid="m"
        />,
      );
      const el = screen.getByTestId("m");
      // Prefer the unclamped number; reason is for call-site banners.
      expect(el).toHaveTextContent("329%");
      expect(el.className).toContain("text-rose-700");
      expect(el.className).not.toMatch(/emerald|green/);
      expect(el.getAttribute("data-tone")).toBe("danger");
    });
  });

  describe("sign-derived tone", () => {
    it("negative money renders text-rose-700 (OverviewTab model)", () => {
      render(
        <Metric
          value={-42.5}
          state="ok"
          format={(n) =>
            n < 0
              ? `-$${Math.abs(n).toFixed(2)}`
              : `$${n.toFixed(2)}`
          }
          data-testid="m"
        />,
      );
      const el = screen.getByTestId("m");
      expect(el.className).toContain("text-rose-700");
      expect(el.getAttribute("data-tone")).toBe("danger");
      expect(el).toHaveTextContent("-$42.50");
    });

    it("positive ok value is not rose and not success-green by default", () => {
      render(
        <Metric
          value={1200}
          state="ok"
          format={(n) => `$${n.toFixed(2)}`}
          data-testid="m"
        />,
      );
      const el = screen.getByTestId("m");
      expect(el.getAttribute("data-tone")).toBe("accent");
      expect(el.className).not.toMatch(/emerald|green|rose/);
      expect(el).toHaveTextContent("$1200.00");
    });
  });

  describe("series / chart empty", () => {
    it("all-zero series renders empty state, not an axis", () => {
      const { container } = render(
        <Metric
          value={0}
          state="empty"
          format="money"
          series={[0, 0, 0, 0]}
          seriesEmptyLabel="No trend yet"
          data-testid="m"
        />,
      );
      // No SVG polyline / chart axis scaffolding for empty series.
      expect(container.querySelector("svg")).toBeNull();
      expect(container.querySelector('[data-testid="metric-series"]')).toBeNull();
      expect(screen.getByTestId("metric-series-empty")).toHaveTextContent(
        "No trend yet",
      );
    });

    it("non-zero series renders a sparkline region", () => {
      const { container } = render(
        <Metric
          value={100}
          state="ok"
          format="count"
          series={[10, 20, 15, 40]}
          data-testid="m"
        />,
      );
      expect(container.querySelector('[data-testid="metric-series"]')).not.toBeNull();
      expect(container.querySelector("svg")).not.toBeNull();
    });
  });

  describe("deriveMetricTone / isAllZeroSeries", () => {
    it("deriveMetricTone never returns success for zero or empty", () => {
      expect(deriveMetricTone("empty", 0)).toBe("neutral");
      expect(deriveMetricTone("ok", 0)).toBe("neutral");
      expect(deriveMetricTone("insufficient", null)).toBe("neutral");
      expect(deriveMetricTone("implausible", 3)).toBe("danger");
      expect(deriveMetricTone("ok", -1)).toBe("danger");
      expect(deriveMetricTone("ok", 10)).toBe("accent");
      // success is never auto-derived from a zero
      expect(deriveMetricTone("ok", 0)).not.toBe("success");
    });

    it("isAllZeroSeries detects empty and all-zero series", () => {
      expect(isAllZeroSeries([])).toBe(true);
      expect(isAllZeroSeries([0, 0, 0])).toBe(true);
      expect(isAllZeroSeries([0, null, 0])).toBe(true);
      expect(isAllZeroSeries([0, 1, 0])).toBe(false);
      expect(isAllZeroSeries([null, undefined])).toBe(true);
    });
  });
});
