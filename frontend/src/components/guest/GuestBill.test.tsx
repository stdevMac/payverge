/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";

import GuestBill from "@/components/guest/GuestBill";
import {
  getBillByNumber,
  getOpenBillByTableCode,
  type BillWithItemsResponse,
} from "@/api/bills";
import { getGuestOrdersByBillNumber } from "@/api/orders";
import { asDollars } from "@/types/money";

jest.mock("@/api/bills", () => ({
  getOpenBillByTableCode: jest.fn(),
  getBillByNumber: jest.fn(),
  // Real fallback behavior: token-first, bill_number for pre-migration bills.
  guestBillRef: (b: { bill_number: string; public_token?: string }) =>
    b.public_token || b.bill_number,
  tryGuestBillRef: (b: { bill_number: string; public_token?: string }) =>
    b.public_token?.trim() || b.bill_number || null,
  isActiveBillStatus: (status: string | undefined) =>
    status === "open" || status === "partial",
}));

jest.mock("@/api/orders", () => ({
  getGuestOrdersByBillNumber: jest.fn(),
}));

jest.mock("@/hooks/usePolling", () => ({
  usePolling: ({ callback }: { callback: () => Promise<void> | void }) => {
    void callback();
    return {
      startPolling: jest.fn(),
      stopPolling: jest.fn(),
      isPolling: false,
    };
  },
}));

jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({
    t: (key: string) => key,
  }),
}));

// GuestBill consumes CustomerAuthContext for the loyalty-redemption card.
// An unauthenticated customer keeps the loyalty effect (and its crm/loyalty
// API calls) short-circuited, which is all these bill/payment tests need.
jest.mock("@/contexts/CustomerAuthContext", () => ({
  useCustomerAuth: () => ({ customer: null, isAuthenticated: false }),
}));

jest.mock("@/components/guest/PaymentSection", () => {
  const MockPaymentSection = () => (
    <div data-testid="payment-section">payment controls</div>
  );
  MockPaymentSection.displayName = "MockPaymentSection";
  return MockPaymentSection;
});

jest.mock("@/components/common/CurrencyConverter", () => {
  const CurrencyPrice = ({ amount }: { amount: number }) => <span>{amount}</span>;
  return {
    __esModule: true,
    default: ({ amount }: { amount: number }) => <span>{amount}</span>,
    CurrencyPrice,
  };
});

describe("GuestBill", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (getGuestOrdersByBillNumber as jest.Mock).mockResolvedValue({
      orders: [],
      total: 0,
    });
  });

  it("falls back to bill-number lookup when the open bill endpoint no longer returns the bill", async () => {
    const openBill: BillWithItemsResponse = {
      bill: {
        id: 1,
        business_id: 42,
        table_id: 3,
        bill_number: "B42-opaque",
        notes: "",
        items: "[]",
        subtotal: asDollars(30),
        tax_amount: asDollars(0),
        service_fee_amount: asDollars(0),
        total_amount: asDollars(30),
        paid_amount: asDollars(0),
        tip_amount: asDollars(0),
        currency: "USD",
        status: "open",
        settlement_address: "0x123",
        tipping_address: "0x456",
        created_at: "2026-01-01T00:00:00Z",
        updated_at: "2026-01-01T00:00:00Z",
      },
      items: [],
    };

    const closedBill: BillWithItemsResponse = {
      ...openBill,
      bill: {
        ...openBill.bill,
        paid_amount: asDollars(30),
        status: "paid",
      },
    };

    (getOpenBillByTableCode as jest.Mock).mockRejectedValue(
      new Error("open bill not found"),
    );
    (getBillByNumber as jest.Mock).mockResolvedValue(closedBill);

    const onBillUpdate = jest.fn();

    render(
      <GuestBill
        bill={openBill}
        business={{
          id: 42,
          name: "Cafe",
          tax_rate: 0,
          service_fee_rate: 0,
        } as any}
        tableCode="T1"
        selectedLanguage="en"
        onPaymentComplete={jest.fn()}
        onBillUpdate={onBillUpdate}
      />,
    );

    await waitFor(() => {
      expect(getBillByNumber).toHaveBeenCalledWith("B42-opaque");
    });
    expect(onBillUpdate).toHaveBeenCalledWith(closedBill);
  });

  it("prefers refreshing the currently viewed bill by bill number before table-level open bill lookup", async () => {
    const openBill: BillWithItemsResponse = {
      bill: {
        id: 1,
        business_id: 42,
        table_id: 3,
        bill_number: "B42-opaque",
        notes: "",
        items: "[]",
        subtotal: asDollars(30),
        tax_amount: asDollars(0),
        service_fee_amount: asDollars(0),
        total_amount: asDollars(30),
        paid_amount: asDollars(0),
        tip_amount: asDollars(0),
        currency: "USD",
        status: "open",
        settlement_address: "0x123",
        tipping_address: "0x456",
        created_at: "2026-01-01T00:00:00Z",
        updated_at: "2026-01-01T00:00:00Z",
      },
      items: [],
    };

    const paidBill: BillWithItemsResponse = {
      ...openBill,
      bill: {
        ...openBill.bill,
        paid_amount: asDollars(30),
        status: "paid",
      },
    };

    const newerTableBill: BillWithItemsResponse = {
      ...openBill,
      bill: {
        ...openBill.bill,
        id: 2,
        bill_number: "B99-newer",
      },
    };

    (getBillByNumber as jest.Mock).mockResolvedValue(paidBill);
    (getOpenBillByTableCode as jest.Mock).mockResolvedValue(newerTableBill);

    const onBillUpdate = jest.fn();

    render(
      <GuestBill
        bill={openBill}
        business={{
          id: 42,
          name: "Cafe",
          tax_rate: 0,
          service_fee_rate: 0,
        } as any}
        tableCode="T1"
        selectedLanguage="en"
        onPaymentComplete={jest.fn()}
        onBillUpdate={onBillUpdate}
      />,
    );

    await waitFor(() => {
      expect(getBillByNumber).toHaveBeenCalledWith("B42-opaque");
    });
    expect(getOpenBillByTableCode).not.toHaveBeenCalled();
    expect(onBillUpdate).toHaveBeenCalledWith(paidBill);
  });

  it("shows payment controls for partial bills", async () => {
    const partialBill: BillWithItemsResponse = {
      bill: {
        id: 1,
        business_id: 42,
        table_id: 3,
        bill_number: "B42-partial",
        notes: "",
        items: "[]",
        subtotal: asDollars(40),
        tax_amount: asDollars(0),
        service_fee_amount: asDollars(0),
        total_amount: asDollars(40),
        paid_amount: asDollars(10),
        tip_amount: asDollars(0),
        currency: "USD",
        status: "partial",
        settlement_address: "0x123",
        tipping_address: "0x456",
        created_at: "2026-01-01T00:00:00Z",
        updated_at: "2026-01-01T00:00:00Z",
      },
      items: [],
    };

    render(
      <GuestBill
        bill={partialBill}
        business={{
          id: 42,
          name: "Cafe",
          tax_rate: 0,
          service_fee_rate: 0,
        } as any}
        tableCode="T1"
        selectedLanguage="en"
        onPaymentComplete={jest.fn()}
      />,
    );

    await waitFor(() => {
      expect(screen.getByTestId("payment-section")).toBeInTheDocument();
    });
  });

  it("announces a paid bill in a polite live region", async () => {
    const paidBill: BillWithItemsResponse = {
      bill: {
        id: 1,
        business_id: 42,
        table_id: 3,
        bill_number: "B42-paid",
        notes: "",
        items: "[]",
        subtotal: asDollars(30),
        tax_amount: asDollars(0),
        service_fee_amount: asDollars(0),
        total_amount: asDollars(30),
        paid_amount: asDollars(30),
        tip_amount: asDollars(0),
        currency: "USD",
        status: "paid",
        settlement_address: "0x123",
        tipping_address: "0x456",
        created_at: "2026-01-01T00:00:00Z",
        updated_at: "2026-01-01T00:00:00Z",
      },
      items: [],
    };

    render(
      <GuestBill
        bill={paidBill}
        business={{
          id: 42,
          name: "Cafe",
          tax_rate: 0,
          service_fee_rate: 0,
        } as any}
        tableCode="T1"
        selectedLanguage="en"
        onPaymentComplete={jest.fn()}
      />,
    );

    const live = await screen.findByTestId("guest-bill-paid-status");
    expect(live).toHaveAttribute("role", "status");
    expect(live).toHaveAttribute("aria-live", "polite");
    expect(live).toHaveTextContent("bill.paidAnnouncement");
  });

  // W2T9b: /guest/bill/:id path-param reads must use the unguessable
  // public_token capability when the backend provides one (mig 000128),
  // and keep resolving pre-migration bills by bill_number.
  describe("guest bill capability token", () => {
    const baseBill: BillWithItemsResponse = {
      bill: {
        id: 1,
        business_id: 42,
        table_id: 3,
        bill_number: "B42-opaque",
        notes: "",
        items: "[]",
        subtotal: asDollars(30),
        tax_amount: asDollars(0),
        service_fee_amount: asDollars(0),
        total_amount: asDollars(30),
        paid_amount: asDollars(0),
        tip_amount: asDollars(0),
        currency: "USD",
        status: "open",
        settlement_address: "0x123",
        tipping_address: "0x456",
        created_at: "2026-01-01T00:00:00Z",
        updated_at: "2026-01-01T00:00:00Z",
      },
      items: [],
    };

    const renderBill = (bill: BillWithItemsResponse) =>
      render(
        <GuestBill
          bill={bill}
          business={{
            id: 42,
            name: "Cafe",
            tax_rate: 0,
            service_fee_rate: 0,
          } as any}
          tableCode="T1"
          selectedLanguage="en"
          onPaymentComplete={jest.fn()}
          onBillUpdate={jest.fn()}
        />,
      );

    it("uses public_token for /guest/bill reads when present", async () => {
      const token = "a1b2c3d4e5f60718293a4b5c6d7e8f90";
      const tokenBill: BillWithItemsResponse = {
        ...baseBill,
        bill: { ...baseBill.bill, public_token: token },
      };
      (getBillByNumber as jest.Mock).mockResolvedValue(tokenBill);

      renderBill(tokenBill);

      await waitFor(() => {
        expect(getGuestOrdersByBillNumber).toHaveBeenCalledWith(
          token,
          expect.anything(),
        );
      });
      await waitFor(() => {
        expect(getBillByNumber).toHaveBeenCalledWith(token);
      });
    });

    it("falls back to bill_number for pre-migration bills without a token", async () => {
      (getBillByNumber as jest.Mock).mockResolvedValue(baseBill);

      renderBill(baseBill);

      await waitFor(() => {
        expect(getGuestOrdersByBillNumber).toHaveBeenCalledWith(
          "B42-opaque",
          expect.anything(),
        );
      });
      await waitFor(() => {
        expect(getBillByNumber).toHaveBeenCalledWith("B42-opaque");
      });
    });
  });
});
