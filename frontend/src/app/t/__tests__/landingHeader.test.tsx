/** @jest-environment jsdom */
/**
 * Task 5.3 — Promote sign-in to header + open/closed pill on /t/{code}.
 *
 * The landing client should:
 *  1. Surface a "Sign in" link in the page header (NOT inside the rewards card)
 *     when business.crm_enabled is true and no customer is logged in.
 *  2. Render an open/closed pill driven by business.hours showing either
 *     "Open until HH:MM" (emerald) or "Opens HH:MM" / "Closed today" (rose).
 */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";

import { GuestTableView } from "@/components/guest/GuestTableView";
import { getTableByCode, getOpenBillByTableCode } from "@/api/bills";
import { crmAPI } from "@/api/crm";

const mockCustomerAuth = {
  customer: null,
  customerId: null,
  isAuthenticated: false,
  loading: false,
};

jest.mock("@/contexts/CustomerAuthContext", () => ({
  useCustomerAuth: () => mockCustomerAuth,
}));

jest.mock("@/api/bills", () => ({
  getTableByCode: jest.fn(),
  getOpenBillByTableCode: jest.fn(),
  getServiceCallStatus: jest.fn().mockResolvedValue({
    status: "none",
    reason: null,
  }),
  createServiceCall: jest.fn(),
  ServiceCallCooldownError: class extends Error {},
}));

jest.mock("@/api/crm", () => ({
  crmAPI: { getProfile: jest.fn() },
}));

jest.mock("@/api/customerTable", () => ({
  checkInCustomerToTable: jest.fn().mockResolvedValue({
    customer_business: { id: 1 },
    bill_attached: false,
  }),
}));

jest.mock("@/i18n/GuestTranslationProvider", () => {
  const translate = (key: string, params?: Record<string, string | number>) => {
    const dict: Record<string, string> = {
      "table.title": `Table ${params?.tableNumber ?? ""}`,
      "table.browseMenu": "Browse menu",
      "table.browseMenuDescription": "See what's on offer",
      "table.currentBill": "Current bill",
      "table.noBill": "No open bill",
      "table.noBillDescription": "Start ordering when ready",
      "table.scanQR": "Scan another table",
      "table.promotions.title": "Active offers",
      "table.promotions.subtitle": "View today's specials",
      "bill.billStatus.open": "Open",
      "menu.items": "items",
      "businessPage.poweredBySecure": "Powered by Payverge",
      "crm.signupPrompt": "Sign up for rewards",
      "crm.signupDescription": "Create an account to earn points",
      "crm.alreadyMember": "Already a member?",
      "crm.signIn": "Sign in",
      "crm.earnPoints": "Earn Points",
      "crm.getRewards": "Get Rewards",
      "crm.joinFailed":
        "We could not connect your rewards account to this table.",
      "landing.signIn": "Sign in",
      "landing.statusOpenUntil": `Open until ${params?.time ?? ""}`,
      "landing.statusOpensAt": `Opens ${params?.time ?? ""}`,
      "landing.statusClosedToday": "Closed today",
    };
    return dict[key] ?? key;
  };
  const setBusinessId = () => {};
  return {
    useGuestTranslation: () => ({ t: translate, setBusinessId }),
  };
});

jest.mock("@/components/navigation/PersistentGuestNav", () => ({
  __esModule: true,
  default: () => null,
}));

jest.mock("@/components/notifications/PaymentNotification", () => ({
  __esModule: true,
  default: () => null,
}));

jest.mock("@/components/notifications/BillUpdateNotification", () => ({
  __esModule: true,
  default: () => null,
}));

jest.mock("@/components/common/CurrencyConverter", () => ({
  CurrencyPrice: ({ amount }: { amount: number }) => (
    <span>{`$${amount.toFixed(2)}`}</span>
  ),
}));

jest.mock("@/components/customer/CustomerAuthModal", () => ({
  __esModule: true,
  default: () => null,
}));

jest.mock("@nextui-org/react", () => ({
  Spinner: () => <div data-testid="spinner" />,
  Image: (props: { src?: string; alt?: string }) => (
    // eslint-disable-next-line @next/next/no-img-element
    <img src={props.src} alt={props.alt ?? ""} />
  ),
  Button: ({
    children,
    onPress,
    startContent,
  }: {
    children: React.ReactNode;
    onPress?: () => void;
    startContent?: React.ReactNode;
  }) => (
    <button type="button" onClick={onPress}>
      {startContent}
      {children}
    </button>
  ),
}));

jest.mock("@/hooks/usePolling", () => ({
  usePolling: () => {},
}));

const mockedGetTable = getTableByCode as jest.MockedFunction<
  typeof getTableByCode
>;
const mockedGetOpenBill = getOpenBillByTableCode as jest.MockedFunction<
  typeof getOpenBillByTableCode
>;
const mockedGetProfile = crmAPI.getProfile as jest.MockedFunction<
  typeof crmAPI.getProfile
>;

function buildTableResponse(overrides: {
  crmEnabled?: boolean;
  hours?: Array<{
    day_of_week: number;
    open_time: string;
    close_time: string;
    is_closed?: boolean;
  }>;
}): any {
  return {
    table: {
      id: 1,
      business_id: 10,
      name: "1",
      code: "AI-T01",
      seats: 4,
      status: "active",
      qr_code_url: "",
    },
    business: {
      id: 10,
      name: "Test Cafe",
      logo: "",
      address: { street: "", city: "" },
      default_currency: "USD",
      display_currency: "USD",
      kitchen_enabled: true,
      orders_enabled: true,
      crm_enabled: overrides.crmEnabled ?? true,
      hours: overrides.hours,
    },
    menu: { categories: "[]" },
    categories: [],
    offers: [],
    bundles: [],
  };
}

describe("/t/{code} landing header", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockedGetProfile.mockRejectedValue(new Error("not logged in"));
    mockedGetOpenBill.mockRejectedValue(new Error("no open bill"));
  });

  it("shows a Sign in link in the header when crm_enabled and signed-out", async () => {
    mockedGetTable.mockResolvedValue(
      buildTableResponse({ crmEnabled: true, hours: [] }),
    );

    render(<GuestTableView tableCode="AI-T01" />);

    // Sign-in trigger is now a button (not an <a>) that dispatches a
    // custom event opening the CRMSignupCard auth modal — the previous
    // <Link href="/login"> 404'd on prod.
    const signInLink = await screen.findByTestId("landing-header-signin");
    expect(signInLink).toBeInTheDocument();
    expect(signInLink.tagName).toBe("BUTTON");
    expect(signInLink).toHaveTextContent(/sign in/i);
  });

  it("does not offer the AI waiter FAB on a Core venue without ai_available (#944)", async () => {
    mockedGetTable.mockResolvedValue({
      ...buildTableResponse({ crmEnabled: false, hours: [] }),
      business: {
        ...buildTableResponse({ crmEnabled: false, hours: [] }).business,
        id: 141,
        ai_available: false,
        ai_settings: {
          ai_enabled: true,
          ai_name: "Mozo",
          ai_priority: "balanced",
          business_page_ai_enabled: true,
        },
      },
    });

    render(<GuestTableView tableCode="CORE141" />);
    await screen.findByText("Browse menu");
    expect(
      screen.queryByRole("button", { name: /open assistant|asistente/i }),
    ).not.toBeInTheDocument();
  });

  it("does not show the header Sign in link when crm_enabled is false", async () => {
    mockedGetTable.mockResolvedValue(
      buildTableResponse({ crmEnabled: false, hours: [] }),
    );

    render(<GuestTableView tableCode="AI-T01" />);

    await waitFor(() =>
      expect(screen.getByText("Browse menu")).toBeInTheDocument(),
    );
    expect(
      screen.queryByTestId("landing-header-signin"),
    ).not.toBeInTheDocument();
  });

  it("renders an Open until pill when hours include the current weekday and now is between open and close", async () => {
    // Use a wide-open window that covers any clock time.
    const today = new Date().getDay();
    mockedGetTable.mockResolvedValue(
      buildTableResponse({
        crmEnabled: true,
        hours: [
          {
            day_of_week: today,
            open_time: "00:00",
            close_time: "23:59",
            is_closed: false,
          },
        ],
      }),
    );

    render(<GuestTableView tableCode="AI-T01" />);

    const pill = await screen.findByTestId("landing-open-pill");
    expect(pill).toHaveTextContent(/Open until/i);
    // The pill now formats times locale-aware (12h for en) instead of echoing
    // the raw 24h close_time, so "23:59" renders as "11:59 PM".
    expect(pill).toHaveTextContent(/11:59\s?PM/i);
  });

  it("renders a Closed today pill when today is marked closed", async () => {
    const today = new Date().getDay();
    mockedGetTable.mockResolvedValue(
      buildTableResponse({
        crmEnabled: true,
        hours: [
          {
            day_of_week: today,
            open_time: "08:00",
            close_time: "20:00",
            is_closed: true,
          },
        ],
      }),
    );

    render(<GuestTableView tableCode="AI-T01" />);

    const pill = await screen.findByTestId("landing-open-pill");
    expect(pill).toHaveTextContent(/closed today/i);
  });
});
