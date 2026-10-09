/** @jest-environment jsdom */
/**
 * Issue 824 — guest /t/<code>/profile must never be a dead, empty spinner.
 *
 * The shipped page gated its ENTIRE render (CustomerProfile auth gate + guest
 * nav) behind getOpenBillByTableCode/getBusinessByTableCode, data that only
 * feeds the bottom nav. During a rate-limited/overloaded dinner rush those
 * calls sit on the 30s axios timeout, so guests stared at a bare full-screen
 * spinner with no copy and no gate. The profile surface (signed-in state or
 * the honest sign-in gate, both owned by CustomerProfile/CustomerAuthContext,
 * which are bounded) must render immediately; the nav hydrates when the
 * supplementary fetches settle.
 */
import React from "react";
import { render, screen, act } from "@testing-library/react";

const mockGetOpenBillByTableCode = jest.fn();
const mockGetBusinessByTableCode = jest.fn();

jest.mock("next/navigation", () => ({
  useParams: () => ({ tableCode: "M03Y18GB3P" }),
  usePathname: () => "/t/M03Y18GB3P/profile",
}));

jest.mock("@/api/bills", () => ({
  getOpenBillByTableCode: (...args: unknown[]) =>
    mockGetOpenBillByTableCode(...args),
  getBusinessByTableCode: (...args: unknown[]) =>
    mockGetBusinessByTableCode(...args),
}));

jest.mock("@nextui-org/react", () => ({
  Spinner: () => <div data-testid="bare-page-spinner" />,
}));

jest.mock("@/components/customer/CustomerProfile", () => {
  const CustomerProfileStub = () => (
    <div data-testid="customer-profile-stub">profile surface</div>
  );
  CustomerProfileStub.displayName = "CustomerProfileStub";
  return CustomerProfileStub;
});

jest.mock("@/components/navigation/PersistentGuestNav", () => {
  const PersistentGuestNavStub = (props: {
    tableCode: string;
    currentBill?: { bill: unknown } | null;
    displayCurrency?: string;
  }) => (
    <nav
      data-testid="guest-nav-stub"
      data-table={props.tableCode}
      data-has-bill={props.currentBill ? "yes" : "no"}
      data-display-currency={props.displayCurrency}
    />
  );
  PersistentGuestNavStub.displayName = "PersistentGuestNavStub";
  return PersistentGuestNavStub;
});

import TableCustomerProfilePage from "./page";

const flush = async () => {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
};

describe("guest profile page dead-spinner gate (issue 824)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("renders the profile surface immediately even when the nav fetches never settle", async () => {
    // Rush conditions: both guest reads hang (rate-limit queue / 30s timeout).
    mockGetOpenBillByTableCode.mockReturnValue(new Promise(() => {}));
    mockGetBusinessByTableCode.mockReturnValue(new Promise(() => {}));

    render(<TableCustomerProfilePage />);
    await flush();

    // The auth gate / signed-in state must be on screen, not a bare spinner.
    expect(screen.getByTestId("customer-profile-stub")).toBeTruthy();
    expect(screen.queryByTestId("bare-page-spinner")).toBeNull();
  });

  it("hydrates the nav with the open bill and business currency once fetches settle", async () => {
    mockGetOpenBillByTableCode.mockResolvedValue({
      bill: { id: 1143, total_amount: 11.64 },
      items: [{ id: 1 }],
    });
    mockGetBusinessByTableCode.mockResolvedValue({
      business: { default_currency: "USD", display_currency: "ARS" },
    });

    render(<TableCustomerProfilePage />);
    await flush();

    const nav = screen.getByTestId("guest-nav-stub");
    expect(nav.getAttribute("data-has-bill")).toBe("yes");
    expect(nav.getAttribute("data-display-currency")).toBe("ARS");
    expect(nav.getAttribute("data-table")).toBe("M03Y18GB3P");
  });

  it("keeps an honest surface when both guest reads 429 (no dead spinner, defaults for nav)", async () => {
    const rateLimited = Object.assign(new Error("rate limited"), {
      status: 429,
      response: { status: 429 },
    });
    mockGetOpenBillByTableCode.mockRejectedValue(rateLimited);
    mockGetBusinessByTableCode.mockRejectedValue(rateLimited);

    render(<TableCustomerProfilePage />);
    await flush();

    expect(screen.getByTestId("customer-profile-stub")).toBeTruthy();
    expect(screen.queryByTestId("bare-page-spinner")).toBeNull();
    const nav = screen.getByTestId("guest-nav-stub");
    expect(nav.getAttribute("data-has-bill")).toBe("no");
    expect(nav.getAttribute("data-display-currency")).toBe("USD");
  });
});
