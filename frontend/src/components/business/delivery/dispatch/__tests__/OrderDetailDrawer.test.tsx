/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor, fireEvent, act } from "@testing-library/react";
import OrderDetailDrawer from "../OrderDetailDrawer";
import * as deliveryApiModule from "@/api/delivery";

jest.mock("@/api/delivery", () => ({
  deliveryApi: {
    getDeliveryOrder: jest.fn(),
  },
}));

jest.mock("framer-motion", () => ({
  useReducedMotion: () => true,
}));

jest.mock("@nextui-org/react", () => ({
  Button: ({ children, onPress, isLoading, isDisabled, ...rest }: any) => (
    <button
      type="button"
      onClick={onPress}
      disabled={isDisabled || isLoading}
      {...rest}
    >
      {children}
    </button>
  ),
  Chip: ({ children }: any) => <span>{children}</span>,
  Spinner: () => <div data-testid="spinner" />,
  Drawer: ({ isOpen, children }: any) =>
    isOpen ? <div data-testid="drawer-root">{children}</div> : null,
  DrawerContent: ({ children, ...rest }: any) => (
    <div {...rest}>
      {typeof children === "function" ? children(() => {}) : children}
    </div>
  ),
  DrawerBody: ({ children }: any) => <div>{children}</div>,
}));

jest.mock("../DriverAssignment", () => ({
  DriverAssignment: () => <div data-testid="driver-assignment" />,
}));

const mockApi = deliveryApiModule.deliveryApi as jest.Mocked<
  typeof deliveryApiModule.deliveryApi
>;

const detailOrder = {
  id: 42,
  business_id: 1,
  bill_id: 99,
  delivery_number: "DEL-042",
  delivery_type: "in_house",
  status: "preparing" as const,
  customer_name: "Dana Guest",
  customer_phone: "555-4242",
  delivery_address: {
    street: "42 Maple St",
    apartment: "3B",
    city: "Austin",
    country: "US",
  },
  delivery_fee: 4.5,
  driver_tip: 2,
  payment_mode_stored: "cash_on_delivery" as const,
  delivery_instructions: "Ring twice",
  contactless_delivery: true,
  leave_at_door: false,
  created_at: "2026-07-06T12:00:00Z",
  updated_at: "2026-07-06T12:30:00Z",
  status_history: [
    {
      id: 1,
      delivery_order_id: 42,
      status: "pending" as const,
      notes: "Order created",
      changed_by: "system",
      created_at: "2026-07-06T12:00:00Z",
    },
    {
      id: 2,
      delivery_order_id: 42,
      status: "preparing" as const,
      notes: "Status changed",
      changed_by: "operator",
      created_at: "2026-07-06T12:10:00Z",
    },
  ],
};

const tString = (key: string, vars?: Record<string, string>) => {
  if (!vars) return key;
  return Object.entries(vars).reduce(
    (acc, [k, v]) => acc.replace(`{${k}}`, v),
    key,
  );
};

describe("OrderDetailDrawer", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockApi.getDeliveryOrder.mockResolvedValue(detailOrder as any);
  });

  it("loads getDeliveryOrder and renders customer, address, tip, bill link, timeline", async () => {
    render(
      <OrderDetailDrawer
        businessId={1}
        orderId={42}
        isOpen
        onClose={jest.fn()}
        drivers={[]}
        onAdvance={jest.fn()}
        onAssignDriver={jest.fn()}
        onCancel={jest.fn()}
        tString={tString}
        currency="USD"
      />,
    );

    await waitFor(() =>
      expect(mockApi.getDeliveryOrder).toHaveBeenCalledWith(1, 42),
    );

    expect(await screen.findByText("Dana Guest")).toBeInTheDocument();
    expect(screen.getByTestId("delivery-order-detail-phone")).toHaveAttribute(
      "href",
      "tel:555-4242",
    );
    expect(screen.getByText(/42 Maple St/)).toBeInTheDocument();
    // Apartment appears in the formatted address and the dedicated Apt line.
    expect(screen.getAllByText(/3B/).length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText("Ring twice")).toBeInTheDocument();
    expect(screen.getByText("dispatch.detail.contactless")).toBeInTheDocument();
    // Operator dashboard deep-link shape (see Kitchen.tsx) — /business/:id
    // WITHOUT /dashboard lands on the guest-facing page.
    expect(screen.getByTestId("delivery-order-detail-bill-link")).toHaveAttribute(
      "href",
      "/business/1/dashboard?tab=bills&billId=99",
    );
    expect(
      screen.getByTestId("delivery-order-detail-timeline"),
    ).toBeInTheDocument();
    expect(screen.getByTestId("delivery-order-detail-advance")).toBeInTheDocument();
    expect(screen.getByTestId("delivery-order-detail-cancel")).toBeInTheDocument();
  });

  it("does not fetch when closed", () => {
    render(
      <OrderDetailDrawer
        businessId={1}
        orderId={42}
        isOpen={false}
        onClose={jest.fn()}
        drivers={[]}
        onAdvance={jest.fn()}
        onAssignDriver={jest.fn()}
        onCancel={jest.fn()}
        tString={tString}
      />,
    );
    expect(mockApi.getDeliveryOrder).not.toHaveBeenCalled();
  });

  it("invokes onAdvance from the drawer action", async () => {
    const onAdvance = jest.fn().mockResolvedValue(undefined);
    mockApi.getDeliveryOrder
      .mockResolvedValueOnce(detailOrder as any)
      .mockResolvedValueOnce({
        ...detailOrder,
        status: "ready",
      } as any);

    render(
      <OrderDetailDrawer
        businessId={1}
        orderId={42}
        isOpen
        onClose={jest.fn()}
        drivers={[]}
        onAdvance={onAdvance}
        onAssignDriver={jest.fn()}
        onCancel={jest.fn()}
        tString={tString}
      />,
    );

    await screen.findByText("Dana Guest");
    await act(async () => {
      fireEvent.click(screen.getByTestId("delivery-order-detail-advance"));
    });
    expect(onAdvance).toHaveBeenCalledWith(42, "ready");
  });
});
