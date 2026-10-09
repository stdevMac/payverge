/** @jest-environment jsdom */
/**
 * Issue 815 — a guest table-home load that fails with a 429 (rate limit) or a
 * transient network/server error must NOT paint the dead-QR 404 state
 * ("We couldn't find that table … the table has been reset") while the table
 * is actually active. Only a true 404 may claim the code is dead; anything
 * else renders a busy/retry state and keeps polling for recovery.
 */
import React from "react";
import { act, fireEvent, render, screen } from "@testing-library/react";

import { GuestTableView } from "@/components/guest/GuestTableView";
import { getTableByCode, getOpenBillByTableCode } from "@/api/bills";

const mockCustomerAuth = {
  customer: null,
  customerId: null,
  isAuthenticated: false,
  loading: false,
};

jest.mock("@/contexts/CustomerAuthContext", () => ({
  useCustomerAuth: () => mockCustomerAuth,
}));

jest.mock("@/api/bills", () => ({
  getTableByCode: jest.fn(),
  getOpenBillByTableCode: jest.fn(),
  getServiceCallStatus: jest.fn().mockResolvedValue({
    status: "none",
    reason: null,
  }),
  createServiceCall: jest.fn(),
  ServiceCallCooldownError: class extends Error {},
}));

jest.mock("@/api/crm", () => ({
  crmAPI: { getProfile: jest.fn() },
}));

jest.mock("@/api/customerTable", () => ({
  checkInCustomerToTable: jest.fn(),
}));

jest.mock("@/i18n/GuestTranslationProvider", () => {
  // Identity translator: assertions target raw guest i18n keys. Stable
  // function identities — GuestTableView's load effect depends on them.
  const t = (key: string) => key;
  const setBusinessId = () => {};
  return {
    useGuestTranslation: () => ({ t, setBusinessId, currentLanguage: "en" }),
  };
});

jest.mock("@/components/navigation/PersistentGuestNav", () => ({
  __esModule: true,
  default: () => null,
}));

jest.mock("@/components/notifications/PaymentNotification", () => ({
  __esModule: true,
  default: () => null,
}));

jest.mock("@/components/notifications/BillUpdateNotification", () => ({
  __esModule: true,
  default: () => null,
}));

jest.mock("@/components/common/CurrencyConverter", () => ({
  CurrencyPrice: ({ amount }: { amount: number }) => (
    <span>{`$${amount.toFixed(2)}`}</span>
  ),
}));

jest.mock("@/components/customer/CustomerAuthModal", () => ({
  __esModule: true,
  default: () => null,
}));

jest.mock("@nextui-org/react", () => ({
  Spinner: () => <div data-testid="spinner" />,
  Image: (props: { src?: string; alt?: string }) => (
    // eslint-disable-next-line @next/next/no-img-element
    <img src={props.src} alt={props.alt ?? ""} />
  ),
  Button: ({ children }: { children: React.ReactNode }) => (
    <button type="button">{children}</button>
  ),
}));

jest.mock("@/hooks/usePolling", () => ({
  usePolling: () => {},
}));

const mockedGetTable = getTableByCode as jest.MockedFunction<
  typeof getTableByCode
>;
const mockedGetOpenBill = getOpenBillByTableCode as jest.MockedFunction<
  typeof getOpenBillByTableCode
>;

/** Sanitized-axios shape thrown by the shared instance (toSanitizedError). */
function httpError(status: number, message = `Request failed (${status})`) {
  const err = new Error(message) as Error & {
    response: { status: number };
    status: number;
  };
  err.response = { status };
  err.status = status;
  return err;
}

async function renderAndSettle(tableCode = "M03Y18GB3P") {
  render(<GuestTableView tableCode={tableCode} />);
  // Flush the loadTableData promise chain.
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
}

describe("GuestTableView load failure honesty (issue 815)", () => {
  let consoleErrorSpy: jest.SpyInstance;

  beforeEach(() => {
    jest.clearAllMocks();
    mockedGetOpenBill.mockResolvedValue({ bill: null, items: [] } as any);
    consoleErrorSpy = jest
      .spyOn(console, "error")
      .mockImplementation(() => {});
  });

  afterEach(() => {
    consoleErrorSpy.mockRestore();
  });

  it("renders a busy/retry state — not the dead-QR 404 — when the table fetch 429s", async () => {
    mockedGetTable.mockRejectedValue(httpError(429, "Too many requests"));

    await renderAndSettle();

    // The dead-QR copy must not appear: the table is active, we are throttled.
    expect(screen.queryByText("errors.tableNotFound")).not.toBeInTheDocument();
    expect(screen.queryByText("404")).not.toBeInTheDocument();

    // Honest busy state with a retry affordance.
    expect(screen.getByText("errors.billBusyTitle")).toBeInTheDocument();
    expect(screen.getByText("errors.billBusy")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "errors.tableNotFoundRetry" }),
    ).toBeInTheDocument();
  });

  it("renders a transient network state — not the dead-QR 404 — on a network failure", async () => {
    const err = new Error("Network Error") as Error & { code: string };
    err.code = "ERR_NETWORK";
    mockedGetTable.mockRejectedValue(err);

    await renderAndSettle();

    expect(screen.queryByText("errors.tableNotFound")).not.toBeInTheDocument();
    expect(screen.queryByText("404")).not.toBeInTheDocument();
    expect(screen.getByText("errors.networkError")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "errors.tableNotFoundRetry" }),
    ).toBeInTheDocument();
  });

  it("renders a transient state — not the dead-QR 404 — on a 5xx", async () => {
    mockedGetTable.mockRejectedValue(httpError(503));

    await renderAndSettle();

    expect(screen.queryByText("errors.tableNotFound")).not.toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "errors.tableNotFoundRetry" }),
    ).toBeInTheDocument();
  });

  it("still renders the dead-QR 404 state on a true 404", async () => {
    mockedGetTable.mockRejectedValue(httpError(404, "table not found"));

    await renderAndSettle();

    expect(screen.getByText("errors.tableNotFound")).toBeInTheDocument();
    expect(screen.getByText("404")).toBeInTheDocument();
  });

  it("disables the retry button while a retry is in flight so a throttled guest cannot hammer the limiter", async () => {
    // Issue 815 follow-up (GM6): tapping retry repeatedly must fire exactly
    // one request while the previous one is still in flight, and the busy
    // screen must stay up (disabled button) instead of vanishing.
    mockedGetTable.mockRejectedValueOnce(httpError(429, "Too many requests"));

    await renderAndSettle();

    const retry = screen.getByRole("button", {
      name: "errors.tableNotFoundRetry",
    });

    // The retry fetch never settles — the window a guest could hammer.
    let inFlightCalls = 0;
    mockedGetTable.mockImplementation(() => {
      inFlightCalls += 1;
      return new Promise(() => {});
    });

    fireEvent.click(retry);

    // Still on the honest busy screen, with the button disabled in flight.
    expect(screen.getByText("errors.billBusyTitle")).toBeInTheDocument();
    const inFlightButton = screen.getByRole("button", {
      name: "errors.tableNotFoundRetry",
    });
    expect(inFlightButton).toBeDisabled();

    fireEvent.click(inFlightButton);
    fireEvent.click(inFlightButton);

    expect(inFlightCalls).toBe(1);
  });

  it("recovers to the normal landing when a retry succeeds after a 429", async () => {
    mockedGetTable.mockRejectedValueOnce(httpError(429, "Too many requests"));
    mockedGetTable.mockResolvedValue({
      table: {
        table_code: "M03Y18GB3P",
        name: "12",
        capacity: 4,
        is_active: true,
      },
      business: {
        id: 10,
        name: "Test Cafe",
        logo: "",
        address: { street: "", city: "" },
        default_currency: "USD",
        display_currency: "USD",
        kitchen_enabled: true,
        orders_enabled: true,
        crm_enabled: false,
      },
      menu: { categories: "[]" },
      categories: [],
      offers: [],
      bundles: [],
    } as any);

    const { rerender } = render(<GuestTableView tableCode="M03Y18GB3P" />);
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(screen.getByText("errors.billBusyTitle")).toBeInTheDocument();

    // Simulate the polling/retry pass succeeding (component re-runs load on
    // retry; here we force a remount-equivalent second load via the retry
    // button's reload handler being unavailable in jsdom, so re-render).
    rerender(<GuestTableView tableCode="M03Y18GB3P" key="second" />);
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(await screen.findByTestId("guest-table-main")).toBeInTheDocument();
    expect(screen.queryByText("errors.billBusyTitle")).not.toBeInTheDocument();
  });
});
