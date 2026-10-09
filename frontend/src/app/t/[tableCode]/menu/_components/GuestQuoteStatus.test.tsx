/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import GuestQuoteStatus from "./GuestQuoteStatus";

describe("GuestQuoteStatus (#428)", () => {
  it("announces a dedicated updating-total status while the quote is pending", () => {
    render(
      <GuestQuoteStatus
        pending
        error={false}
        blocked={false}
        pendingLabel="Updating total…"
        errorLabel="Could not update total"
        blockedLabel="An item is unavailable"
        settledLabel="Total $12.00"
      />,
    );
    expect(screen.getByRole("status")).toHaveTextContent("Updating total…");
    expect(screen.queryByText(/Loading menu/i)).not.toBeInTheDocument();
  });

  it("announces the settled total once after pending completes", () => {
    const props = {
      error: false,
      blocked: false,
      pendingLabel: "Updating total…",
      errorLabel: "Could not update total",
      blockedLabel: "An item is unavailable",
      settledLabel: "Total $12.00",
    };
    const { rerender } = render(<GuestQuoteStatus {...props} pending />);
    expect(screen.getByRole("status")).toHaveTextContent("Updating total…");

    rerender(<GuestQuoteStatus {...props} pending={false} />);
    expect(screen.getByRole("status")).toHaveTextContent("Total $12.00");
  });

  it("uses a distinct error message when the quote fails", () => {
    const { rerender } = render(
      <GuestQuoteStatus
        pending
        error={false}
        blocked={false}
        pendingLabel="Updating total…"
        errorLabel="Could not update total"
        blockedLabel="An item is unavailable"
        settledLabel="Total $12.00"
      />,
    );
    rerender(
      <GuestQuoteStatus
        pending={false}
        error
        blocked={false}
        pendingLabel="Updating total…"
        errorLabel="Could not update total"
        blockedLabel="An item is unavailable"
        settledLabel="Total $12.00"
      />,
    );
    expect(screen.getByRole("status")).toHaveTextContent("Could not update total");
    expect(screen.queryByText("Total $12.00")).not.toBeInTheDocument();
  });
});
