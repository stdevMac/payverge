/** @jest-environment jsdom */
import React from "react";
import { render, screen, act, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { asDollars } from "@/types/money";
import GuestBillSplitPanel from "./GuestBillSplitPanel";
import { SplittingAPI } from "@/api/splitting";
import { requestAlternativePayment } from "@/api/alternativePayments";

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

// Minimal PaymentSection stand-in: surfaces the cashier action and echoes
// the cashierPaymentLoading prop so the test can pin the wiring.
jest.mock("../guest/PaymentSection", () => function MockPaymentSection(props: {
  onCashierPayment?: (paymentDetails: { tipAmount: number }) => void;
  cashierPaymentLoading?: boolean;
}) {
  return (
    <div>
      <span data-testid="cashier-loading">
        {String(props.cashierPaymentLoading ?? false)}
      </span>
      <button
        type="button"
        data-testid="share-cashier"
        onClick={() => props.onCashierPayment?.({ tipAmount: 0 })}
      >
        Cashier
      </button>
    </div>
  );
});

jest.mock("../../i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({
    t: (key: string) => key,
    currentLanguage: undefined,
  }),
}));

const heldHoldResponse = {
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
    updated_at: "2026-07-15T12:00:00Z",
    shares: [],
  },
};

async function holdAShare() {
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
  await user.click(screen.getByRole("button", { name: "bill.splitBill" }));
  await user.click(screen.getByRole("button", { name: /bill\.splitCustom$/ }));
  await user.type(screen.getByLabelText("bill.splitAmountLabel"), "12.50");
  await user.click(screen.getByRole("button", { name: "bill.splitHoldShare" }));
  await screen.findByTestId("share-cashier");
  return user;
}

describe("GuestBillSplitPanel cashier double-tap", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (SplittingAPI.createSplitHold as jest.Mock).mockResolvedValue(heldHoldResponse);
  });

  it("files exactly one cash request on a double-tap and exposes the in-flight state", async () => {
    let resolveRequest: (value: unknown) => void = () => {};
    (requestAlternativePayment as jest.Mock).mockImplementation(
      () => new Promise((resolve) => { resolveRequest = resolve; }),
    );

    const user = await holdAShare();
    expect(screen.getByTestId("cashier-loading")).toHaveTextContent("false");

    await user.click(screen.getByTestId("share-cashier"));
    await user.click(screen.getByTestId("share-cashier"));

    expect(requestAlternativePayment).toHaveBeenCalledTimes(1);
    expect(screen.getByTestId("cashier-loading")).toHaveTextContent("true");

    await act(async () => {
      resolveRequest({ success: true, message: "sent", requestId: "77" });
    });
    await waitFor(() => {
      expect(screen.getByTestId("cashier-loading")).toHaveTextContent("false");
    });
  });
});
