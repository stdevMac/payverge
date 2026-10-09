/** @jest-environment jsdom */
import { render, screen } from "@testing-library/react";
import MenuSkeleton from "../MenuSkeleton";

describe("MenuSkeleton", () => {
  it("renders the skeleton grid with brand tokens", () => {
    const { container } = render(<MenuSkeleton />);
    expect(container.querySelector('[data-testid="menu-skeleton"]')).not.toBeNull();
    expect(container.innerHTML).toMatch(/bg-warm-100|bg-warm-200/);
    expect(container.innerHTML).not.toMatch(/bg-gray-9\d{2}/);
  });

  it("provides a screen-reader status announcement", () => {
    render(<MenuSkeleton />);
    expect(screen.getByRole("status", { name: /loading/i })).toBeInTheDocument();
  });

  it("renders at least 6 placeholder cards", () => {
    const { container } = render(<MenuSkeleton />);
    const cards = container.querySelectorAll('[data-testid="menu-skeleton-card"]');
    expect(cards.length).toBeGreaterThanOrEqual(6);
  });
});
