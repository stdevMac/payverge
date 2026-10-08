/** @jest-environment jsdom */
import React from "react";
import { render, waitFor } from "@testing-library/react";

/**
 * Audit L5-8, "unfiltered all-customer bills on warm nav" half.
 *
 * The CRM "Ver cuentas" button calls `onNavigateToTab("bills?billTab=history&
 * billCustomer=<id>")`. The dashboard's `commitTabChange` handles that with two
 * updates in one event handler:
 *
 *   setActiveTab(tab)            // urgent — commits immediately
 *   router.push(nextUrl)         // App Router navigation — a transition
 *
 * The urgent update wins, so there is a commit where the bills tab is already
 * rendering while `useSearchParams()` still returns the PREVIOUS URL. BillManager
 * seeds `billTab` and `billCustomer` in `useState` initializers, which run once
 * per mount — during that commit. It therefore mounts filter-less, and its
 * URL-sync effect (which reads the LIVE `window.location`, already carrying the
 * pushed params) then deletes `billCustomer` because its own state is empty.
 *
 * Net effect for the operator: the deep link opens an unfiltered, all-customer
 * bill list AND scrubs the customer out of the address bar, so nothing is left
 * to show the link ever carried one. A previous pass read this code statically,
 * saw `billCustomer` whitelisted, seeded, and passed unconditionally as
 * `customerId`, and closed it NOT-REPRODUCIBLE — all true, but the seed reads a
 * stale snapshot.
 *
 * These tests model only the lag itself: params that arrive after mount. They
 * make no assumption about React's internal scheduling, and they assert the
 * component contract that matters either way — a deep-link param must be
 * honored whenever it arrives, and must never be erased before it is consumed.
 */

const mockRouterReplace = jest.fn();
let mockSearchParamValues: Record<string, string | null> = {};

jest.mock("next/navigation", () => ({
  useSearchParams: () => ({
    get: (key: string) => mockSearchParamValues[key] ?? null,
  }),
  useRouter: () => ({ push: jest.fn(), replace: mockRouterReplace }),
  usePathname: () => "/business/1/dashboard",
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => key,
}));

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => ({
    hasAccess: true,
    isSuspended: false,
    loading: false,
  }),
}));

jest.mock("react-hot-toast", () => {
  const error = jest.fn();
  const success = jest.fn();
  const toastFn = Object.assign(jest.fn(), { error, success });
  return { __esModule: true, default: toastFn, toast: toastFn };
});

const mockGetBusinessBills = jest.fn();

jest.mock("@/api/bills", () => ({
  getBusinessBills: (...args: unknown[]) => mockGetBusinessBills(...args),
  getAllBillsForStatus: jest.fn(),
  getBill: jest.fn(),
  closeBill: jest.fn(),
  isActiveBillStatus: (status: string | undefined) =>
    status === "open" || status === "partial",
}));

jest.mock("@/api/business", () => ({
  getBusiness: jest.fn().mockResolvedValue({
    default_currency: "USD",
    timezone: "UTC",
  }),
}));

jest.mock("../BillCreator", () => ({ BillCreator: () => null }));
jest.mock("../BillFilters", () => ({ BillFilters: () => null }));
jest.mock("../PendingOrdersSection", () => ({
  PendingOrdersSection: () => null,
}));
jest.mock("../KitchenOrdersToggle", () => ({
  KitchenOrdersToggle: () => null,
}));
jest.mock("../BillDisplayMode", () => ({
  __esModule: true,
  default: () => null,
}));
jest.mock("../BillDetailsModal", () => ({ BillDetailsModal: () => null }));
jest.mock("../BillsTable", () => ({ BillsTable: () => null }));

import { BillManager } from "@/components/business/BillManager";

const BUSINESS_ID = 1;
const CUSTOMER_ID = 42;

const renderManager = () =>
  render(
    <BillManager
      businessId={BUSINESS_ID}
      globalBills={[]}
      globalOrders={{}}
      kitchenEnabled
      kitchenStatusLoading={false}
      onKitchenStatusChange={jest.fn()}
      onOrderStatusChange={jest.fn()}
    />,
  );

/** The URL `router.push` already committed, before `useSearchParams` catches up. */
const setLiveUrl = (search: string) =>
  window.history.replaceState(
    {},
    "",
    `/business/${BUSINESS_ID}/dashboard${search}`,
  );

const lastBillsCall = () =>
  mockGetBusinessBills.mock.calls[mockGetBusinessBills.mock.calls.length - 1];

describe("BillManager warm-nav deep link (L5-8)", () => {
  beforeEach(() => {
    mockGetBusinessBills.mockReset();
    mockGetBusinessBills.mockResolvedValue({
      bills: [],
      total: 0,
      total_pages: 1,
    });
    mockRouterReplace.mockReset();
    mockSearchParamValues = {};
    setLiveUrl("?tab=bills");
  });

  it("filters by the customer when billCustomer arrives after mount", async () => {
    // Mount in the stale commit: the bills tab is rendering, the pushed URL is
    // already live, but useSearchParams still reflects the CRM tab.
    setLiveUrl(`?tab=bills&billTab=history&billCustomer=${CUSTOMER_ID}`);
    const { rerender } = renderManager();
    await waitFor(() => expect(mockGetBusinessBills).toHaveBeenCalled());

    // The transition lands and the router propagates the new params.
    mockSearchParamValues = {
      billTab: "history",
      billCustomer: String(CUSTOMER_ID),
    };
    rerender(
      <BillManager
        businessId={BUSINESS_ID}
        globalBills={[]}
        globalOrders={{}}
        kitchenEnabled
        kitchenStatusLoading={false}
        onKitchenStatusChange={jest.fn()}
        onOrderStatusChange={jest.fn()}
      />,
    );

    await waitFor(() =>
      expect(lastBillsCall()?.[1]).toEqual(
        expect.objectContaining({ customerId: CUSTOMER_ID }),
      ),
    );
  });

  it("opens the history tab when billTab arrives after mount", async () => {
    setLiveUrl(`?tab=bills&billTab=history&billCustomer=${CUSTOMER_ID}`);
    const { rerender } = renderManager();
    await waitFor(() => expect(mockGetBusinessBills).toHaveBeenCalled());

    mockSearchParamValues = {
      billTab: "history",
      billCustomer: String(CUSTOMER_ID),
    };
    rerender(
      <BillManager
        businessId={BUSINESS_ID}
        globalBills={[]}
        globalOrders={{}}
        kitchenEnabled
        kitchenStatusLoading={false}
        onKitchenStatusChange={jest.fn()}
        onOrderStatusChange={jest.fn()}
      />,
    );

    // History status set, not the active-bills one — the deep link asked for
    // closed history, and landing on "active" is the wrong-tab symptom.
    await waitFor(() =>
      expect(lastBillsCall()?.[1]).toEqual(
        expect.objectContaining({ status: "paid,closed,voided" }),
      ),
    );
  });

  it("never erases a billCustomer it has not consumed yet", async () => {
    setLiveUrl(`?tab=bills&billTab=history&billCustomer=${CUSTOMER_ID}`);
    renderManager();
    await waitFor(() => expect(mockGetBusinessBills).toHaveBeenCalled());

    // The URL-sync effect runs on mount. It must not rewrite the address bar to
    // a URL that has dropped the deep link — that destroys the operator's only
    // evidence the link carried a customer, and races the pending push.
    const scrubbed = mockRouterReplace.mock.calls
      .map((call) => String(call[0]))
      .filter((url) => !url.includes("billCustomer"));
    expect(scrubbed).toEqual([]);
  });
});
