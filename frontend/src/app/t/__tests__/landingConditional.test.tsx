/** @jest-environment jsdom */
/**
 * Task 5.2 — /t/{code} conditional rewards card + promoted current bill.
 *
 * The landing client (GuestTableView) should:
 *  1. Hide the CRM rewards-signup card when the business has crm_enabled=false.
 *  2. Render the rewards card when crm_enabled=true (and no customer is logged in).
 *  3. When an open bill exists, surface the "current bill" card BEFORE the
 *     primary "browse menu" card.
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

function buildTableResponse(overrides?: {
  crmEnabled?: boolean;
  kitchenEnabled?: boolean;
  ordersEnabled?: boolean;
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
      kitchen_enabled: overrides?.kitchenEnabled ?? true,
      orders_enabled: overrides?.ordersEnabled ?? true,
      crm_enabled: overrides?.crmEnabled ?? false,
    },
    menu: { categories: "[]" },
    categories: [],
    offers: [],
    bundles: [],
  };
}

function buildOpenBill(): any {
  return {
    bill: {
      id: 99,
      bill_number: "B-99",
      total_amount: 42.5,
      status: "open",
    },
    items: [
      { id: 1, name: "Item A", quantity: 2, subtotal: 20 },
      { id: 2, name: "Item B", quantity: 1, subtotal: 22.5 },
    ],
  };
}

describe("/t/{code} conditional rewards card + promoted current bill", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockedGetProfile.mockRejectedValue(new Error("not logged in"));
    mockedGetOpenBill.mockRejectedValue(new Error("no open bill"));
  });

  it("hides the rewards card when crm_enabled is false", async () => {
    mockedGetTable.mockResolvedValue(buildTableResponse({ crmEnabled: false }));

    render(<GuestTableView tableCode="AI-T01" />);

    await waitFor(() =>
      expect(screen.getByText("Browse menu")).toBeInTheDocument(),
    );
    expect(screen.queryByText(/Sign up for rewards/i)).not.toBeInTheDocument();
  });

  it("shows the rewards card when crm_enabled is true and no customer is signed in", async () => {
    mockedGetTable.mockResolvedValue(buildTableResponse({ crmEnabled: true }));

    render(<GuestTableView tableCode="AI-T01" />);

    // Wait for loading to finish before asserting on the rewards card.
    await waitFor(() =>
      expect(screen.getByText("Browse menu")).toBeInTheDocument(),
    );
    // Rewards card heading + signup button both contain the phrase.
    expect(screen.getAllByText(/Sign up for rewards/i).length).toBeGreaterThan(
      0,
    );
  });

  it("does not probe the protected customer profile for an anonymous guest", async () => {
    mockedGetTable.mockResolvedValue(buildTableResponse({ crmEnabled: true }));

    render(<GuestTableView tableCode="AI-T01" />);

    await waitFor(() =>
      expect(screen.getByText("Browse menu")).toBeInTheDocument(),
    );
    await Promise.resolve();

    expect(mockedGetProfile).not.toHaveBeenCalled();
  });

  it("renders the current bill card before the menu card when an open bill exists", async () => {
    mockedGetTable.mockResolvedValue(buildTableResponse({ crmEnabled: false }));
    mockedGetOpenBill.mockResolvedValueOnce(buildOpenBill());

    render(<GuestTableView tableCode="AI-T01" />);

    const billHeading = await screen.findByText("Current bill");
    const menuHeading = await screen.findByText("Browse menu");

    // DOM order: the current-bill heading must appear before the menu heading.
    const positionRelation = billHeading.compareDocumentPosition(menuHeading);
    expect(positionRelation & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });
});
