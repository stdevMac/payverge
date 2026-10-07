/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import HoursHistory from "./HoursHistory";
import { timeclockApi } from "@/api/timeclock";

jest.mock("@/api/timeclock");

const labels = {
  title: "Hours",
  subtitle: "Your recent shifts",
  loading: "Loading…",
  empty: "No hours yet",
  weekTotal: "Week total",
  hoursUnit: "h",
  statusPending: "Pending",
  statusApproved: "Approved",
};

function wrap(ui: React.ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

it("renders weekly worked hours and is money-free", async () => {
  (timeclockApi.listMineRange as jest.Mock).mockResolvedValue([
    { id: 1, clock_in_at: "2026-06-29T09:00:00Z", clock_out_at: "2026-06-29T17:00:00Z", break_minutes: 30, status: "approved", worked_minutes: 450, worked_hours: 7.5 },
  ]);
  const { container } = wrap(<HoursHistory businessId="1" labels={labels} locale="en" />);
  await waitFor(() => expect(screen.getAllByText(/7\.5/).length).toBeGreaterThan(0));
  expect(container.textContent || "").not.toMatch(/\$\d/);
});
