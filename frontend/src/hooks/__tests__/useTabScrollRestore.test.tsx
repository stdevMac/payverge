/** @jest-environment jsdom */
/**
 * M2 — scroll retention: switching rails resets the dashboard scroller to the
 * top for fresh tabs, but coming BACK to a tab lands where the operator left
 * off. Positions live in a module-level map (in-memory only; sessionStorage
 * is for M6's form state), keyed by business scope + tab.
 */
import React, { useRef } from "react";
import { render, screen, act } from "@testing-library/react";
import { useTabScrollRestore } from "../useTabScrollRestore";

function ScrollHost({ tab, scope }: { tab: string; scope: string }) {
  const ref = useRef<HTMLDivElement>(null);
  useTabScrollRestore(ref, tab, scope);
  return <div ref={ref} data-testid="scroller" />;
}

/** jsdom has no Element.scrollTo — stub it to mutate scrollTop like a real one. */
function stubScrollTo(el: HTMLElement) {
  const calls: number[] = [];
  (el as any).scrollTo = (opts: { top: number }) => {
    calls.push(opts.top);
    el.scrollTop = opts.top;
  };
  return calls;
}

beforeEach(() => {
  jest.useFakeTimers();
});
afterEach(() => {
  jest.useRealTimers();
});

function flush() {
  act(() => {
    jest.runAllTimers();
  });
}

describe("useTabScrollRestore", () => {
  it("scrolls a fresh tab to the top", () => {
    render(<ScrollHost tab="overview" scope="t5-a" />);
    const el = screen.getByTestId("scroller");
    const calls = stubScrollTo(el);
    el.scrollTop = 500; // content arriving before the restore fires
    flush();
    expect(calls).toContain(0);
    expect(el.scrollTop).toBe(0);
  });

  it("restores the previous position when returning to a tab", () => {
    const { rerender } = render(<ScrollHost tab="overview" scope="t5-b" />);
    const el = screen.getByTestId("scroller");
    stubScrollTo(el);
    flush();

    el.scrollTop = 120; // operator scrolls the overview
    rerender(<ScrollHost tab="kitchen" scope="t5-b" />); // leaving saves 120
    flush();
    expect(el.scrollTop).toBe(0); // fresh tab starts at top

    el.scrollTop = 80; // operator scrolls the kitchen queue
    rerender(<ScrollHost tab="overview" scope="t5-b" />);
    flush();
    expect(el.scrollTop).toBe(120); // back where they left off
  });

  it("keeps positions isolated per business scope", () => {
    const { rerender } = render(<ScrollHost tab="menu" scope="t5-c-biz1" />);
    const el = screen.getByTestId("scroller");
    stubScrollTo(el);
    flush();
    el.scrollTop = 300;
    rerender(<ScrollHost tab="menu" scope="t5-c-biz2" />);
    flush();
    expect(el.scrollTop).toBe(0); // same tab, different business = fresh
  });

  it("saves the position on unmount", () => {
    const { unmount } = render(<ScrollHost tab="bills" scope="t5-d" />);
    const el = screen.getByTestId("scroller");
    stubScrollTo(el);
    flush();
    el.scrollTop = 77;
    unmount();

    render(<ScrollHost tab="bills" scope="t5-d" />);
    const el2 = screen.getByTestId("scroller");
    stubScrollTo(el2);
    flush();
    expect(el2.scrollTop).toBe(77);
  });

  it("restores instantly, overriding the scroller's scroll-smooth class", () => {
    render(<ScrollHost tab="settings" scope="t5-e" />);
    const el = screen.getByTestId("scroller");
    const calls: Array<ScrollBehavior | undefined> = [];
    (el as any).scrollTo = (opts: { top: number; behavior?: ScrollBehavior }) => {
      calls.push(opts.behavior);
      el.scrollTop = opts.top;
    };
    flush();
    expect(calls.length).toBeGreaterThan(0);
    expect(calls.every((b) => b === "instant")).toBe(true);
  });
});
