/** @jest-environment jsdom */
/**
 * Guest split cashier path must localize coded payment failures through
 * presentGuestPaymentError — never surface raw backend English.
 */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { asDollars } from "@/types/money";
import GuestBillSplitPanel from "./GuestBillSplitPanel";
import { SplittingAPI } from "@/api/splitting";
import { requestAlternativePayment } from "@/api/alternativePayments";
import { PaymentMethod } from "@/types/alternativePayments";

const toastErrors: string[] = [];
jest.mock("react-hot-toast", () => {
  const toast: any = (msg: string) => toastErrors.push(msg);
  toast.error = (msg: string) => toastErrors.push(msg);
  toast.success = jest.fn();
  return { __esModule: true, default: toast };
});

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
  onCashierPayment?: (paymentDetails: { tipAmount: number }) => void;
}) {
  return (
    <div>
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

const LOCALIZED: Record<string, string> = {
  "bill.splitBill": "Split Bill",
  "bill.splitChooseMethod": "Choose how to split",
  "bill.splitCustom": "Pay a custom amount",
  "bill.splitCustomDescription": "Choose exactly what you want to cover.",
  "bill.splitAmountLabel": "Amount",
  "bill.splitYourShare": "Your share",
  "bill.splitDisplayNameLabel": "Name for this share (optional)",
  "bill.splitDisplayNamePlaceholder": "Sara",
  "bill.splitHoldShare": "Hold my share",
  "bill.splitShareReady": "Your share is ready",
  "bill.splitRemainingAfterYou": "Remaining after you: {amount}",
  "bill.splitReleaseShare": "Release my share",
  "bill.cashierInstructions": "Go to the cashier",
  "bill.contactBusiness": "Please contact the business",
  "payment.errors.splitShareConflict":
    "This share was already claimed or changed. Refresh and try again.",
  "payment.errors.generic": "Something went wrong. Please try again.",
};

jest.mock("../../i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({
    t: (key: string, values?: Record<string, unknown>) => {
      const template = LOCALIZED[key] ?? key;
      return Object.entries(values ?? {}).reduce(
        (text, [name, value]) => text.replace(`{${name}}`, String(value)),
        template,
      );
    },
    currentLanguage: "en",
  }),
}));

describe("GuestBillSplitPanel payment errors", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    toastErrors.length = 0;
  });

  it("shows localized toast on coded split failure, not raw English", async () => {
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

    const englishBackend = "Share already claimed by another guest";
    const err: any = new Error(englishBackend);
    err.response = {
      status: 409,
      data: { code: "split_share_conflict", error: englishBackend },
    };
    (requestAlternativePayment as jest.Mock).mockRejectedValue(err);

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
      expect(toastErrors).toContain(
        "This share was already claimed or changed. Refresh and try again.",
      );
    });
    expect(toastErrors).not.toContain(englishBackend);
    expect(requestAlternativePayment).toHaveBeenCalledWith(
      "B42",
      "12.50",
      PaymentMethod.CASH,
      "Jane",
      12,
      "1.25",
    );
  });
});
