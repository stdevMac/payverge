/**
 * PG-28 — when GuestBill mounts GuestBillSplitPanel with defaultExpanded,
 * both controls must not expose the identical accessible name "Split Bill".
 *
 * @jest-environment jsdom
 */

import React from "react";
import { render, screen } from "@testing-library/react";
import GuestBillSplitPanel from "./GuestBillSplitPanel";
import { asDollars } from "@/types/money";

jest.mock("@/api/splitting", () => ({
  __esModule: true,
  SplittingAPI: {
    createSplitHold: jest.fn(),
    executeHeldShare: jest.fn(),
    getSplitShareReceipt: jest.fn(),
    getMySplitShares: jest.fn(),
    releaseHeldShare: jest.fn(),
  },
  useSplittingAPI: () => ({
    createSplitHold: jest.fn(),
    executeHeldShare: jest.fn(),
    getSplitShareReceipt: jest.fn(),
    getMySplitShares: jest.fn(),
    releaseHeldShare: jest.fn(),
  }),
  getSplitEventsURL: (billToken: string) => `/split-events/${billToken}`,
}));

jest.mock("@/api/alternativePayments", () => ({
  requestAlternativePayment: jest.fn(),
}));

jest.mock("../guest/PaymentSection", () => () => null);

jest.mock("../../i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({
    t: (key: string) => {
      const map: Record<string, string> = {
        "bill.splitBill": "Split Bill",
        "bill.splitLiveTitle": "Split and pay your share",
        "bill.splitLiveDescription": "Hold the amount you want to cover.",
        "bill.splitChooseMethod": "Choose how to split",
        "bill.splitEqually": "Split equally",
        "bill.splitItems": "Pay for my items",
        "bill.splitCustom": "Pay a custom amount",
      };
      return map[key] ?? key;
    },
    currentLanguage: "en",
  }),
}));

jest.mock("@/components/common/CurrencyConverter", () => ({
  CurrencyPrice: ({ amount }: { amount: number }) => <span>{amount}</span>,
}));

const baseProps = {
  billId: 1,
  billToken: "B42",
  businessId: 2,
  businessName: "Test",
  businessAddress: "0xabc",
  tipAddress: "0xtip",
  tableCode: "T1",
  remainingAmount: asDollars(10),
  defaultCurrency: "USD",
  displayCurrency: "USD",
  items: [] as any[],
  onPaymentComplete: jest.fn(),
  onBillRefresh: jest.fn(),
};

describe("GuestBillSplitPanel PG-28 duplicate accessible name", () => {
  it("does not render a Split Bill button when defaultExpanded (parent disclosure)", () => {
    render(<GuestBillSplitPanel {...baseProps} defaultExpanded />);
    expect(
      screen.queryByRole("button", { name: "Split Bill" }),
    ).not.toBeInTheDocument();
    expect(screen.getByText("Split and pay your share")).toBeInTheDocument();
  });

  it("still offers Split Bill when used as a standalone panel", () => {
    render(<GuestBillSplitPanel {...baseProps} />);
    expect(
      screen.getByRole("button", { name: "Split Bill" }),
    ).toBeInTheDocument();
  });
});
