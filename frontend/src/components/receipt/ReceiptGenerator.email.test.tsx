/** @jest-environment jsdom */
import React from "react";
import { act, fireEvent, render, screen } from "@testing-library/react";

import ReceiptGenerator from "@/components/receipt/ReceiptGenerator";
import { emailBillReceipt } from "@/api/bills";

jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({
    t: (key: string) => key,
    currentLanguage: "en",
  }),
}));

jest.mock("html2canvas", () => ({ __esModule: true, default: jest.fn() }));
jest.mock("jspdf", () => ({ __esModule: true, default: jest.fn() }));

jest.mock("@/api/bills", () => ({
  ...jest.requireActual("@/api/bills"),
  emailBillReceipt: jest.fn(),
}));

jest.mock("@nextui-org/react", () => ({
  Button: ({
    children,
    onPress,
    isLoading,
    ...props
  }: {
    children: React.ReactNode;
    onPress?: () => void;
    isLoading?: boolean;
  }) => (
    <button type="button" onClick={onPress} disabled={isLoading} {...props}>
      {children}
    </button>
  ),
}));

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { success: jest.fn(), error: jest.fn() },
}));

const receiptData = {
  business: { name: "Cafe", tax_rate: 5, service_fee_rate: 7.5 },
  bill: {
    bill_number: "B-1",
    created_at: "2026-06-09T00:00:00Z",
    subtotal: 20,
    tax_amount: 1,
    service_fee_amount: 1.5,
    total_amount: 22.5,
  },
  table: { name: "Terrace 1" },
  items: [{ name: "Coffee", quantity: 1, subtotal: 20 }],
  paymentDetails: {
    totalPaid: 22.5,
    tipAmount: 0,
    paymentMethod: "Card",
    transactionId: "tx-1",
  },
};

describe("ReceiptGenerator email-my-receipt", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("sends the receipt to a typed email on the guest surface", async () => {
    (emailBillReceipt as jest.Mock).mockResolvedValue(undefined);
    render(
      <ReceiptGenerator
        data={receiptData as any}
        currency="USD"
        emailReceipt={{ billToken: "tok-100" }}
      />,
    );
    fireEvent.change(screen.getByLabelText("receipt.emailPlaceholder"), {
      target: { value: "guest@example.com" },
    });
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "receipt.emailMyReceipt" }));
    });
    expect(emailBillReceipt).toHaveBeenCalledWith(
      "tok-100",
      "guest@example.com",
      "en",
      expect.any(Object),
    );
    expect(await screen.findByText("receipt.emailSent")).toBeInTheDocument();
  });
});
