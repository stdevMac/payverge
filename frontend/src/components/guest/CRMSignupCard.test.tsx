/** @jest-environment jsdom */
import React from "react";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";

import CRMSignupCard from "@/components/guest/CRMSignupCard";
import { crmAPI } from "@/api/crm";
import { checkInCustomerToTable } from "@/api/customerTable";

const mockCustomerAuth = {
  customer: null as { id: number } | null,
  loading: false,
};

jest.mock("@/contexts/CustomerAuthContext", () => ({
  useCustomerAuth: () => mockCustomerAuth,
}));

jest.mock("@/api/crm", () => ({
  crmAPI: {
    getProfile: jest.fn(),
  },
}));

jest.mock("@/api/customerTable", () => ({
  checkInCustomerToTable: jest.fn(),
}));

jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({
    t: (key: string) =>
      ({
        "crm.signupPrompt": "Sign up for rewards",
        "crm.signupDescription": "Create an account to earn points",
        "crm.alreadyMember": "Already a member?",
        "crm.signIn": "Sign in",
        "crm.earnPoints": "Earn Points",
        "crm.getRewards": "Get Rewards",
        "crm.joinFailed":
          "We could not connect your rewards account to this table.",
      })[key] ?? key,
  }),
}));

type ModalProps = {
  isOpen: boolean;
  businessName?: string;
  defaultMode?: "login" | "register";
  onSuccess?: (customerId: number, customerData: unknown) => void | Promise<void>;
};

let mockModalProps: ModalProps | null = null;

jest.mock("../customer/CustomerAuthModal", () => ({
  __esModule: true,
  default: (props: ModalProps) => {
    mockModalProps = props;
    return props.isOpen ? <div data-testid="customer-auth-modal" /> : null;
  },
}));

jest.mock("@nextui-org/react", () => ({
  Card: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  CardBody: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  Button: ({
    children,
    onPress,
    startContent,
  }: {
    children: React.ReactNode;
    onPress?: () => void;
    startContent?: React.ReactNode;
  }) => (
    <button type="button" onClick={onPress}>
      {startContent}
      {children}
    </button>
  ),
}));

const mockedCRM = crmAPI as jest.Mocked<typeof crmAPI>;
const mockedCheckIn = checkInCustomerToTable as jest.MockedFunction<
  typeof checkInCustomerToTable
>;

describe("CRMSignupCard", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockModalProps = null;
    mockCustomerAuth.customer = null;
    mockCustomerAuth.loading = false;
    mockedCRM.getProfile.mockRejectedValue(new Error("not logged in"));
    mockedCheckIn.mockResolvedValue({
      customer_business: { id: 10 },
      bill_attached: true,
      bill_id: 20,
    });
  });

  it("checks in to the table before hiding after customer auth", async () => {
    render(
      <CRMSignupCard
        businessName="Cafe"
        tableCode="TABLE123"
        crmEnabled
      />,
    );

    fireEvent.click(
      screen.getAllByRole("button", { name: /sign up for rewards/i })[0],
    );

    expect(mockModalProps).toEqual(
      expect.objectContaining({
        isOpen: true,
        businessName: "Cafe",
        defaultMode: "register",
      }),
    );

    await act(async () => {
      await mockModalProps?.onSuccess?.(7, { id: 7 });
    });

    expect(mockedCheckIn).toHaveBeenCalledWith("TABLE123");
    await waitFor(() => {
      expect(screen.queryByText("Create an account to earn points")).toBeNull();
    });
  });

  it("keeps the join card visible and shows an error when table check-in fails", async () => {
    mockedCheckIn.mockRejectedValueOnce(new Error("check-in failed"));

    render(
      <CRMSignupCard
        businessName="Cafe"
        tableCode="TABLE123"
        crmEnabled
      />,
    );

    fireEvent.click(
      screen.getAllByRole("button", { name: /sign up for rewards/i })[0],
    );

    await act(async () => {
      await mockModalProps?.onSuccess?.(7, { id: 7 });
    });

    expect(mockedCheckIn).toHaveBeenCalledWith("TABLE123");
    expect(
      screen.getAllByText(
        "We could not connect your rewards account to this table.",
      )[0],
    ).toBeInTheDocument();
    expect(
      screen.getAllByRole("button", { name: /sign up for rewards/i }).length,
    ).toBeGreaterThan(0);
  });

  it("requires table check-in before hiding an already authenticated customer", async () => {
    mockCustomerAuth.customer = { id: 9 };

    render(
      <CRMSignupCard
        businessName="Cafe"
        tableCode="TABLE456"
        crmEnabled
      />,
    );

    await waitFor(() => {
      expect(mockedCheckIn).toHaveBeenCalledWith("TABLE456");
    });
    await waitFor(() => {
      expect(screen.queryByText("Create an account to earn points")).toBeNull();
    });
    expect(mockedCRM.getProfile).not.toHaveBeenCalled();
  });

  it("renders full variant by default with earn/get rewards chrome", () => {
    render(
      <CRMSignupCard businessName="Cafe" tableCode="TABLE123" crmEnabled />,
    );

    expect(screen.getByTestId("crm-signup-full")).toBeInTheDocument();
    expect(screen.queryByTestId("crm-signup-compact")).not.toBeInTheDocument();
    expect(screen.getByText("Earn Points")).toBeInTheDocument();
    expect(screen.getByText("Get Rewards")).toBeInTheDocument();
    expect(screen.getByText("Create an account to earn points")).toBeInTheDocument();
  });

  it("renders compact variant without earn/get grid or large description block", () => {
    render(
      <CRMSignupCard
        businessName="Cafe"
        tableCode="TABLE123"
        crmEnabled
        variant="compact"
      />,
    );

    expect(screen.getByTestId("crm-signup-compact")).toBeInTheDocument();
    expect(screen.queryByTestId("crm-signup-full")).not.toBeInTheDocument();
    expect(screen.queryByText("Earn Points")).not.toBeInTheDocument();
    expect(screen.queryByText("Get Rewards")).not.toBeInTheDocument();
    expect(
      screen.queryByText("Create an account to earn points"),
    ).not.toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /sign up for rewards/i }),
    ).toBeInTheDocument();
  });

  it("opens auth modal from compact signup control", () => {
    render(
      <CRMSignupCard
        businessName="Cafe"
        tableCode="TABLE123"
        crmEnabled
        variant="compact"
      />,
    );

    fireEvent.click(
      screen.getByRole("button", { name: /sign up for rewards/i }),
    );
    expect(mockModalProps).toEqual(
      expect.objectContaining({
        isOpen: true,
        defaultMode: "register",
      }),
    );
  });
});
