/** @jest-environment jsdom */
import { render, screen } from "@testing-library/react";
import type { LoyaltyTier } from "@/api/loyalty";
import TierPreview from "./TierPreview";

const tier = (
  name: string,
  threshold: number,
  sort_order: number,
): LoyaltyTier => ({
  name,
  min_lifetime_spent: threshold,
  sort_order,
});

it("does not render duplicate preview segments for duplicate tier names", () => {
  render(
    <TierPreview
      distribution={{ Bronze: 3, Silver: 3 }}
      total={6}
      tiers={[
        tier("Bronze", 0, 0),
        tier("Bronze", 0, 1),
        tier("Silver", 250, 2),
      ]}
    />,
  );

  expect(
    screen.getByText("3 Bronze · 3 Silver (of 6 customers)"),
  ).toBeInTheDocument();
  expect(screen.getAllByTitle("3 Bronze")).toHaveLength(1);
});
