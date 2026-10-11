/**
 * @jest-environment jsdom
 */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { CryptoRefundPanel } from "./CryptoRefundPanel";
import type { Payment } from "@/api/bills";
import type { Dollars } from "@/types/money";

const mockGetDest = jest.fn();
const mockList = jest.fn();
const mockRequest = jest.fn();
const mockApprove = jest.fn();
const mockSubmit = jest.fn();
const mockUnsigned = jest.fn();

jest.mock("@/api/bills", () => {
  const actual = jest.requireActual("@/api/bills");
  return {
    ...actual,
    getPaymentRefundDestination: (...args: unknown[]) => mockGetDest(...args),
    listCryptoRefunds: (...args: unknown[]) => mockList(...args),
    requestCryptoRefund: (...args: unknown[]) => mockRequest(...args),
    approveCryptoRefund: (...args: unknown[]) => mockApprove(...args),
    rejectCryptoRefund: jest.fn(),
    submitCryptoRefundTx: (...args: unknown[]) => mockSubmit(...args),
    getCryptoRefundUnsignedRequest: (...args: unknown[]) => mockUnsigned(...args),
  };
});

jest.mock("@/components/business/managerPin/ManagerPinProvider", () => ({
  useWithManagerPin: () => ({
    withManagerPin: (fn: (pin: string) => Promise<unknown>) => fn("1234"),
  }),
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => {
    const map: Record<string, string> = {
      "billManager.cryptoRefund.title": "On-chain crypto refund",
      "billManager.cryptoRefund.subtitle": "Noncustodial refunds",
      "billManager.cryptoRefund.manualSupportRequired":
        "Refund requires manual support",
      "billManager.cryptoRefund.mainnetOffNotice": "Mainnet signing is off",
      "billManager.cryptoRefund.mainnetOffDisabled": "Instant mainnet signing disabled",
      "billManager.cryptoRefund.mainnetOffSettingsLink": "Open settlement settings",
      "billManager.cryptoRefund.irreversibleNotice": "Irreversible once confirmed",
      "billManager.cryptoRefund.labels.destination": "Verified destination",
      "billManager.cryptoRefund.labels.network": "Network",
      "billManager.cryptoRefund.labels.refundable": "Refundable",
      "billManager.cryptoRefund.labels.refunded": "Refunded",
      "billManager.cryptoRefund.labels.explorer": "View on explorer",
      "billManager.cryptoRefund.labels.txHash": "Transaction hash",
      "billManager.cryptoRefund.labels.txHashHint": "Paste hash",
      "billManager.cryptoRefund.labels.reason": "Reason",
      "billManager.cryptoRefund.labels.reasonPlaceholder": "Why?",
      "billManager.cryptoRefund.actions.request": "Request on-chain refund",
      "billManager.cryptoRefund.actions.approve": "Approve",
      "billManager.cryptoRefund.actions.reject": "Reject",
      "billManager.cryptoRefund.actions.submitTx": "Submit transaction hash",
      "billManager.cryptoRefund.pin.requestTitle": "Confirm",
      "billManager.cryptoRefund.pin.requestDescription": "Request",
      "billManager.cryptoRefund.pin.approveTitle": "Approve",
      "billManager.cryptoRefund.pin.approveDescription": "Approve",
      "billManager.cryptoRefund.pin.rejectTitle": "Reject",
      "billManager.cryptoRefund.pin.rejectDescription": "Reject",
      "billManager.cryptoRefund.pin.submitTitle": "Submit",
      "billManager.cryptoRefund.pin.submitDescription": "Submit",
      "billManager.cryptoRefund.toasts.requested": "Requested",
      "billManager.cryptoRefund.toasts.approved": "Approved",
      "billManager.cryptoRefund.toasts.rejected": "Rejected",
      "billManager.cryptoRefund.toasts.submitted": "Submitted not refunded",
      "billManager.cryptoRefund.errors.loadFailed": "Load failed",
      "billManager.cryptoRefund.errors.manualSupport": "Manual support",
      "billManager.cryptoRefund.errors.reasonTooShort": "Reason short",
      "billManager.cryptoRefund.errors.insufficientBalance": "No balance",
      "billManager.cryptoRefund.errors.requestFailed": "Request failed",
      "billManager.cryptoRefund.errors.approveFailed": "Approve failed",
      "billManager.cryptoRefund.errors.rejectFailed": "Reject failed",
      "billManager.cryptoRefund.errors.invalidTxHash": "Bad hash",
      "billManager.cryptoRefund.errors.submitFailed": "Submit failed",
      // M11d: raw enums / the English honest_lifecycle_label are no longer
      // rendered; each status maps to a translated, tone-colored StatusChip.
      "billManager.cryptoRefund.status.requested": "Requested",
      "billManager.cryptoRefund.status.approved": "Approved",
      "billManager.cryptoRefund.status.awaiting_signature": "Awaiting signature",
      "billManager.cryptoRefund.status.submitted": "Submitted",
      "billManager.cryptoRefund.status.confirming": "Confirming",
      "billManager.cryptoRefund.status.confirmed": "Confirmed",
      "billManager.cryptoRefund.status.failed": "Failed",
      "billManager.cryptoRefund.status.rejected": "Rejected",
      "billManager.cryptoRefund.status.cancelled": "Cancelled",
    };
    return map[key] ?? key;
  },
}));

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { success: jest.fn(), error: jest.fn() },
}));

const payment: Payment = {
  id: 7,
  bill_id: 3,
  amount: 25 as Dollars,
  tip_amount: 0 as Dollars,
  currency: "USD",
  status: "confirmed",
  payment_method: "crypto",
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
};

describe("CryptoRefundPanel", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("shows manual support when destination evidence is missing", async () => {
    mockGetDest.mockResolvedValue({
      payment_id: 7,
      has_evidence: false,
      manual_support_required: true,
    });
    mockList.mockResolvedValue([]);

    render(
      <CryptoRefundPanel
        businessId={9}
        billId={3}
        payment={payment}
        onRefresh={jest.fn()}
      />,
    );

    await waitFor(() => {
      expect(screen.getByText(/manual support/i)).toBeInTheDocument();
    });
  });

  it("never labels submitted as refunded", async () => {
    mockGetDest.mockResolvedValue({
      payment_id: 7,
      has_evidence: true,
      manual_support_required: false,
      chain_id: 84532,
      token: "USDC",
      amount_base_units: 5_000_000,
      masked_address: "0xdead…beef",
      refundable_base_units: 5_000_000,
    });
    mockList.mockResolvedValue([
      {
        id: 1,
        business_id: 9,
        bill_id: 3,
        payment_id: 7,
        chain_id: 84532,
        token: "USDC",
        amount_base_units: 5_000_000,
        masked_recipient: "0xdead…beef",
        recipient_override: false,
        reason: "test",
        status: "submitted",
        honest_lifecycle_label: "Transaction submitted — not yet refunded",
        idempotency_key: "x",
        confirmations: 0,
        mainnet_submission_off: true,
        manual_tx_hash_required: true,
        requested_by: "owner",
        requested_at: new Date().toISOString(),
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      },
    ]);

    render(
      <CryptoRefundPanel
        businessId={9}
        billId={3}
        payment={payment}
        onRefresh={jest.fn()}
      />,
    );

    // M11d: a submitted refund shows the translated "Submitted" status chip
    // (amber/warn tone), never the completed "Refunded" chip.
    await waitFor(() => {
      expect(screen.getByText("Submitted")).toBeInTheDocument();
    });
    // The solid "Refunded" chip is only for confirmed.
    expect(screen.queryByText("Refunded")).not.toBeInTheDocument();
  });

  // Task 8 / finding 5 corrected: mainnet=off must not be a silent no-op —
  // show an explicit disabled control naming the reason + a settings link.
  it("renders an explicit mainnet-off disabled state with a settings link", async () => {
    mockGetDest.mockResolvedValue({
      payment_id: 7,
      has_evidence: true,
      manual_support_required: false,
      chain_id: 8453,
      token: "USDC",
      amount_base_units: 5_000_000,
      masked_address: "0xdead…beef",
      refundable_base_units: 5_000_000,
    });
    mockList.mockResolvedValue([
      {
        id: 1,
        business_id: 9,
        bill_id: 3,
        payment_id: 7,
        chain_id: 8453,
        token: "USDC",
        amount_base_units: 5_000_000,
        masked_recipient: "0xdead…beef",
        recipient_override: false,
        reason: "test",
        status: "requested",
        honest_lifecycle_label: "Refund requested — awaiting approval",
        idempotency_key: "x",
        confirmations: 0,
        mainnet_submission_off: true,
        manual_tx_hash_required: true,
        requested_by: "owner",
        requested_at: new Date().toISOString(),
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      },
    ]);

    render(
      <CryptoRefundPanel
        businessId={9}
        billId={3}
        payment={payment}
        onRefresh={jest.fn()}
      />,
    );

    await waitFor(() => {
      expect(screen.getByText("Mainnet signing is off")).toBeInTheDocument();
    });
    const disabled = screen.getByRole("button", {
      name: /Instant mainnet signing disabled/i,
    });
    expect(disabled).toBeDisabled();
    const settings = screen.getByRole("link", {
      name: /Open settlement settings/i,
    });
    expect(settings).toHaveAttribute("href", expect.stringContaining("tab=settings"));
  });

  it("shows Refunded chip only for confirmed status", async () => {
    mockGetDest.mockResolvedValue({
      payment_id: 7,
      has_evidence: true,
      manual_support_required: false,
      chain_id: 84532,
      token: "USDC",
      amount_base_units: 5_000_000,
      masked_address: "0xdead…beef",
      refundable_base_units: 0,
    });
    mockList.mockResolvedValue([
      {
        id: 2,
        business_id: 9,
        bill_id: 3,
        payment_id: 7,
        chain_id: 84532,
        token: "USDC",
        amount_base_units: 5_000_000,
        masked_recipient: "0xdead…beef",
        recipient_override: false,
        reason: "done",
        status: "confirmed",
        honest_lifecycle_label: "Refunded (on-chain confirmed)",
        idempotency_key: "y",
        confirmations: 3,
        mainnet_submission_off: false,
        manual_tx_hash_required: false,
        requested_by: "owner",
        requested_at: new Date().toISOString(),
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      },
    ]);

    render(
      <CryptoRefundPanel
        businessId={9}
        billId={3}
        payment={payment}
        onRefresh={jest.fn()}
      />,
    );

    await waitFor(() => {
      expect(screen.getByText("Refunded")).toBeInTheDocument();
    });
  });

  it("blocks request when insufficient balance", async () => {
    const toast = require("react-hot-toast").default;
    mockGetDest.mockResolvedValue({
      payment_id: 7,
      has_evidence: true,
      manual_support_required: false,
      chain_id: 84532,
      token: "USDC",
      amount_base_units: 5_000_000,
      masked_address: "0xdead…beef",
      refundable_base_units: 0,
    });
    mockList.mockResolvedValue([]);

    render(
      <CryptoRefundPanel
        businessId={9}
        billId={3}
        payment={payment}
        onRefresh={jest.fn()}
      />,
    );

    await waitFor(() => {
      expect(screen.getByText(/Request on-chain refund/i)).toBeInTheDocument();
    });

    const reason = screen.getByLabelText(/Reason/i);
    await userEvent.type(reason, "guest wants money back");
    await userEvent.click(screen.getByText(/Request on-chain refund/i));

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalled();
    });
    expect(mockRequest).not.toHaveBeenCalled();
  });
});
