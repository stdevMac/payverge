/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent, act } from "@testing-library/react";
import { DriverRow } from "../drivers/DriverRow";
import type { DeliveryDriver } from "@/api/delivery";

const tString = (key: string) => key;

const baseDriver: DeliveryDriver = {
  id: 1,
  business_id: 1,
  name: "Maria Lopez",
  phone: "555-0001",
  email: "maria@example.com",
  vehicle_type: "car",
  vehicle_plate: "ABC-123",
  status: "online",
  is_available: true,
  is_active: true,
};

describe("DriverRow", () => {
  it("renders driver name and phone", () => {
    render(
      <DriverRow
        driver={baseDriver}
        onEdit={jest.fn()}
        onRemove={jest.fn()}
        onToggleAvailability={jest.fn()}
        tString={tString}
      />
    );
    expect(screen.getByText("Maria Lopez")).toBeInTheDocument();
    expect(screen.getByText("555-0001")).toBeInTheDocument();
  });

  it("shows vehicle type and plate", () => {
    render(
      <DriverRow
        driver={baseDriver}
        onEdit={jest.fn()}
        onRemove={jest.fn()}
        onToggleAvailability={jest.fn()}
        tString={tString}
      />
    );
    // vehicle display: "drivers.vehicleTypes.car • ABC-123"
    expect(screen.getByText(/ABC-123/)).toBeInTheDocument();
  });

  it("fires onEdit when edit button is pressed", () => {
    const onEdit = jest.fn();
    render(
      <DriverRow
        driver={baseDriver}
        onEdit={onEdit}
        onRemove={jest.fn()}
        onToggleAvailability={jest.fn()}
        tString={tString}
      />
    );
    // aria-label is "drivers.edit Maria Lopez" — use getByRole with partial name
    fireEvent.click(screen.getByRole("button", { name: /drivers\.edit/i }));
    expect(onEdit).toHaveBeenCalledWith(baseDriver);
  });

  it("fires onRemove when remove button is pressed", () => {
    const onRemove = jest.fn();
    render(
      <DriverRow
        driver={baseDriver}
        onEdit={jest.fn()}
        onRemove={onRemove}
        onToggleAvailability={jest.fn()}
        tString={tString}
      />
    );
    // aria-label is "drivers.remove Maria Lopez" — use getByRole with partial name
    fireEvent.click(screen.getByRole("button", { name: /drivers\.remove/i }));
    expect(onRemove).toHaveBeenCalledWith(baseDriver);
  });

  it("calls onToggleAvailability on switch change", async () => {
    const onToggle = jest.fn().mockResolvedValue(undefined);
    render(
      <DriverRow
        driver={baseDriver}
        onEdit={jest.fn()}
        onRemove={jest.fn()}
        onToggleAvailability={onToggle}
        tString={tString}
      />
    );
    const switchEl = screen.getByRole("switch");
    await act(async () => {
      fireEvent.click(switchEl);
    });
    expect(onToggle).toHaveBeenCalledWith(baseDriver, false);
  });

  it("shows Inactive chip when is_active is false", () => {
    render(
      <DriverRow
        driver={{ ...baseDriver, is_active: false }}
        onEdit={jest.fn()}
        onRemove={jest.fn()}
        onToggleAvailability={jest.fn()}
        tString={tString}
      />
    );
    expect(screen.getByText("drivers.inactive")).toBeInTheDocument();
  });
});
