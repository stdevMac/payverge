/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor, act } from "@testing-library/react";

let mockGuestBillProps: Record<string, unknown> | null = null;

jest.mock("next/navigation", () => ({
  useParams: () => ({ tableCode: "T1" }),
}));

jest.mock("next/dynamic", () => {
  return {
    __esModule: true,
    default: () => {
      const MockGuestBill = (props: Record<string, unknown>) => {
        mockGuestBillProps = props;
        return <div data-testid="guest-bill" />;
      };
      MockGuestBill.displayName = "MockGuestBill";
      return MockGuestBill;
    },
  };
});

jest.mock("next/link", () => ({
  __esModule: true,
  default: ({ children, href }: { children: React.ReactNode; href: string }) => (
    <a href={href}>{children}</a>
  ),
}));

jest.mock("@nextui-org/react", () => ({
  Card: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  CardBody: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  Spinner: () => <div>spinner</div>,
  Button: ({ children }: { children: React.ReactNode }) => (
    <button type="button">{children}</button>
  ),
  Image: ({ alt }: { alt: string }) => <div aria-label={alt} role="img" />,
}));

jest.mock("../../../../components/navigation/PersistentGuestNav", () => {
  const MockPersistentGuestNav = () => <div data-testid="persistent-nav" />;
  MockPersistentGuestNav.displayName = "MockPersistentGuestNav";
  return MockPersistentGuestNav;
});

jest.mock("../../../../components/guest/CallWaiterButton", () => {
  const MockCallWaiterButton = () => <div data-testid="call-waiter" />;
  MockCallWaiterButton.displayName = "MockCallWaiterButton";
  return MockCallWaiterButton;
});
jest.mock("../../../../components/guest/LoyaltyEarnedCard", () => {
  const MockLoyaltyEarnedCard = () => null;
  MockLoyaltyEarnedCard.displayName = "MockLoyaltyEarnedCard";
  return MockLoyaltyEarnedCard;
});
jest.mock("../../../../components/guest/CRMSignupCard", () => {
  const MockCRMSignupCard = () => null;
  MockCRMSignupCard.displayName = "MockCRMSignupCard";
  return MockCRMSignupCard;
});

jest.mock("../../../../components/receipt/ReceiptGenerator", () => {
  const MockReceiptGenerator = () => null;
  MockReceiptGenerator.displayName = "MockReceiptGenerator";
  return MockReceiptGenerator;
});

jest.mock("../../../../components/payment/PaymentStatusChecker", () => {
  const MockPaymentStatusChecker = () => null;
  MockPaymentStatusChecker.displayName = "MockPaymentStatusChecker";
  return MockPaymentStatusChecker;
});

jest.mock("../../../../components/common/CurrencyConverter", () => ({
  CurrencyPrice: ({ amount }: { amount: number }) => (
    <span>{`price-${amount}`}</span>
  ),
}));

const mockSetBusinessId = jest.fn();

jest.mock("../../../../i18n/GuestTranslationProvider", () => ({
  GuestTranslationProvider: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
  useGuestTranslation: () => ({
    t: (key: string) => key,
    currentLanguage: "en",
    setBusinessId: mockSetBusinessId,
  }),
}));

jest.mock("../../../../api/bills", () => ({
  getTableByCode: jest.fn(),
  getOpenBillByTableCode: jest.fn(),
  getBusinessByTableCode: jest.fn(),
  getBillByNumber: jest.fn(),
  getMenuByTableCode: jest.fn(),
  guestBillRef: (b: { bill_number: string; public_token?: string }) =>
    b.public_token || b.bill_number,
  tryGuestBillRef: (b: { bill_number: string; public_token?: string }) =>
    b.public_token?.trim() || b.bill_number || null,
}));

let mockOnActiveBillChange: ((hasActiveBill: boolean) => void) | null = null;
jest.mock("@/hooks/useGuestBillSync", () => ({
  useGuestBillSync: (opts: { onActiveBillChange: (v: boolean) => void }) => {
    mockOnActiveBillChange = opts.onActiveBillChange;
    return { refresh: jest.fn(), connected: false };
  },
}));

const { default: GuestBillPage } = require("./page");
const {
  getBillByNumber,
  getBusinessByTableCode,
  getOpenBillByTableCode,
  getTableByCode,
} = require("../../../../api/bills");

const paidBillFixture = {
  bill: {
    id: 7,
    bill_number: "B-77",
    created_at: "2026-06-06T12:00:00Z",
    subtotal: 40.0,
    tax_amount: 4.0,
    service_fee_amount: 1.8,
    total_amount: 45.8,
  },
  items: [{ name: "Steak", quantity: 1, subtotal: 40.0 }],
};

describe("GuestBillPage — cashier-paid thank-you screen", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockGuestBillProps = null;
    sessionStorage.clear();
    // No payment_id params: the cashier path, not the checker path.
    window.history.replaceState({}, "", "/t/T1/bill");

    (getTableByCode as jest.Mock).mockResolvedValue({
      table: { id: 3, business_id: 42, name: "Table 1" },
      business: { id: 42, name: "Cafe" },
      categories: [],
    });
    (getBusinessByTableCode as jest.Mock).mockResolvedValue({
      business: {
        id: 42,
        name: "Cafe",
        default_currency: "USD",
        display_currency: "USD",
      },
      trustpilot_enabled: false,
    });
    (getOpenBillByTableCode as jest.Mock).mockResolvedValue(paidBillFixture);
  });

  it("shows the real bill total, not $0, after pay-at-cashier", async () => {
    render(<GuestBillPage />);

    await waitFor(() => {
      expect(screen.getByTestId("guest-bill")).toBeTruthy();
      expect(mockGuestBillProps).not.toBeNull();
    }, {
      timeout: 3000,
    });

    await act(async () => {
      (
        mockGuestBillProps!.onBillPaidAtCashier as (b: unknown) => void
      )(paidBillFixture);
    });

    await waitFor(() => {
      expect(screen.getByText("bill.totalPaid")).toBeTruthy();
    }, { timeout: 3000 });

    const totalRow = screen.getByText("bill.totalPaid").closest("div")!;
    expect(totalRow.textContent).toContain("price-45.8");
    expect(totalRow.textContent).not.toContain("price-0");
  });

  it("lands on the receipt when live sync reports the bill gone after a cashier payment", async () => {
    (getBillByNumber as jest.Mock).mockResolvedValue({
      ...paidBillFixture,
      bill: { ...paidBillFixture.bill, status: "paid" },
    });
    render(<GuestBillPage />);
    await waitFor(() => expect(screen.getByTestId("guest-bill")).toBeTruthy(), {
      timeout: 3000,
    });

    await act(async () => {
      mockOnActiveBillChange!(false);
    });

    await waitFor(() => {
      expect(screen.getByText("bill.totalPaid")).toBeTruthy();
    }, { timeout: 3000 });
    expect(getBillByNumber).toHaveBeenCalledWith("B-77");
  });

  it("still clears the bill when live sync reports it gone and it was not paid", async () => {
    (getBillByNumber as jest.Mock).mockResolvedValue({
      ...paidBillFixture,
      bill: { ...paidBillFixture.bill, status: "cancelled" },
    });
    render(<GuestBillPage />);
    await waitFor(() => expect(screen.getByTestId("guest-bill")).toBeTruthy(), {
      timeout: 3000,
    });

    await act(async () => {
      mockOnActiveBillChange!(false);
    });

    await waitFor(() => {
      expect(screen.queryByTestId("guest-bill")).toBeNull();
    }, { timeout: 3000 });
    expect(screen.queryByText("bill.totalPaid")).toBeNull();
  });

  it("looks the bill up once while in flight and drops a stale result after a new bill opens", async () => {
    let resolveLookup: (value: unknown) => void = () => {};
    (getBillByNumber as jest.Mock).mockImplementation(
      () => new Promise((resolve) => { resolveLookup = resolve; }),
    );
    render(<GuestBillPage />);
    await waitFor(() => expect(screen.getByTestId("guest-bill")).toBeTruthy(), {
      timeout: 3000,
    });

    // Sync fires twice (SSE + poll) before the lookup resolves.
    await act(async () => {
      mockOnActiveBillChange!(false);
      mockOnActiveBillChange!(false);
    });
    expect(getBillByNumber).toHaveBeenCalledTimes(1);

    // A new bill opens on the table before the old lookup resolves.
    const newBill = {
      ...paidBillFixture,
      bill: { ...paidBillFixture.bill, id: 8, bill_number: "B-78", total_amount: 12 },
    };
    (getOpenBillByTableCode as jest.Mock).mockResolvedValue(newBill);
    await act(async () => {
      mockOnActiveBillChange!(true);
    });
    await waitFor(() => {
      expect(
        (mockGuestBillProps?.bill as { bill?: { bill_number?: string } } | undefined)
          ?.bill?.bill_number,
      ).toBe("B-78");
    }, { timeout: 3000 });

    // The old bill's lookup now reports it paid: it must not replace the new bill.
    await act(async () => {
      resolveLookup({ ...paidBillFixture, bill: { ...paidBillFixture.bill, status: "paid" } });
    });
    expect(screen.queryByText("bill.totalPaid")).toBeNull();
    expect(screen.getByTestId("guest-bill")).toBeTruthy();
  });
});
