/** @jest-environment jsdom */
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";
import { useStaffPermissions } from "./useStaffPermissions";
import { getStaffPermissions } from "@/api/rbac";
import { queryKeys } from "@/api/queryKeys";

jest.mock("@/api/rbac", () => ({
  getStaffPermissions: jest.fn(),
}));

const mockGetStaffPermissions = getStaffPermissions as jest.Mock;

const fixture = {
  permissions: ["bills:read", "menu:read", "analytics:sales"],
  role_permissions: ["bills:read", "menu:read"],
  custom_grants: ["analytics:sales"],
  custom_denies: [],
};

const wrapper = ({ children }: { children: React.ReactNode }) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return React.createElement(QueryClientProvider, { client }, children);
};

describe("useStaffPermissions", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("loads permissions when enabled with businessId and staffId", async () => {
    mockGetStaffPermissions.mockResolvedValue(fixture);
    const { result } = renderHook(
      () => useStaffPermissions("42", 7, true),
      { wrapper },
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual(fixture);
    expect(mockGetStaffPermissions).toHaveBeenCalledWith("42", "7");
    expect(result.current.dataUpdatedAt).toBeGreaterThan(0);
  });

  it("uses the staff.permissions query key", async () => {
    mockGetStaffPermissions.mockResolvedValue(fixture);
    const { result } = renderHook(
      () => useStaffPermissions("42", "7", true),
      { wrapper },
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toBeDefined();
    expect(queryKeys.staff.permissions("42", "7")).toEqual([
      "staff",
      "42",
      "permissions",
      "7",
    ]);
  });

  it("does not fetch when enabled is false", () => {
    renderHook(() => useStaffPermissions("42", 7, false), { wrapper });
    expect(mockGetStaffPermissions).not.toHaveBeenCalled();
  });

  it("does not fetch when businessId is missing", () => {
    renderHook(() => useStaffPermissions(undefined, 7, true), { wrapper });
    expect(mockGetStaffPermissions).not.toHaveBeenCalled();
  });

  it("does not fetch when staffId is missing", () => {
    renderHook(() => useStaffPermissions("42", undefined, true), { wrapper });
    expect(mockGetStaffPermissions).not.toHaveBeenCalled();
  });
});
