/** @jest-environment jsdom */
import React from "react";
import { render } from "@testing-library/react";

import SageMark from "../SageMark";

describe("SageMark", () => {
  it("renders a brand badge with the Sage sparkles glyph by default", () => {
    const { container } = render(<SageMark />);
    const badge = container.firstChild as HTMLElement;
    // Defaults: solid + md.
    expect(badge.className).toContain("bg-brand");
    expect(badge.className).toContain("w-10");
    expect(badge.className).toContain("rounded-xl");
    // Sparkles renders as an inline SVG (lucide), never a `Bot`/robot.
    // Assert the lucide *sparkles* class specifically so reintroducing `Bot`
    // (or any other glyph) fails this lock rather than passing on "an svg exists".
    expect(container.querySelector("svg")).toBeInTheDocument();
    expect(container.querySelector(".lucide-sparkles")).toBeTruthy();
    expect(container.querySelector(".lucide-bot")).toBeNull();
  });

  it('applies the soft brand/10 surface when variant="soft"', () => {
    const { container } = render(<SageMark variant="soft" size="sm" />);
    const badge = container.firstChild as HTMLElement;
    expect(badge.className).toContain("bg-brand/10");
    expect(badge.className).toContain("text-brand");
    expect(badge.className).toContain("w-8");
    expect(badge.className).toContain("rounded-lg");
  });
});
