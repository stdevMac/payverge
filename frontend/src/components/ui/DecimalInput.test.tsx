/** @jest-environment jsdom */
/**
 * DecimalInput + applyDecimalBlur — character-by-character typing must keep
 * the separator (L3-17 / L5-13 / L5-33). Full-string fireEvent.change alone is
 * insufficient: it passes on the broken numeric-state pattern.
 */
import React, { useState } from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import {
  DecimalInput,
  applyDecimalBlur,
} from "./DecimalInput";

describe("applyDecimalBlur", () => {
  it("parses locale decimals and returns null for garbage", () => {
    expect(applyDecimalBlur("12.50").value).toBe(12.5);
    expect(applyDecimalBlur("2,5").value).toBe(2.5);
    expect(applyDecimalBlur("abc").ok).toBe(false);
    expect(applyDecimalBlur("").ok).toBe(false);
  });

  it("clamps and step-snaps only at the blur boundary", () => {
    expect(applyDecimalBlur("125", { min: 0, max: 100 }).value).toBe(100);
    expect(applyDecimalBlur("8.4", { min: 0, max: 168, step: 0.5 }).value).toBe(
      8.5,
    );
    expect(applyDecimalBlur("-3", { min: 0 }).value).toBe(0);
  });
});

/** Controlled harness that mirrors production parents. */
function Harness({
  initial = "",
  min,
  max,
  step,
  maxFractionDigits,
  testId = "decimal-input",
}: {
  initial?: string;
  min?: number;
  max?: number;
  step?: number;
  maxFractionDigits?: number;
  testId?: string;
}) {
  const [text, setText] = useState(initial);
  const [parsed, setParsed] = useState<number | null>(null);
  return (
    <div>
      <DecimalInput
        aria-label="amount"
        value={text}
        onValueChange={setText}
        onParsedChange={setParsed}
        min={min}
        max={max}
        step={step}
        maxFractionDigits={maxFractionDigits}
        data-testid={testId}
      />
      <span data-testid="parsed">{parsed === null ? "null" : String(parsed)}</span>
      <span data-testid="raw">{text}</span>
    </div>
  );
}

/** Type one character at a time into a controlled input (the real bug path). */
function typeChars(el: HTMLElement, chars: string) {
  let acc = (el as HTMLInputElement).value || "";
  for (const ch of chars) {
    if (ch === "{backspace}") {
      acc = acc.slice(0, -1);
    } else {
      acc = acc + ch;
    }
    fireEvent.change(el, { target: { value: acc } });
  }
}

describe("DecimalInput character-by-character (L3-17 family)", () => {
  it("keeps the decimal separator while typing 12.50", () => {
    render(<Harness />);
    const input = screen.getByTestId("decimal-input");
    typeChars(input, "12.50");
    expect(screen.getByTestId("raw")).toHaveTextContent("12.50");
    expect((input as HTMLInputElement).value).toBe("12.50");
  });

  it("keeps a comma separator while typing 2,5 (es locale path)", () => {
    render(<Harness />);
    const input = screen.getByTestId("decimal-input");
    typeChars(input, "2,5");
    expect((input as HTMLInputElement).value).toBe("2,5");
    expect(screen.getByTestId("raw")).toHaveTextContent("2,5");
  });

  it("does not clamp on change — intermediate 125 stays until blur when max=100", () => {
    render(<Harness max={100} min={0} />);
    const input = screen.getByTestId("decimal-input");
    // Simulates typing 1.25 where after "1" "2" "5" without separator someone
    // restores; more importantly typing "1" "2" "5" with max clamp on change
    // would have become 100 mid-stream (LoyaltyTab regression).
    typeChars(input, "125");
    expect((input as HTMLInputElement).value).toBe("125");
    fireEvent.blur(input);
    expect(screen.getByTestId("parsed")).toHaveTextContent("100");
    // Integer clamp rewrites without forced fraction when source had no sep.
    expect((input as HTMLInputElement).value).toBe("100");
  });

  it("does not step-snap on change — 8. is reachable for step 0.5", () => {
    render(<Harness min={0} max={168} step={0.5} />);
    const input = screen.getByTestId("decimal-input");
    typeChars(input, "8.");
    expect((input as HTMLInputElement).value).toBe("8.");
    typeChars(input, "5");
    expect((input as HTMLInputElement).value).toBe("8.5");
    fireEvent.blur(input);
    expect(screen.getByTestId("parsed")).toHaveTextContent("8.5");
  });

  it("reports null for garbage on blur (not 0)", () => {
    render(<Harness />);
    const input = screen.getByTestId("decimal-input");
    typeChars(input, "abc");
    fireEvent.blur(input);
    expect(screen.getByTestId("parsed")).toHaveTextContent("null");
    expect((input as HTMLInputElement).value).toBe("abc");
  });

  it("allows intermediate empty and lone separator without wiping to 0", () => {
    render(<Harness initial="1" />);
    const input = screen.getByTestId("decimal-input");
    fireEvent.change(input, { target: { value: "" } });
    expect((input as HTMLInputElement).value).toBe("");
    fireEvent.change(input, { target: { value: "2" } });
    fireEvent.change(input, { target: { value: "2," } });
    expect((input as HTMLInputElement).value).toBe("2,");
  });
});
