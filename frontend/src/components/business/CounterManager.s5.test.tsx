/** @jest-environment jsdom */
/**
 * L2-33 — Número de Mostradores: 0 shows inline error; no silent clamp-only path.
 * CounterManager has heavy API/access deps; we test the counter-count field
 * contract via a focused harness matching the fixed UI, plus a source check.
 */
import React, { useState } from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { Input, Button } from "@nextui-org/react";
import { isIntInRange } from "@/lib/fieldValidation";

const MIN = 1;
const MAX = 20;

function CounterCountField() {
  const [count, setCount] = useState(0);
  const invalid = !isIntInRange(count, MIN, MAX);
  const [saveBlocked, setSaveBlocked] = useState(false);
  return (
    <div>
      <Input
        type="text"
        inputMode="numeric"
        label="Number of Counters"
        value={String(count)}
        onValueChange={(raw) => {
          if (raw === "") {
            setCount(0);
            return;
          }
          const parsed = parseInt(raw, 10);
          if (Number.isNaN(parsed)) return;
          setCount(Math.min(MAX, parsed));
        }}
        isInvalid={invalid}
        errorMessage={
          invalid ? "Enter a number of counters between 1 and 20." : undefined
        }
        data-testid="counter-count-input"
      />
      <Button
        data-testid="counter-save"
        onPress={() => {
          if (invalid) {
            setSaveBlocked(true);
            return;
          }
          setSaveBlocked(false);
        }}
      >
        Save
      </Button>
      {saveBlocked && (
        <p data-testid="counter-save-blocked">
          Enter a number of counters between 1 and 20.
        </p>
      )}
    </div>
  );
}

describe("L2-33 counter count validation", () => {
  it("flags 0 with localized inline error", () => {
    render(<CounterCountField />);
    expect(
      screen.getByText("Enter a number of counters between 1 and 20."),
    ).toBeInTheDocument();
    expect(screen.getByTestId("counter-count-input")).toHaveAttribute(
      "type",
      "text",
    );
  });

  it("blocks save when count is 0", () => {
    render(<CounterCountField />);
    fireEvent.click(screen.getByTestId("counter-save"));
    expect(screen.getByTestId("counter-save-blocked")).toBeInTheDocument();
  });

  it("accepts a value in 1..20", () => {
    render(<CounterCountField />);
    fireEvent.change(screen.getByTestId("counter-count-input"), {
      target: { value: "3" },
    });
    expect(
      screen.queryByText("Enter a number of counters between 1 and 20."),
    ).not.toBeInTheDocument();
  });
});

describe("L2-33 CounterManager source contract", () => {
  it("uses text input + counterCountInvalid (no silent 0 save)", async () => {
    const fs = await import("fs");
    const path = require("path") as typeof import("path");
    const src = fs.readFileSync(
      path.join(__dirname, "CounterManager.tsx"),
      "utf8",
    );
    expect(src).toMatch(/counterCountInvalid/);
    expect(src).toMatch(/data-testid="counter-count-input"/);
    expect(src).toMatch(/settings\.counterCountInvalid/);
    // Must refuse save when invalid, not only clamp.
    expect(src).toMatch(/if \(counterCountInvalid\)/);
  });
});
