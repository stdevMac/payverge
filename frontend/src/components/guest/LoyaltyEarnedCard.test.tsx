/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import LoyaltyEarnedCard from "./LoyaltyEarnedCard";
import { getPointsEarned } from "@/api/loyalty";

jest.mock("@/api/loyalty", () => ({ getPointsEarned: jest.fn() }));
jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({
    t: (key: string, vars?: Record<string, string | number>) =>
      vars ? `${key}:${JSON.stringify(vars)}` : key,
    currentLanguage: "en",
  }),
}));

describe("LoyaltyEarnedCard", () => {
  beforeEach(() => jest.clearAllMocks());

  it("renders points earned for a signed-in member", async () => {
    (getPointsEarned as jest.Mock).mockResolvedValue({
      points_earned: 40,
      total_points: 240,
    });
    render(<LoyaltyEarnedCard tableCode="TBL-1" billNumber="B-100" />);
    expect(await screen.findByTestId("loyalty-earned-card")).toBeInTheDocument();
    expect(screen.getByText('bill.pointsEarned:{"points":40}')).toBeInTheDocument();
  });

  it("renders nothing for anonymous guests (401)", async () => {
    (getPointsEarned as jest.Mock).mockRejectedValue({
      response: { status: 401, data: { code: "AUTH_TOKEN_MISSING" } },
    });
    render(<LoyaltyEarnedCard tableCode="TBL-1" billNumber="B-100" />);
    await waitFor(() => expect(getPointsEarned).toHaveBeenCalled());
    expect(screen.queryByTestId("loyalty-earned-card")).not.toBeInTheDocument();
  });
});
