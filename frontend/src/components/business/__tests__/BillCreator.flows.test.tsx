/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { BillCreator } from "../BillCreator";

// --- Mocks for the @/api/* boundaries ---
const mockCreateBill = jest.fn();
const mockCreateOrder = jest.fn();
const mockGetTablesWithStatus = jest.fn();
const mockGetMenu = jest.fn();
const mockGetOffers = jest.fn();
const mockGetBundles = jest.fn();
const mockGetAvailableCounters = jest.fn();
const mockGetSummary = jest.fn();
const mockUseOrderQuote = jest.fn();

const mockToastSuccess = jest.fn();
const mockToastError = jest.fn();

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: {
    success: (...a: unknown[]) => mockToastSuccess(...a),
    error: (...a: unknown[]) => mockToastError(...a),
  },
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  // Echo the key so assertions can target the message key directly.
  getTranslation: (key: string) => key,
}));

jest.mock("@/api/bills", () => ({
  createBill: (...a: unknown[]) => mockCreateBill(...a),
  getActiveBillConflictID: (error: {
    response?: { status?: number; data?: { active_bill_id?: unknown } };
  }) =>
    error.response?.status === 409 &&
    typeof error.response?.data?.active_bill_id === "number"
      ? error.response.data.active_bill_id
      : null,
  isActiveBillStatus: (status: string | undefined) =>
    status === "open" || status === "partial",
}));

jest.mock("@/api/business", () => ({
  businessApi: {
    getOffers: (...a: unknown[]) => mockGetOffers(...a),
    getBundles: (...a: unknown[]) => mockGetBundles(...a),
  },
  getTablesWithStatus: (...a: unknown[]) => mockGetTablesWithStatus(...a),
  getMenu: (...a: unknown[]) => mockGetMenu(...a),
}));

jest.mock("@/api/orders", () => ({
  createOrder: (...a: unknown[]) => mockCreateOrder(...a),
}));

jest.mock("@/hooks/useOrderQuote", () => ({
  useOrderQuote: (...a: unknown[]) => mockUseOrderQuote(...a),
}));

jest.mock("@/api/currency", () => ({
  formatCurrency: (amount: number) => `$${(amount || 0).toFixed(2)}`,
}));

jest.mock("@/api/counters", () => ({
  getAvailableCounters: (...a: unknown[]) => mockGetAvailableCounters(...a),
}));

jest.mock("@/api/inventory", () => ({
  inventoryApi: {
    getSummary: (...a: unknown[]) => mockGetSummary(...a),
  },
}));

// ItemCustomizer is irrelevant to these flows.
jest.mock("../ItemCustomizer", () => ({
  ItemCustomizer: () => null,
}));

const occupiedTable = { id: 7, name: "T7", is_active: true, seats: 4 };
const freeTable = { id: 8, name: "T8", is_active: true, seats: 2 };

const simpleMenu = {
  categories: [
    {
      name: "Mains",
      items: [
        {
          id: "item-1",
          name: "Burger",
          price: 10,
          is_available: true,
          options: [],
        },
      ],
    },
  ],
};

// Build a status-board response (getTablesWithStatus) from the fixed table set
// and the list of active bills; a table is "occupied" when a bill targets it.
function statusBoard(
  tables: Array<{ id: number; name: string; is_active: boolean }>,
  activeBills: Array<{ table_id?: number; [k: string]: unknown }>,
) {
  const occupied = new Set(
    activeBills.map((b) => b.table_id).filter((id): id is number => !!id),
  );
  return {
    tables: tables.map((t) => ({
      table: t,
      status: occupied.has(t.id) ? "occupied" : "available",
      active_bills: occupied.has(t.id) ? [{ id: 1 }] : [],
      active_bills_count: occupied.has(t.id) ? 1 : 0,
      active_bill_physical_item_quantity: 0,
      reservations: [],
      reservations_count: 0,
    })),
  };
}

function primeHappyLoad(
  activeBills: Array<{ table_id?: number; [k: string]: unknown }> = [],
) {
  mockGetTablesWithStatus.mockResolvedValue(
    statusBoard([occupiedTable, freeTable], activeBills),
  );
  mockGetMenu.mockResolvedValue(simpleMenu);
  mockGetOffers.mockResolvedValue([]);
  mockGetBundles.mockResolvedValue([]);
  mockGetAvailableCounters.mockResolvedValue({ counters: [] });
  mockGetSummary.mockResolvedValue(null);
}

beforeEach(() => {
  jest.clearAllMocks();
  mockUseOrderQuote.mockReturnValue({
    quote: {
      subtotal: 10,
      discount: 0,
      net_subtotal: 10,
      tax: 0,
      service_fee: 0,
      tip: 0,
      total: 10,
      lines: [{ key: "item-1", unit_price: 10, quantity: 1, subtotal: 10 }],
    },
    isPending: false,
    error: null,
    isValid: true,
    blockedLines: [],
    refresh: jest.fn(),
  });
});

describe("BillCreator accessible empty-location state", () => {
  it("keeps the warning chip and disabled table placeholder at AA text contrast", async () => {
    mockGetTablesWithStatus.mockResolvedValue({ tables: [] });
    mockGetMenu.mockResolvedValue(simpleMenu);
    mockGetOffers.mockResolvedValue([]);
    mockGetBundles.mockResolvedValue([]);
    mockGetAvailableCounters.mockResolvedValue({ counters: [] });
    mockGetSummary.mockResolvedValue(null);

    render(
      <BillCreator
        isOpen
        onClose={jest.fn()}
        businessId={1}
        onBillCreated={jest.fn()}
      />,
    );

    expect(
      await screen.findByText("billCreator.header.locationRequired"),
    ).toHaveClass("!text-amber-950");
    expect(
      await screen.findByText("billCreator.header.itemsSelected", {
        exact: false,
      }),
    ).toHaveClass("!text-ink-950");
    const primaryPrice = (await screen.findAllByText("$10.00")).find((node) =>
      node.classList.contains("!text-ink-950"),
    );
    expect(primaryPrice).toHaveClass("!text-ink-950");

    const disabledPlaceholder = await screen.findByText(
      "billCreator.form.noTablesAvailable",
    );
    expect(disabledPlaceholder).toHaveClass("!text-ink-700");
    expect(disabledPlaceholder.closest('[data-slot="base"]')).toHaveClass(
      "!opacity-100",
    );
    expect(screen.getByText("billCreator.form.allTablesHaveBills")).toHaveClass(
      "text-amber-800",
    );
  });
});

describe("BillCreator deep-link to an occupied table (F2)", () => {
  it("warns instead of silently selecting an occupied deep-linked table", async () => {
    primeHappyLoad([{ id: "bill-1", table_id: 7, status: "open" }]);

    render(
      <BillCreator
        isOpen
        onClose={jest.fn()}
        businessId={1}
        onBillCreated={jest.fn()}
        initialTableId={7}
      />,
    );

    await waitFor(() => {
      expect(mockToastError).toHaveBeenCalledWith(
        "billCreator.messages.tableOccupied",
      );
    });
  });

  it("auto-selects a free deep-linked table without warning", async () => {
    primeHappyLoad([]);

    render(
      <BillCreator
        isOpen
        onClose={jest.fn()}
        businessId={1}
        onBillCreated={jest.fn()}
        initialTableId={8}
      />,
    );

    await waitFor(() => expect(mockGetTablesWithStatus).toHaveBeenCalled());
    expect(mockToastError).not.toHaveBeenCalledWith(
      "billCreator.messages.tableOccupied",
    );
  });
});

describe("BillCreator partial failure: bill created but order fails (F3)", () => {
  it("refreshes the list and shows a targeted order-failed message", async () => {
    primeHappyLoad([]);
    mockCreateBill.mockResolvedValue({ bill: { id: 99 } });
    mockCreateOrder.mockRejectedValue(new Error("kitchen down"));

    const onBillCreated = jest.fn();

    render(
      <BillCreator
        isOpen
        onClose={jest.fn()}
        businessId={1}
        onBillCreated={onBillCreated}
        initialTableId={8}
      />,
    );

    // Wait for the menu item to render, then add it.
    const burger = await screen.findByText("Burger");
    fireEvent.click(burger);

    // Create button label includes the createBill key.
    const createBtn = await screen.findByText(
      /billCreator\.buttons\.createBill/,
    );
    fireEvent.click(createBtn);

    await waitFor(() => expect(mockCreateBill).toHaveBeenCalled());
    await waitFor(() => expect(mockCreateOrder).toHaveBeenCalled());

    // Bill row already persisted -> list must refresh.
    await waitFor(() => expect(onBillCreated).toHaveBeenCalled());
    // Targeted message, not the generic create-failed.
    expect(mockToastError).toHaveBeenCalledWith(
      "billCreator.messages.orderCreateFailed",
    );
    expect(mockToastError).not.toHaveBeenCalledWith(
      "billCreator.messages.billCreateFailed",
    );
  });
});

describe("BillCreator counter occupancy conflict", () => {
  it("navigates to the active bill returned by the 409", async () => {
    primeHappyLoad([]);
    mockCreateBill.mockRejectedValue({
      response: {
        status: 409,
        data: { code: "counter_occupied", active_bill_id: 73 },
      },
    });
    const onViewBill = jest.fn();
    const onBillCreated = jest.fn();
    const onClose = jest.fn();

    render(
      <BillCreator
        isOpen
        onClose={onClose}
        businessId={1}
        onBillCreated={onBillCreated}
        onViewBill={onViewBill}
        initialTableId={8}
      />,
    );

    fireEvent.click(await screen.findByText("Burger"));
    fireEvent.click(
      await screen.findByRole("button", {
        name: /billCreator\.buttons\.createBill/,
      }),
    );

    await waitFor(() => expect(onViewBill).toHaveBeenCalledWith(73));
    expect(onBillCreated).toHaveBeenCalled();
    expect(onClose).toHaveBeenCalled();
    expect(mockCreateOrder).not.toHaveBeenCalled();
    expect(mockToastError).toHaveBeenCalledWith(
      "billCreator.messages.locationOccupied",
    );
  });
});

describe("BillCreator authoritative item orderability", () => {
  it("keeps an inventory-blocked item visible with its reason but cannot add it", async () => {
    primeHappyLoad([]);
    mockGetMenu.mockResolvedValue({
      ...simpleMenu,
      item_orderability: {
        "item-1": { state: "inventory_out", orderable: false },
      },
    });

    render(
      <BillCreator
        isOpen
        onClose={jest.fn()}
        businessId={1}
        onBillCreated={jest.fn()}
      />,
    );

    const burger = await screen.findByText("Burger");
    expect(
      screen.getByText("billCreator.inventoryWarnings.outDescription"),
    ).toBeInTheDocument();
    fireEvent.click(burger);
    expect(
      screen.getByText("billCreator.billSummary.noItemsSelected"),
    ).toBeInTheDocument();
  });

  it("shows inventory warnings while allowing the item to be added", async () => {
    primeHappyLoad([]);
    mockGetMenu.mockResolvedValue({
      ...simpleMenu,
      item_orderability: {
        "item-1": { state: "inventory_warning", orderable: true },
      },
    });

    render(
      <BillCreator
        isOpen
        onClose={jest.fn()}
        businessId={1}
        onBillCreated={jest.fn()}
      />,
    );

    const burger = await screen.findByText("Burger");
    expect(
      screen.getByText("billCreator.inventoryWarnings.lowDescription"),
    ).toBeInTheDocument();
    fireEvent.click(burger);
    await waitFor(() => {
      expect(
        screen.queryByText("billCreator.billSummary.noItemsSelected"),
      ).not.toBeInTheDocument();
    });
  });
});

describe("BillCreator authoritative quote gate", () => {
  it("renders tax, service fee, and the authoritative final total", async () => {
    primeHappyLoad([]);
    mockUseOrderQuote.mockReturnValue({
      quote: {
        subtotal: 10,
        discount: 2,
        net_subtotal: 8,
        tax: 0.8,
        service_fee: 0.4,
        tip: 0,
        total: 9.2,
        lines: [],
      },
      isPending: false,
      error: null,
      isValid: true,
      blockedLines: [],
      refresh: jest.fn(),
    });

    render(
      <BillCreator
        isOpen
        onClose={jest.fn()}
        businessId={1}
        onBillCreated={jest.fn()}
        initialTableId={8}
      />,
    );

    fireEvent.click(await screen.findByText("Burger"));
    expect(screen.getByText("billCreator.billSummary.tax")).toBeInTheDocument();
    expect(
      screen.getByText("billCreator.billSummary.serviceFee"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("billCreator.billSummary.finalTotal"),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", {
        name: /billCreator\.buttons\.createBill \$9\.20/,
      }),
    ).toBeEnabled();
  });

  it.each([
    ["pending", { quote: null, isPending: true, error: null, isValid: false }],
    [
      "failed",
      {
        quote: null,
        isPending: false,
        error: new Error("quote failed"),
        isValid: false,
      },
    ],
    ["missing", { quote: null, isPending: false, error: null, isValid: false }],
  ])(
    "disables Create Bill when the current quote is %s",
    async (_state, quoteState) => {
      primeHappyLoad([]);
      mockUseOrderQuote.mockReturnValue({ ...quoteState, refresh: jest.fn() });

      render(
        <BillCreator
          isOpen
          onClose={jest.fn()}
          businessId={1}
          onBillCreated={jest.fn()}
          initialTableId={8}
        />,
      );

      fireEvent.click(await screen.findByText("Burger"));
      const createButton = await screen.findByRole("button", {
        name: /billCreator\.buttons\.createBill/,
      });
      expect(createButton).toBeDisabled();
      fireEvent.click(createButton);
      expect(mockCreateBill).not.toHaveBeenCalled();
    },
  );

  it("visibly explains and blocks an authoritative orderability rejection", async () => {
    primeHappyLoad([]);
    mockUseOrderQuote.mockReturnValue({
      quote: {
        subtotal: 10,
        discount: 0,
        net_subtotal: 10,
        tax: 0,
        service_fee: 0,
        tip: 0,
        total: 10,
        lines: [
          {
            key: "item-1",
            line_type: "menu_item",
            unit_price: 10,
            quantity: 1,
            subtotal: 10,
            orderability: { state: "inventory_out", orderable: false },
          },
        ],
      },
      isPending: false,
      error: null,
      isValid: false,
      blockedLines: [
        {
          key: "item-1",
          line_type: "menu_item",
          unit_price: 10,
          quantity: 1,
          subtotal: 10,
          orderability: { state: "inventory_out", orderable: false },
        },
      ],
      refresh: jest.fn(),
    });

    render(
      <BillCreator
        isOpen
        onClose={jest.fn()}
        businessId={1}
        onBillCreated={jest.fn()}
        initialTableId={8}
      />,
    );

    fireEvent.click(await screen.findByText("Burger"));
    expect(
      screen.getByText("billCreator.messages.quoteBlocked"),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", {
        name: /billCreator\.buttons\.createBill/,
      }),
    ).toBeDisabled();
  });
});
