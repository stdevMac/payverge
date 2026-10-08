/** @jest-environment jsdom */
import { render, screen } from "@testing-library/react";
import ReviewsEmptyState from "../ReviewsEmptyState";

describe("ReviewsEmptyState", () => {
  test("renders centered with min-height and a brand-colored CTA", () => {
    const { container } = render(
      <ReviewsEmptyState
         
        primaryColor="#1a6b6a"
        radiusClass="rounded-lg"
        reviewLink="https://example.com/review"
        ctaLabel="Write a review"
        bodyText="Be the first to leave a review."
      />,
    );
    const root = container.firstElementChild as HTMLElement;
    expect(root.className).toMatch(/min-h-\[280px\]/);
    expect(root.className).toMatch(/flex flex-col items-center justify-center/);
    expect(screen.getByText(/Be the first/i)).toBeInTheDocument();
    const cta = screen.getByRole("link", { name: /Write a review/i });
    expect(cta).toBeInTheDocument();
  });

  test("renders body text only (no CTA) when reviewLink is omitted", () => {
    render(
      <ReviewsEmptyState
         
        primaryColor="#1a6b6a"
        radiusClass="rounded-lg"
        ctaLabel="Write a review"
        bodyText="No reviews yet."
      />,
    );
    expect(screen.getByText("No reviews yet.")).toBeInTheDocument();
    expect(screen.queryByRole("link")).toBeNull();
  });
});
