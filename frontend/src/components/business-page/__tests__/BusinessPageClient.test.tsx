/** @jest-environment jsdom */
import { render, screen } from "@testing-library/react";
import HeroSkeleton from "../HeroSkeleton";

describe("HeroSkeleton", () => {
  it("renders the skeleton hero shape with brand tokens", () => {
    const { container } = render(<HeroSkeleton />);
    expect(container.querySelector('[data-testid="hero-skeleton"]')).not.toBeNull();
    expect(container.innerHTML).toMatch(/bg-warm-100|bg-warm-200/);
    expect(container.innerHTML).not.toMatch(/bg-gray-9\d{2}/);
  });

  it("provides a screen-reader status announcement", () => {
    render(<HeroSkeleton />);
    expect(screen.getByRole("status", { name: /loading/i })).toBeInTheDocument();
  });
});
