/** @jest-environment jsdom */

import { render, screen } from "@testing-library/react";
import SectionHeader from "../SectionHeader";

describe("SectionHeader", () => {
  const baseSettings = {
     
    primary_color: "#1a6b6a",
    corner_radius: "medium",
  };

  it("renders the badge text without an animate-pulse dot", () => {
    const { container } = render(
      <SectionHeader title="What guests say" badge="Reviews" designSettings={baseSettings} />,
    );
    expect(screen.getByText("Reviews")).toBeInTheDocument();
    expect(container.querySelector(".animate-pulse")).toBeNull();
    expect(container.querySelector(".motion-safe\\:animate-pulse")).toBeNull();
  });

  it("does not wrap content in a backdrop-blur card", () => {
    const { container } = render(
      <SectionHeader title="Hello" designSettings={baseSettings} />,
    );
    expect(container.querySelector(".backdrop-blur-md")).toBeNull();
    expect(container.querySelector(".rounded-3xl.shadow-sm.border")).toBeNull();
  });

  it("defaults to centered alignment", () => {
    const { container } = render(
      <SectionHeader title="Hello" designSettings={baseSettings} />,
    );
    expect(container.innerHTML).toMatch(/\btext-center\b/);
    expect(container.innerHTML).not.toMatch(/\btext-start\b/);
    expect(container.innerHTML).not.toMatch(/\btext-left\b/);
  });

  it("uses logical start alignment via centered=false so RTL inherits correctly", () => {
    const { container } = render(
      <SectionHeader title="Hello" designSettings={baseSettings} centered={false} />,
    );
    const headline = container.querySelector("h2");
    expect(headline).not.toBeNull();
    const header = container.querySelector("header");
    expect(header?.className).toMatch(/\bitems-start\b/);
    expect(header?.className).toMatch(/\btext-start\b/);
    expect(header?.className).not.toMatch(/\btext-left\b/);
    expect(header?.className).not.toMatch(/\btext-center\b/);
  });

  it("renders subtitle when provided", () => {
    render(
      <SectionHeader
        title="Hello"
        subtitle="A subtitle goes here."
        designSettings={baseSettings}
      />,
    );
    expect(screen.getByText("A subtitle goes here.")).toBeInTheDocument();
  });

  it("defaults to h2 but can be overridden via the `as` prop", () => {
    const { container, rerender } = render(
      <SectionHeader title="default" designSettings={baseSettings} />,
    );
    expect(container.querySelector("h2")?.textContent).toBe("default");
    expect(container.querySelector("h1")).toBeNull();

    rerender(
      <SectionHeader title="overridden" designSettings={baseSettings} as="h1" />,
    );
    expect(container.querySelector("h1")?.textContent).toBe("overridden");
  });
});
