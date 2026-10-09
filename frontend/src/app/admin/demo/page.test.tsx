/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import AdminDemoPage from "./page";
import type { AdminDemoSummary } from "@/api/adminDemo";

const mockGet = jest.fn();
const mockEnsure = jest.fn();
const mockReset = jest.fn();
const mockAppend = jest.fn();
const mockVerify = jest.fn();

jest.mock("@/api/adminDemo", () => ({
  getAdminDemo: (...args: unknown[]) => mockGet(...args),
  ensureAdminDemo: (...args: unknown[]) => mockEnsure(...args),
  resetAdminDemo: (...args: unknown[]) => mockReset(...args),
  appendAdminDemoDay: (...args: unknown[]) => mockAppend(...args),
  verifyAdminDemo: (...args: unknown[]) => mockVerify(...args),
}));

const summary: AdminDemoSummary = {
  instance: {
    id: 1,
    admin_user_id: 5,
    primary_business_id: 10,
    secondary_business_id: 11,
    status: "ready",
    seed_version: "test",
    baseline_start_date: "2026-06-03T00:00:00Z",
    last_simulated_business_date: "2026-07-02T00:00:00Z",
    timezone: "America/New_York",
    created_at: "2026-07-02T00:00:00Z",
    updated_at: "2026-07-02T00:00:00Z",
  },
  businesses: [
    {
      id: 10,
      name: "Payverge Core Demo Kitchen",
      business_id: "demo-admin-5-core",
      is_demo: true,
    },
    {
      id: 11,
      name: "Payverge AI Pro Demo Lounge",
      business_id: "demo-admin-5-ai-pro",
      is_demo: true,
    },
  ],
  access: [
    {
      business_id: 10,
      business_name: "Payverge Core Demo Kitchen",
      role: "manager",
      name: "Maya Manager",
      email: "demo+admin5-business10-staff1@example.com",
      login_path: "/staff/login",
      pin_hint: "1234",
    },
  ],
  runs: [
    {
      id: 1,
      admin_user_id: 5,
      run_type: "ensure",
      status: "succeeded",
      started_at: "2026-07-02T12:00:00Z",
    },
  ],
  verification: {
    status: "passed",
    errors: [],
    coverage: [
      { key: "menu_images", status: "passed", count: 2 },
      { key: "bills", status: "passed", count: 300 },
      { key: "orders", status: "passed", count: 300 },
    ],
  },
  heartbeat: {
    status: "fresh",
    last_append_at: "2026-07-02T12:00:00Z",
    append_interval_seconds: 3600,
    stale_after_seconds: 7200,
  },
};

beforeEach(() => {
  jest.clearAllMocks();
  mockGet.mockResolvedValue(summary);
  mockEnsure.mockResolvedValue(summary);
  mockReset.mockResolvedValue(summary);
  mockAppend.mockResolvedValue(summary);
  mockVerify.mockResolvedValue(summary.verification);
});

describe("AdminDemoPage", () => {
  it("renders demo businesses, coverage, heartbeat, and recent runs", async () => {
    render(<AdminDemoPage />);

    expect(await screen.findByText("Demo Center")).toBeInTheDocument();
    expect(screen.getAllByText("Payverge Core Demo Kitchen").length).toBeGreaterThan(
      0,
    );
    expect(screen.getByText("Payverge AI Pro Demo Lounge")).toBeInTheDocument();
    expect(screen.getByText("Demo Staff Access")).toBeInTheDocument();
    expect(
      screen.getByText("demo+admin5-business10-staff1@example.com"),
    ).toBeInTheDocument();
    expect(screen.getByText("menu images")).toBeInTheDocument();
    expect(screen.getByText("ensure")).toBeInTheDocument();
    expect(screen.getByText("Append heartbeat")).toBeInTheDocument();
    expect(screen.getByText("fresh")).toBeInTheDocument();
  });

  it("surfaces a stale append heartbeat warning", async () => {
    mockGet.mockResolvedValue({
      ...summary,
      heartbeat: {
        status: "stale",
        last_append_at: "2026-06-01T12:00:00Z",
        append_interval_seconds: 3600,
        stale_after_seconds: 7200,
      },
    });
    render(<AdminDemoPage />);
    expect(await screen.findByText("stale")).toBeInTheDocument();
    expect(
      screen.getByText(/Hourly append is behind/i),
    ).toBeInTheDocument();
  });

  it("runs manual actions and refreshes the summary", async () => {
    render(<AdminDemoPage />);
    await screen.findByText("Demo Center");

    fireEvent.click(screen.getByRole("button", { name: /Append Day/i }));
    await waitFor(() => expect(mockAppend).toHaveBeenCalledTimes(1));

    fireEvent.click(screen.getByRole("button", { name: /Verify/i }));
    await waitFor(() => expect(mockVerify).toHaveBeenCalledTimes(1));

    fireEvent.click(screen.getByRole("button", { name: /Reset/i }));
    await waitFor(() => expect(mockReset).toHaveBeenCalledTimes(1));
  });
});
