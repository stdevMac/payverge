/** @jest-environment jsdom */
// src/components/dashboard/charts/RankedBarList.test.tsx
import { render, screen, within } from "@testing-library/react";
import { RankedBarList } from "./RankedBarList";

describe("RankedBarList", () => {
  const items = [
    { label: "Card", value: 30 },
    { label: "Crypto", value: 45 },
    { label: "Cash", value: 15 },
  ];

  it("sorts descending by value", () => {
    render(<RankedBarList items={items} ariaLabel="payment mix" />);
    const rows = screen.getAllByRole("listitem");
    expect(within(rows[0]).getByText("Crypto")).toBeInTheDocument();
    expect(within(rows[2]).getByText("Cash")).toBeInTheDocument();
  });

  it("scales bar widths against the max (zero baseline)", () => {
    render(<RankedBarList items={items} ariaLabel="payment mix" />);
    const bars = screen.getAllByTestId("ranked-bar-fill");
    expect(bars[0].style.width).toBe("100%");
    expect(parseFloat(bars[2].style.width)).toBeCloseTo(33.3, 0);
  });

  it("formats values with the provided formatter", () => {
    render(<RankedBarList items={items} ariaLabel="mix" formatValue={(n) => `${n} txns`} />);
    expect(screen.getByText("45 txns")).toBeInTheDocument();
  });

  it("renders a labeled empty state instead of a blank list when there are no items", () => {
    render(<RankedBarList items={[]} ariaLabel="payment mix" />);
    expect(screen.queryByRole("list")).not.toBeInTheDocument();
    const empty = screen.getByRole("status", { name: "payment mix" });
    expect(empty).toBeInTheDocument();
    // Locale-neutral em-dash by default (no untranslated English).
    expect(empty).toHaveTextContent("—");
  });

  it("uses a provided localized emptyLabel when empty", () => {
    render(<RankedBarList items={[]} ariaLabel="payment mix" emptyLabel="Sin datos" />);
    expect(screen.getByText("Sin datos")).toBeInTheDocument();
  });
});
