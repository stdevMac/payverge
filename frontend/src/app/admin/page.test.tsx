/** @jest-environment jsdom */
import { render, screen, waitFor } from "@testing-library/react";
import React from "react";
import AdminDashboard from "./page";
import { getAdminStats, type AdminStats } from "@/api/admin";

jest.mock("@/api/admin", () => ({
  getAdminStats: jest.fn(),
}));

jest.mock("@/components/admin/SystemHealthPanel", () => ({
  SystemHealthPanel: () => <div data-testid="system-health" />,
}));

jest.mock("@/components/dashboard/charts/AdminLineChart", () => ({
  AdminLineChart: () => <div data-testid="admin-line" />,
}));

jest.mock("@/components/dashboard/charts/AdminBarChart", () => ({
  AdminBarChart: () => <div data-testid="admin-bar" />,
}));

const stats: AdminStats = {
  total_businesses: 0,
  active_businesses: 0,
  inactive_businesses: 0,
  business_growth: [],
  total_users: 4,
  users_by_role: { admin: 1 },
  user_growth: [],
  total_payment_volume: 70_936,
  payment_volume_growth: [],
  average_transaction_size: 94.2,
  failed_webhooks_count: 0,
  gross_merchandise_volume: 68_000,
  revenue_growth: [],
  total_bills: 753,
  recognized_bills: 700,
  bills_by_status: { paid: 700 },
  bill_growth: [],
  recent_admin_actions: [],
  recent_errors: [],
};

describe("Admin dashboard KPI scope labels", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (getAdminStats as jest.Mock).mockResolvedValue(stats);
  });

  it("does not mix excludes-demo headcount with includes-demo dollars unlabeled", async () => {
    render(<AdminDashboard />);

    await waitFor(() => {
      expect(screen.getByText("$70,936")).toBeInTheDocument();
    });

    expect(screen.getByText("753")).toBeInTheDocument();

    const businessesCard = screen.getByText("Businesses").closest("a");
    expect(businessesCard).toHaveTextContent(/excludes demo/i);
    expect(businessesCard).not.toHaveTextContent(/includes demo/i);

    expect(screen.getAllByText(/includes demo/i).length).toBeGreaterThanOrEqual(
      2,
    );
    expect(screen.getAllByText(/lifetime/i).length).toBeGreaterThanOrEqual(2);
  });

  it("shows no SaaS billing KPIs (MRR, subscription revenue, churn, failed payments)", async () => {
    render(<AdminDashboard />);
    await waitFor(() => {
      expect(screen.getByText("$70,936")).toBeInTheDocument();
    });
    expect(screen.queryByText("MRR")).not.toBeInTheDocument();
    expect(screen.queryByText("Subscription Revenue")).not.toBeInTheDocument();
    expect(screen.queryByText("Churn")).not.toBeInTheDocument();
    expect(screen.queryByText("Failed Payments (30d)")).not.toBeInTheDocument();
    expect(screen.queryByText("Subscribers")).not.toBeInTheDocument();
    expect(screen.getByText("Failed Webhooks")).toBeInTheDocument();
  });

  it("formats volume in the venues' currency instead of dollars", async () => {
    (getAdminStats as jest.Mock).mockResolvedValue({
      ...stats,
      payment_volume_currency: "ARS",
      payment_volume_currencies: ["ARS"],
    });
    render(<AdminDashboard />);
    await waitFor(() => {
      expect(screen.getByText(/ARS\s70,936/)).toBeInTheDocument();
    });
    expect(screen.queryByText("$70,936")).not.toBeInTheDocument();
  });

  it("shows unconverted mixed-currency sums without a currency sign", async () => {
    (getAdminStats as jest.Mock).mockResolvedValue({
      ...stats,
      payment_volume_currency: "",
      payment_volume_currencies: ["ARS", "USD"],
    });
    render(<AdminDashboard />);
    await waitFor(() => {
      expect(screen.getByText("70,936")).toBeInTheDocument();
    });
    expect(screen.queryByText("$70,936")).not.toBeInTheDocument();
    expect(
      screen.getAllByText(/mixed currencies \(ARS, USD\), not converted/i).length,
    ).toBeGreaterThanOrEqual(1);
  });
  it("keeps a seven-digit average transaction compact with the exact figure in the title", async () => {
    (getAdminStats as jest.Mock).mockResolvedValue({
      ...stats,
      average_transaction_size: 12_345_678.9,
    });
    render(<AdminDashboard />);
    await waitFor(() => {
      expect(screen.getByText("Avg Transaction")).toBeInTheDocument();
    });
    const tile = screen.getByText("Avg Transaction").closest("[title]");
    expect(tile).toHaveAttribute("title", expect.stringContaining("12,345,678.90"));
    expect(screen.queryByText("$12,345,678.90")).not.toBeInTheDocument();
    expect(screen.getByText(/\$12\.3M/)).toBeInTheDocument();
  });
});
