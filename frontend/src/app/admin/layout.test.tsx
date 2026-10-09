/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import AdminLayoutClient from "@/app/admin/AdminLayoutClient";

const mockReplace = jest.fn();

jest.mock("next/navigation", () => ({
  useRouter: () => ({ replace: mockReplace }),
  usePathname: () => "/admin",
}));

const mockUseAuth = jest.fn();
jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => mockUseAuth(),
}));

const mockUseUserStore = jest.fn();
jest.mock("@/store/useUserStore", () => ({
  useUserStore: () => mockUseUserStore(),
}));

jest.mock("next/dynamic", () => {
  // Hoisted factory: require locally (cannot close over outer React).
  // Production defers AdminShell via next/dynamic; tests resolve the mock shell.
  return function mockDynamic() {
    const { createElement } = require("react") as typeof import("react");
    const { AdminShell } = require("@/components/admin/AdminShell") as {
      AdminShell: React.ComponentType<{ children?: React.ReactNode }>;
    };
    return function DynamicAdminShell({
      children,
    }: {
      children?: React.ReactNode;
    }) {
      return createElement(AdminShell, null, children);
    };
  };
});

jest.mock("@/components/admin/AdminShell", () => ({
  AdminShell: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="admin-shell">{children}</div>
  ),
}));

describe("admin layout (client gate)", () => {
  beforeEach(() => {
    mockReplace.mockReset();
    mockUseAuth.mockReset();
    mockUseUserStore.mockReset();
  });

  it("renders the loading spinner while HybridAuthProvider is still initialising", () => {
    mockUseAuth.mockReturnValue({
      isInitialized: false,
      isLoading: true,
      oauthData: null,
      staffData: null,
    });
    mockUseUserStore.mockReturnValue({ user: null });

    render(
      <AdminLayoutClient>
        <div>admin page</div>
      </AdminLayoutClient>,
    );

    expect(screen.queryByText("admin page")).toBeNull();
    expect(screen.queryByTestId("admin-shell")).toBeNull();
    expect(mockReplace).not.toHaveBeenCalled();
  });

  it("redirects non-admin sessions to /dashboard and renders nothing", () => {
    mockUseAuth.mockReturnValue({
      isInitialized: true,
      isLoading: false,
      oauthData: { role: "user" },
      staffData: null,
    });
    mockUseUserStore.mockReturnValue({ user: { role: "user" } });

    render(
      <AdminLayoutClient>
        <div>admin page</div>
      </AdminLayoutClient>,
    );

    expect(screen.queryByText("admin page")).toBeNull();
    expect(screen.queryByTestId("admin-shell")).toBeNull();
    expect(mockReplace).toHaveBeenCalledWith("/dashboard");
  });

  it("renders admin chrome for admin sessions", () => {
    mockUseAuth.mockReturnValue({
      isInitialized: true,
      isLoading: false,
      oauthData: { role: "admin" },
      staffData: null,
    });
    mockUseUserStore.mockReturnValue({ user: { role: "admin" } });

    render(
      <AdminLayoutClient>
        <div>admin page</div>
      </AdminLayoutClient>,
    );

    expect(screen.getByTestId("admin-shell")).toBeTruthy();
    expect(screen.getByText("admin page")).toBeTruthy();
    expect(mockReplace).not.toHaveBeenCalled();
  });

  it("rejects staff sessions even when staff role is admin (API rejects staff too)", () => {
    mockUseAuth.mockReturnValue({
      isInitialized: true,
      isLoading: false,
      oauthData: null,
      staffData: { role: "admin" },
    });
    mockUseUserStore.mockReturnValue({ user: null });

    render(
      <AdminLayoutClient>
        <div>admin page</div>
      </AdminLayoutClient>,
    );

    expect(screen.queryByText("admin page")).toBeNull();
    expect(mockReplace).toHaveBeenCalledWith("/dashboard");
  });
});
