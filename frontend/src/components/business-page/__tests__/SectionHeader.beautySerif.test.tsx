/** @jest-environment jsdom */
import React from "react";
import { render } from "@testing-library/react";
import SectionHeader from "../SectionHeader";

describe("SectionHeader — crisp serif + no orphan divider (BEAUTY-1 / BEAUTY-7)", () => {
  it("the serif heading uses font-title with NO synthesized faux-bold weight", () => {
    const { getByRole } = render(
      <SectionHeader
        title="Our Menu"
        designSettings={{ primary_color: "#1a6b6a" } as any}
      />
    );
    // SectionHeader default `as` is h2.
    const heading = getByRole("heading", { name: /our menu/i });
    expect(heading.className).toContain("font-title");
    // DM Serif Display ships weight 400 only — semibold/bold = faux-bold smear.
    expect(heading.className).not.toContain("font-semibold");
    expect(heading.className).not.toContain("font-bold");
  });

  it("does not render the orphan brand divider rule under the title (BEAUTY-7)", () => {
    const { container } = render(
      <SectionHeader
        title="Our Menu"
        designSettings={{ primary_color: "#1a6b6a" } as any}
      />
    );
    // The orphan rule was `block h-px w-12 mt-1 rounded-full` — assert it is gone.
    expect(container.querySelector('span.h-px.w-12')).toBeNull();
  });
});
