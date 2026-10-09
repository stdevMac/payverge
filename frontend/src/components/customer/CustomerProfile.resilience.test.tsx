/** @jest-environment jsdom */
import React from "react";
import { act, fireEvent, render, screen } from "@testing-library/react";
import CustomerProfile from "./CustomerProfile";
import { crmAPI } from "@/api/crm";

const mockRefreshCustomer = jest.fn();
let mockCustomer: object | null = { id: 1, name: "Ada", email: "ada@example.com" };

jest.mock("@/contexts/CustomerAuthContext", () => ({
  useCustomerAuth: () => ({
    customer: mockCustomer,
    loading: false,
    refreshCustomer: mockRefreshCustomer,
  }),
}));

jest.mock("@/api/crm", () => ({
  crmAPI: { getBusinesses: jest.fn() },
}));

jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({
    t: (key: string) => key,
    currentLanguage: "en",
  }),
}));

jest.mock("./CustomerAuthModal", () => ({
  __esModule: true,
  default: ({ isOpen }: { isOpen: boolean }) =>
    isOpen ? <div data-testid="auth-modal" /> : null,
}));

// Lightweight NextUI stand-ins so onPress is clickable in jsdom.
jest.mock("@nextui-org/react", () => {
  const Passthrough = ({ children }: { children?: React.ReactNode }) => (
    <div>{children}</div>
  );
  return {
    Card: Passthrough,
    CardBody: Passthrough,
    CardHeader: Passthrough,
    Chip: Passthrough,
    Switch: () => <input type="checkbox" readOnly />,
    Avatar: () => <div />,
    Modal: ({
      children,
      isOpen,
    }: {
      children: React.ReactNode;
      isOpen: boolean;
    }) => (isOpen ? <div>{children}</div> : null),
    ModalContent: Passthrough,
    ModalHeader: Passthrough,
    ModalBody: Passthrough,
    ModalFooter: Passthrough,
    Spinner: () => <div />,
    Input: () => <input />,
    Button: ({
      children,
      onPress,
    }: {
      children: React.ReactNode;
      onPress?: () => void;
    }) => (
      <button type="button" onClick={onPress}>
        {children}
      </button>
    ),
  };
});

describe("CustomerProfile resilience", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockCustomer = { id: 1, name: "Ada", email: "ada@example.com" };
  });

  it("shows an inline error with retry when the loyalty/businesses load fails", async () => {
    (crmAPI.getBusinesses as jest.Mock)
      .mockRejectedValueOnce(new Error("network"))
      .mockResolvedValueOnce([]);

    render(<CustomerProfile />);

    expect(
      await screen.findByText("customerProfile.loadError"),
    ).toBeInTheDocument();

    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "customerProfile.retry" }));
    });

    expect(screen.queryByText("customerProfile.loadError")).not.toBeInTheDocument();
  });

  it("offers a sign-in CTA when signed out instead of a dead not-found", async () => {
    mockCustomer = null;
    render(<CustomerProfile />);

    const signIn = await screen.findByRole("button", {
      name: "customerProfile.signIn",
    });
    fireEvent.click(signIn);
    expect(screen.getByTestId("auth-modal")).toBeInTheDocument();
  });
});
