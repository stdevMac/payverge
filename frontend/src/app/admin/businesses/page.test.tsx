/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import AdminBusinessesPage from "./page";
import type { AdminBusinessDetail } from "@/api/adminBusiness";

const mockGetList = jest.fn();
const mockGetDetail = jest.fn();

jest.mock("@/api/adminBusiness", () => ({
  adminBusinessAPI: {
    getList: (...args: unknown[]) => mockGetList(...args),
    getDetail: (...args: unknown[]) => mockGetDetail(...args),
    suspend: jest.fn(),
    reactivate: jest.fn(),
  },
}));

function makeDetail(
  lifecycle: { is_active: boolean; closed_at: string | null } = {
    is_active: true,
    closed_at: null,
  },
): AdminBusinessDetail {
  const status = lifecycle.closed_at
    ? "closed"
    : lifecycle.is_active
      ? "active"
      : "suspended";
  return {
    business: {
      id: 7,
      name: "Test Cafe",
      address: "123 Main",
      slug: "test-cafe",
      status,
      is_active: lifecycle.is_active,
      closed_at: lifecycle.closed_at,
      created_at: "2025-01-01T00:00:00Z",
      has_settlement_address: true,
      has_tipping_address: false,
    },
    owner: {
      id: 42,
      email_domain: "x.com",
      created_at: "2024-06-01T00:00:00Z",
    },
    staff: [
      { id: 3, name: "Ada", email_domain: "floor.example", role: "server" },
    ],
    recent_activity: {
      recent_orders: 0,
      recent_payments: 0,
      // @ts-expect-error branded type not needed for test
      total_revenue: 0,
      // @ts-expect-error branded type not needed for test
      total_tips: 0,
    },
    admin_actions: [],
  };
}

beforeEach(() => {
  jest.clearAllMocks();
  mockGetList.mockResolvedValue({
    businesses: [
      {
        id: 7,
        name: "Test Cafe",
        owner_name: "Owner",
        owner_email: "owner@x.com",
        status: "active",
        kind: "restaurant",
        created_at: "2025-01-01T00:00:00Z",
        last_active_at: null,
      },
    ],
    total: 1,
  });
});

describe("AdminBusinessesPage — search debounce", () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });
  afterEach(() => {
    jest.runOnlyPendingTimers();
    jest.useRealTimers();
  });

  it("debounces typing into the search box (one trailing fetch, not one per keystroke)", async () => {
    mockGetDetail.mockResolvedValue(makeDetail());
    render(<AdminBusinessesPage />);

    // Initial load fires once.
    await waitFor(() => expect(mockGetList).toHaveBeenCalledTimes(1));

    const input = screen.getByPlaceholderText(/Search by name/i);
    mockGetList.mockClear();

    fireEvent.change(input, { target: { value: "c" } });
    fireEvent.change(input, { target: { value: "ca" } });
    fireEvent.change(input, { target: { value: "caf" } });
    fireEvent.change(input, { target: { value: "cafe" } });

    // No fetch yet — debounce window hasn't elapsed.
    expect(mockGetList).not.toHaveBeenCalled();

    jest.advanceTimersByTime(350);

    await waitFor(() => expect(mockGetList).toHaveBeenCalledTimes(1));
    expect(mockGetList).toHaveBeenCalledWith(
      expect.objectContaining({ search: "cafe", page: 1 }),
    );
  });
});

describe("AdminBusinessesPage — admin lifecycle", () => {
  async function openAdminActions(detail: AdminBusinessDetail) {
    mockGetDetail.mockResolvedValue(detail);
    render(<AdminBusinessesPage />);
    fireEvent.click(await screen.findByText("Test Cafe"));
    fireEvent.click(await screen.findByText("Admin Actions"));
  }

  it("has no subscription tab or plan filter", async () => {
    mockGetDetail.mockResolvedValue(makeDetail());
    render(<AdminBusinessesPage />);
    fireEvent.click(await screen.findByText("Test Cafe"));
    await screen.findByText("Admin Actions");
    expect(screen.queryByText("Subscription")).not.toBeInTheDocument();
    expect(screen.queryByText(/All plans/i)).not.toBeInTheDocument();
  });

  it("offers suspend for an active venue", async () => {
    await openAdminActions(makeDetail());
    expect(await screen.findByText("Suspend Business")).toBeInTheDocument();
    expect(screen.queryByText("Reactivate Business")).not.toBeInTheDocument();
  });

  it("offers reactivate when the venue is suspended (is_active=false)", async () => {
    await openAdminActions(makeDetail({ is_active: false, closed_at: null }));
    expect(await screen.findByText("Reactivate Business")).toBeInTheDocument();
    expect(screen.queryByText("Suspend Business")).not.toBeInTheDocument();
  });

  it("shows the owner domain and wallet flags without a full email", async () => {
    mockGetDetail.mockResolvedValue(makeDetail());
    render(<AdminBusinessesPage />);
    fireEvent.click(await screen.findByText("Test Cafe"));
    const dialog = await screen.findByRole("dialog");

    // NextUI Tabs mount the selected panel a render after the tab list.
    expect(await within(dialog).findByText("Email domain")).toBeInTheDocument();
    expect(within(dialog).getByText("x.com")).toBeInTheDocument();
    expect(within(dialog).getByText("42")).toBeInTheDocument();
    expect(within(dialog).getByText("Settlement wallet")).toBeInTheDocument();
    expect(within(dialog).getByText("Configured")).toBeInTheDocument();
    expect(within(dialog).getByText("Tipping wallet")).toBeInTheDocument();
    expect(within(dialog).getByText("Not set")).toBeInTheDocument();
    expect(within(dialog).queryByText("owner@x.com")).not.toBeInTheDocument();

    fireEvent.click(within(dialog).getByText("Staff"));
    expect(await within(dialog).findByText("floor.example")).toBeInTheDocument();
    expect(within(dialog).queryByText("ada@floor.example")).not.toBeInTheDocument();
  });
});
