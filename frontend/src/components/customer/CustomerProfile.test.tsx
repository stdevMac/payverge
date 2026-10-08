/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import CustomerProfile from "./CustomerProfile";
import { crmAPI } from "@/api/crm";
import toast from "react-hot-toast";

jest.mock("@/api/crm", () => ({
  crmAPI: {
    getBusinesses: jest.fn().mockResolvedValue([]),
    updateProfile: jest.fn(),
    updatePreferences: jest.fn(),
    deleteAccount: jest.fn(),
  },
}));

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { success: jest.fn(), error: jest.fn() },
}));

jest.mock("@/api/currency", () => ({
  formatCurrency: (n: number) => `$${n.toFixed(2)}`,
}));

const mockRefreshCustomer = jest.fn();
// Stable identity so CustomerProfile's [customer] effect doesn't re-fire every
// render (which would loop setFormData -> "Maximum update depth exceeded").
const mockCustomer = {
  id: 1,
  name: "Ada",
  email: "ada@example.com",
  phone: "",
  birthday: "",
  preferences: {},
};
jest.mock("@/contexts/CustomerAuthContext", () => ({
  useCustomerAuth: () => ({
    customer: mockCustomer,
    loading: false,
    refreshCustomer: mockRefreshCustomer,
  }),
}));

// The diner's selected storefront locale, mutable per-test (mock-prefixed for
// jest's factory hoisting) so we can prove money follows it.
let mockProfileLocale = "en";

// Identity-ish t: echo the leaf so we can drive the UI by stable strings.
jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({
    t: (key: string) => key.replace(/^customerProfile\./, ""),
    currentLanguage: mockProfileLocale,
  }),
}));

// Lightweight NextUI stand-ins.
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
    Input: ({
      label,
      value,
      onValueChange,
    }: {
      label: string;
      value: string;
      onValueChange?: (v: string) => void;
    }) => (
      <input
        aria-label={label}
        value={value}
        onChange={(e) => onValueChange?.(e.target.value)}
      />
    ),
    Button: ({
      children,
      onPress,
      isLoading,
    }: {
      children: React.ReactNode;
      onPress?: () => void;
      isLoading?: boolean;
    }) => (
      <button type="button" disabled={isLoading} onClick={onPress}>
        {children}
      </button>
    ),
  };
});

describe("CustomerProfile silent-failure handling", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockProfileLocale = "en";
  });

  // The edit toggle is an icon-only button (no text). It's the 2nd icon button
  // in the header (settings is 1st). Click it to reveal the edit form.
  function enterEditMode() {
    const buttons = screen.getAllByRole("button");
    // settings, edit, then delete card button — edit is index 1.
    fireEvent.click(buttons[1]);
  }

  it("shows an error toast and keeps edit mode open when profile save fails", async () => {
    (crmAPI.updateProfile as jest.Mock).mockRejectedValue(new Error("400"));

    render(<CustomerProfile />);
    await waitFor(() => expect(screen.getByText("profileInfo")).toBeInTheDocument());

    // Enter edit mode: settings (1st) then edit (2nd) are the only icon-only
    // buttons in the header before "saveChanges" appears.
    enterEditMode();

    // Save.
    fireEvent.click(screen.getByText("saveChanges"));

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith("updateError");
    });
    // Edit form is still mounted (name input present) -> not closed silently.
    expect(screen.getByLabelText("name")).toBeInTheDocument();
    expect(mockRefreshCustomer).not.toHaveBeenCalled();
  });

  it("shows a success toast and refreshes on a successful profile save", async () => {
    (crmAPI.updateProfile as jest.Mock).mockResolvedValue({});

    render(<CustomerProfile />);
    await waitFor(() => expect(screen.getByText("profileInfo")).toBeInTheDocument());
    enterEditMode();
    fireEvent.click(screen.getByText("saveChanges"));

    await waitFor(() => {
      expect(toast.success).toHaveBeenCalledWith("updateSuccess");
    });
    expect(mockRefreshCustomer).toHaveBeenCalled();
  });
});

describe("CustomerProfile multi-currency spend total localization", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockProfileLocale = "en";
  });

  // When a customer has spent across businesses using DIFFERENT currencies, the
  // total can't be labeled with one currency, so it renders as a bare number.
  // That number must still group in the diner's selected locale, not a hardcoded
  // en-US. 1500 + 1234 = 2734 -> German "2.734", en-US "2,734".
  function twoCurrencyBusinesses() {
    return [
      { id: 1, total_spent: 1500, visit_count: 2, loyalty_points: 0, business: { name: "A", default_currency: "USD" } },
      { id: 2, total_spent: 1234, visit_count: 1, loyalty_points: 0, business: { name: "B", default_currency: "EUR" } },
    ];
  }

  it("groups the currency-less total in the diner's selected locale (de)", async () => {
    mockProfileLocale = "de";
    (crmAPI.getBusinesses as jest.Mock).mockResolvedValue(twoCurrencyBusinesses());

    render(<CustomerProfile />);

    expect(await screen.findByText("2.734")).toBeInTheDocument();
    expect(screen.queryByText("2,734")).not.toBeInTheDocument();
  });

  it("groups the currency-less total in en-US when no locale is selected", async () => {
    (crmAPI.getBusinesses as jest.Mock).mockResolvedValue(twoCurrencyBusinesses());

    render(<CustomerProfile />);

    expect(await screen.findByText("2,734")).toBeInTheDocument();
  });
});
