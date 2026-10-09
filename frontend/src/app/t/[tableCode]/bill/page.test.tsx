/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

jest.mock("next/navigation", () => ({
  useParams: () => ({ tableCode: "T1" }),
}));

jest.mock("next/dynamic", () => {
  const MockGuestBill = () => <div data-testid="guest-bill" />;
  MockGuestBill.displayName = "MockGuestBill";

  return {
    __esModule: true,
    default: () => MockGuestBill,
  };
});

jest.mock("next/link", () => ({
  __esModule: true,
  default: ({
    children,
    href,
  }: {
    children: React.ReactNode;
    href: string;
  }) => <a href={href}>{children}</a>,
}));

jest.mock("@nextui-org/react", () => ({
  Card: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  CardBody: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
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
  const MockPaymentStatusChecker = ({
    paymentId,
    onPaymentConfirmed,
  }: {
    paymentId: string;
    onPaymentConfirmed?: (paymentDetails: {
      totalPaid: number;
      tipAmount: number;
      paymentMethod: string;
      transactionId: string;
      splitShareId?: number;
    }) => void;
  }) => (
    <div data-testid="payment-status-checker">
      <span>{paymentId}</span>
      <button
        type="button"
        onClick={() =>
          onPaymentConfirmed?.({
            totalPaid: 12.5,
            tipAmount: 1,
            paymentMethod: "paypal",
            transactionId: paymentId,
            splitShareId: 42,
          })
        }
      >
        confirm split share
      </button>
    </div>
  );
  MockPaymentStatusChecker.displayName = "MockPaymentStatusChecker";

  return MockPaymentStatusChecker;
});

const mockSetBusinessId = jest.fn();
jest.mock("../../../../i18n/GuestTranslationProvider", () => ({
  GuestTranslationProvider: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
  useGuestTranslation: () => ({
    t: (key: string, vars?: Record<string, unknown>) => {
      if (vars?.tableCode != null) return `${key}-${vars.tableCode}`;
      if (vars?.seconds != null) return `${key}-${vars.seconds}`;
      return key;
    },
    currentLanguage: "en",
    // Stable identity — loadTableData lists setBusinessId in its deps; a new
    // jest.fn() every render would re-fire the load effect in a tight loop.
    setBusinessId: mockSetBusinessId,
  }),
}));

jest.mock("../../../../api/bills", () => {
  class RateLimitCooldownError extends Error {
    retryAfterSeconds: number;
    constructor(retryAfterSeconds: number) {
      super("Request is rate limited");
      this.name = "RateLimitCooldownError";
      this.retryAfterSeconds = retryAfterSeconds;
    }
  }
  return {
    getTableByCode: jest.fn(),
    getOpenBillByTableCode: jest.fn(),
    getBusinessByTableCode: jest.fn(),
    getBillByNumber: jest.fn(),
    getMenuByTableCode: jest.fn(),
    guestBillRef: (b: { bill_number: string; public_token?: string }) =>
      b.public_token || b.bill_number,
    tryGuestBillRef: (b: { bill_number: string; public_token?: string }) =>
      b.public_token?.trim() || b.bill_number || null,
    RateLimitCooldownError,
    parseRetryAfterSeconds: (error: unknown, fallback = 5) => {
      const data = (error as { response?: { data?: { retry_after?: number } } })
        ?.response?.data;
      if (data?.retry_after != null) return Number(data.retry_after);
      return fallback;
    },
  };
});

jest.mock("../../../../api/splitting", () => ({
  SplittingAPI: {
    releaseHeldShare: jest.fn().mockResolvedValue({
      success: true,
      state: {},
      share: { id: 42, status: "released" },
    }),
  },
}));

jest.mock("@/hooks/useGuestBillSync", () => ({
  useGuestBillSync: () => ({ refresh: jest.fn(), connected: false }),
}));

const { default: GuestBillPage } = require("./page");
const {
  getBillByNumber,
  getBusinessByTableCode,
  getOpenBillByTableCode,
  getTableByCode,
  RateLimitCooldownError,
} = require("../../../../api/bills");
const { SplittingAPI } = require("../../../../api/splitting");

describe("GuestBillPage", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    window.history.replaceState(
      {},
      "",
      "/t/T1/bill?payment_id=pay_123&payment_method=paypal&bill_token=B42-opaque",
    );

    (getTableByCode as jest.Mock).mockResolvedValue({
      table: { id: 3, business_id: 42, name: "Table 1" },
      business: { id: 42, name: "Cafe" },
      categories: [],
    });
    (getBusinessByTableCode as jest.Mock).mockResolvedValue({
      business: { id: 42, name: "Cafe", default_currency: "USD", display_currency: "USD" },
      trustpilot_enabled: false,
    });
    (getOpenBillByTableCode as jest.Mock).mockRejectedValue(
      new Error("no open bill"),
    );
    (getBillByNumber as jest.Mock).mockResolvedValue({
      bill: { bill_number: "B42-opaque", total_amount: 50, paid_amount: 50 },
      items: [],
    });
  });

  it("keeps the payment status checker visible when the open bill is already gone on return", async () => {
    render(<GuestBillPage />);

    await waitFor(() => {
      expect(getTableByCode).toHaveBeenCalledWith("T1", "en");
      expect(getBusinessByTableCode).toHaveBeenCalledWith("T1");
      expect(getOpenBillByTableCode).toHaveBeenCalledWith("T1");
      expect(screen.getByTestId("payment-status-checker")).toHaveTextContent("pay_123");
    });
  });

  it("ignores stale stored payment state for a different bill token", async () => {
    sessionStorage.setItem(
      "payverge_payment",
      JSON.stringify({
        billId: 99,
        billToken: "OTHER-BILL",
        paymentId: "pay_stale",
        method: "paypal",
        amount: 19,
        tipAmount: 1,
      }),
    );
    window.history.replaceState(
      {},
      "",
      "/t/T1/bill?payment=success&method=paypal&bill_token=B42-opaque",
    );

    render(<GuestBillPage />);

    await waitFor(() => {
      expect(getTableByCode).toHaveBeenCalledWith("T1", "en");
      expect(getBusinessByTableCode).toHaveBeenCalledWith("T1");
      expect(getOpenBillByTableCode).toHaveBeenCalledWith("T1");
    });

    expect(screen.queryByTestId("payment-status-checker")).toBeNull();
  });

  it("keeps the live bill open after a split-share plugin return is confirmed", async () => {
    sessionStorage.setItem(
      "payverge_payment",
      JSON.stringify({
        billToken: "B42-opaque",
        paymentId: "pay_split",
        method: "paypal",
      }),
    );
    window.history.replaceState(
      {},
      "",
      "/t/T1/bill?payment=success&method=paypal&bill_token=B42-opaque",
    );
    (getOpenBillByTableCode as jest.Mock).mockResolvedValue({
      bill: {
        bill_number: "B42-opaque",
        status: "partial",
        total_amount: 50,
        paid_amount: 12.5,
      },
      items: [],
    });

    render(<GuestBillPage />);

    await waitFor(() => {
      expect(screen.getByTestId("payment-status-checker")).toHaveTextContent(
        "pay_split",
      );
    });

    const openBillCallsBeforeConfirm = (getOpenBillByTableCode as jest.Mock).mock.calls.length;
    fireEvent.click(screen.getByText("confirm split share"));

    await waitFor(() => {
      expect(screen.queryByTestId("payment-status-checker")).toBeNull();
    });
    expect((getOpenBillByTableCode as jest.Mock).mock.calls.length).toBeGreaterThan(
      openBillCallsBeforeConfirm,
    );
    expect(screen.queryByText("bill.thankYouTitle")).toBeNull();
    expect(getBillByNumber).not.toHaveBeenCalled();
    expect(sessionStorage.getItem("payverge_payment")).toBeNull();
  });

  it("releases a held split share when a redirect plugin payment is cancelled", async () => {
    sessionStorage.setItem(
      "payverge_payment",
      JSON.stringify({
        billToken: "B42-opaque",
        paymentId: "pay_split_cancelled",
        method: "paypal",
        splitShareId: 42,
      }),
    );
    window.history.replaceState(
      {},
      "",
      "/t/T1/bill?payment=cancelled&method=paypal&bill_token=B42-opaque",
    );

    render(<GuestBillPage />);

    await waitFor(() => {
      expect(SplittingAPI.releaseHeldShare).toHaveBeenCalledWith("B42-opaque", 42);
    });
    expect(sessionStorage.getItem("payverge_payment")).toBeNull();
  });

  it("resumes payment status check using legacy bill_number query as one-release fallback", async () => {
    window.history.replaceState(
      {},
      "",
      "/t/T1/bill?payment_id=pay_legacy&payment_method=paypal&bill_number=legacy-token-or-number",
    );

    render(<GuestBillPage />);

    await waitFor(() => {
      expect(screen.getByTestId("payment-status-checker")).toHaveTextContent(
        "pay_legacy",
      );
    });
  });
});

describe("GuestBillPage transient table load", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    window.history.replaceState({}, "", "/t/T1/bill");
    (getBusinessByTableCode as jest.Mock).mockResolvedValue({
      business: { id: 42, name: "Cafe" },
      trustpilot_enabled: false,
    });
    (getOpenBillByTableCode as jest.Mock).mockResolvedValue({
      bill: {
        bill_number: "B99",
        status: "open",
        total_amount: 5.82,
        paid_amount: 0,
      },
      items: [],
    });
  });

  it("does not treat a network miss as table-not-found", async () => {
    (getTableByCode as jest.Mock).mockRejectedValue({
      code: "ERR_NETWORK",
      message: "Network Error",
    });

    render(<GuestBillPage />);

    await waitFor(() => {
      expect(screen.getByText("errors.networkError")).toBeInTheDocument();
    });
    expect(screen.queryByText("errors.tableNotFound")).toBeNull();
    expect(
      screen.queryByText("errors.tableNotFoundDescription-T1"),
    ).toBeNull();
  });

  it("retries a one-shot table 404 and renders the live bill", async () => {
    (getTableByCode as jest.Mock)
      .mockRejectedValueOnce({ response: { status: 404 } })
      .mockResolvedValue({
        table: { id: 3, business_id: 42, name: "Table 1", table_code: "T1" },
        business: { id: 42, name: "Cafe" },
        categories: [],
      });

    render(<GuestBillPage />);

    await waitFor(() => {
      expect(getTableByCode).toHaveBeenCalledTimes(2);
    });
    expect(screen.queryByText("errors.tableNotFound")).toBeNull();
    expect(
      screen.queryByText("errors.tableNotFoundDescription-T1"),
    ).toBeNull();
  });

  it("retries a 200 with an empty body instead of rendering Mesa No Encontrada", async () => {
    // An edge/proxy hiccup answers 200 with no body: axios resolves with an
    // empty `data`, so the load never rejects and the 404 retry never fires.
    (getTableByCode as jest.Mock)
      .mockResolvedValueOnce(undefined)
      .mockResolvedValue({
        table: { id: 3, business_id: 42, name: "Table 1", table_code: "T1" },
        business: { id: 42, name: "Cafe" },
        categories: [],
      });

    render(<GuestBillPage />);

    await waitFor(() => {
      expect(getTableByCode).toHaveBeenCalledTimes(2);
    });
    expect(screen.queryByText("errors.tableNotFound")).toBeNull();
    expect(
      screen.queryByText("errors.tableNotFoundDescription-T1"),
    ).toBeNull();
  });

  it("never claims the code is missing without a confirmed 404", async () => {
    (getTableByCode as jest.Mock).mockResolvedValue(undefined);

    render(<GuestBillPage />);

    await waitFor(() => {
      expect(screen.getByText("errors.networkError")).toBeInTheDocument();
    });
    expect(screen.queryByText("errors.tableNotFound")).toBeNull();
    expect(
      screen.queryByText("errors.tableNotFoundDescription-T1"),
    ).toBeNull();
  });
});

describe("GuestBillPage 429 cooldown (REV-4)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    window.history.replaceState({}, "", "/t/T1/bill");
    (getBusinessByTableCode as jest.Mock).mockResolvedValue({
      business: {
        id: 42,
        name: "Cafe",
        default_currency: "USD",
        display_currency: "USD",
      },
      trustpilot_enabled: false,
    });
  });

  it(
    "shows localized countdown card on 429 and auto-retries when timer hits 0",
    async () => {
      (getTableByCode as jest.Mock)
        .mockRejectedValueOnce(new RateLimitCooldownError(1))
        .mockResolvedValue({
          table: { id: 3, business_id: 42, name: "Table 1" },
          business: { id: 42, name: "Cafe" },
          categories: [],
        });
      (getOpenBillByTableCode as jest.Mock)
        .mockRejectedValueOnce(new RateLimitCooldownError(1))
        .mockResolvedValue({
          bill: {
            bill_number: "B99",
            public_token: "tok-99",
            status: "open",
            total_amount: 20,
            paid_amount: 0,
          },
          items: [],
        });

      render(<GuestBillPage />);

      await waitFor(() => {
        expect(
          screen.getByTestId("bill-rate-limit-cooldown"),
        ).toBeInTheDocument();
      });
      expect(screen.getByText("errors.billBusyTitle")).toBeInTheDocument();
      expect(screen.getByTestId("bill-rate-limit-countdown")).toHaveTextContent(
        /errors\.billBusyRetryIn-/,
      );

      await waitFor(
        () => {
          expect(screen.queryByTestId("bill-rate-limit-cooldown")).toBeNull();
        },
        { timeout: 4000 },
      );
      expect(
        (getTableByCode as jest.Mock).mock.calls.length,
      ).toBeGreaterThanOrEqual(2);
    },
    10000,
  );

  it(
    "stops auto-retry after the cap and shows manual-retry-only state",
    async () => {
      (getTableByCode as jest.Mock).mockRejectedValue(
        new RateLimitCooldownError(1),
      );
      (getOpenBillByTableCode as jest.Mock).mockRejectedValue(
        new RateLimitCooldownError(1),
      );

      render(<GuestBillPage />);

      await waitFor(() => {
        expect(
          screen.getByTestId("bill-rate-limit-cooldown"),
        ).toBeInTheDocument();
      });

      // Wall-clock: 3 auto windows of 1s + load fan-out. Poll until manual-only.
      await waitFor(
        () => {
          const el = screen.queryByTestId("bill-rate-limit-cooldown");
          expect(el).not.toBeNull();
          expect(el).toHaveAttribute("data-manual-only", "true");
        },
        { timeout: 12000 },
      );
      expect(screen.queryByTestId("bill-rate-limit-countdown")).toBeNull();
      expect(
        screen.getByTestId("bill-rate-limit-manual-only"),
      ).toBeInTheDocument();

      const callsAtCap = (getTableByCode as jest.Mock).mock.calls.length;
      await new Promise((r) => setTimeout(r, 1500));
      expect((getTableByCode as jest.Mock).mock.calls.length).toBe(callsAtCap);
    },
    20000,
  );
});
