/** @jest-environment jsdom */
// src/components/dashboard/charts/TrendChart.test.tsx
import { render } from "@testing-library/react";

const mockLineSpy = jest.fn();
jest.mock("react-chartjs-2", () => ({
  Line: (props: any) => {
    mockLineSpy(props);
    return <div data-testid="line-chart" />;
  },
}));

import { TrendChart } from "./TrendChart";

describe("TrendChart", () => {
  beforeEach(() => mockLineSpy.mockClear());

  it("passes a single teal series with no curve and a zero baseline", () => {
    render(
      <TrendChart
        points={[{ date: "2026-05-01", value: 10 }, { date: "2026-05-02", value: 20 }]}
        ariaLabel="revenue trend"
        locale="en"
      />,
    );
    const props = mockLineSpy.mock.calls[0][0];
    expect(props.data.datasets).toHaveLength(1);
    // ISO bucket dates render as short localized day labels, not raw ISO.
    expect(props.data.labels).toEqual(["May 1", "May 2"]);
    expect(props.options.elements.line.tension).toBe(0);
    expect(props.options.scales.y.beginAtZero).toBe(true);
  });

  it("formats x-axis labels with the operator locale (L4-10 — no silent English default)", () => {
    render(
      <TrendChart
        points={[{ date: "2026-08-04", value: 3 }, { date: "2026-08-05", value: 5 }]}
        ariaLabel="conversations trend"
        locale="es"
      />,
    );
    const props = mockLineSpy.mock.calls[0][0];
    // Spanish short month — not English "Aug 4".
    const labels: string[] = props.data.labels;
    expect(labels.join(" ")).not.toMatch(/\bAug\b/);
    expect(labels[0]).toMatch(/4/);
    expect(labels[1]).toMatch(/5/);
  });

  it("renders a locale-neutral empty state (no untranslated 'no data') when there are no points", () => {
    const { getByRole, queryByText } = render(
      <TrendChart points={[]} ariaLabel="revenue trend" locale="en" />,
    );
    expect(mockLineSpy).not.toHaveBeenCalled();
    expect(queryByText(/no data/i)).not.toBeInTheDocument();
    expect(getByRole("status", { name: "revenue trend" })).toHaveTextContent("—");
  });

  it("uses a provided localized emptyLabel when empty", () => {
    const { getByText } = render(
      <TrendChart points={[]} ariaLabel="revenue trend" emptyLabel="Sin datos" locale="es" />,
    );
    expect(getByText("Sin datos")).toBeInTheDocument();
  });

  it("renders empty state (not an axis) when every point is zero", () => {
    const { getByTestId } = render(
      <TrendChart
        points={[
          { date: "2026-05-01", value: 0 },
          { date: "2026-05-02", value: 0 },
          { date: "2026-05-03", value: null },
        ]}
        ariaLabel="revenue trend"
        emptyLabel="No activity"
        locale="en"
      />,
    );
    expect(mockLineSpy).not.toHaveBeenCalled();
    expect(getByTestId("chart-empty")).toHaveTextContent("No activity");
  });
});
