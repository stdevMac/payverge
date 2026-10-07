/** @jest-environment jsdom */
// src/components/dashboard/charts/HourOfDayChart.test.tsx
import { render } from "@testing-library/react";

const mockBarSpy = jest.fn();
jest.mock("react-chartjs-2", () => ({
  Bar: (props: any) => {
    mockBarSpy(props);
    return <div data-testid="bar-chart" />;
  },
}));

import { HourOfDayChart } from "./HourOfDayChart";

describe("HourOfDayChart", () => {
  beforeEach(() => mockBarSpy.mockClear());

  it("labels bars by hour and uses a zero baseline", () => {
    render(
      <HourOfDayChart
        hours={[{ hour: 9, value: 100 }, { hour: 12, value: 400 }]}
        ariaLabel="revenue by hour"
      />,
    );
    const props = mockBarSpy.mock.calls[0][0];
    expect(props.data.labels).toEqual(["9:00", "12:00"]);
    expect(props.options.scales.y.beginAtZero).toBe(true);
  });

  it("renders a locale-neutral empty state (no untranslated 'no data') when there are no hours", () => {
    const { getByRole, queryByText } = render(
      <HourOfDayChart hours={[]} ariaLabel="revenue by hour" />,
    );
    expect(mockBarSpy).not.toHaveBeenCalled();
    expect(queryByText(/no data/i)).not.toBeInTheDocument();
    expect(getByRole("status", { name: "revenue by hour" })).toHaveTextContent("—");
  });

  it("uses a provided localized emptyLabel when empty", () => {
    const { getByText } = render(
      <HourOfDayChart hours={[]} ariaLabel="revenue by hour" emptyLabel="Sin datos" />,
    );
    expect(getByText("Sin datos")).toBeInTheDocument();
  });

  it("renders empty state when every hour value is zero (no fabricated axis)", () => {
    const { getByTestId } = render(
      <HourOfDayChart
        hours={[
          { hour: 9, value: 0 },
          { hour: 12, value: 0 },
        ]}
        ariaLabel="revenue by hour"
        emptyLabel="Quiet day"
      />,
    );
    expect(mockBarSpy).not.toHaveBeenCalled();
    expect(getByTestId("chart-empty")).toHaveTextContent("Quiet day");
  });
});
