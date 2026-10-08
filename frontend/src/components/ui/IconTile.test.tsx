/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import { ChefHat } from "lucide-react";
import IconTile from "./IconTile";

describe("IconTile", () => {
  it("renders the tinted md tile by default", () => {
    render(<IconTile icon={ChefHat} data-testid="tile" />);
    const tile = screen.getByTestId("tile");
    expect(tile.className).toContain("h-10 w-10");
    expect(tile.className).toContain("rounded-xl");
    expect(tile.className).toContain("bg-brand/10");
    expect(tile.className).toContain("text-brand");
  });

  it("renders the lg variant for empty/activation states", () => {
    render(<IconTile icon={ChefHat} size="lg" data-testid="tile" />);
    const tile = screen.getByTestId("tile");
    expect(tile.className).toContain("h-12 w-12");
    expect(tile.className).toContain("rounded-2xl");
  });

  it("hides the icon from the accessibility tree", () => {
    const { container } = render(<IconTile icon={ChefHat} />);
    expect(container.querySelector("svg")).toHaveAttribute(
      "aria-hidden",
      "true",
    );
  });

  it("appends caller className to the tile", () => {
    render(<IconTile icon={ChefHat} className="mt-4" data-testid="tile" />);
    expect(screen.getByTestId("tile").className).toContain("mt-4");
  });
});
