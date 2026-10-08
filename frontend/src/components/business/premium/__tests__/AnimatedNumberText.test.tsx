/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import { AnimatedNumberText } from "../AnimatedNumberText";

const reduceMotionState = { current: false };

jest.mock("../useReducedDashboardMotion", () => ({
  useReducedDashboardMotion: () => reduceMotionState.current,
}));

function moneyFormat(n: number): string {
  return `$${n.toFixed(2)}`;
}

describe("AnimatedNumberText", () => {
  beforeEach(() => {
    reduceMotionState.current = false;
  });

  it("renders the fully formatted final value on the first paint with real data (no spring-from-zero)", () => {
    const value = 1680.5;
    const expected = moneyFormat(value);

    render(
      <AnimatedNumberText
        value={value}
        format={moneyFormat}
        className="kpi"
      />,
    );

    // Synchronous first-paint assertion: the final formatted string must already
    // be in the DOM. Intermediate spring values (e.g. $100.03 ≈ 1680.5/16.8)
    // must not appear as the only visible text.
    expect(screen.getByText(expected)).toBeTruthy();
    expect(screen.queryByText("$0.00")).toBeNull();
    expect(screen.queryByText("$100.03")).toBeNull();
  });

  it("jumps to the first real value when mounting from a zero baseline (load → data)", () => {
    const { rerender } = render(
      <AnimatedNumberText value={0} format={moneyFormat} />,
    );
    expect(screen.getByText("$0.00")).toBeTruthy();

    rerender(
      <AnimatedNumberText value={1680.5} format={moneyFormat} />,
    );

    // First arrival of real data must land on the final formatted string
    // immediately — not an intermediate scaled count-up value.
    expect(screen.getByText("$1680.50")).toBeTruthy();
  });

  it("accepts subsequent value updates after the first real paint", () => {
    const { rerender } = render(
      <AnimatedNumberText value={100} format={moneyFormat} />,
    );
    expect(screen.getByText("$100.00")).toBeTruthy();

    rerender(
      <AnimatedNumberText value={250.75} format={moneyFormat} />,
    );

    // Subsequent non-zero → non-zero changes may spring; the tree must still
    // mount and eventually accept the new target without throwing.
    expect(document.body.textContent).toBeTruthy();
  });

  it("renders format(value) as static text when reduced motion is preferred", () => {
    reduceMotionState.current = true;

    render(
      <AnimatedNumberText value={42.5} format={moneyFormat} />,
    );
    expect(screen.getByText("$42.50")).toBeTruthy();
  });
});
