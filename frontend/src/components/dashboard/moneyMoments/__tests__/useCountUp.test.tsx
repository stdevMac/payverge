/** @jest-environment jsdom */
import { render, screen } from "@testing-library/react";
import { useCountUp } from "../useCountUp";

function Probe({ target, startFrom }: { target: number; startFrom?: number }) {
  const v = useCountUp(target, { startFrom });
  return <span data-testid="v">{Math.round(v)}</span>;
}

function mockReducedMotion(reduce: boolean) {
  (window as any).matchMedia = (query: string) => ({
    matches: reduce && query.includes("reduce"),
    media: query,
    addEventListener: () => {},
    removeEventListener: () => {},
    addListener: () => {},
    removeListener: () => {},
    dispatchEvent: () => false,
    onchange: null,
  });
}

describe("useCountUp", () => {
  it("snaps straight to the target under reduced motion", () => {
    mockReducedMotion(true);
    render(<Probe target={42} startFrom={0} />);
    expect(screen.getByTestId("v").textContent).toBe("42");
  });

  it("shows the target value with no mount animation when startFrom is omitted", () => {
    mockReducedMotion(false);
    render(<Probe target={17} />);
    expect(screen.getByTestId("v").textContent).toBe("17");
  });
});
