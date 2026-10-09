/** @jest-environment jsdom */
// src/components/dashboard/charts/Sparkline.test.tsx
import { render } from "@testing-library/react";
import { Sparkline } from "./Sparkline";

describe("Sparkline", () => {
  it("renders an svg polyline for non-empty data", () => {
    const { container } = render(<Sparkline data={[1, 5, 2, 8, 3]} ariaLabel="trend" />);
    const poly = container.querySelector("polyline");
    expect(poly).toBeInTheDocument();
    expect(poly!.getAttribute("points")!.trim().split(/\s+/)).toHaveLength(5);
  });

  it("renders nothing meaningful for empty data (no fake data)", () => {
    const { container } = render(<Sparkline data={[]} ariaLabel="trend" />);
    expect(container.querySelector("polyline")).not.toBeInTheDocument();
  });

  it("exposes an accessible label", () => {
    const { getByRole } = render(<Sparkline data={[1, 2]} ariaLabel="revenue trend" />);
    expect(getByRole("img", { name: "revenue trend" })).toBeInTheDocument();
  });
});
