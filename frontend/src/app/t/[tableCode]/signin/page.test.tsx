/** @jest-environment jsdom */
/**
 * Issue 878 — /t/{code}/signin was a 404, so INICIAR SESIÓN deep links
 * from the table home dead-ended. The table-context sign-in surface must
 * exist and render the guest auth gate without waiting on nav fetches.
 */
import React from "react";
import { render, screen } from "@testing-library/react";

jest.mock("next/navigation", () => ({
  useParams: () => ({ tableCode: "FV214XU12D" }),
  usePathname: () => "/t/FV214XU12D/signin",
}));

jest.mock("@/api/bills", () => ({
  getOpenBillByTableCode: jest.fn(() => new Promise(() => {})),
  getBusinessByTableCode: jest.fn(() => new Promise(() => {})),
}));

jest.mock("@/components/customer/CustomerProfile", () => {
  const CustomerProfileStub = () => (
    <div data-testid="customer-profile-stub">sign-in gate</div>
  );
  CustomerProfileStub.displayName = "CustomerProfileStub";
  return CustomerProfileStub;
});

jest.mock("@/components/navigation/PersistentGuestNav", () => {
  const PersistentGuestNavStub = (props: { tableCode: string }) => (
    <nav data-testid="guest-nav-stub" data-table={props.tableCode} />
  );
  PersistentGuestNavStub.displayName = "PersistentGuestNavStub";
  return PersistentGuestNavStub;
});

import TableSignInPage from "./page";

describe("guest table sign-in page (issue 878)", () => {
  it("renders the sign-in gate immediately for a table-context deep link", () => {
    render(<TableSignInPage />);
    expect(screen.getByTestId("guest-signin-shell")).toBeTruthy();
    expect(screen.getByTestId("customer-profile-stub")).toHaveTextContent(
      /sign-in gate/i,
    );
    expect(screen.getByTestId("guest-nav-stub")).toHaveAttribute(
      "data-table",
      "FV214XU12D",
    );
  });

  // Cross-branch guard (878 x 947/863): every guest surface offsets its bottom
  // padding by the cookie banner height as well as the persistent nav. This
  // page is the one that would otherwise stay under the cookie sheet.
  it("leaves room for the cookie banner above the persistent nav", () => {
    render(<TableSignInPage />);
    const shell = screen.getByTestId("guest-signin-shell");
    expect(shell.className).toContain("var(--guest-nav-height,5.5rem)");
    expect(shell.className).toContain("var(--cookie-banner-height,0px)");
    expect(shell.className).toContain("env(safe-area-inset-bottom)");
  });
});
