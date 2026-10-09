/** @jest-environment jsdom */
import React from "react";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { asDollars } from "@/types/money";
import GuestBillSplitPanel from "./GuestBillSplitPanel";
import { SplittingAPI } from "@/api/splitting";
import { requestAlternativePayment } from "@/api/alternativePayments";
import { PaymentMethod } from "@/types/alternativePayments";
import fs from "fs";
import path from "path";

jest.mock("@/api/splitting", () => ({
  __esModule: true,
  ...(() => {
    const api = {
      createSplitHold: jest.fn(),
      executeHeldShare: jest.fn(),
      getSplitShareReceipt: jest.fn(),
      getMySplitShares: jest.fn(),
      releaseHeldShare: jest.fn(),
    };
    return {
      SplittingAPI: api,
      useSplittingAPI: () => api,
      getSplitEventsURL: (billToken: string) => `/split-events/${billToken}`,
    };
  })(),
}));

jest.mock("@/api/alternativePayments", () => ({
  requestAlternativePayment: jest.fn(),
}));

jest.mock("../guest/PaymentSection", () => function MockPaymentSection(props: {
  amount: number;
  tipBaseAmount?: number;
  onCashierPayment?: (paymentDetails: { tipAmount: number }) => void;
  onPaymentComplete: (paymentDetails: {
    totalPaid: number;
    tipAmount: number;
    paymentMethod: string;
    transactionId?: string;
  }) => void;
}) {
  return (
    <div>
      <span data-testid="share-tip-base">
        {props.tipBaseAmount === undefined ? "" : props.tipBaseAmount.toFixed(2)}
      </span>
      <button
        type="button"
        data-testid="share-payment"
        onClick={() =>
          props.onPaymentComplete({
            totalPaid: props.amount,
            tipAmount: 1.25,
            paymentMethod: "usdc_payment",
            transactionId: "0xtx",
          })
        }
      >
        Share payment {props.amount.toFixed(2)}
      </button>
      <button
        type="button"
        data-testid="share-cashier"
        onClick={() => props.onCashierPayment?.({ tipAmount: 1.25 })}
      >
        Cashier
      </button>
    </div>
  );
});

// Controllable guest locale so a test can assert money is formatted in the
// diner's locale; undefined (the default) keeps every other test on en-US.
let mockSplitLocale: string | undefined;

jest.mock("../../i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({
    t: (key: string, values?: Record<string, unknown>) => {
      const map: Record<string, string> = {
        "bill.splitBill": "Split Bill",
        "bill.splitChooseMethod": "Choose how to split",
        "bill.splitEqually": "Split equally",
        "bill.splitEquallyDescription": "Divide the remaining balance into equal shares.",
        "bill.splitPeopleCount": "People",
        "bill.splitSharesCovered": "Shares you cover",
        "bill.splitPeopleOutOfRange": "Enter between {min} and {max} people.",
        "bill.splitSharesOutOfRange": "You can cover between {min} and {max} shares.",
        "bill.splitItems": "Pay for my items",
        "bill.splitItemsDescription": "Claim the lines you ate.",
        "bill.splitItemFraction": "Share",
        "bill.splitCustomFraction": "Custom fraction",
        "bill.splitCustomFractionPlaceholder": "2/5",
        "bill.splitItemUnavailable": "Already claimed",
        "bill.splitItemRemaining": "{fraction} available",
        "bill.splitItemLineTotal": "Line total",
        "bill.splitYourShareWithTax": "Your share (with tax)",
        "bill.splitYourShareRemaining": "Your share of the remaining balance",
        "bill.splitPriorPaymentNote":
          "Previous payments of {amount} have already been applied proportionally.",
        "bill.splitSelectedItemsTotal": "Selected items",
        "bill.splitCustom": "Pay a custom amount",
        "bill.splitCustomDescription": "Choose exactly what you want to cover.",
        "bill.splitAmountLabel": "Amount",
        "bill.splitAmountOverRemaining":
          "That’s more than the remaining balance — your share is capped to what’s left.",
        "bill.splitAmountNonPositive": "Enter a positive amount to hold.",
        "bill.splitYourShare": "Your share",
        "bill.splitDisplayNameLabel": "Name for this share (optional)",
        "bill.splitDisplayNamePlaceholder": "Sara",
        "bill.splitPayRemaining": "Pay the remaining balance",
        "bill.splitHoldShare": "Hold my share",
        "bill.splitReleaseShare": "Release my share",
        "bill.splitShareReleased": "Your share was released.",
        "bill.splitShareReady": "Your share is ready",
        "bill.splitRemainingAfterYou": "Remaining after you: {amount}",
        "bill.splitHoldCountdown": "Still there? This hold releases in {time}.",
        "bill.splitRoster": "Who's paying",
        "bill.splitGuestFallback": "Guest {id}",
        "bill.splitStatusHeld": "Held",
        "bill.splitStatusPaying": "Paying now",
        "bill.splitStatusSettled": "Paid",
        "bill.splitReceiptTitle": "Your receipt",
        "bill.splitReceiptSubtitle": "This share is paid.",
        "bill.splitReceiptItems": "Items",
        "bill.splitReceiptSubtotal": "Subtotal",
        "bill.splitReceiptTax": "Tax",
        "bill.splitReceiptService": "Service",
        "bill.splitReceiptTip": "Tip",
        "bill.splitReceiptTotalPaid": "Total paid",
        "bill.splitReceiptTender": "Tender",
        "bill.splitNewShare": "Split another share",
      };
      const template = map[key] ?? key;
      return Object.entries(values ?? {}).reduce(
        (text, [name, value]) => text.replace(`{${name}}`, String(value)),
        template,
      );
    },
    currentLanguage: mockSplitLocale,
  }),
}));

describe("GuestBillSplitPanel", () => {
  it("formats the live split progress in the diner's locale", async () => {
    mockSplitLocale = "de";
    const originalEventSource = globalThis.EventSource;
    const sources: Array<{ emit: (type: string, payload: unknown) => void }> = [];

    class MockEventSource {
      static readonly CONNECTING = 0;
      static readonly OPEN = 1;
      static readonly CLOSED = 2;
      readyState = 1;
      withCredentials = false;
      onerror: ((event: Event) => void) | null = null;
      onmessage: ((event: MessageEvent) => void) | null = null;
      onopen: ((event: Event) => void) | null = null;
      close = jest.fn();
      url: string;
      private listeners = new Map<string, Array<(event: MessageEvent) => void>>();
      constructor(url: string) {
        this.url = url;
        sources.push(this);
      }
      addEventListener(type: string, listener: (event: MessageEvent) => void) {
        const existing = this.listeners.get(type) ?? [];
        existing.push(listener);
        this.listeners.set(type, existing);
      }
      removeEventListener(type: string, listener: (event: MessageEvent) => void) {
        const existing = this.listeners.get(type) ?? [];
        this.listeners.set(
          type,
          existing.filter((candidate) => candidate !== listener),
        );
      }
      dispatchEvent() {
        return true;
      }
      emit(type: string, payload: unknown) {
        const event = { data: JSON.stringify(payload) } as MessageEvent;
        for (const listener of this.listeners.get(type) ?? []) {
          listener(event);
        }
      }
    }

    Object.defineProperty(globalThis, "EventSource", {
      configurable: true,
      value: MockEventSource,
    });

    try {
      render(
        <GuestBillSplitPanel
          billId={1}
          billToken="B42"
          businessId={2}
          businessName="Test Resto"
          businessAddress="0xsettlement"
          tipAddress="0xtip"
          tableCode="T1"
          remainingAmount={asDollars(50)}
          defaultCurrency="USD"
          displayCurrency="USD"
          items={[]}
          onPaymentComplete={jest.fn()}
          onBillRefresh={jest.fn()}
        />,
      );

      await userEvent.click(screen.getByRole("button", { name: "Split Bill" }));
      act(() => {
        sources[0].emit("connected", {
          state: {
            bill_number: "B42",
            status: "partial",
            total_amount: asDollars(50),
            total_cents: 5000,
            paid_amount: asDollars(20),
            paid_cents: 2000,
            held_amount: asDollars(0),
            held_cents: 0,
            available_amount: asDollars(30),
            available_cents: 3000,
            updated_at: "2026-06-13T12:02:00Z",
            shares: [],
          },
        });
      });

      // The live progress renders CurrencyPrice for paid/total;
      // German uses a comma decimal ("20,00 $" / "50,00 $"), which en-US
      // ("$20.00" / "$50.00") never produces.
      expect(await screen.findByText(/50,00/)).toBeInTheDocument();
      expect(screen.getByText(/20,00/)).toBeInTheDocument();
    } finally {
      Object.defineProperty(globalThis, "EventSource", {
        configurable: true,
        value: originalEventSource,
      });
    }
  });

  beforeEach(() => {
    jest.clearAllMocks();
    mockSplitLocale = undefined;
    (SplittingAPI.getMySplitShares as jest.Mock).mockResolvedValue([]);
    (SplittingAPI.releaseHeldShare as jest.Mock).mockResolvedValue({
      success: true,
      share: {
        id: 4,
        mode: "custom",
        amount: asDollars(12.5),
        amount_cents: 1250,
        status: "released",
      },
      state: {
        bill_number: "B42",
        status: "open",
        total_amount: asDollars(50),
        total_cents: 5000,
        paid_amount: asDollars(0),
        paid_cents: 0,
        held_amount: asDollars(0),
        held_cents: 0,
        available_amount: asDollars(50),
        available_cents: 5000,
        updated_at: "2026-06-13T12:01:00Z",
        shares: [],
      },
    });
  });

  it("creates a custom hold and renders per-share payment", async () => {
    (SplittingAPI.createSplitHold as jest.Mock).mockResolvedValue({
      success: true,
      share: {
        id: 4,
        mode: "custom",
        amount: asDollars(12.5),
        amount_cents: 1250,
        status: "held",
      },
      state: {
        bill_number: "B42",
        status: "open",
        total_amount: asDollars(50),
        total_cents: 5000,
        paid_amount: asDollars(0),
        paid_cents: 0,
        held_amount: asDollars(12.5),
        held_cents: 1250,
        available_amount: asDollars(37.5),
        available_cents: 3750,
        updated_at: "2026-06-13T12:00:00Z",
        shares: [],
      },
    });

    render(
      <GuestBillSplitPanel
        billId={1}
        billToken="B42"
        businessId={2}
        businessName="Test Resto"
        businessAddress="0xsettlement"
        tipAddress="0xtip"
        tableCode="T1"
        remainingAmount={asDollars(50)}
        defaultCurrency="USD"
        displayCurrency="USD"
        items={[]}
        onPaymentComplete={jest.fn()}
        onBillRefresh={jest.fn()}
      />,
    );

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Split Bill" }));
    await user.click(screen.getByRole("button", { name: "Pay a custom amount" }));
    await user.type(screen.getByLabelText("Amount"), "12.50");
    await user.click(screen.getByRole("button", { name: "Hold my share" }));

    await waitFor(() => {
      expect(SplittingAPI.createSplitHold).toHaveBeenCalledWith(
        "B42",
        expect.objectContaining({ mode: "custom", amount: asDollars(12.5) }),
      );
    });
    expect(await screen.findByText("Your share is ready")).toBeInTheDocument();
    expect(screen.getByTestId("share-payment")).toHaveTextContent("12.50");
    // Remaining after you = total - paid - share (50 - 0 - 12.50), NOT
    // available - share (which double-counts and shows $0).
    expect(
      screen.getByText((content) => content.includes("Remaining after you")),
    ).toHaveTextContent(/37\.50|37,50/);
  });

  it("passes the pre-tax share amount as the payment sheet tip base", async () => {
    (SplittingAPI.createSplitHold as jest.Mock).mockResolvedValue({
      success: true,
      share: {
        id: 4,
        mode: "custom",
        amount: asDollars(50),
        amount_cents: 5000,
        status: "held",
      },
      state: {
        bill_number: "B42",
        status: "open",
        total_amount: asDollars(100),
        total_cents: 10000,
        paid_amount: asDollars(0),
        paid_cents: 0,
        held_amount: asDollars(50),
        held_cents: 5000,
        available_amount: asDollars(50),
        available_cents: 5000,
        updated_at: "2026-06-13T12:00:00Z",
        shares: [],
      },
    });

    render(
      <GuestBillSplitPanel
        billId={1}
        billToken="B42"
        businessId={2}
        businessName="Test Resto"
        businessAddress="0xsettlement"
        tipAddress="0xtip"
        tableCode="T1"
        remainingAmount={asDollars(100)}
        billSubtotal={asDollars(80)}
        billTaxAmount={asDollars(8)}
        billServiceFeeAmount={asDollars(12)}
        defaultCurrency="USD"
        displayCurrency="USD"
        items={[]}
        onPaymentComplete={jest.fn()}
        onBillRefresh={jest.fn()}
      />,
    );

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Split Bill" }));
    await user.click(screen.getByRole("button", { name: "Pay a custom amount" }));
    await user.type(screen.getByLabelText("Amount"), "50.00");
    await user.click(screen.getByRole("button", { name: "Hold my share" }));

    expect(await screen.findByTestId("share-tip-base")).toHaveTextContent("40.00");
  });

  it("submits an optional guest display name with the held share", async () => {
    (SplittingAPI.createSplitHold as jest.Mock).mockResolvedValue({
      success: true,
      share: {
        id: 4,
        display_name: "Sara",
        mode: "custom",
        amount: asDollars(12.5),
        amount_cents: 1250,
        status: "held",
      },
      state: {
        bill_number: "B42",
        status: "open",
        total_amount: asDollars(50),
        total_cents: 5000,
        paid_amount: asDollars(0),
        paid_cents: 0,
        held_amount: asDollars(12.5),
        held_cents: 1250,
        available_amount: asDollars(37.5),
        available_cents: 3750,
        updated_at: "2026-06-13T12:00:00Z",
        shares: [],
      },
    });

    render(
      <GuestBillSplitPanel
        billId={1}
        billToken="B42"
        businessId={2}
        businessName="Test Resto"
        businessAddress="0xsettlement"
        tipAddress="0xtip"
        tableCode="T1"
        remainingAmount={asDollars(50)}
        defaultCurrency="USD"
        displayCurrency="USD"
        items={[]}
        onPaymentComplete={jest.fn()}
        onBillRefresh={jest.fn()}
      />,
    );

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Split Bill" }));
    await user.type(screen.getByLabelText("Name for this share (optional)"), "Sara");
    await user.click(screen.getByRole("button", { name: "Pay a custom amount" }));
    await user.type(screen.getByLabelText("Amount"), "12.50");
    await user.click(screen.getByRole("button", { name: "Hold my share" }));

    await waitFor(() => {
      expect(SplittingAPI.createSplitHold).toHaveBeenCalledWith(
        "B42",
        expect.objectContaining({
          display_name: "Sara",
          mode: "custom",
          amount: asDollars(12.5),
        }),
      );
    });
  });

  it("holds the exact remaining balance from the custom fast path", async () => {
    (SplittingAPI.createSplitHold as jest.Mock).mockResolvedValue({
      success: true,
      share: {
        id: 4,
        mode: "custom",
        amount: asDollars(50),
        amount_cents: 5000,
        status: "held",
      },
      state: {
        bill_number: "B42",
        status: "open",
        total_amount: asDollars(50),
        total_cents: 5000,
        paid_amount: asDollars(0),
        paid_cents: 0,
        held_amount: asDollars(50),
        held_cents: 5000,
        available_amount: asDollars(0),
        available_cents: 0,
        updated_at: "2026-06-13T12:00:00Z",
        shares: [],
      },
    });

    render(
      <GuestBillSplitPanel
        billId={1}
        billToken="B42"
        businessId={2}
        businessName="Test Resto"
        businessAddress="0xsettlement"
        tipAddress="0xtip"
        tableCode="T1"
        remainingAmount={asDollars(50)}
        defaultCurrency="USD"
        displayCurrency="USD"
        items={[]}
        onPaymentComplete={jest.fn()}
        onBillRefresh={jest.fn()}
      />,
    );

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Split Bill" }));
    await user.click(screen.getByRole("button", { name: "Pay a custom amount" }));
    await user.click(screen.getByRole("button", { name: "Pay the remaining balance" }));
    await user.click(screen.getByRole("button", { name: "Hold my share" }));

    await waitFor(() => {
      expect(SplittingAPI.createSplitHold).toHaveBeenCalled();
    });
    const payload = (SplittingAPI.createSplitHold as jest.Mock).mock.calls[0][1];
    expect(payload).toEqual(
      expect.objectContaining({
        mode: "custom",
        cover_remaining: true,
      }),
    );
    expect(payload.amount).toBeUndefined();
  });

  it("caps oversized custom amounts to the available balance before holding", async () => {
    (SplittingAPI.createSplitHold as jest.Mock).mockResolvedValue({
      success: true,
      share: {
        id: 4,
        mode: "custom",
        amount: asDollars(10),
        amount_cents: 1000,
        status: "held",
      },
      state: {
        bill_number: "B42",
        status: "open",
        total_amount: asDollars(50),
        total_cents: 5000,
        paid_amount: asDollars(40),
        paid_cents: 4000,
        held_amount: asDollars(10),
        held_cents: 1000,
        available_amount: asDollars(0),
        available_cents: 0,
        updated_at: "2026-06-13T12:00:00Z",
        shares: [],
      },
    });

    render(
      <GuestBillSplitPanel
        billId={1}
        billToken="B42"
        businessId={2}
        businessName="Test Resto"
        businessAddress="0xsettlement"
        tipAddress="0xtip"
        tableCode="T1"
        remainingAmount={asDollars(10)}
        defaultCurrency="USD"
        displayCurrency="USD"
        items={[]}
        onPaymentComplete={jest.fn()}
        onBillRefresh={jest.fn()}
      />,
    );

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Split Bill" }));
    await user.click(screen.getByRole("button", { name: "Pay a custom amount" }));
    await user.type(screen.getByLabelText("Amount"), "20.00");
    await user.click(screen.getByRole("button", { name: "Hold my share" }));

    await waitFor(() => {
      expect(SplittingAPI.createSplitHold).toHaveBeenCalledWith(
        "B42",
        expect.objectContaining({ mode: "custom", amount: asDollars(10) }),
      );
    });
  });

  it("refuses equal hold when people count is out of range instead of silent-clamping", async () => {
    // PG-30: previously typed 150/125 were clamped to 100/100 on hold. Now the
    // values stay visible, hold is disabled, and createSplitHold is not called.
    render(
      <GuestBillSplitPanel
        billId={1}
        billToken="B42"
        businessId={2}
        businessName="Test Resto"
        businessAddress="0xsettlement"
        tipAddress="0xtip"
        tableCode="T1"
        remainingAmount={asDollars(100)}
        defaultCurrency="USD"
        displayCurrency="USD"
        items={[]}
        onPaymentComplete={jest.fn()}
        onBillRefresh={jest.fn()}
      />,
    );

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Split Bill" }));
    await user.clear(screen.getByLabelText("People"));
    await user.type(screen.getByLabelText("People"), "150");
    await user.clear(screen.getByLabelText("Shares you cover"));
    await user.type(screen.getByLabelText("Shares you cover"), "125");

    expect(screen.getByLabelText("People")).toHaveValue(150);
    expect(screen.getByRole("button", { name: "Hold my share" })).toBeDisabled();
    expect(SplittingAPI.createSplitHold).not.toHaveBeenCalled();
  });

  it("shows a countdown and retires the local held share when it expires", async () => {
    jest.useFakeTimers();
    jest.setSystemTime(new Date("2026-06-13T12:00:00Z"));
    const onBillRefresh = jest.fn();
    (SplittingAPI.createSplitHold as jest.Mock).mockResolvedValue({
      success: true,
      share: {
        id: 4,
        mode: "custom",
        amount: asDollars(12.5),
        amount_cents: 1250,
        status: "held",
        hold_expires_at: "2026-06-13T12:02:00Z",
      },
      state: {
        bill_number: "B42",
        status: "open",
        total_amount: asDollars(50),
        total_cents: 5000,
        paid_amount: asDollars(0),
        paid_cents: 0,
        held_amount: asDollars(12.5),
        held_cents: 1250,
        available_amount: asDollars(37.5),
        available_cents: 3750,
        updated_at: "2026-06-13T12:00:00Z",
        shares: [],
      },
    });

    try {
      render(
        <GuestBillSplitPanel
          billId={1}
          billToken="B42"
          businessId={2}
          businessName="Test Resto"
          businessAddress="0xsettlement"
          tipAddress="0xtip"
          tableCode="T1"
          remainingAmount={asDollars(50)}
          defaultCurrency="USD"
          displayCurrency="USD"
          items={[]}
          onPaymentComplete={jest.fn()}
          onBillRefresh={onBillRefresh}
        />,
      );

      const user = userEvent.setup({ advanceTimers: jest.advanceTimersByTime });
      await user.click(screen.getByRole("button", { name: "Split Bill" }));
      await user.click(screen.getByRole("button", { name: "Pay a custom amount" }));
      await user.type(screen.getByLabelText("Amount"), "12.50");
      await user.click(screen.getByRole("button", { name: "Hold my share" }));

      expect(
        await screen.findByText("Still there? This hold releases in 2:00."),
      ).toBeInTheDocument();

      await act(async () => {
        jest.advanceTimersByTime(120_000);
      });

      await waitFor(() => {
        expect(screen.queryByText("Your share is ready")).not.toBeInTheDocument();
      });
      expect(screen.getByRole("button", { name: "Hold my share" })).toBeInTheDocument();
      expect(onBillRefresh).toHaveBeenCalled();
    } finally {
      jest.useRealTimers();
    }
  });

  it("creates an item hold with fractional claims and no client amount", async () => {
    (SplittingAPI.createSplitHold as jest.Mock).mockResolvedValue({
      success: true,
      share: {
        id: 8,
        mode: "items",
        amount: asDollars(17.25),
        amount_cents: 1725,
        status: "held",
        claimed_item_ids: ["shared-bottle"],
        claimed_fractions: { "shared-bottle": "1/2" },
      },
      state: {
        bill_number: "B42",
        status: "open",
        total_amount: asDollars(50),
        total_cents: 5000,
        paid_amount: asDollars(0),
        paid_cents: 0,
        held_amount: asDollars(17.25),
        held_cents: 1725,
        available_amount: asDollars(32.75),
        available_cents: 3275,
        updated_at: "2026-06-13T12:00:00Z",
        shares: [],
      },
    });

    render(
      <GuestBillSplitPanel
        billId={1}
        billToken="B42"
        businessId={2}
        businessName="Test Resto"
        businessAddress="0xsettlement"
        tipAddress="0xtip"
        tableCode="T1"
        remainingAmount={asDollars(50)}
        defaultCurrency="USD"
        displayCurrency="USD"
        items={[
          {
            id: "shared-bottle",
            name: "Shared Bottle",
            price: asDollars(34.5),
            quantity: 1,
            subtotal: asDollars(34.5),
          },
        ]}
        onPaymentComplete={jest.fn()}
        onBillRefresh={jest.fn()}
      />,
    );

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Split Bill" }));
    await user.click(screen.getByRole("button", { name: "Pay for my items" }));
    await user.selectOptions(screen.getByLabelText("Share"), "1/2");
    await user.click(screen.getByRole("button", { name: /Shared Bottle/ }));
    await user.click(screen.getByRole("button", { name: "Hold my share" }));

    await waitFor(() => {
      expect(SplittingAPI.createSplitHold).toHaveBeenCalledWith(
        "B42",
        expect.not.objectContaining({ amount: expect.anything() }),
      );
      expect(SplittingAPI.createSplitHold).toHaveBeenCalledWith(
        "B42",
        expect.objectContaining({
          mode: "items",
          claimed_item_ids: ["shared-bottle"],
          claimed_fractions: { "shared-bottle": "1/2" },
        }),
      );
    });
    expect(await screen.findByText("Your share is ready")).toBeInTheDocument();
    expect(screen.getByTestId("share-payment")).toHaveTextContent("17.25");
  });

  it("creates an item hold with a custom fractional claim", async () => {
    (SplittingAPI.createSplitHold as jest.Mock).mockResolvedValue({
      success: true,
      share: {
        id: 18,
        mode: "items",
        amount: asDollars(13.8),
        amount_cents: 1380,
        status: "held",
        claimed_item_ids: ["shared-bottle"],
        claimed_fractions: { "shared-bottle": "2/5" },
      },
      state: {
        bill_number: "B42",
        status: "open",
        total_amount: asDollars(50),
        total_cents: 5000,
        paid_amount: asDollars(0),
        paid_cents: 0,
        held_amount: asDollars(13.8),
        held_cents: 1380,
        available_amount: asDollars(36.2),
        available_cents: 3620,
        updated_at: "2026-06-13T12:00:00Z",
        shares: [],
      },
    });

    render(
      <GuestBillSplitPanel
        billId={1}
        billToken="B42"
        businessId={2}
        businessName="Test Resto"
        businessAddress="0xsettlement"
        tipAddress="0xtip"
        tableCode="T1"
        remainingAmount={asDollars(50)}
        defaultCurrency="USD"
        displayCurrency="USD"
        items={[
          {
            id: "shared-bottle",
            name: "Shared Bottle",
            price: asDollars(34.5),
            quantity: 1,
            subtotal: asDollars(34.5),
          },
        ]}
        onPaymentComplete={jest.fn()}
        onBillRefresh={jest.fn()}
      />,
    );

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Split Bill" }));
    await user.click(screen.getByRole("button", { name: "Pay for my items" }));
    await user.selectOptions(screen.getByLabelText("Share"), "custom");
    await user.clear(screen.getByLabelText("Custom fraction"));
    await user.type(screen.getByLabelText("Custom fraction"), "2/5");
    await user.click(screen.getByRole("button", { name: /Shared Bottle/ }));
    await user.click(screen.getByRole("button", { name: "Hold my share" }));

    await waitFor(() => {
      expect(SplittingAPI.createSplitHold).toHaveBeenCalledWith(
        "B42",
        expect.objectContaining({
          mode: "items",
          claimed_item_ids: ["shared-bottle"],
          claimed_fractions: { "shared-bottle": "2/5" },
        }),
      );
    });
  });

  it("previews item shares with proportional tax and service included", async () => {
    render(
      <GuestBillSplitPanel
        billId={1}
        billToken="B42"
        businessId={2}
        businessName="Test Resto"
        businessAddress="0xsettlement"
        tipAddress="0xtip"
        tableCode="T1"
        remainingAmount={asDollars(12)}
        billSubtotal={asDollars(10)}
        billTaxAmount={asDollars(1)}
        billServiceFeeAmount={asDollars(1)}
        defaultCurrency="USD"
        displayCurrency="USD"
        items={[
          {
            id: "entree",
            name: "Entree",
            price: asDollars(10),
            quantity: 1,
            subtotal: asDollars(10),
          },
        ]}
        onPaymentComplete={jest.fn()}
        onBillRefresh={jest.fn()}
      />,
    );

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Split Bill" }));
    await user.click(screen.getByRole("button", { name: "Pay for my items" }));
    await user.click(screen.getByRole("button", { name: /Entree/ }));

    expect(screen.getByText("$12.00")).toBeInTheDocument();
  });

  it("limits an item claim to the live remaining fraction", async () => {
    const originalEventSource = globalThis.EventSource;
    const sources: Array<{
      emit: (type: string, payload: unknown) => void;
      close: jest.Mock;
    }> = [];

    class MockEventSource {
      static readonly CONNECTING = 0;
      static readonly OPEN = 1;
      static readonly CLOSED = 2;
      readonly CONNECTING = 0;
      readonly OPEN = 1;
      readonly CLOSED = 2;
      url: string;
      withCredentials = false;
      readyState = 1;
      onerror: ((event: Event) => void) | null = null;
      onmessage: ((event: MessageEvent) => void) | null = null;
      onopen: ((event: Event) => void) | null = null;
      close = jest.fn();
      private listeners = new Map<string, Array<(event: MessageEvent) => void>>();

      constructor(url: string) {
        this.url = url;
        sources.push(this);
      }

      addEventListener(type: string, listener: (event: MessageEvent) => void) {
        const existing = this.listeners.get(type) ?? [];
        existing.push(listener);
        this.listeners.set(type, existing);
      }

      removeEventListener(type: string, listener: (event: MessageEvent) => void) {
        const existing = this.listeners.get(type) ?? [];
        this.listeners.set(
          type,
          existing.filter((candidate) => candidate !== listener),
        );
      }

      dispatchEvent() {
        return true;
      }

      emit(type: string, payload: unknown) {
        const event = { data: JSON.stringify(payload) } as MessageEvent;
        for (const listener of this.listeners.get(type) ?? []) {
          listener(event);
        }
      }
    }

    Object.defineProperty(globalThis, "EventSource", {
      configurable: true,
      value: MockEventSource,
    });

    (SplittingAPI.createSplitHold as jest.Mock).mockResolvedValue({
      success: true,
      share: {
        id: 8,
        mode: "items",
        amount: asDollars(17.25),
        amount_cents: 1725,
        status: "held",
        claimed_item_ids: ["shared-bottle"],
        claimed_fractions: { "shared-bottle": "1/2" },
      },
      state: {
        bill_number: "B42",
        status: "open",
        total_amount: asDollars(50),
        total_cents: 5000,
        paid_amount: asDollars(0),
        paid_cents: 0,
        held_amount: asDollars(34.5),
        held_cents: 3450,
        available_amount: asDollars(15.5),
        available_cents: 1550,
        updated_at: "2026-06-13T12:00:00Z",
        shares: [],
      },
    });

    try {
      render(
        <GuestBillSplitPanel
          billId={1}
          billToken="B42"
          businessId={2}
          businessName="Test Resto"
          businessAddress="0xsettlement"
          tipAddress="0xtip"
          tableCode="T1"
          remainingAmount={asDollars(50)}
          defaultCurrency="USD"
          displayCurrency="USD"
          items={[
            {
              id: "shared-bottle",
              name: "Shared Bottle",
              price: asDollars(34.5),
              quantity: 1,
              subtotal: asDollars(34.5),
            },
          ]}
          onPaymentComplete={jest.fn()}
          onBillRefresh={jest.fn()}
        />,
      );

      const user = userEvent.setup();
      await user.click(screen.getByRole("button", { name: "Split Bill" }));
      act(() => {
        sources[0].emit("connected", {
          state: {
            bill_number: "B42",
            status: "open",
            total_amount: asDollars(50),
            total_cents: 5000,
            paid_amount: asDollars(0),
            paid_cents: 0,
            held_amount: asDollars(17.25),
            held_cents: 1725,
            available_amount: asDollars(32.75),
            available_cents: 3275,
            updated_at: "2026-06-13T12:00:00Z",
            shares: [
              {
                id: 7,
                display_name: "Guest 7",
                mode: "items",
                amount: asDollars(17.25),
                amount_cents: 1725,
                status: "held",
                claimed_item_ids: ["shared-bottle"],
                claimed_fractions: { "shared-bottle": "1/2" },
              },
            ],
          },
        });
      });
      await user.click(screen.getByRole("button", { name: "Pay for my items" }));

      expect(await screen.findByText("1/2 available")).toBeInTheDocument();
      await user.click(screen.getByRole("button", { name: /Shared Bottle/ }));
      await user.click(screen.getByRole("button", { name: "Hold my share" }));

      await waitFor(() => {
        expect(SplittingAPI.createSplitHold).toHaveBeenCalledWith(
          "B42",
          expect.objectContaining({
            mode: "items",
            claimed_item_ids: ["shared-bottle"],
            claimed_fractions: { "shared-bottle": "1/2" },
          }),
        );
      });
    } finally {
      Object.defineProperty(globalThis, "EventSource", {
        configurable: true,
        value: originalEventSource,
      });
    }
  });

  it("renders a per-share receipt after successful payment settlement", async () => {
    const onPaymentComplete = jest.fn();
    const onBillRefresh = jest.fn();
    (SplittingAPI.createSplitHold as jest.Mock).mockResolvedValue({
      success: true,
      share: {
        id: 4,
        mode: "custom",
        amount: asDollars(12.5),
        amount_cents: 1250,
        status: "held",
      },
      state: {
        bill_number: "B42",
        status: "open",
        total_amount: asDollars(50),
        total_cents: 5000,
        paid_amount: asDollars(0),
        paid_cents: 0,
        held_amount: asDollars(12.5),
        held_cents: 1250,
        available_amount: asDollars(37.5),
        available_cents: 3750,
        updated_at: "2026-06-13T12:00:00Z",
        shares: [],
      },
    });
    (SplittingAPI.executeHeldShare as jest.Mock).mockResolvedValue({
      success: true,
      applied: true,
      bill_status: "partial",
      remaining_amount: asDollars(37.5),
      remaining_cents: 3750,
      share: {
        id: 4,
        mode: "custom",
        amount: asDollars(12.5),
        amount_cents: 1250,
        tip_amount: asDollars(1.25),
        tip_cents: 125,
        status: "settled",
        tender: "crypto",
      },
    });
    (SplittingAPI.getSplitShareReceipt as jest.Mock).mockResolvedValue({
      share_id: 4,
      bill_number: "B42",
      display_name: "Sara",
      mode: "custom",
      status: "settled",
      tender: "crypto",
      subtotal: asDollars(11),
      subtotal_cents: 1100,
      tax: asDollars(1),
      tax_cents: 100,
      service_fee: asDollars(0.5),
      service_fee_cents: 50,
      amount: asDollars(12.5),
      amount_cents: 1250,
      tip_amount: asDollars(1.25),
      tip_cents: 125,
      grand_total: asDollars(13.75),
      grand_total_cents: 1375,
      items: [],
    });

    render(
      <GuestBillSplitPanel
        billId={1}
        billToken="B42"
        businessId={2}
        businessName="Test Resto"
        businessAddress="0xsettlement"
        tipAddress="0xtip"
        tableCode="T1"
        remainingAmount={asDollars(50)}
        defaultCurrency="USD"
        displayCurrency="USD"
        items={[]}
        onPaymentComplete={onPaymentComplete}
        onBillRefresh={onBillRefresh}
      />,
    );

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Split Bill" }));
    await user.click(screen.getByRole("button", { name: "Pay a custom amount" }));
    await user.type(screen.getByLabelText("Amount"), "12.50");
    await user.click(screen.getByRole("button", { name: "Hold my share" }));
    await user.click(await screen.findByTestId("share-payment"));

    await waitFor(() => {
      expect(SplittingAPI.executeHeldShare).toHaveBeenCalledWith(
        "B42",
        expect.objectContaining({
          share_id: 4,
          payment_method: "crypto",
          transaction_hash: "0xtx",
          tip_amount: asDollars(1.25),
        }),
      );
      expect(SplittingAPI.getSplitShareReceipt).toHaveBeenCalledWith("B42", 4);
    });
    expect(await screen.findByText("Your receipt")).toBeInTheDocument();
    expect(screen.getByText("Total paid")).toBeInTheDocument();
    expect(screen.getByText("Tender")).toBeInTheDocument();
    expect(onPaymentComplete).not.toHaveBeenCalled();
    expect(onBillRefresh).toHaveBeenCalled();
  });

  it("keeps a held share local when settlement confirmation fails", async () => {
    const onPaymentComplete = jest.fn();
    const onBillRefresh = jest.fn();
    const consoleError = jest.spyOn(console, "error").mockImplementation(() => {});
    (SplittingAPI.createSplitHold as jest.Mock).mockResolvedValue({
      success: true,
      share: {
        id: 4,
        mode: "custom",
        amount: asDollars(12.5),
        amount_cents: 1250,
        status: "held",
      },
      state: {
        bill_number: "B42",
        status: "open",
        total_amount: asDollars(50),
        total_cents: 5000,
        paid_amount: asDollars(0),
        paid_cents: 0,
        held_amount: asDollars(12.5),
        held_cents: 1250,
        available_amount: asDollars(37.5),
        available_cents: 3750,
        updated_at: "2026-06-13T12:00:00Z",
        shares: [],
      },
    });
    (SplittingAPI.executeHeldShare as jest.Mock).mockRejectedValue(
      new Error("split settlement failed"),
    );

    try {
      render(
        <GuestBillSplitPanel
          billId={1}
          billToken="B42"
          businessId={2}
          businessName="Test Resto"
          businessAddress="0xsettlement"
          tipAddress="0xtip"
          tableCode="T1"
          remainingAmount={asDollars(50)}
          defaultCurrency="USD"
          displayCurrency="USD"
          items={[]}
          onPaymentComplete={onPaymentComplete}
          onBillRefresh={onBillRefresh}
        />,
      );

      const user = userEvent.setup();
      await user.click(screen.getByRole("button", { name: "Split Bill" }));
      await user.click(screen.getByRole("button", { name: "Pay a custom amount" }));
      await user.type(screen.getByLabelText("Amount"), "12.50");
      await user.click(screen.getByRole("button", { name: "Hold my share" }));
      await user.click(await screen.findByTestId("share-payment"));

      await waitFor(() => {
        expect(SplittingAPI.executeHeldShare).toHaveBeenCalled();
      });
      expect(onPaymentComplete).not.toHaveBeenCalled();
      expect(onBillRefresh).toHaveBeenCalled();
      expect(screen.getByText("Your share is ready")).toBeInTheDocument();
      expect(screen.queryByText("Your receipt")).not.toBeInTheDocument();
    } finally {
      consoleError.mockRestore();
    }
  });

  it("requests a pending cashier payment for the held split share", async () => {
    (SplittingAPI.createSplitHold as jest.Mock).mockResolvedValue({
      success: true,
      share: {
        id: 12,
        display_name: "Jane",
        mode: "custom",
        amount: asDollars(12.5),
        amount_cents: 1250,
        status: "held",
      },
      state: {
        bill_number: "B42",
        status: "open",
        total_amount: asDollars(50),
        total_cents: 5000,
        paid_amount: asDollars(0),
        paid_cents: 0,
        held_amount: asDollars(12.5),
        held_cents: 1250,
        available_amount: asDollars(37.5),
        available_cents: 3750,
        updated_at: "2026-06-13T12:00:00Z",
        shares: [],
      },
    });
    (requestAlternativePayment as jest.Mock).mockResolvedValue({
      success: true,
      message: "sent",
      requestId: "77",
    });

    render(
      <GuestBillSplitPanel
        billId={1}
        billToken="B42"
        businessId={2}
        businessName="Test Resto"
        businessAddress="0xsettlement"
        tipAddress="0xtip"
        tableCode="T1"
        remainingAmount={asDollars(50)}
        defaultCurrency="USD"
        displayCurrency="USD"
        items={[]}
        onPaymentComplete={jest.fn()}
        onBillRefresh={jest.fn()}
      />,
    );

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Split Bill" }));
    await user.click(screen.getByRole("button", { name: "Pay a custom amount" }));
    await user.type(screen.getByLabelText("Amount"), "12.50");
    await user.click(screen.getByRole("button", { name: "Hold my share" }));
    await user.click(await screen.findByTestId("share-cashier"));

    await waitFor(() => {
      expect(requestAlternativePayment).toHaveBeenCalledWith(
        "B42",
        "12.50",
        PaymentMethod.CASH,
        "Jane",
        12,
        "1.25",
      );
    });
    expect(SplittingAPI.executeHeldShare).not.toHaveBeenCalled();
  });

  it("shows the cashier confirmation modal after a share cashier request", async () => {
    (SplittingAPI.createSplitHold as jest.Mock).mockResolvedValue({
      success: true,
      share: {
        id: 4,
        mode: "custom",
        amount: asDollars(12.5),
        amount_cents: 1250,
        status: "held",
        // Future expiry so the hold-countdown effect does not retire the share
        // before the cashier confirmation modal can mount.
        hold_expires_at: new Date(Date.now() + 120_000).toISOString(),
      },
      state: {
        bill_number: "B42",
        status: "open",
        total_amount: asDollars(50),
        total_cents: 5000,
        paid_amount: asDollars(0),
        paid_cents: 0,
        held_amount: asDollars(12.5),
        held_cents: 1250,
        available_amount: asDollars(37.5),
        available_cents: 3750,
        updated_at: "2026-06-13T12:00:00Z",
        shares: [],
      },
    });
    (requestAlternativePayment as jest.Mock).mockResolvedValue({ success: true });

    render(
      <GuestBillSplitPanel
        billId={1}
        billToken="B42"
        businessId={2}
        businessName="Test Resto"
        businessAddress="0xsettlement"
        tipAddress="0xtip"
        tableCode="T1"
        remainingAmount={asDollars(50)}
        defaultCurrency="USD"
        displayCurrency="USD"
        items={[]}
        onPaymentComplete={jest.fn()}
        onBillRefresh={jest.fn()}
      />,
    );

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Split Bill" }));
    await user.click(screen.getByRole("button", { name: "Pay a custom amount" }));
    await user.type(screen.getByLabelText("Amount"), "12.50");
    await user.click(screen.getByRole("button", { name: "Hold my share" }));

    await user.click(await screen.findByTestId("share-cashier"));

    expect(await screen.findByText("bill.pleasePayAtCashier")).toBeInTheDocument();
    expect(screen.getByText("bill.cashierRequestSentNote")).toBeInTheDocument();
  });

  it("loads the share receipt when a live update settles the held cashier share", async () => {
    const originalEventSource = globalThis.EventSource;
    const sources: Array<{
      emit: (type: string, payload: unknown) => void;
      close: jest.Mock;
    }> = [];

    class MockEventSource {
      static readonly CONNECTING = 0;
      static readonly OPEN = 1;
      static readonly CLOSED = 2;
      readonly CONNECTING = 0;
      readonly OPEN = 1;
      readonly CLOSED = 2;
      url: string;
      withCredentials = false;
      readyState = 1;
      onerror: ((event: Event) => void) | null = null;
      onmessage: ((event: MessageEvent) => void) | null = null;
      onopen: ((event: Event) => void) | null = null;
      close = jest.fn();
      private listeners = new Map<string, Array<(event: MessageEvent) => void>>();

      constructor(url: string) {
        this.url = url;
        sources.push(this);
      }

      addEventListener(type: string, listener: (event: MessageEvent) => void) {
        const existing = this.listeners.get(type) ?? [];
        existing.push(listener);
        this.listeners.set(type, existing);
      }

      removeEventListener(type: string, listener: (event: MessageEvent) => void) {
        const existing = this.listeners.get(type) ?? [];
        this.listeners.set(
          type,
          existing.filter((candidate) => candidate !== listener),
        );
      }

      dispatchEvent() {
        return true;
      }

      emit(type: string, payload: unknown) {
        const event = { data: JSON.stringify(payload) } as MessageEvent;
        for (const listener of this.listeners.get(type) ?? []) {
          listener(event);
        }
      }
    }

    Object.defineProperty(globalThis, "EventSource", {
      configurable: true,
      value: MockEventSource,
    });

    (SplittingAPI.createSplitHold as jest.Mock).mockResolvedValue({
      success: true,
      share: {
        id: 12,
        display_name: "Jane",
        mode: "custom",
        amount: asDollars(12.5),
        amount_cents: 1250,
        status: "held",
      },
      state: {
        bill_number: "B42",
        status: "open",
        total_amount: asDollars(50),
        total_cents: 5000,
        paid_amount: asDollars(0),
        paid_cents: 0,
        held_amount: asDollars(12.5),
        held_cents: 1250,
        available_amount: asDollars(37.5),
        available_cents: 3750,
        updated_at: "2026-06-13T12:00:00Z",
        shares: [],
      },
    });
    (SplittingAPI.getSplitShareReceipt as jest.Mock).mockResolvedValue({
      share_id: 12,
      bill_number: "B42",
      display_name: "Jane",
      mode: "custom",
      status: "settled",
      tender: "cash",
      subtotal: asDollars(12),
      subtotal_cents: 1200,
      tax: asDollars(0.5),
      tax_cents: 50,
      service_fee: asDollars(0),
      service_fee_cents: 0,
      amount: asDollars(12.5),
      amount_cents: 1250,
      tip_amount: asDollars(0),
      tip_cents: 0,
      grand_total: asDollars(12.5),
      grand_total_cents: 1250,
      items: [],
    });

    try {
      render(
        <GuestBillSplitPanel
          billId={1}
          billToken="B42"
          businessId={2}
          businessName="Test Resto"
          businessAddress="0xsettlement"
          tipAddress="0xtip"
          tableCode="T1"
          remainingAmount={asDollars(50)}
          defaultCurrency="USD"
          displayCurrency="USD"
          items={[]}
          onPaymentComplete={jest.fn()}
          onBillRefresh={jest.fn()}
        />,
      );

      fireEvent.click(screen.getByRole("button", { name: "Split Bill" }));
      fireEvent.click(screen.getByRole("button", { name: "Pay a custom amount" }));
      fireEvent.change(screen.getByLabelText("Amount"), { target: { value: "12.50" } });
      fireEvent.click(screen.getByRole("button", { name: "Hold my share" }));

      await screen.findByText("Your share is ready");
      act(() => {
        sources[0].emit("bill.split.updated", {
          data: {
            bill_number: "B42",
            status: "partial",
            total_amount: asDollars(50),
            total_cents: 5000,
            paid_amount: asDollars(12.5),
            paid_cents: 1250,
            held_amount: asDollars(0),
            held_cents: 0,
            available_amount: asDollars(37.5),
            available_cents: 3750,
            updated_at: "2026-06-13T12:01:00Z",
            shares: [
              {
                id: 12,
                display_name: "Jane",
                mode: "custom",
                amount: asDollars(12.5),
                amount_cents: 1250,
                status: "settled",
                tender: "cash",
              },
            ],
          },
        });
      });

      await waitFor(() => {
        expect(SplittingAPI.getSplitShareReceipt).toHaveBeenCalledWith("B42", 12);
      });
      expect(await screen.findByText("Your receipt")).toBeInTheDocument();
    } finally {
      Object.defineProperty(globalThis, "EventSource", {
        configurable: true,
        value: originalEventSource,
      });
    }
  });

  it("recovers a held guest split share after refresh", async () => {
    (SplittingAPI.getMySplitShares as jest.Mock).mockResolvedValue([
      {
        id: 33,
        display_name: "Sara",
        mode: "custom",
        amount: asDollars(9),
        amount_cents: 900,
        status: "held",
      },
    ]);

    render(
      <GuestBillSplitPanel
        billId={1}
        billToken="B42"
        businessId={2}
        businessName="Test Resto"
        businessAddress="0xsettlement"
        tipAddress="0xtip"
        tableCode="T1"
        remainingAmount={asDollars(50)}
        defaultCurrency="USD"
        displayCurrency="USD"
        items={[]}
        onPaymentComplete={jest.fn()}
        onBillRefresh={jest.fn()}
      />,
    );

    await waitFor(() => {
      expect(SplittingAPI.getMySplitShares).toHaveBeenCalledWith("B42");
    });
    expect(await screen.findByText("Your share is ready")).toBeInTheDocument();
    expect(screen.getByTestId("share-payment")).toHaveTextContent("9.00");
  });

  it("releases the held split share from the payment sheet", async () => {
    (SplittingAPI.createSplitHold as jest.Mock).mockResolvedValue({
      success: true,
      share: {
        id: 4,
        mode: "custom",
        amount: asDollars(12.5),
        amount_cents: 1250,
        status: "held",
      },
      state: {
        bill_number: "B42",
        status: "open",
        total_amount: asDollars(50),
        total_cents: 5000,
        paid_amount: asDollars(0),
        paid_cents: 0,
        held_amount: asDollars(12.5),
        held_cents: 1250,
        available_amount: asDollars(37.5),
        available_cents: 3750,
        updated_at: "2026-06-13T12:00:00Z",
        shares: [],
      },
    });

    render(
      <GuestBillSplitPanel
        billId={1}
        billToken="B42"
        businessId={2}
        businessName="Test Resto"
        businessAddress="0xsettlement"
        tipAddress="0xtip"
        tableCode="T1"
        remainingAmount={asDollars(50)}
        defaultCurrency="USD"
        displayCurrency="USD"
        items={[]}
        onPaymentComplete={jest.fn()}
        onBillRefresh={jest.fn()}
      />,
    );

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Split Bill" }));
    await user.click(screen.getByRole("button", { name: "Pay a custom amount" }));
    await user.type(screen.getByLabelText("Amount"), "12.50");
    await user.click(screen.getByRole("button", { name: "Hold my share" }));
    await user.click(await screen.findByRole("button", { name: "Release my share" }));

    await waitFor(() => {
      expect(SplittingAPI.releaseHeldShare).toHaveBeenCalledWith("B42", 4);
    });
    await waitFor(() => {
      expect(screen.queryByText("Your share is ready")).not.toBeInTheDocument();
    });
  });

  it("renders the live payer roster from split state updates", async () => {
    const originalEventSource = globalThis.EventSource;
    const sources: Array<{
      emit: (type: string, payload: unknown) => void;
      close: jest.Mock;
    }> = [];

    class MockEventSource {
      static readonly CONNECTING = 0;
      static readonly OPEN = 1;
      static readonly CLOSED = 2;
      readonly CONNECTING = 0;
      readonly OPEN = 1;
      readonly CLOSED = 2;
      url: string;
      withCredentials = false;
      readyState = 1;
      onerror: ((event: Event) => void) | null = null;
      onmessage: ((event: MessageEvent) => void) | null = null;
      onopen: ((event: Event) => void) | null = null;
      close = jest.fn();
      private listeners = new Map<string, Array<(event: MessageEvent) => void>>();

      constructor(url: string) {
        this.url = url;
        sources.push(this);
      }

      addEventListener(type: string, listener: (event: MessageEvent) => void) {
        const existing = this.listeners.get(type) ?? [];
        existing.push(listener);
        this.listeners.set(type, existing);
      }

      removeEventListener(type: string, listener: (event: MessageEvent) => void) {
        const existing = this.listeners.get(type) ?? [];
        this.listeners.set(
          type,
          existing.filter((candidate) => candidate !== listener),
        );
      }

      dispatchEvent() {
        return true;
      }

      emit(type: string, payload: unknown) {
        const event = { data: JSON.stringify(payload) } as MessageEvent;
        for (const listener of this.listeners.get(type) ?? []) {
          listener(event);
        }
      }
    }

    Object.defineProperty(globalThis, "EventSource", {
      configurable: true,
      value: MockEventSource,
    });

    try {
      render(
        <GuestBillSplitPanel
          billId={1}
          billToken="B42"
          businessId={2}
          businessName="Test Resto"
          businessAddress="0xsettlement"
          tipAddress="0xtip"
          tableCode="T1"
          remainingAmount={asDollars(50)}
          defaultCurrency="USD"
          displayCurrency="USD"
          items={[]}
          onPaymentComplete={jest.fn()}
          onBillRefresh={jest.fn()}
        />,
      );

      await userEvent.click(screen.getByRole("button", { name: "Split Bill" }));
      act(() => {
        sources[0].emit("connected", {
          state: {
            bill_number: "B42",
            status: "partial",
            total_amount: asDollars(50),
            total_cents: 5000,
            paid_amount: asDollars(20),
            paid_cents: 2000,
            held_amount: asDollars(12.5),
            held_cents: 1250,
            available_amount: asDollars(17.5),
            available_cents: 1750,
            updated_at: "2026-06-13T12:02:00Z",
            shares: [
              {
                id: 7,
                display_name: "Sara",
                mode: "items",
                amount: asDollars(12.5),
                amount_cents: 1250,
                status: "held",
              },
              {
                id: 8,
                mode: "custom",
                amount: asDollars(20),
                amount_cents: 2000,
                status: "settled",
                tender: "cash",
              },
            ],
          },
        });
      });

      expect(await screen.findByText("Who's paying")).toBeInTheDocument();
      expect(screen.getByText("Sara")).toBeInTheDocument();
      expect(screen.getByText("Guest 8")).toBeInTheDocument();
      expect(screen.getByText("Paying now")).toBeInTheDocument();
      expect(screen.getByText("Paid")).toBeInTheDocument();
      expect(screen.getAllByText("$12.50").length).toBeGreaterThanOrEqual(1);
      expect(screen.getAllByText("$20.00").length).toBeGreaterThanOrEqual(1);

      // The live progress bar animates its width on every incoming SSE payment
      // from other diners, so it must respect prefers-reduced-motion (WCAG AA).
      const progressFill = document.querySelector('[class*="transition-[width]"]');
      expect(progressFill).not.toBeNull();
      expect(progressFill?.className).toContain("motion-reduce:transition-none");
    } finally {
      Object.defineProperty(globalThis, "EventSource", {
        configurable: true,
        value: originalEventSource,
      });
    }
  });
});

describe("display-currency consistency (Wave 5)", () => {
  it.each([
    "src/components/splitting/GuestBillSplitPanel.tsx",
    "src/components/guest/GuestBill.tsx",
  ])("%s renders no charge-currency fmtCurrency amounts", (rel) => {
    const source = fs.readFileSync(path.join(process.cwd(), rel), "utf8");
    expect(source).not.toMatch(/fmtCurrency\(/);
  });
});

describe("split panel small fixes (Wave 5)", () => {
  it("lets the people input be empty while typing instead of snapping to 1", async () => {
    render(
      <GuestBillSplitPanel
        billId={1}
        billToken="B42"
        businessId={2}
        businessName="Test Resto"
        businessAddress="0xsettlement"
        tipAddress="0xtip"
        tableCode="T1"
        remainingAmount={asDollars(50)}
        defaultCurrency="USD"
        displayCurrency="USD"
        items={[]}
        onPaymentComplete={jest.fn()}
        onBillRefresh={jest.fn()}
      />,
    );
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Split Bill" }));
    await user.click(screen.getByRole("button", { name: "Split equally" }));

    const peopleInput = screen.getByLabelText("People") as HTMLInputElement;
    fireEvent.change(peopleInput, { target: { value: "" } });
    expect(peopleInput.value).toBe("");
    fireEvent.change(peopleInput, { target: { value: "4" } });
    expect(peopleInput.value).toBe("4");
  });
});

describe("PG-23 equal seats exhausted", () => {
  it("shows $0 and disables hold when all equal seats are taken but leftover cents remain", async () => {
    // Concurrent floor holds can leave a few cents with every seat claimed.
    // BE rejects further equal holds (remainingPeople=0); FE must not show the
    // leftover as "your share" or enable Hold.
    const originalEventSource = globalThis.EventSource;
    const sources: Array<{ emit: (type: string, payload: unknown) => void }> = [];

    class MockEventSource {
      static readonly CONNECTING = 0;
      static readonly OPEN = 1;
      static readonly CLOSED = 2;
      readyState = 1;
      withCredentials = false;
      onerror: ((event: Event) => void) | null = null;
      onmessage: ((event: MessageEvent) => void) | null = null;
      onopen: ((event: Event) => void) | null = null;
      close = jest.fn();
      url: string;
      private listeners = new Map<string, Array<(event: MessageEvent) => void>>();
      constructor(url: string) {
        this.url = url;
        sources.push(this);
      }
      addEventListener(type: string, listener: (event: MessageEvent) => void) {
        const existing = this.listeners.get(type) ?? [];
        existing.push(listener);
        this.listeners.set(type, existing);
      }
      removeEventListener(type: string, listener: (event: MessageEvent) => void) {
        const existing = this.listeners.get(type) ?? [];
        this.listeners.set(
          type,
          existing.filter((candidate) => candidate !== listener),
        );
      }
      dispatchEvent() {
        return true;
      }
      emit(type: string, payload: unknown) {
        const event = { data: JSON.stringify(payload) } as MessageEvent;
        for (const listener of this.listeners.get(type) ?? []) {
          listener(event);
        }
      }
    }

    Object.defineProperty(globalThis, "EventSource", {
      configurable: true,
      value: MockEventSource,
    });

    try {
      const user = userEvent.setup();
      render(
        <GuestBillSplitPanel
          billId={1}
          billToken="B42"
          businessId={2}
          businessName="Test Resto"
          businessAddress="0xsettlement"
          tipAddress="0xtip"
          tableCode="T1"
          remainingAmount={asDollars(0.03)}
          defaultCurrency="USD"
          displayCurrency="USD"
          items={[]}
          onPaymentComplete={jest.fn()}
          onBillRefresh={jest.fn()}
          defaultExpanded
        />,
      );

      await user.click(screen.getByRole("button", { name: "Split equally" }));
      fireEvent.change(screen.getByLabelText("People"), { target: { value: "3" } });
      fireEvent.change(screen.getByLabelText("Shares you cover"), {
        target: { value: "1" },
      });

      await waitFor(() => expect(sources.length).toBeGreaterThan(0));
      act(() => {
        sources[0].emit("bill.split.updated", {
          bill_number: "B42",
          status: "open",
          total_amount: asDollars(100.01),
          total_cents: 10001,
          paid_amount: asDollars(0),
          paid_cents: 0,
          held_amount: asDollars(99.98),
          held_cents: 9998,
          available_amount: asDollars(0.03),
          available_cents: 3,
          updated_at: "2026-08-05T12:00:00Z",
          shares: [
            {
              id: 1,
              mode: "equal",
              amount: asDollars(33.33),
              amount_cents: 3333,
              status: "held",
              claimed_fractions: { __equal_shares__: "1" },
            },
            {
              id: 2,
              mode: "equal",
              amount: asDollars(33.33),
              amount_cents: 3333,
              status: "held",
              claimed_fractions: { __equal_shares__: "1" },
            },
            {
              id: 3,
              mode: "equal",
              amount: asDollars(33.32),
              amount_cents: 3332,
              status: "settled",
              claimed_fractions: { __equal_shares__: "1" },
            },
          ],
        });
      });

      // Must not present leftover $0.03 as claimable equal share (would 400 on BE).
      await waitFor(() => {
        expect(screen.getByTestId("split-your-share-amount")).toHaveTextContent("$0.00");
      });
      expect(screen.getByRole("button", { name: "Hold my share" })).toBeDisabled();
    } finally {
      Object.defineProperty(globalThis, "EventSource", {
        configurable: true,
        value: originalEventSource,
      });
    }
  });

  it("advances wall-clock past OTHER guests' equal hold expiry without SSE (fake timers)", async () => {
    // Wiring proof for pure equalSplitPreview: clock tick while live equal
    // holds have hold_expires_at (not only local heldShare). Emit future holds
    // → advanceTimers past expiry without re-emitting → share becomes claimable.
    jest.useFakeTimers();
    jest.setSystemTime(new Date("2026-08-05T12:00:00.000Z"));
    const originalEventSource = globalThis.EventSource;
    const sources: Array<{ emit: (type: string, payload: unknown) => void }> = [];

    class MockEventSource {
      static readonly CONNECTING = 0;
      static readonly OPEN = 1;
      static readonly CLOSED = 2;
      readyState = 1;
      withCredentials = false;
      onerror: ((event: Event) => void) | null = null;
      onmessage: ((event: MessageEvent) => void) | null = null;
      onopen: ((event: Event) => void) | null = null;
      close = jest.fn();
      url: string;
      private listeners = new Map<string, Array<(event: MessageEvent) => void>>();
      constructor(url: string) {
        this.url = url;
        sources.push(this);
      }
      addEventListener(type: string, listener: (event: MessageEvent) => void) {
        const existing = this.listeners.get(type) ?? [];
        existing.push(listener);
        this.listeners.set(type, existing);
      }
      removeEventListener(type: string, listener: (event: MessageEvent) => void) {
        const existing = this.listeners.get(type) ?? [];
        this.listeners.set(
          type,
          existing.filter((candidate) => candidate !== listener),
        );
      }
      dispatchEvent() {
        return true;
      }
      emit(type: string, payload: unknown) {
        const event = { data: JSON.stringify(payload) } as MessageEvent;
        for (const listener of this.listeners.get(type) ?? []) {
          listener(event);
        }
      }
    }

    Object.defineProperty(globalThis, "EventSource", {
      configurable: true,
      value: MockEventSource,
    });

    try {
      const user = userEvent.setup({ advanceTimers: jest.advanceTimersByTime });
      render(
        <GuestBillSplitPanel
          billId={1}
          billToken="B42"
          businessId={2}
          businessName="Test Resto"
          businessAddress="0xsettlement"
          tipAddress="0xtip"
          tableCode="T1"
          remainingAmount={asDollars(50)}
          defaultCurrency="USD"
          displayCurrency="USD"
          items={[]}
          onPaymentComplete={jest.fn()}
          onBillRefresh={jest.fn()}
          defaultExpanded
        />,
      );

      await user.click(screen.getByRole("button", { name: "Split equally" }));
      fireEvent.change(screen.getByLabelText("People"), { target: { value: "2" } });
      fireEvent.change(screen.getByLabelText("Shares you cover"), {
        target: { value: "1" },
      });

      await waitFor(() => expect(sources.length).toBeGreaterThan(0));
      const futureExpiry = new Date("2026-08-05T12:00:30.000Z").toISOString();
      act(() => {
        sources[0].emit("bill.split.updated", {
          bill_number: "B42",
          status: "open",
          total_amount: asDollars(50),
          total_cents: 5000,
          paid_amount: asDollars(0),
          paid_cents: 0,
          held_amount: asDollars(50),
          held_cents: 5000,
          available_amount: asDollars(0),
          available_cents: 0,
          updated_at: "2026-08-05T12:00:00Z",
          shares: [
            {
              id: 1,
              mode: "equal",
              amount: asDollars(25),
              amount_cents: 2500,
              status: "held",
              hold_expires_at: futureExpiry,
              claimed_fractions: { __equal_shares__: "1" },
            },
            {
              id: 2,
              mode: "equal",
              amount: asDollars(25),
              amount_cents: 2500,
              status: "held",
              hold_expires_at: futureExpiry,
              claimed_fractions: { __equal_shares__: "1" },
            },
          ],
        });
      });

      // Seats full + available 0 → $0 while holds are live.
      await waitFor(() => {
        expect(screen.getByTestId("split-your-share-amount")).toHaveTextContent(
          "$0.00",
        );
      });
      expect(screen.getByRole("button", { name: "Hold my share" })).toBeDisabled();

      // Holds expire and release balance without a new SSE frame for shares
      // (available_cents updates as if stream refreshed held→free). Tick past
      // expiry so seat count drops; emit available refresh only (same seats
      // still "held" with past expiry — BE would free them at request time).
      act(() => {
        jest.advanceTimersByTime(31_000);
      });
      act(() => {
        sources[0].emit("bill.split.updated", {
          bill_number: "B42",
          status: "open",
          total_amount: asDollars(50),
          total_cents: 5000,
          paid_amount: asDollars(0),
          paid_cents: 0,
          held_amount: asDollars(0),
          held_cents: 0,
          available_amount: asDollars(50),
          available_cents: 5000,
          updated_at: "2026-08-05T12:00:31Z",
          shares: [
            {
              id: 1,
              mode: "equal",
              amount: asDollars(25),
              amount_cents: 2500,
              status: "held",
              hold_expires_at: futureExpiry,
              claimed_fractions: { __equal_shares__: "1" },
            },
            {
              id: 2,
              mode: "equal",
              amount: asDollars(25),
              amount_cents: 2500,
              status: "held",
              hold_expires_at: futureExpiry,
              claimed_fractions: { __equal_shares__: "1" },
            },
          ],
        });
      });

      // floor(5000/2)*1 = 2500 → $25.00; Hold enabled after wall-clock past expiry.
      await waitFor(() => {
        expect(screen.getByTestId("split-your-share-amount")).toHaveTextContent(
          "$25.00",
        );
      });
      expect(screen.getByRole("button", { name: "Hold my share" })).toBeEnabled();
    } finally {
      Object.defineProperty(globalThis, "EventSource", {
        configurable: true,
        value: originalEventSource,
      });
      jest.useRealTimers();
    }
  });
});

describe("PG-30 PERSONAS min/max bounds", () => {
  it("keeps out-of-range people values visible with an inline reason (no silent snap)", async () => {
    const user = userEvent.setup();
    render(
      <GuestBillSplitPanel
        billId={1}
        billToken="B42"
        businessId={2}
        businessName="Test Resto"
        businessAddress="0xsettlement"
        tipAddress="0xtip"
        tableCode="T1"
        remainingAmount={asDollars(50)}
        defaultCurrency="USD"
        displayCurrency="USD"
        items={[]}
        onPaymentComplete={jest.fn()}
        onBillRefresh={jest.fn()}
        defaultExpanded
      />,
    );

    await user.click(screen.getByRole("button", { name: "Split equally" }));
    const peopleInput = screen.getByLabelText("People") as HTMLInputElement;

    // 0 must not snap to 1 — show why and disable hold.
    fireEvent.change(peopleInput, { target: { value: "0" } });
    expect(peopleInput.value).toBe("0");
    expect(screen.getByTestId("split-people-range-reason")).toHaveTextContent(
      /between 1 and 100/i,
    );
    expect(screen.getByRole("button", { name: "Hold my share" })).toBeDisabled();

    // Over the sane max must not silent-clamp.
    fireEvent.change(peopleInput, { target: { value: "101" } });
    expect(peopleInput.value).toBe("101");
    expect(screen.getByTestId("split-people-range-reason")).toHaveTextContent(
      /between 1 and 100/i,
    );
    expect(screen.getByRole("button", { name: "Hold my share" })).toBeDisabled();

    // Valid again clears the reason.
    fireEvent.change(peopleInput, { target: { value: "4" } });
    expect(peopleInput.value).toBe("4");
    expect(screen.queryByTestId("split-people-range-reason")).not.toBeInTheDocument();
  });
});

describe("PG-25 custom split amount reasons", () => {
  it("explains over-remaining clamp and non-positive amounts inline", async () => {
    const user = userEvent.setup();
    render(
      <GuestBillSplitPanel
        billId={1}
        billToken="B42"
        businessId={2}
        businessName="Test Resto"
        businessAddress="0xsettlement"
        tipAddress="0xtip"
        tableCode="T1"
        remainingAmount={asDollars(67.27)}
        defaultCurrency="USD"
        displayCurrency="USD"
        items={[]}
        onPaymentComplete={jest.fn()}
        onBillRefresh={jest.fn()}
        defaultExpanded
      />,
    );

    await user.click(screen.getByRole("button", { name: "Pay a custom amount" }));
    const amountInput = screen.getByLabelText("Amount");

    await user.clear(amountInput);
    await user.type(amountInput, "999999");
    expect(screen.getByTestId("split-custom-amount-reason")).toHaveTextContent(
      /more than the remaining balance/i,
    );
    // Share still shows the capped remaining amount, but the reason is visible.
    expect(screen.getByTestId("split-your-share-amount")).toHaveTextContent("$67.27");
    expect(screen.getByRole("button", { name: "Hold my share" })).toBeEnabled();

    await user.clear(amountInput);
    await user.type(amountInput, "0");
    expect(screen.getByTestId("split-custom-amount-reason")).toHaveTextContent(
      /positive amount/i,
    );
    expect(screen.getByRole("button", { name: "Hold my share" })).toBeDisabled();

    await user.clear(amountInput);
    await user.type(amountInput, "-5");
    expect(screen.getByTestId("split-custom-amount-reason")).toHaveTextContent(
      /positive amount/i,
    );
    expect(screen.getByRole("button", { name: "Hold my share" })).toBeDisabled();
  });
});

describe("PG-24 combo parent-only claim", () => {
  it("does not offer bundle_item children as claimable rows; parent bundle is claimable", async () => {
    const user = userEvent.setup();
    render(
      <GuestBillSplitPanel
        billId={1}
        billToken="B42"
        businessId={2}
        businessName="Test Resto"
        businessAddress="0xsettlement"
        tipAddress="0xtip"
        tableCode="T1"
        remainingAmount={asDollars(15)}
        billSubtotal={asDollars(15)}
        defaultCurrency="USD"
        displayCurrency="USD"
        items={[
          {
            id: "combo-parent",
            name: "Burger Combo",
            price: asDollars(15),
            quantity: 1,
            subtotal: asDollars(15),
            item_type: "bundle",
          },
          {
            id: "combo-child-burger",
            name: "Burger (combo child)",
            price: asDollars(0),
            quantity: 1,
            subtotal: asDollars(0),
            item_type: "bundle_item",
          },
          {
            id: "combo-child-fries",
            name: "Fries (combo child)",
            // Even a positive child subtotal must not become claimable.
            price: asDollars(3),
            quantity: 1,
            subtotal: asDollars(3),
            item_type: "bundle_item",
          },
        ]}
        onPaymentComplete={jest.fn()}
        onBillRefresh={jest.fn()}
        defaultExpanded
      />,
    );

    await user.click(screen.getByRole("button", { name: "Pay for my items" }));

    expect(screen.getByRole("button", { name: /Burger Combo/i })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Burger \(combo child\)/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Fries \(combo child\)/i })).not.toBeInTheDocument();
  });
});

describe("PG-26 claimed item line total vs your share labels", () => {
  afterEach(() => {
    mockSplitLocale = undefined;
  });

  it("labels the item row as line total and the allocated amount as share with tax when they differ", async () => {
    // Item line price $68.00; remaining balance $67.27 (net after discounts).
    // Claiming the full item must show both figures with distinct labels so
    // guests can reconcile them — without changing allocation math.
    const user = userEvent.setup();
    render(
      <GuestBillSplitPanel
        billId={1}
        billToken="B42"
        businessId={2}
        businessName="Test Resto"
        businessAddress="0xsettlement"
        tipAddress="0xtip"
        tableCode="T1"
        remainingAmount={asDollars(67.27)}
        billSubtotal={asDollars(68)}
        defaultCurrency="USD"
        displayCurrency="USD"
        items={[
          {
            id: "steak",
            name: "Steak frites",
            price: asDollars(68),
            quantity: 1,
            subtotal: asDollars(68),
            item_type: "menu_item",
          },
        ]}
        onPaymentComplete={jest.fn()}
        onBillRefresh={jest.fn()}
        defaultExpanded
      />,
    );

    await user.click(screen.getByRole("button", { name: "Pay for my items" }));
    await user.click(screen.getByRole("button", { name: /Steak frites/i }));

    // Row amount is the item's list/line total — labelled as such.
    expect(screen.getByTestId("split-item-line-total-label")).toHaveTextContent(
      "Line total",
    );
    expect(screen.getByTestId("split-item-line-total-amount")).toHaveTextContent(
      "$68.00",
    );

    // Share summary is the allocated pay amount (proportional to remaining),
    // labelled so it is not confused with the line total.
    expect(screen.getByTestId("split-your-share-label")).toHaveTextContent(
      "Your share of the remaining balance",
    );
    expect(screen.getByTestId("split-prior-payment-note")).toHaveTextContent(
      "Previous payments of 0.73",
    );
    expect(screen.getByTestId("split-your-share-amount")).toHaveTextContent(
      "$67.27",
    );

    // Selected items subtotal is also labelled when it differs from the share.
    expect(screen.getByTestId("split-selected-items-total-label")).toHaveTextContent(
      "Selected items",
    );
    expect(screen.getByTestId("split-selected-items-total-amount")).toHaveTextContent(
      "$68.00",
    );
  });
});

