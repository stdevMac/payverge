/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import AdminUsersPage from "./page";
import type { AdminUserDetail } from "@/api/admin";

jest.setTimeout(15000);

const mockGetUserList = jest.fn();
const mockGetDetail = jest.fn();

jest.mock("next/navigation", () => ({
  useRouter: () => ({ replace: jest.fn() }),
  useSearchParams: () => new URLSearchParams(),
}));

jest.mock("@/api/admin", () => ({
  getUserList: (...args: unknown[]) => mockGetUserList(...args),
  adminUserAPI: {
    getDetail: (...args: unknown[]) => mockGetDetail(...args),
    close: jest.fn(),
    resetPassword: jest.fn(),
  },
}));

function makeDetail(
  lifecycle: { is_active: boolean; closed_at: string | null } = {
    is_active: true,
    closed_at: null,
  },
): AdminUserDetail {
  return {
    user: {
      id: 5,
      email: "u@x.com",
      name: "User",
      role: "admin",
      auth_method: "email",
      email_verified: true,
      created_at: "2025-01-01T00:00:00Z",
      picture: "",
    },
    business: {
      id: 9,
      business_id: "biz_9",
      name: "Cafe",
      is_active: lifecycle.is_active,
      closed_at: lifecycle.closed_at,
      closed_reason: lifecycle.closed_at ? "owner request" : "",
    },
    admin_actions: [],
  };
}

beforeEach(() => {
  jest.clearAllMocks();
  mockGetUserList.mockResolvedValue({
    users: [
      {
        id: 5,
        name: "User",
        email: "u@x.com",
        business_name: "Cafe",
        status: "active",
        joined_at: "2025-01-01T00:00:00Z",
        can_merge: false,
      },
    ],
    total: 1,
  });
});

describe("AdminUsersPage — search debounce", () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });
  afterEach(() => {
    jest.runOnlyPendingTimers();
    jest.useRealTimers();
  });

  it("debounces typing into the search box (one trailing fetch, not one per keystroke)", async () => {
    mockGetDetail.mockResolvedValue(makeDetail());
    render(<AdminUsersPage />);

    await waitFor(() => expect(mockGetUserList).toHaveBeenCalledTimes(1));

    const input = screen.getByPlaceholderText(/Search by name/i);
    mockGetUserList.mockClear();

    fireEvent.change(input, { target: { value: "u" } });
    fireEvent.change(input, { target: { value: "us" } });
    fireEvent.change(input, { target: { value: "use" } });
    fireEvent.change(input, { target: { value: "user" } });

    expect(mockGetUserList).not.toHaveBeenCalled();

    jest.advanceTimersByTime(350);

    await waitFor(() => expect(mockGetUserList).toHaveBeenCalledTimes(1));
    expect(mockGetUserList).toHaveBeenCalledWith(
      expect.objectContaining({ search: "user", page: 1 }),
    );
  });
});

describe("AdminUsersPage — admin lifecycle", () => {
  async function openDetail(detail: AdminUserDetail) {
    mockGetDetail.mockResolvedValue(detail);
    render(<AdminUsersPage />);
    fireEvent.click(await screen.findByText("User"));
    await screen.findByText("User Information");
  }

  it("has no plan column, Payments tab or Gift Time card", async () => {
    await openDetail(makeDetail());
    expect(screen.queryByText("Plan")).not.toBeInTheDocument();
    expect(screen.queryByText("Payments")).not.toBeInTheDocument();
    fireEvent.click(screen.getByText("Actions"));
    expect(await screen.findByText("Account Actions")).toBeInTheDocument();
    expect(screen.queryByText("Gift Time")).not.toBeInTheDocument();
  });

  it("shows a suspended business as Suspended", async () => {
    await openDetail(makeDetail({ is_active: false, closed_at: null }));
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByText("Suspended")).toBeInTheDocument();
  });

  it("shows a closed business with its close date and reason", async () => {
    await openDetail(
      makeDetail({ is_active: false, closed_at: "2025-06-01T00:00:00Z" }),
    );
    expect(screen.getAllByText("Closed").length).toBeGreaterThan(0);
    expect(screen.getByText(/owner request/)).toBeInTheDocument();
  });

  it("filters the list by lifecycle status", async () => {
    render(<AdminUsersPage />);
    await waitFor(() => expect(mockGetUserList).toHaveBeenCalledTimes(1));
    expect(mockGetUserList).toHaveBeenCalledWith(
      expect.not.objectContaining({ plan: expect.anything() }),
    );
  });
});
