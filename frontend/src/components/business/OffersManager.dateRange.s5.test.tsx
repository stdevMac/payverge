/** @jest-environment jsdom */
/**
 * L3-15 — offer end date must not use native min= HTML5 English bubbles;
 * date-range invalid is shown via localized isInvalid / message.
 *
 * OffersManager is large; this harness reuses the same dateRangeInvalid
 * predicate and asserts the shipped end-date field attributes via a thin
 * mirror of the fixed control contract (no min=, isInvalid when inverted).
 */
import React, { useState } from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { Input } from "@nextui-org/react";

/**
 * Same predicate as OffersManager (F27 / L3-15). Kept local so the test
 * documents the contract the manager enforces without mounting the full
 * menu/plugin tree.
 */
function dateRangeInvalid(start: string, end: string): boolean {
  return !!start && !!end && end < start;
}

function OfferDateRangeFields() {
  const [start, setStart] = useState("2026-08-10");
  const [end, setEnd] = useState("2026-08-01");
  const invalid = dateRangeInvalid(start, end);
  return (
    <div>
      <Input
        type="date"
        label="Start"
        value={start}
        onValueChange={setStart}
        data-testid="offer-start-date"
      />
      <Input
        type="date"
        label="End"
        value={end}
        // L3-15: deliberately NO min={start} — that was the Chrome English bubble.
        onValueChange={setEnd}
        isInvalid={invalid}
        errorMessage={invalid ? "The end date can't be before the start date." : undefined}
        data-testid="offer-end-date"
      />
      {invalid && (
        <p role="alert" data-testid="offer-date-range-error">
          The end date can&apos;t be before the start date.
        </p>
      )}
    </div>
  );
}

describe("L3-15 offer date-range localized validation contract", () => {
  it("shows localized range error without native min attribute on end date", () => {
    render(<OfferDateRangeFields />);
    const end = screen.getByTestId("offer-end-date");
    expect(end).not.toHaveAttribute("min");
    expect(screen.getByTestId("offer-date-range-error")).toBeInTheDocument();
    expect(
      screen.getAllByText("The end date can't be before the start date.").length,
    ).toBeGreaterThan(0);
  });

  it("clears error when end is after start", () => {
    render(<OfferDateRangeFields />);
    fireEvent.change(screen.getByTestId("offer-end-date"), {
      target: { value: "2026-08-20" },
    });
    expect(screen.queryByTestId("offer-date-range-error")).not.toBeInTheDocument();
  });
});

describe("L3-15 OffersManager source contract", () => {
  it("shipped OffersManager does not set min= on end date and uses dateRangeError", async () => {
    // Structural check against the real source so the harness above cannot
    // drift from production.
    const fs = await import("fs");
    const path = require("path") as typeof import("path");
    const src = fs.readFileSync(
      path.join(__dirname, "OffersManager.tsx"),
      "utf8",
    );
    // No native min coupling on end date (the L3-15 bug).
    expect(src).not.toMatch(/end_date[\s\S]{0,200}min=\{formData\.start_date/);
    expect(src).toMatch(/dateRangeInvalid/);
    expect(src).toMatch(/messages\.dateRangeError/);
    expect(src).toMatch(/data-testid="offer-end-date"/);
  });
});
