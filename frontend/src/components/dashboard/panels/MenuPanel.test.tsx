/** @jest-environment jsdom */
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import MenuPanel from "./MenuPanel";
import { analyticsApi } from "@/api/analytics";

jest.mock("@/api/analytics", () => ({ __esModule: true, analyticsApi: { getItemAnalytics: jest.fn() } }));
jest.mock("@/api/business", () => ({ getBusiness: jest.fn().mockResolvedValue({ default_currency: "USD" }) }));

describe("MenuPanel", () => {
  it("renders ranked top items (no doughnut) and the item table", async () => {
    (analyticsApi.getItemAnalytics as jest.Mock).mockResolvedValue([
      { item_id: "1", item_name: "Burger", category: "Mains", total_sold: 40, revenue: 400, bills_featured: 30, avg_price: 10, popularity_rank: 1 },
      { item_id: "2", item_name: "Fries", category: "Sides", total_sold: 25, revenue: 100, bills_featured: 20, avg_price: 4, popularity_rank: 2 },
    ]);
    render(<MenuPanel businessId="42" />);
    await waitFor(() => expect(screen.getByRole("list", { name: /top items/i })).toBeInTheDocument());
    // "Burger" appears in both the ranked list and the detail table — use getAllByText
    expect(screen.getAllByText("Burger").length).toBeGreaterThan(0);
    expect(screen.getByRole("list", { name: /categor/i })).toBeInTheDocument();
  });

  it("navigates to Menu Builder with the item name search (wave 4)", async () => {
    (analyticsApi.getItemAnalytics as jest.Mock).mockResolvedValue([
      { item_id: "1", item_name: "Burger", category: "Mains", total_sold: 40, revenue: 400, bills_featured: 30, avg_price: 10, popularity_rank: 1 },
    ]);
    const onNavigateToTab = jest.fn();
    render(
      <MenuPanel businessId="42" onNavigateToTab={onNavigateToTab} />,
    );
    const link = await screen.findByRole("button", {
      name: "View in Menu Builder",
    });
    fireEvent.click(link);
    expect(onNavigateToTab).toHaveBeenCalledWith("menu?menuSearch=Burger");
  });
});
