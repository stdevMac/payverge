/** @jest-environment jsdom */
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";

import * as api from "@/api/print";
import { SplittingAPI } from "@/api/splitting";
import {
  BILL_DETAILS_FOOTER_ACTIONS_CLASS,
  BillDetailsModal,
} from "./BillDetailsModal";
import { ManagerPinProvider } from "./managerPin";

// BillDetailsModal uses React Query (record-payment mutation), so every render
// needs a fresh QueryClient with retries disabled.
function renderWithClient(ui: React.ReactElement) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>{ui}</QueryClientProvider>,
  );
}

jest.mock("@/api/print");
jest.mock("./printers/useIframePrint", () => ({
  useIframePrint: () => ({ print: jest.fn().mockResolvedValue(undefined) }),
}));
// Print Bill is gated on owner or staff print:bill — default owner so print
// flow tests reach the button.
let mockIsStaffUser = false;
let mockStaffPermissions: string[] = [];
jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({ isStaffUser: mockIsStaffUser }),
}));
jest.mock("@/contexts/StaffPermissionsContext", () => ({
  useStaffPermissionsContext: () => ({
    permissions: mockStaffPermissions,
    rolePermissions: [],
    customGrants: [],
    isLoading: false,
    isError: false,
    refetch: jest.fn(),
  }),
}));
jest.mock("@/hooks/useAlternativePayments", () => ({
  useBusinessAlternativePayments: jest.fn(() => ({
    paymentBreakdown: null,
    alternativePayments: [],
    pendingPayments: [],
    loading: false,
    error: null,
    refresh: jest.fn(),
    markPayment: jest.fn(),
  })),
}));
// IMP-15: BillDetailsModal now renders BillVoidRefundActions which calls
// /inside/bills/:id/audit on mount. Stub the bills API so the test render
// stays self-contained and doesn't try to reach a backend.
jest.mock("@/api/bills", () => ({
  ...jest.requireActual("@/api/bills"),
  getBillAudit: jest.fn(() => new Promise(() => {})),
  voidBill: jest.fn(),
  refundBillPayment: jest.fn(),
}));
jest.mock("@/api/splitting", () => ({
  SplittingAPI: {
    getSplitState: jest.fn(() => new Promise(() => {})),
  },
}));
let mockLastSSEOptions:
  | {
      businessId: number;
      enabled: boolean;
      onEvent: (event: { type: string; data: Record<string, unknown> }) => void;
      onReconnect?: () => void;
    }
  | undefined;
jest.mock("@/hooks/useSSEEvents", () => ({
  useSSEEvents: jest.fn((options) => {
    mockLastSSEOptions = options;
    return { degraded: false, blocked: false, reconnect: jest.fn() };
  }),
}));

// SimpleLocale provider mock — BillDetailsModal calls useSimpleLocale().
// Match the shape of the existing provider.
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  // BillVoidRefundActions (rendered inside the modal) calls getTranslation
  // directly; echo the key so assertions stay key-based.
  getTranslation: (key: string) => key,
}));

function fakeBill() {
  return {
    bill: {
      id: 99,
      bill_number: "B-1",
      public_token: "public-token-B-1",
      status: "open",
      total_amount: 0,
      tip_amount: 0,
      paid_amount: 0,
      subtotal: 0,
      tax_amount: 0,
      service_fee_amount: 0,
      table_id: 1,
      notes: "",
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
      business_id: 42,
      items: "[]",
      settlement_address: "",
      tipping_address: "",
    },
    items: [],
    history: [],
  } as unknown as Parameters<typeof BillDetailsModal>[0]["bill"];
}

it("labels sellable item quantity and excludes bundle children and discounts", () => {
  const bill = fakeBill();
  (bill as any).items = [
    {
      id: "menu",
      name: "Burger",
      price: 10,
      quantity: 2,
      subtotal: 20,
      item_type: "menu_item",
    },
    {
      id: "bundle",
      name: "Combo",
      price: 15,
      quantity: 1,
      subtotal: 15,
      item_type: "bundle",
      bundle_id: 10,
      bundle_occurrence_id: "occ-1",
    },
    {
      id: "child",
      name: "Fries",
      price: 0,
      quantity: 1,
      subtotal: 0,
      item_type: "bundle_item",
      parent_bundle_id: 10,
      bundle_occurrence_id: "occ-1",
    },
    {
      id: "discount",
      name: "Promo",
      price: -2,
      quantity: 1,
      subtotal: -2,
      item_type: "discount",
    },
  ];

  renderWithClient(
    <ManagerPinProvider>
      <BillDetailsModal
        isOpen
        onClose={() => {}}
        bill={bill}
        onCloseBill={() => {}}
        onBillUpdated={() => {}}
        businessId={42}
        tString={(key) => {
          if (key === "preparedUnitsOne") return "{count} item";
          if (key === "preparedUnitsOther") return "{count} items";
          if (key === "preparedUnits") return "{count} items";
          if (key === "bundleIncluded") return "Included";
          return key;
        }}
        currency="USD"
      />
    </ManagerPinProvider>,
  );

  // Two sellable LINES (burger + combo). Quantity 2 on the burger must not
  // make the header say 3 — #683.
  expect(screen.getByText("2 items")).toBeInTheDocument();
  expect(screen.queryByText("3 items")).toBeNull();
  expect(screen.getByText("Included")).toBeInTheDocument();
  expect(screen.queryByText(/tableColumns\.items \(4\)/)).toBeNull();
});

it("counts bill header items as lines, not units (#683)", () => {
  const bill = fakeBill();
  (bill as any).items = [
    {
      id: "tacos",
      name: "Tacos",
      price: 12,
      quantity: 1,
      subtotal: 12,
      item_type: "menu_item",
    },
    {
      id: "tea",
      name: "Iced Tea",
      price: 3,
      quantity: 2,
      subtotal: 6,
      item_type: "menu_item",
    },
  ];

  renderWithClient(
    <ManagerPinProvider>
      <BillDetailsModal
        isOpen
        onClose={() => {}}
        bill={bill}
        onCloseBill={() => {}}
        onBillUpdated={() => {}}
        businessId={42}
        tString={(key) => {
          if (key === "preparedUnitsOne") return "{count} item";
          if (key === "preparedUnitsOther") return "{count} items";
          if (key === "preparedUnits") return "{count} items";
          return key;
        }}
        currency="USD"
      />
    </ManagerPinProvider>,
  );

  expect(screen.getByText("2 items")).toBeInTheDocument();
  expect(screen.queryByText("3 items")).toBeNull();
});

it("counts Spanish bill header as lines, not units (#683)", () => {
  const bill = fakeBill();
  (bill as any).items = [
    {
      id: "tacos",
      name: "Tacos",
      price: 12,
      quantity: 1,
      subtotal: 12,
      item_type: "menu_item",
    },
    {
      id: "tea",
      name: "Iced Tea",
      price: 3,
      quantity: 2,
      subtotal: 6,
      item_type: "menu_item",
    },
  ];

  renderWithClient(
    <ManagerPinProvider>
      <BillDetailsModal
        isOpen
        onClose={() => {}}
        bill={bill}
        onCloseBill={() => {}}
        onBillUpdated={() => {}}
        businessId={42}
        tString={(key) => {
          if (key === "preparedUnitsOne") return "{count} ÍTEM";
          if (key === "preparedUnitsOther") return "{count} ÍTEMS";
          if (key === "preparedUnits") return "{count} ÍTEMS";
          return key;
        }}
        currency="USD"
      />
    </ManagerPinProvider>,
  );

  expect(screen.getByText("2 ÍTEMS")).toBeInTheDocument();
  expect(screen.queryByText("3 ÍTEMS")).toBeNull();
});

// A bill carrying a loyalty discount (baked into total_amount by the backend)
// must show a persistent "loyaltyDiscount" line in the operator summary so the
// itemized totals reconcile with the reduced total. Without it, an operator
// opening a redeemed bill sees subtotal+tax+fee that don't add up to the lower
// Total, with nothing explaining the gap.
it("renders a loyalty-discount line in the summary when the bill has one", async () => {
  const bill = fakeBill();
  // Summary only renders when there are items; give it one and a discount.
  (bill as any).items = [
    { id: "i1", name: "Burger", price: 30, quantity: 1, subtotal: 30 },
  ];
  (bill as any).bill.subtotal = 30;
  (bill as any).bill.total_amount = 25;
  (bill as any).bill.loyalty_discount = 5;

  renderWithClient(
    <ManagerPinProvider>
      <BillDetailsModal
        isOpen
        onClose={() => {}}
        bill={bill}
        onCloseBill={() => {}}
        onBillUpdated={() => {}}
        businessId={42}
        tString={(k) => k}
        currency="USD"
      />
    </ManagerPinProvider>,
  );

  // Identity tString renders the i18n key verbatim.
  const label = await screen.findByText("loyaltyDiscount");
  expect(label).toBeInTheDocument();
  // The line shows a negative credit and the discount amount.
  const row = label.parentElement;
  expect(row?.textContent ?? "").toMatch(/-/);
  expect(row?.textContent ?? "").toMatch(/5/);
});

// F2/F3: a counter bill has table_id === 0 (the "no table" sentinel) plus a
// counter_id. The header must read as the counter location, never the raw
// "Table 0". With identity tString the label resolves to the "counter" key.
it("renders a counter bill header as the counter location, not Table 0", () => {
  const bill = fakeBill();
  (bill as any).bill.table_id = 0;
  (bill as any).bill.counter_id = 3;

  renderWithClient(
    <ManagerPinProvider>
      <BillDetailsModal
        isOpen
        onClose={() => {}}
        bill={bill}
        onCloseBill={() => {}}
        onBillUpdated={() => {}}
        businessId={42}
        tString={(k) => k}
        currency="USD"
      />
    </ManagerPinProvider>,
  );

  // The counter label renders (identity tString echoes the key)...
  expect(screen.getByText("counter")).toBeInTheDocument();
  // ...and the raw "Table 0" sentinel never surfaces.
  expect(screen.queryByText(/table\s+0/i)).toBeNull();
});

// G-01: a discount line item renders in its own green "applied discounts" band.
// It must NOT also appear in the main item list (visibleItems previously only
// excluded bundle_item, so a discount showed up twice). Bill math is unaffected
// — the subtotal already nets the discount once; this is a display dedup that
// matches the guest bill, which shows an applied discount exactly once.
it("renders a discount line exactly once (own band, not duplicated in the item list)", () => {
  const bill = fakeBill();
  (bill as any).items = [
    {
      id: "i1",
      name: "VIP Tasting Bundle",
      price: 420,
      quantity: 1,
      subtotal: 420,
      item_type: "menu_item",
    },
    {
      id: "d1",
      name: "Offer: Welcome Discount",
      price: -42,
      quantity: 1,
      subtotal: -42,
      item_type: "discount",
    },
  ];
  (bill as any).bill.subtotal = 378;
  (bill as any).bill.total_amount = 378;

  renderWithClient(
    <ManagerPinProvider>
      <BillDetailsModal
        isOpen
        onClose={() => {}}
        bill={bill}
        onCloseBill={() => {}}
        onBillUpdated={() => {}}
        businessId={42}
        tString={(k) => k}
        currency="USD"
      />
    </ManagerPinProvider>,
  );

  // Exactly one occurrence of the discount name (the green band), not two.
  expect(screen.getAllByText("Offer: Welcome Discount")).toHaveLength(1);
  // The non-discount item still renders.
  expect(screen.getByText("VIP Tasting Bundle")).toBeInTheDocument();
});

// #105 — duplicate promo lines with the same name must stay distinguishable
// (per-order cohort attribution) so operators have control context.
it("attributes duplicate discount lines to their source orders", () => {
  const bill = fakeBill();
  (bill as any).items = [
    {
      id: "i1",
      name: "Burger",
      price: 20,
      quantity: 1,
      subtotal: 20,
      item_type: "menu_item",
    },
    {
      id: "d1",
      name: "Weekday Lunch 15% Off",
      price: -3,
      quantity: 1,
      subtotal: -3,
      item_type: "discount",
      order_id: 101,
    },
    {
      id: "d2",
      name: "Weekday Lunch 15% Off",
      price: -1.5,
      quantity: 1,
      subtotal: -1.5,
      item_type: "discount",
      order_id: 102,
    },
  ];

  renderWithClient(
    <ManagerPinProvider>
      <BillDetailsModal
        isOpen
        onClose={() => {}}
        bill={bill}
        onCloseBill={() => {}}
        onBillUpdated={() => {}}
        businessId={42}
        tString={(k) =>
          k === "discountOrderAttribution" ? "From order #{orderId}" : k
        }
        currency="USD"
      />
    </ManagerPinProvider>,
  );

  expect(screen.getAllByTestId("bill-discount-line")).toHaveLength(2);
  expect(screen.getByText("From order #101")).toBeInTheDocument();
  expect(screen.getByText("From order #102")).toBeInTheDocument();
  // Still excluded from the food list.
  expect(screen.getAllByText("Weekday Lunch 15% Off")).toHaveLength(2);
  expect(screen.getByText("Burger")).toBeInTheDocument();
});

it("shows each repeated bundle occurrence with only its own children", () => {
  const bill = fakeBill();
  (bill as any).items = [
    {
      id: "bundle-a",
      name: "Combo",
      price: 15,
      quantity: 1,
      subtotal: 15,
      item_type: "bundle",
      bundle_id: 10,
      bundle_occurrence_id: "bundle-a",
    },
    {
      id: "child-a",
      name: "Burger",
      price: 12,
      quantity: 1,
      subtotal: 0,
      item_type: "bundle_item",
      parent_bundle_id: 10,
      bundle_occurrence_id: "bundle-a",
    },
    {
      id: "bundle-b",
      name: "Combo",
      price: 15,
      quantity: 1,
      subtotal: 15,
      item_type: "bundle",
      bundle_id: 10,
      bundle_occurrence_id: "bundle-b",
    },
    {
      id: "child-b",
      name: "Cola",
      price: 4,
      quantity: 1,
      subtotal: 0,
      item_type: "bundle_item",
      parent_bundle_id: 10,
      bundle_occurrence_id: "bundle-b",
    },
  ];

  renderWithClient(
    <ManagerPinProvider>
      <BillDetailsModal
        bill={bill}
        isOpen
        onClose={jest.fn()}
        onCloseBill={jest.fn()}
        onBillUpdated={jest.fn()}
        tString={(key) => key}
        businessId={42}
      />
    </ManagerPinProvider>,
  );

  expect(screen.getAllByText("1 × Burger")).toHaveLength(1);
  expect(screen.getAllByText("1 × Cola")).toHaveLength(1);
});

it("reconstructs separate repeated bundle occurrences in legacy snapshots", () => {
  const bill = fakeBill();
  (bill as any).items = [
    {
      id: "legacy-bundle-a",
      name: "Combo",
      price: 15,
      quantity: 1,
      subtotal: 15,
      item_type: "bundle",
      bundle_id: 10,
    },
    {
      id: "legacy-child-a",
      name: "Burger",
      price: 12,
      quantity: 1,
      subtotal: 0,
      item_type: "bundle_item",
      parent_bundle_id: 10,
    },
    {
      id: "legacy-bundle-b",
      name: "Combo",
      price: 15,
      quantity: 1,
      subtotal: 15,
      item_type: "bundle",
      bundle_id: 10,
    },
    {
      id: "legacy-child-b",
      name: "Cola",
      price: 4,
      quantity: 1,
      subtotal: 0,
      item_type: "bundle_item",
      parent_bundle_id: 10,
    },
  ];
  (bill as any).bill.items = JSON.stringify((bill as any).items);

  renderWithClient(
    <ManagerPinProvider>
      <BillDetailsModal
        bill={bill}
        isOpen
        onClose={jest.fn()}
        onCloseBill={jest.fn()}
        onBillUpdated={jest.fn()}
        tString={(key) => key}
        businessId={42}
      />
    </ManagerPinProvider>,
  );

  expect(screen.getAllByText("1 × Burger")).toHaveLength(1);
  expect(screen.getAllByText("1 × Cola")).toHaveLength(1);
});

it("enqueues a bill job for the station without printing a second local copy", async () => {
  (api.createBillPrintJob as jest.Mock).mockResolvedValueOnce({
    id: 5,
    payload_html: "<html>hi</html>",
  });

  renderWithClient(
    <ManagerPinProvider>
      <BillDetailsModal
        isOpen
        onClose={() => {}}
        bill={fakeBill()}
        onCloseBill={() => {}}
        onBillUpdated={() => {}}
        businessId={42}
        tString={(k) => k}
        currency="USD"
      />
    </ManagerPinProvider>,
  );
  // tString is the identity function here, so the print button renders its
  // i18n key verbatim rather than a translated "Print Bill" label.
  fireEvent.click(screen.getByRole("button", { name: "buttons.printBill" }));

  await waitFor(() =>
    expect(api.createBillPrintJob).toHaveBeenCalledWith(42, 99, "en"),
  );
});

// Print Bill is hidden for staff without print:bill.
it("hides the Print Bill button when staff lack print:bill", () => {
  mockIsStaffUser = true;
  mockStaffPermissions = ["bills:read"];

  renderWithClient(
    <ManagerPinProvider>
      <BillDetailsModal
        isOpen
        onClose={() => {}}
        bill={fakeBill()}
        onCloseBill={() => {}}
        onBillUpdated={() => {}}
        businessId={42}
        tString={(k) => k}
        currency="USD"
      />
    </ManagerPinProvider>,
  );

  expect(
    screen.queryByRole("button", { name: "buttons.printBill" }),
  ).toBeNull();
  mockIsStaffUser = false;
  mockStaffPermissions = [];
});

// F4: the header status must render via the shared StatusChip with a translated
// label, not the raw uppercased status code, and "Bill #" must be localized.
it("renders the bill status via StatusChip with a translated label, not a raw code", () => {
  const bill = fakeBill();
  (bill as any).bill.status = "partial";

  const tString = (k: string) =>
    k === "billStatuses.partial"
      ? "Parcial"
      : k === "billNumber"
        ? "Cuenta"
        : k;

  renderWithClient(
    <ManagerPinProvider>
      <BillDetailsModal
        isOpen
        onClose={() => {}}
        bill={bill}
        onCloseBill={() => {}}
        onBillUpdated={() => {}}
        businessId={42}
        tString={tString}
        currency="USD"
      />
    </ManagerPinProvider>,
  );

  // Translated status label present; raw uppercased code absent.
  expect(screen.getByText("Parcial")).toBeInTheDocument();
  expect(screen.queryByText("PARTIAL")).toBeNull();
  // Localized "Bill #" prefix.
  expect(screen.getByText(/Cuenta #/)).toBeInTheDocument();
});

// B1 (regression from F4): a voided bill must read as "this is reversed" — the
// status chip carries the danger tone (rose), not the neutral warm-gray of the
// UNKNOWN fallback. Before BILL_STATUSES gained a `voided` entry, a voided bill
// fell through to UNKNOWN_ENTRY (neutral tone + AlertCircle icon), silently
// dropping the danger signal the deleted getStatusColor used to apply.
it("renders a voided bill status with the danger tone, not the unknown fallback", () => {
  const bill = fakeBill();
  (bill as any).bill.status = "voided";

  // Identity tString -> labelOverride becomes the raw key, so assert on the
  // chip's tone/label data attributes (how StatusChip exposes its mapping).
  renderWithClient(
    <ManagerPinProvider>
      <BillDetailsModal
        isOpen
        onClose={() => {}}
        bill={bill}
        onCloseBill={() => {}}
        onBillUpdated={() => {}}
        businessId={42}
        tString={(k) => k}
        currency="USD"
      />
    </ManagerPinProvider>,
  );

  const chip = document.querySelector(
    '[data-status-kind="bill"][data-status="voided"]',
  );
  expect(chip).not.toBeNull();
  // Danger tone, not the neutral UNKNOWN fallback.
  expect(chip?.getAttribute("data-status-tone")).toBe("danger");
  expect(chip?.getAttribute("data-status-tone")).not.toBe("neutral");
  // Resolved to the real voided entry, not the "Unknown" fallback label.
  expect(chip?.textContent ?? "").not.toMatch(/Unknown/);
});

it("shows live split progress and payer roster in the operator bill modal", async () => {
  (SplittingAPI.getSplitState as jest.Mock).mockResolvedValueOnce({
    bill_number: "B-1",
    status: "partial",
    total_amount: 100,
    total_cents: 10000,
    paid_amount: 40,
    paid_cents: 4000,
    held_amount: 25,
    held_cents: 2500,
    available_amount: 35,
    available_cents: 3500,
    updated_at: new Date().toISOString(),
    shares: [
      {
        id: 1,
        display_name: "Sara",
        mode: "items",
        amount: 25,
        amount_cents: 2500,
        status: "held",
      },
      {
        id: 2,
        display_name: "Guest 2",
        mode: "custom",
        amount: 40,
        amount_cents: 4000,
        tip_amount: 6,
        tip_cents: 600,
        status: "settled",
        tender: "paypal",
      },
    ],
  });

  const bill = fakeBill();
  (bill as any).bill.status = "partial";
  (bill as any).bill.total_amount = 100;
  (bill as any).bill.paid_amount = 40;
  (bill as any).items = [
    { id: "i1", name: "Shared mezze", price: 100, quantity: 1, subtotal: 100 },
  ];

  renderWithClient(
    <ManagerPinProvider>
      <BillDetailsModal
        isOpen
        onClose={() => {}}
        bill={bill}
        onCloseBill={() => {}}
        onBillUpdated={() => {}}
        businessId={42}
        tString={(k) => k}
        currency="USD"
      />
    </ManagerPinProvider>,
  );

  expect(await screen.findByText("splitSummary.title")).toBeInTheDocument();
  expect(screen.getByText("splitSummary.paidOfTotal")).toBeInTheDocument();
  expect(screen.getByText("splitSummary.held")).toBeInTheDocument();
  expect(screen.getByText("splitSummary.available")).toBeInTheDocument();
  expect(screen.getByText("Sara")).toBeInTheDocument();
  expect(screen.getByText("Guest 2")).toBeInTheDocument();
  expect(screen.getByText("splitSummary.status.held")).toBeInTheDocument();
  expect(screen.getByText("splitSummary.status.settled")).toBeInTheDocument();
  expect(
    screen.getByRole("link", { name: "splitSummary.managePayments" }),
  ).toHaveAttribute("href", "/business/42/bills/99/alternative-payments");
});

it("loads operator split state with the bill public token, never its display number", async () => {
  (SplittingAPI.getSplitState as jest.Mock).mockResolvedValueOnce({
    bill_number: "B-1",
    status: "open",
    total_amount: 0,
    total_cents: 0,
    paid_amount: 0,
    paid_cents: 0,
    held_amount: 0,
    held_cents: 0,
    available_amount: 0,
    available_cents: 0,
    updated_at: new Date().toISOString(),
    shares: [],
  });

  renderWithClient(
    <ManagerPinProvider>
      <BillDetailsModal
        isOpen
        onClose={() => {}}
        bill={fakeBill()}
        onCloseBill={() => {}}
        onBillUpdated={() => {}}
        businessId={42}
        tString={(key) => key}
        currency="USD"
      />
    </ManagerPinProvider>,
  );

  await waitFor(() =>
    expect(SplittingAPI.getSplitState).toHaveBeenCalledWith("public-token-B-1"),
  );
  expect(SplittingAPI.getSplitState).not.toHaveBeenCalledWith("B-1");
});

it("refetches the operator split summary on a slimmed bill split SSE frame", async () => {
  mockLastSSEOptions = undefined;
  // Initial mount fetch: one held share.
  (SplittingAPI.getSplitState as jest.Mock).mockResolvedValueOnce({
    bill_number: "B-1",
    status: "partial",
    total_amount: 100,
    total_cents: 10000,
    paid_amount: 0,
    paid_cents: 0,
    held_amount: 25,
    held_cents: 2500,
    available_amount: 75,
    available_cents: 7500,
    updated_at: new Date().toISOString(),
    shares: [
      {
        id: 1,
        display_name: "Sara",
        mode: "items",
        amount: 25,
        amount_cents: 2500,
        status: "held",
      },
    ],
  });
  // The refetch triggered by the slim SSE frame returns the settled state with
  // per-share amounts/tender — proving the amounts come from REST, not the frame.
  (SplittingAPI.getSplitState as jest.Mock).mockResolvedValueOnce({
    bill_number: "B-1",
    status: "partial",
    total_amount: 100,
    total_cents: 10000,
    paid_amount: 25,
    paid_cents: 2500,
    held_amount: 0,
    held_cents: 0,
    available_amount: 75,
    available_cents: 7500,
    updated_at: new Date().toISOString(),
    shares: [
      {
        id: 1,
        display_name: "Sara",
        mode: "items",
        amount: 25,
        amount_cents: 2500,
        status: "settled",
        tender: "paypal",
      },
    ],
  });

  const bill = fakeBill();
  (bill as any).bill.status = "partial";
  (bill as any).bill.total_amount = 100;
  (bill as any).bill.paid_amount = 0;

  renderWithClient(
    <ManagerPinProvider>
      <BillDetailsModal
        isOpen
        onClose={() => {}}
        bill={bill}
        onCloseBill={() => {}}
        onBillUpdated={() => {}}
        businessId={42}
        tString={(k) => k}
        currency="USD"
      />
    </ManagerPinProvider>,
  );

  expect(await screen.findByText("Sara")).toBeInTheDocument();
  expect(screen.getByText("splitSummary.status.held")).toBeInTheDocument();
  expect(mockLastSSEOptions).toMatchObject({ businessId: 42, enabled: true });

  // Slim operator-stream frame: counts + status only, NO per-share money/tender.
  act(() => {
    mockLastSSEOptions?.onEvent({
      type: "bill.split.updated",
      data: {
        bill_number: "B-1",
        status: "partial",
        share_count: 1,
        settled_count: 1,
        held_count: 0,
        updated_at: new Date().toISOString(),
      },
    });
  });

  expect(
    await screen.findByText("splitSummary.status.settled"),
  ).toBeInTheDocument();
  expect(screen.queryByText("splitSummary.status.held")).toBeNull();
  expect(screen.getByText(/paypal/)).toBeInTheDocument();
  expect(screen.getByText(/\$25\.00 \/ \$100\.00/)).toBeInTheDocument();
});

it("keeps the newest operator split summary when an older fetch resolves late", async () => {
  mockLastSSEOptions = undefined;
  let resolveInitialFetch: (state: unknown) => void = () => {};
  (SplittingAPI.getSplitState as jest.Mock).mockReturnValueOnce(
    new Promise((resolve) => {
      resolveInitialFetch = resolve;
    }),
  );
  (SplittingAPI.getSplitState as jest.Mock).mockResolvedValueOnce({
    bill_number: "B-1",
    status: "partial",
    total_amount: 100,
    total_cents: 10000,
    paid_amount: 25,
    paid_cents: 2500,
    held_amount: 0,
    held_cents: 0,
    available_amount: 75,
    available_cents: 7500,
    updated_at: new Date().toISOString(),
    shares: [
      {
        id: 1,
        display_name: "Sara",
        mode: "items",
        amount: 25,
        amount_cents: 2500,
        status: "settled",
        tender: "paypal",
      },
    ],
  });

  const bill = fakeBill();
  (bill as any).bill.status = "partial";
  (bill as any).bill.total_amount = 100;
  (bill as any).bill.paid_amount = 0;

  renderWithClient(
    <ManagerPinProvider>
      <BillDetailsModal
        isOpen
        onClose={() => {}}
        bill={bill}
        onCloseBill={() => {}}
        onBillUpdated={() => {}}
        businessId={42}
        tString={(k) => k}
        currency="USD"
      />
    </ManagerPinProvider>,
  );

  expect(mockLastSSEOptions).toMatchObject({ businessId: 42, enabled: true });
  act(() => {
    mockLastSSEOptions?.onEvent({
      type: "bill.split.updated",
      data: {
        bill_number: "B-1",
        status: "partial",
        share_count: 1,
        settled_count: 1,
        held_count: 0,
        updated_at: new Date().toISOString(),
      },
    });
  });

  expect(
    await screen.findByText("splitSummary.status.settled"),
  ).toBeInTheDocument();
  expect(screen.getByText(/paypal/)).toBeInTheDocument();

  await act(async () => {
    resolveInitialFetch({
      bill_number: "B-1",
      status: "partial",
      total_amount: 100,
      total_cents: 10000,
      paid_amount: 0,
      paid_cents: 0,
      held_amount: 25,
      held_cents: 2500,
      available_amount: 75,
      available_cents: 7500,
      updated_at: new Date().toISOString(),
      shares: [
        {
          id: 1,
          display_name: "Sara",
          mode: "items",
          amount: 25,
          amount_cents: 2500,
          status: "held",
        },
      ],
    });
  });

  expect(screen.getByText("splitSummary.status.settled")).toBeInTheDocument();
  expect(screen.queryByText("splitSummary.status.held")).toBeNull();
});

// Q-2: a typed BillItem renders its price/subtotal, proving the `(item: any)` +
// `|| 0` masks are gone. Reuses this file's fakeBill()/mocks.
it("renders a typed item price and subtotal (Q-2: no || 0 money mask)", () => {
  const bill = fakeBill();
  (bill as any).items = [
    {
      id: "i1",
      name: "Latte",
      price: 6.25,
      quantity: 2,
      subtotal: 12.5,
      item_type: "menu_item",
    },
  ];
  (bill as any).bill.subtotal = 12.5;
  (bill as any).bill.total_amount = 12.5;

  renderWithClient(
    <ManagerPinProvider>
      <BillDetailsModal
        isOpen
        onClose={() => {}}
        bill={bill}
        onCloseBill={() => {}}
        onBillUpdated={() => {}}
        businessId={42}
        tString={(k) => k}
        currency="USD"
      />
    </ManagerPinProvider>,
  );

  // $6.25 appears in the "2 × $6.25" unit-price cell.
  expect(screen.getByText(/\$6\.25/)).toBeInTheDocument();
  // $12.50 may appear in multiple places (item subtotal + bill total); at least one renders.
  expect(screen.getAllByText(/\$12\.50/).length).toBeGreaterThan(0);
});

// L6-3: PaymentHistory / outstanding drilldown open closed bills in operator
// mode. Fiscal write must be gated on isEditableBill (open only); closed bills
// must not expose a save control, and operator-mode copy must not use guest
// "your details" voice.
const nyFiscalT = (key: string) => {
  const copy: Record<string, string> = {
    "fiscalCustomer.toggle": "Need a fiscal invoice with your details?",
    "fiscalCustomer.operatorToggle":
      "Need a fiscal invoice with customer details?",
    "fiscalCustomer.hint":
      "Optional. Add your tax ID so we can issue a fiscal invoice (factura) in your name.",
    "fiscalCustomer.docTypeLabel": "Document type",
    "fiscalCustomer.docTypeNone": "Consumidor Final",
    "fiscalCustomer.docTypeDni": "DNI",
    "fiscalCustomer.docTypeCuit": "CUIT",
    "fiscalCustomer.docTypeCuil": "CUIL",
    "fiscalCustomer.taxConditionLabel": "Tax condition",
    "fiscalCustomer.taxConsumidorFinal": "Consumidor Final",
    "fiscalCustomer.taxResponsableInscripto": "Responsable Inscripto",
    "fiscalCustomer.cuitHint":
      "A Factura A requires a CUIT/CUIL. Without one, we'll issue a Factura B.",
    "fiscalCustomer.save": "Save",
  };
  return copy[key] ?? key;
};

function expandOperatorFiscalIfPresent() {
  const toggle =
    screen.queryByText("Need a fiscal invoice with your details?") ||
    screen.queryByText("Need a fiscal invoice with customer details?");
  if (!toggle) return;
  fireEvent.click(toggle);
  const tax = screen.queryByLabelText("Tax condition");
  if (tax) {
    fireEvent.change(tax, { target: { value: "responsable_inscripto" } });
  }
}

describe("BillDetailsModal fiscal identity country gate (#548)", () => {
  it("does not render Consumidor Final / CUIT / Factura A for a NY/USD venue", () => {
    renderWithClient(
      <ManagerPinProvider>
        <BillDetailsModal
          isOpen
          onClose={() => {}}
          bill={fakeBill()}
          onCloseBill={() => {}}
          onBillUpdated={() => {}}
          businessId={42}
          tString={nyFiscalT}
          currency="USD"
          country="US"
          mode="operator"
        />
      </ManagerPinProvider>,
    );

    expandOperatorFiscalIfPresent();

    expect(screen.queryAllByText("Consumidor Final")).toHaveLength(0);
    expect(screen.queryAllByText("CUIT")).toHaveLength(0);
    expect(screen.queryAllByText(/Factura A/)).toHaveLength(0);
    expect(screen.queryByTestId("bill-fiscal-identity")).not.toBeInTheDocument();
  });

  it("fails closed when the venue country is unknown", () => {
    renderWithClient(
      <ManagerPinProvider>
        <BillDetailsModal
          isOpen
          onClose={() => {}}
          bill={fakeBill()}
          onCloseBill={() => {}}
          onBillUpdated={() => {}}
          businessId={42}
          tString={nyFiscalT}
          currency="USD"
        />
      </ManagerPinProvider>,
    );

    expandOperatorFiscalIfPresent();

    expect(screen.queryAllByText("Consumidor Final")).toHaveLength(0);
    expect(screen.queryAllByText("CUIT")).toHaveLength(0);
    expect(screen.queryAllByText(/Factura A/)).toHaveLength(0);
    expect(screen.queryByTestId("bill-fiscal-identity")).not.toBeInTheDocument();
  });

  it("still renders the AFIP identity form for an AR venue", () => {
    renderWithClient(
      <ManagerPinProvider>
        <BillDetailsModal
          isOpen
          onClose={() => {}}
          bill={fakeBill()}
          onCloseBill={() => {}}
          onBillUpdated={() => {}}
          businessId={42}
          tString={nyFiscalT}
          currency="ARS"
          country="AR"
          mode="operator"
        />
      </ManagerPinProvider>,
    );

    fireEvent.click(
      screen.getByText("Need a fiscal invoice with customer details?"),
    );
    fireEvent.change(screen.getByLabelText("Tax condition"), {
      target: { value: "responsable_inscripto" },
    });

    expect(screen.getByTestId("bill-fiscal-identity")).toBeInTheDocument();
    expect(screen.getAllByText("Consumidor Final").length).toBeGreaterThan(0);
    expect(screen.getByText("CUIT")).toBeInTheDocument();
    expect(screen.getByText(/Factura A/)).toBeInTheDocument();
  });
});

describe("L6-3 operator mode fiscal gating", () => {
  it("hides fiscal save and disables fiscal fields on a closed bill in operator mode", async () => {
    const bill = fakeBill();
    (bill as any).bill.status = "closed";
    (bill as any).bill.fiscal_customer_doc_number = "20123456789";
    (bill as any).bill.fiscal_customer_doc_type = "CUIT";
    (bill as any).bill.fiscal_customer_name = "Acme SA";

    renderWithClient(
      <ManagerPinProvider>
        <BillDetailsModal
          isOpen
          onClose={() => {}}
          bill={bill}
          onCloseBill={() => {}}
          onBillUpdated={() => {}}
          businessId={42}
          mode="operator"
          tString={(k) => k}
          currency="USD"
          country="AR"
        />
      </ManagerPinProvider>,
    );

    // Operator-voiced toggle key (identity tString returns the key).
    expect(
      screen.getByText("fiscalCustomer.operatorToggle"),
    ).toBeInTheDocument();
    // Guest-voiced toggle must not appear.
    expect(screen.queryByText("fiscalCustomer.toggle")).toBeNull();

    // Expand fiscal panel (defaultExpanded when identity present).
    expect(screen.queryByText("fiscalCustomer.save")).toBeNull();

    const docInput = screen.getByLabelText("fiscalCustomer.docNumberLabel");
    expect(docInput).toBeDisabled();
  });

  it("allows fiscal save on an open bill even in operator mode", () => {
    const bill = fakeBill();
    (bill as any).bill.status = "open";

    renderWithClient(
      <ManagerPinProvider>
        <BillDetailsModal
          isOpen
          onClose={() => {}}
          bill={bill}
          onCloseBill={() => {}}
          onBillUpdated={() => {}}
          businessId={42}
          mode="operator"
          tString={(k) => k}
          currency="USD"
          country="AR"
        />
      </ManagerPinProvider>,
    );

    // Expand if collapsed.
    const toggle = screen.getByText("fiscalCustomer.operatorToggle");
    fireEvent.click(toggle);
    expect(screen.getByText("fiscalCustomer.save")).toBeInTheDocument();
  });
});

const footerLabels: Record<string, string> = {
  "buttons.close": "Close",
  "buttons.printBill": "Print bill",
  "buttons.closeWithoutPayment": "Close without payment",
  "buttons.closeBill": "Close Bill",
  "recordPayment.actions.record": "Record payment",
};

// Related to issue 368: sticky footer at 390 clipped to "without pay" / "cord payme".
it("stacks footer actions so every money label is fully visible", () => {
  const bill = fakeBill();
  (bill as any).items = [
    {
      id: "i1",
      name: "Harvest Bowl",
      price: 18.5,
      quantity: 1,
      subtotal: 18.5,
    },
  ];
  (bill as any).bill.total_amount = 21.53;
  (bill as any).bill.paid_amount = 0;
  (bill as any).bill.status = "open";

  renderWithClient(
    <ManagerPinProvider>
      <BillDetailsModal
        isOpen
        onClose={() => {}}
        bill={bill}
        onCloseBill={() => {}}
        onBillUpdated={() => {}}
        businessId={42}
        tString={(key) => footerLabels[key] ?? key}
        currency="USD"
      />
    </ManagerPinProvider>,
  );

  const footer = screen.getByTestId("bill-details-footer");
  expect(footer.className).toBe(BILL_DETAILS_FOOTER_ACTIONS_CLASS);
  expect(footer.className).toMatch(/flex-col-reverse/);
  expect(footer.className).toMatch(/sm:flex-wrap/);
  expect(footer.className).not.toMatch(/flex-nowrap/);

  const close = within(footer).getByRole("button", { name: "Close" });
  const print = within(footer).getByRole("button", { name: "Print bill" });
  const closeUnpaid = within(footer).getByRole("button", {
    name: "Close without payment",
  });
  const record = within(footer).getByRole("button", { name: "Record payment" });

  expect(close).toHaveTextContent(/^Close$/);
  expect(print).toHaveTextContent(/^Print bill$/);
  expect(closeUnpaid).toHaveTextContent(/^Close without payment$/);
  expect(record).toHaveTextContent(/^Record payment$/);

  expect(footer.textContent).toContain("Close without payment");
  expect(footer.textContent).toContain("Record payment");

  for (const button of [close, print, closeUnpaid, record]) {
    expect(button.className).toMatch(/w-full/);
    expect(button.className).toMatch(/whitespace-nowrap/);
    expect(button.className).toMatch(/sm:w-auto/);
  }

  // Related to issue 373: in-card Record payment sat under the stacked
  // footer at 390. Keep the trigger for the footer click, hide it visually.
  const inCard = screen.getByTestId("bill-record-payment-open");
  expect(inCard.className).toMatch(/max-sm:hidden/);
});

it("closes on Escape so the bill detail is not stuck in the URL (#106)", () => {
  const onClose = jest.fn();
  renderWithClient(
    <ManagerPinProvider>
      <BillDetailsModal
        isOpen
        onClose={onClose}
        bill={fakeBill()}
        onCloseBill={() => {}}
        onBillUpdated={() => {}}
        businessId={42}
        tString={(key) => footerLabels[key] ?? key}
        currency="USD"
      />
    </ManagerPinProvider>,
  );

  fireEvent.keyDown(window, { key: "Escape" });
  expect(onClose).toHaveBeenCalled();
});

// Guest checkout writes bill rows atomically, so approving that order adds
// nothing and the backend records items_added: 0 with already_billed: true.
// The timeline must not tell staff "0 item(s) moved into the live bill" for
// an order whose items are plainly on the bill.
it("does not claim 0 items moved when an approved guest order was already billed", () => {
  const bill = fakeBill();
  (bill as any).history = [
    {
      id: 1,
      bill_id: 99,
      business_id: 42,
      event_type: "order.approved",
      order_number: "G7-1",
      actor_email: "owner@example.com",
      created_at: new Date().toISOString(),
      details: { items_added: 0, already_billed: true },
    },
    {
      id: 2,
      bill_id: 99,
      business_id: 42,
      event_type: "order.approved",
      order_number: "S-2",
      actor_email: "owner@example.com",
      created_at: new Date().toISOString(),
      details: { items_added: 3 },
    },
  ];
  const copy: Record<string, string> = {
    "history.events.orderApprovedDescription":
      "{count} item(s) moved into the live bill.",
    "history.events.orderApprovedAlreadyBilledDescription":
      "Items were already on the bill; approval sent them to the kitchen.",
  };

  renderWithClient(
    <ManagerPinProvider>
      <BillDetailsModal
        isOpen
        onClose={() => {}}
        bill={bill}
        onCloseBill={() => {}}
        onBillUpdated={() => {}}
        businessId={42}
        tString={(k) => copy[k] ?? k}
        currency="USD"
      />
    </ManagerPinProvider>,
  );

  expect(screen.queryByText(/^0 item\(s\) moved/)).toBeNull();
  expect(
    screen.getByText(
      "Items were already on the bill; approval sent them to the kitchen.",
    ),
  ).toBeInTheDocument();
  expect(
    screen.getByText("3 item(s) moved into the live bill."),
  ).toBeInTheDocument();
});
