/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import DeliveryHistory from "../DeliveryHistory";
import * as deliveryApiModule from "@/api/delivery";

jest.mock("@/api/delivery", () => ({
  deliveryApi: {
    getBusinessDeliveries: jest.fn(),
    getAvailableDrivers: jest.fn(),
    getDeliveryOrder: jest.fn(),
    updateDeliveryOrderStatus: jest.fn(),
    assignDriver: jest.fn(),
    cancelDeliveryOrder: jest.fn(),
  },
}));

jest.mock("../useDeliveryQueries", () => ({
  useDeliveryBusiness: () => ({
    data: { default_currency: "USD", timezone: "UTC" },
    isLoading: false,
  }),
}));

const mockShowSuccess = jest.fn();
const mockShowError = jest.fn();
jest.mock("@/contexts/ToastContext", () => ({
  useToast: () => ({ showSuccess: mockShowSuccess, showError: mockShowError }),
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => key,
}));

jest.mock("framer-motion", () => ({
  useReducedMotion: () => true,
}));

jest.mock("@nextui-org/react", () => ({
  Button: ({ children, onPress, isDisabled, ...rest }: any) => (
    <button type="button" onClick={onPress} disabled={isDisabled} {...rest}>
      {children}
    </button>
  ),
  Chip: ({ children }: any) => <span>{children}</span>,
  Input: ({ label, value, onValueChange, ...rest }: any) => (
    <label>
      {label}
      <input
        value={value ?? ""}
        onChange={(e) => onValueChange?.(e.target.value)}
        {...rest}
      />
    </label>
  ),
  Skeleton: () => <div data-testid="skeleton" />,
  Drawer: ({ isOpen, children }: any) =>
    isOpen ? <div data-testid="drawer">{children}</div> : null,
  DrawerContent: ({ children }: any) => (
    <div>{typeof children === "function" ? children(() => {}) : children}</div>
  ),
  DrawerBody: ({ children }: any) => <div>{children}</div>,
  Spinner: () => <div data-testid="spinner" />,
}));

const mockApi = deliveryApiModule.deliveryApi as jest.Mocked<
  typeof deliveryApiModule.deliveryApi
>;

function terminalOrder() {
  const timestamp = new Date(Date.now() - 36 * 60 * 60 * 1000).toISOString();
  return {
    id: 9,
    business_id: 1,
    delivery_number: "DEL-009",
    delivery_type: "in_house",
    status: "failed",
    customer_name: "Yesterday Fail",
    customer_phone: "555-0009",
    delivery_address: { street: "9 Past St", city: "Austin", country: "US" },
    delivery_fee: 3,
    contactless_delivery: false,
    leave_at_door: false,
    // Within the default History 7-day window so client-side toDate filter keeps it.
    created_at: timestamp,
    updated_at: timestamp,
  };
}

describe("DeliveryHistory", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockApi.getBusinessDeliveries.mockResolvedValue({
      deliveries: [terminalOrder() as any],
      total: 1,
      has_more: false,
    });
    mockApi.getAvailableDrivers.mockResolvedValue([]);
  });

  it("fetches terminal statuses with since + offset pagination params", async () => {
    render(<DeliveryHistory businessId={1} />);

    await waitFor(() =>
      expect(mockApi.getBusinessDeliveries).toHaveBeenCalled(),
    );

    const call = mockApi.getBusinessDeliveries.mock.calls[0];
    expect(call[0]).toBe(1);
    expect(call[1]).toEqual(
      expect.objectContaining({
        status: expect.arrayContaining(["delivered", "cancelled", "failed"]),
        // The backend parses `since` with time.Parse(time.RFC3339, ...) which
        // REQUIRES a timezone offset — a bare local timestamp 400s every load.
        since: expect.stringMatching(/(Z|[+-]\d{2}:\d{2})$/),
        limit: expect.any(Number),
        offset: 0,
      }),
    );
    // Belt-and-suspenders: the value must round-trip through Date parsing.
    const since = (call[1] as { since: string }).since;
    expect(Number.isNaN(new Date(since).getTime())).toBe(false);
  });

  it("renders history rows for terminal orders outside the live board", async () => {
    render(<DeliveryHistory businessId={1} />);

    await waitFor(() =>
      expect(mockApi.getBusinessDeliveries).toHaveBeenCalled(),
    );
    const row = await screen.findByTestId(
      "history-row-9",
      {},
      { timeout: 5_000 },
    );
    expect(row.textContent).toMatch(/Yesterday Fail|DEL-009/);
    // Detail drawer open path is covered by OrderDetailDrawer tests; here we
    // only assert History surfaces the row so yesterday's failed delivery is
    // reachable (the board's same-day filter is bypassed).
    expect(screen.getByTestId("delivery-history")).toBeInTheDocument();
  }, 10_000);
});
