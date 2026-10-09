/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";
import DispatchConsole, { isSameBusinessDay } from "../DispatchConsole";
import * as deliveryApiModule from "@/api/delivery";

// Mock the delivery transport only — the module's pure helpers (notably
// `getDeliveryClaimConflict`, which every mutation's catch block runs) stay
// real so error paths exercise the actual 409 mapping.
jest.mock("@/api/delivery", () => ({
  ...jest.requireActual("@/api/delivery"),
  deliveryApi: {
    getBusinessDeliveries: jest.fn(),
    getAvailableDrivers: jest.fn(),
    getDeliveryOrder: jest.fn(),
    updateDeliveryOrderStatus: jest.fn(),
    assignDriver: jest.fn(),
    cancelDeliveryOrder: jest.fn(),
    claimDeliveryOrder: jest.fn(),
    releaseDeliveryOrder: jest.fn(),
  },
}));

jest.mock("../useDeliveryQueries", () => ({
  useDeliveryBusiness: () => ({
    data: { default_currency: "USD", timezone: "UTC" },
    isLoading: false,
  }),
}));

// The claim affordances read the acting principal. `staffData: null` is the
// owner principal — the one allowed to take over another actor's claim.
jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({ staffData: null }),
}));

// Mock toast with stable function references.
jest.mock("@/contexts/ToastContext", () => {
  const showSuccess = jest.fn();
  const showError = jest.fn();
  return {
    useToast: () => ({ showSuccess, showError }),
    __toastFns: { showSuccess, showError },
  };
});

// Mock translation. The truncation banner key keeps its placeholders so the
// component-side {shown}/{total} interpolation is exercised by tests.
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (_key: string) =>
    _key === "deliverySettings.dispatch.truncation.showing"
      ? "Showing {shown} of {total}"
      : _key,
}));

const mockUseSSEEvents = jest.fn();
jest.mock("@/hooks/useSSEEvents", () => ({
  useSSEEvents: (options: unknown) => {
    mockUseSSEEvents(options);
    return { retriesExhausted: false, reconnect: jest.fn() };
  },
}));

// Mock NextUI
jest.mock("@nextui-org/react", () => ({
  Button: ({ children, onPress, isLoading, isDisabled, startContent: _sc, endContent: _ec, "aria-label": ariaLabel, ...rest }: any) => (
    <button type="button" onClick={onPress} disabled={isDisabled || isLoading} aria-label={ariaLabel} {...rest}>
      {isLoading ? "loading" : children}
    </button>
  ),
  Chip: ({ children }: any) => <span>{children}</span>,
  Skeleton: () => <div data-testid="skeleton" />,
  Input: ({ label, value, onChange, placeholder }: any) => (
    <input aria-label={label || placeholder} placeholder={placeholder} value={value ?? ""} onChange={onChange} />
  ),
  Textarea: ({ label, value, onChange, onValueChange, placeholder, isDisabled }: any) => (
    <textarea
      aria-label={label || placeholder}
      placeholder={placeholder}
      value={value ?? ""}
      disabled={isDisabled}
      onChange={(e) => {
        onChange?.(e);
        onValueChange?.(e.target.value);
      }}
    />
  ),
  Select: ({ children, label, onChange }: any) => (
    <select aria-label={label} onChange={onChange}>{children}</select>
  ),
  SelectItem: ({ children, value }: any) => <option value={value}>{children}</option>,
  Modal: ({ isOpen, children }: any) =>
    isOpen ? <div data-testid="modal">{children}</div> : null,
  ModalContent: ({ children }: any) => (
    <div>{typeof children === "function" ? children(() => {}) : children}</div>
  ),
  ModalHeader: ({ children }: any) => <div>{children}</div>,
  ModalBody: ({ children }: any) => <div>{children}</div>,
  ModalFooter: ({ children }: any) => <div>{children}</div>,
  Spinner: () => <div data-testid="spinner" />,
  Drawer: ({ isOpen, children }: any) =>
    isOpen ? <div data-testid="drawer">{children}</div> : null,
  DrawerContent: ({ children }: any) => (
    <div>{typeof children === "function" ? children(() => {}) : children}</div>
  ),
  DrawerBody: ({ children }: any) => <div>{children}</div>,
}));

const mockOrders = [
  {
    id: 1,
    business_id: 1,
    delivery_number: "DEL-001",
    delivery_type: "in_house",
    status: "pending",
    customer_name: "Alice Smith",
    customer_phone: "555-0001",
    delivery_address: { street: "1 Main St", city: "Austin", country: "US" },
    delivery_fee: 3.99,
    contactless_delivery: false,
    leave_at_door: false,
    created_at: "2026-01-01T10:00:00Z",
    updated_at: "2026-01-01T10:00:00Z",
  },
  {
    id: 2,
    business_id: 1,
    delivery_number: "DEL-002",
    delivery_type: "in_house",
    status: "delivered",
    customer_name: "Bob Jones",
    customer_phone: "555-0002",
    delivery_address: { street: "2 Oak Ave", city: "Austin", country: "US" },
    delivery_fee: 5.0,
    contactless_delivery: false,
    leave_at_door: false,
    // Terminal orders are date-filtered to today (the console caps its load at
    // 200 with no `since`, so stale terminal orders would otherwise pile up).
    // Use "now" so this delivered order stays visible for the filter tests.
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
  },
];

const mockDrivers = [
  { id: 10, business_id: 1, name: "Carlos", phone: "555-9999", status: "available", is_available: true, is_active: true },
];

const mockDeliveryApi = deliveryApiModule.deliveryApi as jest.Mocked<typeof deliveryApiModule.deliveryApi>;

/** Wrap an orders array into the DeliveryListResult shape returned by the API */
function deliveryList(
  orders: (typeof mockOrders)[number][],
  extras?: { total?: number; has_more?: boolean },
): any {
  return {
    deliveries: orders,
    total: extras?.total ?? orders.length,
    has_more: extras?.has_more ?? false,
  };
}

/**
 * Board load fires two list queries (active statuses + terminal-today). Route
 * mock responses by the presence of `since` so tests can seed each window.
 */
function mockBoardLoads(
  activeOrders: (typeof mockOrders)[number][],
  terminalOrders: (typeof mockOrders)[number][] = [],
  extras?: {
    activeTotal?: number;
    activeHasMore?: boolean;
    terminalTotal?: number;
    terminalHasMore?: boolean;
  },
) {
  mockDeliveryApi.getBusinessDeliveries.mockImplementation(
    async (_id: number, options?: { since?: string }) => {
      if (options?.since) {
        return deliveryList(terminalOrders, {
          total: extras?.terminalTotal,
          has_more: extras?.terminalHasMore,
        });
      }
      return deliveryList(activeOrders, {
        total: extras?.activeTotal,
        has_more: extras?.activeHasMore,
      });
    },
  );
}

beforeEach(() => {
  jest.clearAllMocks();
  mockBoardLoads([]);
  mockDeliveryApi.getAvailableDrivers.mockResolvedValue([]);
  window.matchMedia = ((query: string) => ({
    matches: false,
    media: query,
    addEventListener: jest.fn(),
    removeEventListener: jest.fn(),
    addListener: jest.fn(),
    removeListener: jest.fn(),
    dispatchEvent: jest.fn(),
    onchange: null,
  })) as typeof window.matchMedia;
});

// ─── Auto-refresh tests ───────────────────────────────────────────────────────

describe("DispatchConsole auto-refresh", () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });
  afterEach(() => {
    jest.useRealTimers();
  });

  it("polls getBusinessDeliveries every 30 s when active orders exist", async () => {
    mockBoardLoads([mockOrders[0]]);
    mockDeliveryApi.getAvailableDrivers.mockResolvedValue([]);

    const { unmount } = render(<DispatchConsole businessId={1} />);

    // Flush the initial load: advance 0ms to let effects run, drain microtasks.
    await act(async () => {
      jest.advanceTimersByTime(0);
      await Promise.resolve();
      await Promise.resolve();
    });

    // Board load = 2 list queries (active + terminal).
    expect(mockDeliveryApi.getBusinessDeliveries).toHaveBeenCalledTimes(2);

    // The initial load resolves with an active order, so activeCount becomes 1
    // and the interval effect sets up a 30 s interval.
    // Advance exactly 30 s to fire the interval callback, then drain microtasks.
    await act(async () => {
      jest.advanceTimersByTime(30_000);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(mockDeliveryApi.getBusinessDeliveries).toHaveBeenCalledTimes(4);

    unmount();
  });

  it("continues idle polling when no active orders remain", async () => {
    // Only a delivered order — no active
    mockBoardLoads([], [mockOrders[1]]);
    mockDeliveryApi.getAvailableDrivers.mockResolvedValue([]);

    render(<DispatchConsole businessId={1} />);

    await waitFor(() =>
      expect(mockDeliveryApi.getBusinessDeliveries).toHaveBeenCalledTimes(2)
    );

    // 30 s is reserved for active work; idle queues poll less aggressively.
    await act(async () => {
      jest.advanceTimersByTime(30_000);
    });
    await act(async () => {
      await Promise.resolve();
    });

    expect(mockDeliveryApi.getBusinessDeliveries).toHaveBeenCalledTimes(2);

    await act(async () => {
      jest.advanceTimersByTime(30_000);
    });
    await act(async () => {
      await Promise.resolve();
    });

    expect(mockDeliveryApi.getBusinessDeliveries).toHaveBeenCalledTimes(4);
  });

  it("clears interval on unmount (no extra fetches after unmount)", async () => {
    mockBoardLoads([mockOrders[0]]);
    mockDeliveryApi.getAvailableDrivers.mockResolvedValue([]);

    const { unmount } = render(<DispatchConsole businessId={1} />);

    await waitFor(() =>
      expect(mockDeliveryApi.getBusinessDeliveries).toHaveBeenCalledTimes(2)
    );

    unmount();

    await act(async () => {
      jest.advanceTimersByTime(60_000);
    });
    await act(async () => {
      await Promise.resolve();
    });

    // Still only the initial board load — interval was cleared on unmount
    expect(mockDeliveryApi.getBusinessDeliveries).toHaveBeenCalledTimes(2);
  });
});

// ─── Functional tests ─────────────────────────────────────────────────────────

describe("DispatchConsole", () => {
  it("refreshes when a delivery SSE event arrives", async () => {
    mockBoardLoads([]);
    mockDeliveryApi.getAvailableDrivers.mockResolvedValue([]);

    render(<DispatchConsole businessId={1} />);

    await waitFor(() =>
      expect(mockDeliveryApi.getBusinessDeliveries).toHaveBeenCalledTimes(2)
    );

    const lastCall = mockUseSSEEvents.mock.calls[mockUseSSEEvents.mock.calls.length - 1];
    const options = lastCall[0] as {
      onEvent: (event: { type: string; data: Record<string, unknown> }) => void;
    };

    await act(async () => {
      options.onEvent({ type: "delivery.created", data: {} });
      await Promise.resolve();
    });

    await waitFor(() =>
      expect(mockDeliveryApi.getBusinessDeliveries).toHaveBeenCalledTimes(4)
    );
  });

  // DEL-OP-5: an SSE burst (each status advance / assign / payment emits an
  // event) must not trigger one full board reload per event. The first event
  // reloads immediately (fresh feel); the rest of the burst coalesces into a
  // single trailing reload. Each reload is 2 list queries.
  it("coalesces a burst of SSE delivery events into one leading + one trailing reload (DEL-OP-5)", async () => {
    jest.useFakeTimers();
    try {
      mockBoardLoads([]);
      mockDeliveryApi.getAvailableDrivers.mockResolvedValue([]);

      render(<DispatchConsole businessId={1} />);
      await act(async () => {
        jest.advanceTimersByTime(0);
        await Promise.resolve();
        await Promise.resolve();
      });
      expect(mockDeliveryApi.getBusinessDeliveries).toHaveBeenCalledTimes(2);

      const lastCall = mockUseSSEEvents.mock.calls[mockUseSSEEvents.mock.calls.length - 1];
      const options = lastCall[0] as {
        onEvent: (event: { type: string; data: Record<string, unknown> }) => void;
      };

      // Rapid burst of 5 events → exactly one immediate (leading) reload.
      await act(async () => {
        for (let i = 0; i < 5; i++) {
          options.onEvent({ type: "delivery.updated", data: {} });
        }
        await Promise.resolve();
        await Promise.resolve();
      });
      expect(mockDeliveryApi.getBusinessDeliveries).toHaveBeenCalledTimes(4);

      // The suppressed events collapse into a single trailing reload.
      await act(async () => {
        jest.advanceTimersByTime(2_000);
        await Promise.resolve();
        await Promise.resolve();
      });
      expect(mockDeliveryApi.getBusinessDeliveries).toHaveBeenCalledTimes(6);

      // Nothing else pending from the burst.
      await act(async () => {
        jest.advanceTimersByTime(2_000);
        await Promise.resolve();
      });
      expect(mockDeliveryApi.getBusinessDeliveries).toHaveBeenCalledTimes(6);
    } finally {
      jest.useRealTimers();
    }
  });

  it("loads the board with status + since filters (no silent 200-row dump)", async () => {
    mockBoardLoads([mockOrders[0]], [mockOrders[1]]);
    render(<DispatchConsole businessId={1} />);

    await waitFor(() =>
      expect(mockDeliveryApi.getBusinessDeliveries).toHaveBeenCalledTimes(2)
    );

    const calls = mockDeliveryApi.getBusinessDeliveries.mock.calls;
    const activeCall = calls.find((c) => !(c[1] as { since?: string })?.since);
    const terminalCall = calls.find((c) => (c[1] as { since?: string })?.since);

    expect(activeCall?.[1]).toEqual(
      expect.objectContaining({
        status: expect.arrayContaining(["preparing", "ready", "in_transit"]),
        limit: expect.any(Number),
      }),
    );
    expect((activeCall?.[1] as { status: string[] }).status).not.toContain("delivered");
    expect(terminalCall?.[1]).toEqual(
      expect.objectContaining({
        status: expect.arrayContaining(["delivered", "cancelled", "failed"]),
        since: expect.any(String),
        limit: expect.any(Number),
      }),
    );
    // Must never request the old silent {limit: 200} unfiltered window.
    for (const call of calls) {
      const opts = call[1] as { limit?: number; status?: unknown };
      expect(opts?.status).toBeDefined();
      expect(opts?.limit).not.toBe(200);
    }
  });

  it("shows a truncation banner and load-more when the server reports has_more", async () => {
    mockBoardLoads([mockOrders[0]], [], {
      activeTotal: 250,
      activeHasMore: true,
      terminalTotal: 40,
    });

    render(<DispatchConsole businessId={1} />);

    await waitFor(() => {
      expect(screen.getByTestId("dispatch-truncation-banner")).toBeInTheDocument();
    });
    expect(screen.getByTestId("dispatch-load-more")).toBeInTheDocument();
    // Banner denominator is the SERVER total for both windows (250 active +
    // 40 terminal-today), not just the loaded rows.
    expect(
      screen.getByTestId("dispatch-truncation-banner").textContent,
    ).toContain("290");
  });

  it("renders empty state when no orders", async () => {
    render(<DispatchConsole businessId={1} />);
    await waitFor(() => {
      expect(
        screen.getByText("deliverySettings.dispatch.emptyBoard.title"),
      ).toBeInTheDocument();
    });
    expect(
      screen.getByText("deliverySettings.dispatch.emptyBoard.subtitle"),
    ).toBeInTheDocument();
  });

  it("keeps Out for delivery and Delivered lanes at 1024 with zero orders (issue 238)", async () => {
    window.matchMedia = ((query: string) => ({
      matches: query.includes("1024"),
      media: query,
      addEventListener: jest.fn(),
      removeEventListener: jest.fn(),
      addListener: jest.fn(),
      removeListener: jest.fn(),
      dispatchEvent: jest.fn(),
      onchange: null,
    })) as typeof window.matchMedia;
    mockBoardLoads([]);
    render(<DispatchConsole businessId={1} />);
    await waitFor(() => {
      expect(screen.getByTestId("dispatch-board-scroll")).toBeInTheDocument();
    });
    expect(screen.getByTestId("dispatch-column-out_for_delivery")).toBeInTheDocument();
    expect(screen.getByTestId("dispatch-column-delivered")).toBeInTheDocument();
    expect(screen.queryByText("deliverySettings.dispatch.emptyBoard.subtitle")).not.toBeInTheDocument();
  });

  it("renders order list when orders are returned", async () => {
    mockBoardLoads([mockOrders[0]], [mockOrders[1]]);
    mockDeliveryApi.getAvailableDrivers.mockResolvedValue(mockDrivers as any);

    render(<DispatchConsole businessId={1} />);

    await waitFor(() => {
      expect(screen.getByText("#DEL-001")).toBeInTheDocument();
    });
    expect(screen.getByText(/Alice Smith/)).toBeInTheDocument();
    expect(screen.getByText("#DEL-002")).toBeInTheDocument();
    expect(screen.getByText(/Bob Jones/)).toBeInTheDocument();
  });

  it("filter chip changes the visible orders", async () => {
    mockBoardLoads([mockOrders[0]], [mockOrders[1]]);
    mockDeliveryApi.getAvailableDrivers.mockResolvedValue([]);

    render(<DispatchConsole businessId={1} />);

    await waitFor(() => {
      expect(screen.getByText("#DEL-001")).toBeInTheDocument();
    });

    // Both orders visible under "all"
    expect(screen.getByText("#DEL-001")).toBeInTheDocument();
    expect(screen.getByText("#DEL-002")).toBeInTheDocument();

    // Click "delivered" filter (the StatusFilter tab, not the order status chip
    // which now also renders a humanized label). The tab's aria-label routes
    // through tString("dispatch.filters.filterByAria", {status}); with the
    // key-echo mock it collapses to the raw key for every tab, so select by the
    // tab whose visible chip text is the delivered label.
    const deliveredFilter = screen
      .getAllByRole("tab")
      .find((el) =>
        el.textContent?.includes("deliverySettings.dispatch.filters.delivered")
      );
    expect(deliveredFilter).toBeDefined();
    fireEvent.click(deliveredFilter!);

    // Only delivered order visible
    expect(screen.queryByText("#DEL-001")).not.toBeInTheDocument();
    expect(screen.getByText("#DEL-002")).toBeInTheDocument();
  });

  it("Advance status button calls API for fulfillment statuses", async () => {
    // Intake (pending/confirmed) has no dispatch advance — use preparing so the
    // operator next status is ready (Accept lives in the Bills queue).
    const preparingOrder = {
      ...mockOrders[0],
      id: 3,
      delivery_number: "DEL-003",
      status: "preparing" as const,
      customer_name: "Carol Ready",
    };
    mockBoardLoads([preparingOrder]);
    mockDeliveryApi.getAvailableDrivers.mockResolvedValue([]);
    mockDeliveryApi.updateDeliveryOrderStatus.mockResolvedValue(undefined as any);

    render(<DispatchConsole businessId={1} />);

    await waitFor(() => {
      expect(screen.getByText("#DEL-003")).toBeInTheDocument();
    });

    const advanceBtn = screen.getByText(/actions\.advance/);
    await act(async () => {
      fireEvent.click(advanceBtn);
    });

    expect(mockDeliveryApi.updateDeliveryOrderStatus).toHaveBeenCalledWith(
      1,
      3,
      "ready"
    );
  });

  it("does not offer advance on pending intake orders", async () => {
    mockBoardLoads([mockOrders[0]]);
    mockDeliveryApi.getAvailableDrivers.mockResolvedValue([]);

    render(<DispatchConsole businessId={1} />);

    await waitFor(() => {
      expect(screen.getByText("#DEL-001")).toBeInTheDocument();
    });

    expect(screen.queryByText(/actions\.advance/)).not.toBeInTheDocument();
  });

  it("Cancel opens modal, submits with reason, calls API", async () => {
    mockBoardLoads([mockOrders[0]]);
    mockDeliveryApi.getAvailableDrivers.mockResolvedValue([]);
    mockDeliveryApi.cancelDeliveryOrder.mockResolvedValue(undefined as any);

    render(<DispatchConsole businessId={1} />);

    await waitFor(() => {
      expect(screen.getByText("#DEL-001")).toBeInTheDocument();
    });

    const cancelBtn = screen.getByText(/actions\.cancel/);
    fireEvent.click(cancelBtn);

    await waitFor(() => {
      expect(screen.getByText(/cancel\.title/)).toBeInTheDocument();
    });

    const textarea = screen.getByPlaceholderText(/cancel\.reasonPlaceholder/);
    fireEvent.change(textarea, { target: { value: "Customer not available" } });

    const confirmBtn = screen.getByText(/cancel\.confirm/);
    await act(async () => {
      fireEvent.click(confirmBtn);
    });

    expect(mockDeliveryApi.cancelDeliveryOrder).toHaveBeenCalledWith(
      1,
      1,
      "Customer not available"
    );
  });
});

// ─── Header active-order count ────────────────────────────────────────────────
// Intake (pending/confirmed) is on the board's Awaiting payment lane so the
// header must count it — otherwise Bills shows a live unpaid delivery while
// Dispatch reads "0 active orders". getTranslation is mocked to echo the key,
// so the singular key (…activeOrders) means count === 1 and the plural key
// (…activeOrdersPlural) means count !== 1.
describe("DispatchConsole board header count", () => {
  it("counts a pending (intake) order as a board-active order", async () => {
    mockBoardLoads([mockOrders[0]]);

    render(<DispatchConsole businessId={1} />);

    await waitFor(() =>
      expect(mockDeliveryApi.getBusinessDeliveries).toHaveBeenCalledTimes(2)
    );

    await waitFor(() =>
      expect(
        screen.getByText("deliverySettings.dispatch.header.activeOrders"),
      ).toBeInTheDocument(),
    );
  });

  it("counts a preparing order as a board-active order", async () => {
    const preparing = { ...mockOrders[0], status: "preparing" };
    mockBoardLoads([preparing as typeof mockOrders[0]]);

    render(<DispatchConsole businessId={1} />);

    await waitFor(() =>
      expect(mockDeliveryApi.getBusinessDeliveries).toHaveBeenCalledTimes(2)
    );

    await waitFor(() =>
      expect(
        screen.getByText("deliverySettings.dispatch.header.activeOrders"),
      ).toBeInTheDocument(),
    );
  });
});

// ─── Business-timezone "today" cutoff (DEL-OP-8) ──────────────────────────────
// The Delivered/Cancelled/Failed columns drop orders from previous days. That
// day boundary must follow the BUSINESS's calendar day, not the device's —
// a traveling owner or overseas VA must see the restaurant's operating day.
describe("isSameBusinessDay", () => {
  const now = new Date("2026-07-06T01:00:00Z");

  it("treats a late-evening delivery as today in the business timezone even across the UTC midnight", () => {
    // 23:30 UTC Jul 5 = 20:30 Jul 5 in Buenos Aires; now (01:00 UTC Jul 6) is
    // 22:00 Jul 5 in Buenos Aires → same business day.
    const iso = "2026-07-05T23:30:00Z";
    expect(isSameBusinessDay(iso, "America/Argentina/Buenos_Aires", now)).toBe(true);
    // The same pair straddles midnight in UTC → different UTC days.
    expect(isSameBusinessDay(iso, "UTC", now)).toBe(false);
  });

  it("drops a delivery that is yesterday in the business timezone", () => {
    // 12:00 UTC Jul 5 = 09:00 Jul 5 in Buenos Aires; now = 22:00 Jul 5 there → same day.
    expect(isSameBusinessDay("2026-07-05T12:00:00Z", "America/Argentina/Buenos_Aires", now)).toBe(true);
    // 12:00 UTC Jul 4 is Jul 4 everywhere relevant → not today.
    expect(isSameBusinessDay("2026-07-04T12:00:00Z", "America/Argentina/Buenos_Aires", now)).toBe(false);
  });

  it("falls back to the device day for a missing or invalid timezone", () => {
    const sameInstant = now.toISOString();
    expect(isSameBusinessDay(sameInstant, undefined, now)).toBe(true);
    expect(isSameBusinessDay(sameInstant, "Not/AZone", now)).toBe(true);
  });

  it("returns false for missing or unparseable timestamps", () => {
    expect(isSameBusinessDay(undefined, "UTC", now)).toBe(false);
    expect(isSameBusinessDay("garbage", "UTC", now)).toBe(false);
  });
});

describe("DispatchConsole abort on in-app navigation (#715)", () => {
  it("does not show the load-error board when the first fetch is aborted", async () => {
    const aborted = Object.assign(new Error("canceled"), {
      name: "AbortError",
      code: "ERR_CANCELED",
    });
    mockDeliveryApi.getBusinessDeliveries.mockRejectedValue(aborted);
    mockDeliveryApi.getAvailableDrivers.mockRejectedValue(aborted);

    render(<DispatchConsole businessId={1} />);
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(screen.queryByTestId("dispatch-load-error")).not.toBeInTheDocument();
  });

  it("still surfaces a real network failure", async () => {
    mockDeliveryApi.getBusinessDeliveries.mockRejectedValue(new Error("network"));
    mockDeliveryApi.getAvailableDrivers.mockRejectedValue(new Error("network"));

    render(<DispatchConsole businessId={1} />);
    expect(await screen.findByTestId("dispatch-load-error")).toBeInTheDocument();
    expect(
      screen.getByText("deliverySettings.dispatch.loadError"),
    ).toBeInTheDocument();
  });
});
