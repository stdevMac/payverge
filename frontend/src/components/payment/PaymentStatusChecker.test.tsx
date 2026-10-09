/** @jest-environment jsdom */
import React, { act } from "react";
import { render, waitFor } from "@testing-library/react";

import PaymentStatusChecker from "@/components/payment/PaymentStatusChecker";
import { paymentPluginAPI } from "@/api/plugins";

jest.mock("@/api/plugins", () => ({
  paymentPluginAPI: {
    getPluginPaymentStatus: jest.fn(),
  },
}));

jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({
    t: (key: string) => key,
  }),
}));

jest.mock("@nextui-org/react", () => ({
  Modal: ({
    isOpen,
    children,
  }: {
    isOpen: boolean;
    children: React.ReactNode;
  }) => (isOpen ? <div>{children}</div> : null),
  ModalContent: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  ModalHeader: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  ModalBody: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  Card: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  CardBody: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  Button: ({ children }: { children: React.ReactNode }) => (
    <button type="button">{children}</button>
  ),
  Spinner: () => <div>spinner</div>,
}));

describe("PaymentStatusChecker", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("falls back to the pending payment totals when the status endpoint has no metadata", async () => {
    const onPaymentConfirmed = jest.fn();
    (paymentPluginAPI.getPluginPaymentStatus as jest.Mock).mockResolvedValue({
      status: "paid",
      metadata: null,
    });

    render(
      <PaymentStatusChecker
        isOpen={true}
        onClose={jest.fn()}
        billToken="B42-opaque"
        paymentId="pay_123"
        paymentMethod="paypal"
        fallbackTotalPaid={33.5}
        fallbackTipAmount={3.5}
        onPaymentConfirmed={onPaymentConfirmed}
      />,
    );

    await waitFor(() => {
      expect(onPaymentConfirmed).toHaveBeenCalledWith({
        totalPaid: 33.5,
        tipAmount: 3.5,
        paymentMethod: "paypal",
        transactionId: "pay_123",
      });
    });
  });

  // Audit F-02: stablecoin plugins report `amount` as the on-chain token
  // decimal (USDT to transfer) and `total_amount` as the fiat figure — the
  // confirmed total must use the fiat one.
  it("prefers metadata.total_amount (fiat) over metadata.amount (on-chain decimal)", async () => {
    const onPaymentConfirmed = jest.fn();
    (paymentPluginAPI.getPluginPaymentStatus as jest.Mock).mockResolvedValue({
      status: "completed",
      metadata: {
        amount: "12.345678",
        total_amount: 12.34,
        tip_amount: 1.5,
      },
    });

    render(
      <PaymentStatusChecker
        isOpen={true}
        onClose={jest.fn()}
        billToken="B42-opaque"
        paymentId="pay_456"
        paymentMethod="usdc_payment"
        fallbackTotalPaid={0}
        fallbackTipAmount={0}
        onPaymentConfirmed={onPaymentConfirmed}
      />,
    );

    await waitFor(() => {
      expect(onPaymentConfirmed).toHaveBeenCalledWith({
        totalPaid: 12.34,
        tipAmount: 1.5,
        paymentMethod: "usdc_payment",
        transactionId: "pay_456",
      });
    });
  });

  // Slow crypto confirmations can land after the 5-minute foreground window.
  // Timing out must NOT abandon the payment: a low-frequency backstop poll has
  // to keep running so a late "completed" still flips the UI and fires
  // onPaymentConfirmed rather than stranding the guest on "contact support".
  it("keeps a backstop poll after timeout so a late confirmation still resolves", async () => {
    jest.useFakeTimers();
    try {
      const onPaymentConfirmed = jest.fn();
      const getStatus = paymentPluginAPI.getPluginPaymentStatus as jest.Mock;
      getStatus.mockResolvedValue({ status: "pending", metadata: null });

      const { queryByText } = render(
        <PaymentStatusChecker
          isOpen={true}
          onClose={jest.fn()}
          billToken="B99-slow"
          paymentId="pay_slow"
          paymentMethod="usdc_payment"
          fallbackTotalPaid={40}
          fallbackTipAmount={5}
          onPaymentConfirmed={onPaymentConfirmed}
        />,
      );

      // Flush the initial poll → status becomes "pending".
      await act(async () => {
        await Promise.resolve();
      });

      // Cross the 5-minute foreground window → status becomes "timed_out".
      await act(async () => {
        jest.advanceTimersByTime(5 * 60 * 1000 + 3000);
        await Promise.resolve();
        await Promise.resolve();
      });
      expect(onPaymentConfirmed).not.toHaveBeenCalled();
      // Timed-out state must STICK (not flicker back to "pending"): the
      // reassuring timed-out title stays on screen while the backstop runs.
      expect(queryByText(/Timed Out/i)).not.toBeNull();

      // The payment finally confirms on-chain, after the foreground window.
      getStatus.mockResolvedValue({
        status: "completed",
        metadata: { total_amount: 40, tip_amount: 5 },
      });

      // Advance one backstop interval → the late confirmation must resolve.
      await act(async () => {
        jest.advanceTimersByTime(20 * 1000);
        await Promise.resolve();
        await Promise.resolve();
      });

      await waitFor(() => {
        expect(onPaymentConfirmed).toHaveBeenCalledWith({
          totalPaid: 40,
          tipAmount: 5,
          paymentMethod: "usdc_payment",
          transactionId: "pay_slow",
        });
      });
    } finally {
      jest.useRealTimers();
    }
  });

  // The details row must show a human label, never the raw plugin id.
  // Wave 5: labels come from the shared paymentMethodLabels module (localized
  // paymentMethods.* keys). t() is mocked to return the key itself.
  it("renders a human payment-method label, not the raw plugin id", async () => {
    (paymentPluginAPI.getPluginPaymentStatus as jest.Mock).mockResolvedValue({
      status: "pending",
      metadata: null,
    });

    const { queryByText } = render(
      <PaymentStatusChecker
        isOpen={true}
        onClose={jest.fn()}
        billToken="B-label"
        paymentId="pay_label"
        paymentMethod="cross_chain_payment"
        onPaymentConfirmed={jest.fn()}
      />,
    );

    await waitFor(() => {
      expect(queryByText("paymentMethods.crossChain")).not.toBeNull();
    });
    expect(queryByText("cross_chain_payment")).toBeNull();
  });

  it("passes through split share metadata from confirmed plugin payments", async () => {
    const onPaymentConfirmed = jest.fn();
    (paymentPluginAPI.getPluginPaymentStatus as jest.Mock).mockResolvedValue({
      status: "paid",
      metadata: {
        total_amount: 12.5,
        tip_amount: 1,
        split_share_id: 42,
      },
    });

    render(
      <PaymentStatusChecker
        isOpen={true}
        onClose={jest.fn()}
        billToken="B42-opaque"
        paymentId="pay_split"
        paymentMethod="paypal"
        fallbackTotalPaid={0}
        fallbackTipAmount={0}
        onPaymentConfirmed={onPaymentConfirmed}
      />,
    );

    await waitFor(() => {
      expect(onPaymentConfirmed).toHaveBeenCalledWith({
        totalPaid: 12.5,
        tipAmount: 1,
        paymentMethod: "paypal",
        transactionId: "pay_split",
        splitShareId: 42,
      });
    });
  });
});
