/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";
import DriversManager from "../DriversManager";
import * as deliveryApiModule from "@/api/delivery";

// Mock delivery API
jest.mock("@/api/delivery", () => ({
  VEHICLE_TYPES: ["bicycle", "scooter", "motorcycle", "car", "van"],
  deliveryApi: {
    getBusinessDrivers: jest.fn(),
    createDriver: jest.fn(),
    updateDriver: jest.fn(),
    deleteDriver: jest.fn(),
  },
}));

// Mock toast — names must be prefixed with "mock" per Jest hoisting rules
const mockShowSuccess = jest.fn();
const mockShowError = jest.fn();
jest.mock("@/contexts/ToastContext", () => ({
  useToast: () => ({ showSuccess: mockShowSuccess, showError: mockShowError }),
}));

// Mock translation
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (_key: string) => _key,
}));

const mockApi = deliveryApiModule.deliveryApi as jest.Mocked<
  typeof deliveryApiModule.deliveryApi
>;

const mockDrivers = [
  {
    id: 1,
    business_id: 1,
    name: "Alice Driver",
    phone: "555-0001",
    email: "",
    vehicle_type: "car",
    vehicle_plate: "AAA-111",
    status: "online",
    is_available: true,
    is_active: true,
  },
  {
    id: 2,
    business_id: 1,
    name: "Bob Driver",
    phone: "555-0002",
    email: "",
    vehicle_type: "bicycle",
    vehicle_plate: "",
    status: "offline",
    is_available: false,
    is_active: true,
  },
];

beforeEach(() => {
  jest.clearAllMocks();
  mockShowSuccess.mockReset();
  mockShowError.mockReset();
  mockApi.getBusinessDrivers.mockResolvedValue([]);
});

describe("DriversManager", () => {
  it("renders empty state when no drivers", async () => {
    await act(async () => {
      render(<DriversManager businessId={1} />);
    });
    expect(screen.getByText("deliverySettings.drivers.empty")).toBeInTheDocument();
  });

  it("renders list of drivers", async () => {
    mockApi.getBusinessDrivers.mockResolvedValue(mockDrivers as any);
    await act(async () => {
      render(<DriversManager businessId={1} />);
    });
    expect(screen.getByText("Alice Driver")).toBeInTheDocument();
    expect(screen.getByText("Bob Driver")).toBeInTheDocument();
  });

  it("Add driver button opens the editor modal", async () => {
    await act(async () => {
      render(<DriversManager businessId={1} />);
    });
    fireEvent.click(
      screen.getByText("deliverySettings.drivers.addDriver")
    );
    expect(
      screen.getByText("deliverySettings.drivers.modal.createTitle")
    ).toBeInTheDocument();
  });

  it("availability toggle calls updateDriver with flipped value", async () => {
    mockApi.getBusinessDrivers.mockResolvedValue(mockDrivers as any);
    mockApi.updateDriver.mockResolvedValue({ ...mockDrivers[0], is_available: false } as any);

    await act(async () => {
      render(<DriversManager businessId={1} />);
    });

    const switches = screen.getAllByRole("switch");
    await act(async () => {
      fireEvent.click(switches[0]);
    });

    await waitFor(() => {
      expect(mockApi.updateDriver).toHaveBeenCalledWith(
        1,
        1,
        { is_available: false }
      );
    });
  });

  it("edit submits license_number and list reflects the server-returned driver", async () => {
    const driverWithLicense = {
      ...mockDrivers[0],
      license_number: "DL-ORIG",
    };
    mockApi.getBusinessDrivers.mockResolvedValue([driverWithLicense] as any);

    const updatedDriver = {
      ...driverWithLicense,
      name: "Alice Renamed", // server-returned name differs from the seeded one
      license_number: "DL-UPDATED",
      vehicle_plate: "", // cleared plate
    };
    mockApi.updateDriver.mockResolvedValue(updatedDriver as any);

    await act(async () => {
      render(<DriversManager businessId={1} />);
    });

    // Open the edit modal for Alice Driver
    const editButtons = screen.getAllByRole("button", { name: /drivers\.edit/i });
    await act(async () => {
      fireEvent.click(editButtons[0]);
    });

    expect(
      screen.getByText("deliverySettings.drivers.modal.editTitle")
    ).toBeInTheDocument();

    // Find the license number input (labelled by the translation key)
    const licenseInput = screen.getByDisplayValue("DL-ORIG");
    await act(async () => {
      fireEvent.change(licenseInput, { target: { value: "DL-UPDATED" } });
    });

    // Submit the form
    const saveButton = screen.getByText("deliverySettings.drivers.modal.save");
    await act(async () => {
      fireEvent.click(saveButton);
    });

    // The API call must include license_number
    await waitFor(() => {
      expect(mockApi.updateDriver).toHaveBeenCalledWith(
        1,
        1,
        expect.objectContaining({ license_number: "DL-UPDATED" })
      );
    });

    // The driver row must reflect the server-returned driver (not the
    // pre-update state): renamed driver visible, cleared plate gone.
    await waitFor(() => {
      expect(mockShowSuccess).toHaveBeenCalled();
    });
    expect(screen.getByText("Alice Renamed")).toBeInTheDocument();
    expect(screen.queryByText("Alice Driver")).not.toBeInTheDocument();
    expect(screen.queryByText(/AAA-111/)).not.toBeInTheDocument();
  });

  it("rapid double-click does not trigger duplicate API calls", async () => {
    mockApi.getBusinessDrivers.mockResolvedValue(mockDrivers as any);
    // Slow API — resolves after a delay so the second click lands while in-flight
    let resolveToggle!: () => void;
    mockApi.updateDriver.mockReturnValue(
      new Promise<any>((res) => {
        resolveToggle = () => res({ ...mockDrivers[0], is_available: false });
      })
    );

    await act(async () => {
      render(<DriversManager businessId={1} />);
    });

    const switches = screen.getAllByRole("switch");

    // First click — starts the in-flight call
    fireEvent.click(switches[0]);
    // Second click while the first is still in-flight — should be ignored
    fireEvent.click(switches[0]);

    // Resolve the pending API call
    await act(async () => {
      resolveToggle();
      await Promise.resolve();
    });

    // Should only have been called once despite two clicks
    expect(mockApi.updateDriver).toHaveBeenCalledTimes(1);
  });
});
