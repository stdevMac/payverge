/** @jest-environment jsdom */
/**
 * DEL-OP-4: DispatchConsole.handleAssignDriver must REJECT on a failed
 * assignment (after showing the error toast) so downstream UI — the inline
 * assign panel on DispatchCard and the DriverAssignment selection — does not
 * close/clear as if the assignment succeeded. Mirrors handleCancel's contract.
 *
 * OrderRow is mocked here to capture the exact onAssignDriver prop the console
 * wires, so this suite lives apart from DispatchConsole.test.tsx (which needs
 * the real OrderRow).
 */
import React from "react";
import { render, waitFor } from "@testing-library/react";
import DispatchConsole from "../DispatchConsole";
import * as deliveryApiModule from "@/api/delivery";

// Stub the transport, keep the module's pure helpers real — the error path
// under test runs the actual `getDeliveryClaimConflict` mapping, so a plain
// non-409 failure has to fall through to the generic toast on its own.
jest.mock("@/api/delivery", () => ({
  ...jest.requireActual("@/api/delivery"),
  deliveryApi: {
    getBusinessDeliveries: jest.fn(),
    getAvailableDrivers: jest.fn(),
    updateDeliveryOrderStatus: jest.fn(),
    assignDriver: jest.fn(),
    cancelDeliveryOrder: jest.fn(),
    claimDeliveryOrder: jest.fn(),
    releaseDeliveryOrder: jest.fn(),
  },
}));

// The claim affordances read the acting principal. `staffData: null` is the
// owner principal — the one allowed to take over another actor's claim.
jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({ staffData: null }),
}));

jest.mock("@/contexts/ToastContext", () => {
  const showSuccess = jest.fn();
  const showError = jest.fn();
  return {
    useToast: () => ({ showSuccess, showError }),
    __toastFns: { showSuccess, showError },
  };
});

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => key,
}));

jest.mock("@/hooks/useSSEEvents", () => ({
  useSSEEvents: () => ({ retriesExhausted: false, reconnect: jest.fn() }),
}));

jest.mock("../useDeliveryQueries", () => ({
  useDeliveryBusiness: () => ({
    data: { default_currency: "USD", timezone: "UTC" },
    isLoading: false,
  }),
}));

jest.mock("@nextui-org/react", () => ({
  Button: ({ children, onPress, isLoading, isDisabled, startContent: _sc, "aria-label": ariaLabel, ...rest }: any) => (
    <button type="button" onClick={onPress} disabled={isDisabled || isLoading} aria-label={ariaLabel} {...rest}>
      {isLoading ? "loading" : children}
    </button>
  ),
  Chip: ({ children }: any) => <span>{children}</span>,
  Skeleton: () => <div data-testid="skeleton" />,
  Textarea: (props: any) => <textarea {...props} />,
  Input: (props: any) => <input {...props} />,
  Spinner: () => <div data-testid="spinner" />,
  Modal: ({ isOpen, children }: any) => (isOpen ? <div>{children}</div> : null),
  ModalContent: ({ children }: any) => <div>{typeof children === "function" ? children(() => {}) : children}</div>,
  ModalHeader: ({ children }: any) => <div>{children}</div>,
  ModalBody: ({ children }: any) => <div>{children}</div>,
  ModalFooter: ({ children }: any) => <div>{children}</div>,
  // OrderDetailDrawer (mounted closed by DispatchConsole) uses the NextUI
  // Drawer family — render nothing while closed, children when open.
  Drawer: ({ isOpen, children }: any) => (isOpen ? <div>{children}</div> : null),
  DrawerContent: ({ children }: any) => (
    <div>{typeof children === "function" ? children(() => {}) : children}</div>
  ),
  DrawerBody: ({ children }: any) => <div>{children}</div>,
}));

type CapturedRowProps = {
  onAssignDriver: (orderId: number, driverId: number) => Promise<void>;
};

const capturedRowProps: CapturedRowProps[] = [];

jest.mock("../dispatch/OrderRow", () => {
  const actual = jest.requireActual("../dispatch/OrderRow");
  return {
    ...actual,
    OrderRow: (props: CapturedRowProps) => {
      capturedRowProps.push(props);
      return <div data-testid="mock-order-row" />;
    },
  };
});

const mockDeliveryApi = deliveryApiModule.deliveryApi as jest.Mocked<
  typeof deliveryApiModule.deliveryApi
>;

const readyOrder = {
  id: 7,
  business_id: 1,
  delivery_number: "DEL-007",
  delivery_type: "in_house",
  status: "ready",
  customer_name: "Cara",
  customer_phone: "555-0007",
  delivery_address: { street: "7 Elm", city: "Austin", country: "US" },
  delivery_fee: 4.5,
  contactless_delivery: false,
  leave_at_door: false,
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
};

beforeEach(() => {
  jest.clearAllMocks();
  capturedRowProps.length = 0;
  mockDeliveryApi.getBusinessDeliveries.mockResolvedValue({
    deliveries: [readyOrder],
    total: 1,
    has_more: false,
  } as any);
  mockDeliveryApi.getAvailableDrivers.mockResolvedValue([]);
});

describe("DispatchConsole.handleAssignDriver failure contract (DEL-OP-4)", () => {
  it("rejects when the assign API fails, after surfacing the error toast", async () => {
    mockDeliveryApi.assignDriver.mockRejectedValue(new Error("boom"));

    render(<DispatchConsole businessId={1} />);
    await waitFor(() => expect(capturedRowProps.length).toBeGreaterThan(0));

    const { onAssignDriver } = capturedRowProps[capturedRowProps.length - 1];
    await expect(onAssignDriver(7, 10)).rejects.toBeDefined();

    const toast = jest.requireMock("@/contexts/ToastContext") as {
      __toastFns: { showError: jest.Mock };
    };
    expect(toast.__toastFns.showError).toHaveBeenCalled();
  });

  it("resolves when the assign API succeeds", async () => {
    mockDeliveryApi.assignDriver.mockResolvedValue(undefined as any);

    render(<DispatchConsole businessId={1} />);
    await waitFor(() => expect(capturedRowProps.length).toBeGreaterThan(0));

    const { onAssignDriver } = capturedRowProps[capturedRowProps.length - 1];
    await expect(onAssignDriver(7, 10)).resolves.toBeUndefined();
  });
});
