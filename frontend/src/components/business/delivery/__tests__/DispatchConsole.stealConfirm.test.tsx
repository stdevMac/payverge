/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import DispatchConsole from "../DispatchConsole";
import * as deliveryApiModule from "@/api/delivery";
import type { DeliveryOrder } from "@/api/delivery";
import type { Dollars } from "@/types/money";

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

jest.mock("@nextui-org/react", () => ({
  Button: ({
    children,
    onPress,
    isLoading,
    isDisabled,
    "aria-label": ariaLabel,
    startContent: _startContent,
    ...rest
  }: {
    children?: React.ReactNode;
    onPress?: () => void;
    isLoading?: boolean;
    isDisabled?: boolean;
    "aria-label"?: string;
    startContent?: React.ReactNode;
  }) => (
    <button
      type="button"
      onClick={onPress}
      disabled={isDisabled || isLoading}
      aria-label={ariaLabel}
      {...rest}
    >
      {children}
    </button>
  ),
  Chip: ({ children }: { children?: React.ReactNode }) => <span>{children}</span>,
  Skeleton: () => <div data-testid="skeleton" />,
  Textarea: ({
    label,
    value,
    onValueChange,
  }: {
    label?: string;
    value?: string;
    onValueChange?: (next: string) => void;
  }) => (
    <textarea
      aria-label={label}
      value={value ?? ""}
      onChange={(event) => onValueChange?.(event.target.value)}
    />
  ),
  Modal: ({ isOpen, children }: { isOpen?: boolean; children?: React.ReactNode }) =>
    isOpen ? <div data-testid="modal">{children}</div> : null,
  ModalContent: ({
    children,
  }: {
    children?: React.ReactNode | ((onClose: () => void) => React.ReactNode);
  }) => <div>{typeof children === "function" ? children(() => {}) : children}</div>,
  ModalHeader: ({ children }: { children?: React.ReactNode }) => <div>{children}</div>,
  ModalBody: ({ children }: { children?: React.ReactNode }) => <div>{children}</div>,
  ModalFooter: ({ children }: { children?: React.ReactNode }) => <div>{children}</div>,
  Spinner: () => <div data-testid="spinner" />,
  Drawer: ({ isOpen, children }: { isOpen?: boolean; children?: React.ReactNode }) =>
    isOpen ? <div data-testid="drawer">{children}</div> : null,
  DrawerContent: ({
    children,
  }: {
    children?: React.ReactNode | ((onClose: () => void) => React.ReactNode);
  }) => <div>{typeof children === "function" ? children(() => {}) : children}</div>,
  DrawerBody: ({ children }: { children?: React.ReactNode }) => <div>{children}</div>,
}));

const mockDeliveryApi =
  deliveryApiModule.deliveryApi as jest.Mocked<typeof deliveryApiModule.deliveryApi>;

function preparingOrder(): DeliveryOrder {
  return {
    id: 11,
    business_id: 1,
    delivery_number: "DEL-011",
    delivery_type: "in_house",
    status: "preparing",
    customer_name: "Ana Diaz",
    customer_phone: "555-0011",
    delivery_address: { street: "1 Main St", city: "Austin", country: "US" },
    delivery_fee: 4 as Dollars,
    contactless_delivery: false,
    leave_at_door: false,
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
  };
}

describe("DispatchConsole steal confirmation", () => {
  beforeEach(() => {
    jest.clearAllMocks();
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
    mockDeliveryApi.getBusinessDeliveries.mockImplementation(
      async (_id: number, options?: { since?: string }) => {
        if (options?.since) {
          return { deliveries: [], total: 0, has_more: false };
        }
        return { deliveries: [preparingOrder()], total: 1, has_more: false };
      },
    );
    mockDeliveryApi.getAvailableDrivers.mockResolvedValue([]);
  });

  it("opens ConfirmationModal and steals after confirm", async () => {
    mockDeliveryApi.claimDeliveryOrder
      .mockRejectedValueOnce({
        response: {
          status: 409,
          data: { code: "claim_steal_required", claimed_by_name: "Carlos" },
        },
      })
      .mockResolvedValueOnce({} as never);

    render(<DispatchConsole businessId={1} />);

    fireEvent.click(await screen.findByTestId("dispatch-claim-btn"));

    expect(await screen.findByText("deliverySettings.dispatch.claim.stealRequired")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "deliverySettings.dispatch.claim.steal" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "deliverySettings.dispatch.claim.cancel" }),
    ).toBeInTheDocument();

    fireEvent.click(
      screen.getByRole("button", { name: "deliverySettings.dispatch.claim.steal" }),
    );

    await waitFor(() => {
      expect(mockDeliveryApi.claimDeliveryOrder).toHaveBeenLastCalledWith(1, 11, {
        steal: true,
      });
    });
  });
});
