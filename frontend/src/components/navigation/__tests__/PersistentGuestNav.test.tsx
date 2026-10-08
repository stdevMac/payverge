/**
 * @jest-environment jsdom
 */
import React from "react";
import { render, screen } from "@testing-library/react";
import PersistentGuestNav, {
  GUEST_NAV_CSS_VAR,
  GUEST_NAV_HEIGHT_PX,
} from "../PersistentGuestNav";

let mockPathname = "/t/TBL1/menu";
jest.mock("next/navigation", () => ({
  usePathname: () => mockPathname,
}));

let mockCustomerId: string | null = null;
jest.mock("@/contexts/CustomerAuthContext", () => ({
  useCustomerAuth: () => ({ customerId: mockCustomerId }),
}));

jest.mock("../../../i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({
    currentLanguage: "en",
    t: (key: string, params?: Record<string, string | number>) => {
      const table: Record<string, string> = {
        "navigation.table": "Table",
        "navigation.menu": "Menu",
        "navigation.bill": "Bill",
        "navigation.billLockedHint": "Add an item to start your bill",
        "navigation.billWithSummary": "Bill, {count} items, {amount} due",
        "navigation.billWithSummaryOne": "Bill, 1 item, {amount} due",
        "navigation.profile": "Profile",
      };
      const raw = table[key] ?? key;
      if (!params) return raw;
      return Object.entries(params).reduce(
        (acc, [name, value]) => acc.replaceAll(`{${name}}`, String(value)),
        raw,
      );
    },
  }),
}));

// SimpleLanguageSelector loads business data over the network; stub it.
jest.mock("../../guest/SimpleLanguageSelector", () => ({
  SimpleLanguageSelector: () => <div data-testid="lang-selector" />,
}));

jest.mock("../../common/CurrencyConverter", () => ({
  CurrencyPrice: () => <span>price</span>,
}));

jest.mock("@/utils/guestCurrencyFormatter", () => ({
  formatGuestCurrency: (amount: number) => `$${amount}`,
  formatConvertedGuestCurrency: (amount: number) => `$${amount}`,
  normalizeGuestLocale: () => "en",
}));

describe("PersistentGuestNav", () => {
  beforeEach(() => {
    mockPathname = "/t/TBL1/menu";
    mockCustomerId = null;
    document.documentElement.style.removeProperty(GUEST_NAV_CSS_VAR);
  });

  it("renders the language selector wrapper without an sm:hidden gate", () => {
    render(
      <PersistentGuestNav tableCode="TBL1" isOrderingEnabled currentBill={null} />,
    );
    const wrapper = screen.getByTestId("lang-selector").parentElement;
    expect(wrapper?.className).not.toContain("sm:hidden");
  });

  it("publishes --guest-nav-height so content shells can clear the dock", () => {
    render(
      <PersistentGuestNav tableCode="TBL1" isOrderingEnabled currentBill={null} />,
    );
    expect(screen.getByTestId("persistent-guest-nav")).toBeTruthy();
    expect(
      document.documentElement.style.getPropertyValue(GUEST_NAV_CSS_VAR),
    ).toBe(`${GUEST_NAV_HEIGHT_PX}px`);
  });

  // Empty bill is navigable — lock toast removed so guests reach the empty shell.
  it("links to the bill page when ordering is on and the bill is empty", () => {
    render(
      <PersistentGuestNav tableCode="TBL1" isOrderingEnabled currentBill={null} />,
    );
    const billLink = screen.getByRole("link", { name: /bill/i });
    expect(billLink).toHaveAttribute("href", "/t/TBL1/bill");
    expect(screen.queryByRole("button", { name: /bill/i })).toBeNull();
  });

  // Regression: pay-only venues (in-app ordering disabled) can still have an
  // operator-opened bill. The guest MUST be able to reach and pay it, so the
  // Bill tab has to render as a working link — not be hidden with the rest of
  // the ordering UI.
  it("still shows a working Bill tab when ordering is disabled but a bill exists", () => {
    render(
      <PersistentGuestNav
        tableCode="TBL1"
        isOrderingEnabled={false}
        currentBill={{ bill: { total_amount: 42 }, items: [{ quantity: 1 }] }}
      />,
    );
    const billLink = screen.getByRole("link", { name: /bill/i });
    expect(billLink).toHaveAttribute("href", "/t/TBL1/bill");
  });

  it("names an active-bill destination with Bill plus count and amount (#421)", () => {
    render(
      <PersistentGuestNav
        tableCode="TBL1"
        isOrderingEnabled
        currentBill={{
          bill: { total_amount: 21.53 },
          items: [{ quantity: 1 }],
        }}
      />,
    );
    const billLink = screen.getByRole("link", {
      name: /bill.*1 item.*\$21\.53/i,
    });
    expect(billLink).toHaveAttribute("href", "/t/TBL1/bill");
    expect(billLink).not.toHaveAttribute("aria-current");
  });

  it("marks the current bottom-nav destination with aria-current=page", () => {
    const { unmount } = render(
      <PersistentGuestNav tableCode="TBL1" isOrderingEnabled currentBill={null} />,
    );
    expect(screen.getByRole("link", { name: /menu/i })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(screen.getByRole("link", { name: /table/i })).not.toHaveAttribute(
      "aria-current",
    );
    expect(screen.getByRole("link", { name: /^bill$/i })).not.toHaveAttribute(
      "aria-current",
    );
    unmount();

    mockPathname = "/t/TBL1";
    const tableView = render(
      <PersistentGuestNav tableCode="TBL1" isOrderingEnabled currentBill={null} />,
    );
    expect(screen.getByRole("link", { name: /table/i })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(screen.getByRole("link", { name: /menu/i })).not.toHaveAttribute(
      "aria-current",
    );
    tableView.unmount();

    mockPathname = "/t/TBL1/bill";
    const billView = render(
      <PersistentGuestNav
        tableCode="TBL1"
        isOrderingEnabled
        currentBill={{
          bill: { total_amount: 21.53 },
          items: [{ quantity: 1 }],
        }}
      />,
    );
    expect(screen.getByRole("link", { name: /bill/i })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(screen.getByRole("link", { name: /menu/i })).not.toHaveAttribute(
      "aria-current",
    );
    billView.unmount();

    mockPathname = "/t/TBL1/profile";
    mockCustomerId = "cust-1";
    render(
      <PersistentGuestNav tableCode="TBL1" isOrderingEnabled currentBill={null} />,
    );
    expect(screen.getByRole("link", { name: /profile/i })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(screen.getByRole("link", { name: /menu/i })).not.toHaveAttribute(
      "aria-current",
    );
  });

  it("hides the Bill tab only when ordering is disabled AND there is no bill", () => {
    render(
      <PersistentGuestNav
        tableCode="TBL1"
        isOrderingEnabled={false}
        currentBill={null}
      />,
    );
    expect(screen.queryByRole("link", { name: /bill/i })).toBeNull();
    expect(screen.queryByRole("button", { name: /bill/i })).toBeNull();
  });
});
