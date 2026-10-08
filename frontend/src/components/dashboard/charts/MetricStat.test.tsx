/** @jest-environment jsdom */
// src/components/dashboard/charts/MetricStat.test.tsx
import { render, screen } from "@testing-library/react";
import { MetricStat } from "./MetricStat";

describe("MetricStat", () => {
  it("shows label and value", () => {
    render(<MetricStat label="Revenue" value="$1,200" />);
    expect(screen.getByText("Revenue")).toBeInTheDocument();
    expect(screen.getByText("$1,200")).toBeInTheDocument();
  });

  it("renders an up delta with a glyph + sign (not color alone)", () => {
    render(<MetricStat label="Revenue" value="$1,200" delta={{ percent: 12.3 }} />);
    const delta = screen.getByText(/12\.3%/);
    expect(delta.textContent).toMatch(/▲/);
  });

  it("renders a down delta with a down glyph", () => {
    render(<MetricStat label="Tips" value="$80" delta={{ percent: -4 }} />);
    expect(screen.getByText(/4\.0%/).textContent).toMatch(/▼/);
  });

  it("omits the delta block when no delta is given", () => {
    const { container } = render(<MetricStat label="Bills" value={42} />);
    expect(container.querySelector('[data-testid="metric-delta"]')).toBeNull();
  });

  it("renders a subtext qualifier under the value when provided", () => {
    render(<MetricStat label="Tip rate" value="18.0%" subtext="Excellent" />);
    expect(screen.getByText("Excellent")).toBeInTheDocument();
  });

  it("omits the subtext block when no subtext is given", () => {
    const { container } = render(<MetricStat label="Bills" value={42} />);
    expect(container.querySelector('[data-testid="metric-subtext"]')).toBeNull();
  });

  it("renders zero delta as neutral em-dash (not green up)", () => {
    render(<MetricStat label="Revenue" value="$1,200" delta={{ percent: 0 }} />);
    const delta = screen.getByTestId("metric-delta");
    expect(delta.textContent).toMatch(/—/);
    expect(delta.textContent).not.toMatch(/▲/);
    expect(delta.textContent).not.toMatch(/▼/);
    expect(delta).toHaveClass("text-ink-500");
  });

});
