/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";
import { DriverEditorModal } from "../drivers/DriverEditorModal";
import type { DeliveryDriver } from "@/api/delivery";

const tString = (key: string) => key;

const existingDriver: DeliveryDriver = {
  id: 5,
  business_id: 1,
  name: "Juan Diaz",
  phone: "555-0002",
  email: "juan@example.com",
  vehicle_type: "motorcycle",
  vehicle_plate: "XYZ-999",
  status: "offline",
  is_available: false,
  is_active: true,
};

describe("DriverEditorModal", () => {
  it("renders empty form in create mode", () => {
    render(
      <DriverEditorModal
        isOpen
        driver={null}
        onClose={jest.fn()}
        onSubmit={jest.fn()}
        tString={tString}
      />
    );
    expect(screen.getByText("drivers.modal.createTitle")).toBeInTheDocument();
    expect(screen.getByLabelText(/drivers.fields.name/i)).toHaveValue("");
  });

  it("renders prefilled form in edit mode", () => {
    render(
      <DriverEditorModal
        isOpen
        driver={existingDriver}
        onClose={jest.fn()}
        onSubmit={jest.fn()}
        tString={tString}
      />
    );
    expect(screen.getByText("drivers.modal.editTitle")).toBeInTheDocument();
    expect(screen.getByDisplayValue("Juan Diaz")).toBeInTheDocument();
    expect(screen.getByDisplayValue("555-0002")).toBeInTheDocument();
  });

  // DEL-OP-6: a rejected onSubmit (create/update failed — the parent already
  // toasts) must be handled inside the modal. It must not escape the onPress
  // handler as an unhandled promise rejection, and the form must stay usable.
  it("handles a rejected onSubmit without an unhandled rejection and stays open (DEL-OP-6)", async () => {
    const unhandled: unknown[] = [];
    const onUnhandled = (event: PromiseRejectionEvent) => {
      unhandled.push(event.reason);
      event.preventDefault();
    };
    window.addEventListener("unhandledrejection", onUnhandled);
    try {
      const onSubmit = jest.fn().mockRejectedValue(new Error("save failed"));
      render(
        <DriverEditorModal
          isOpen
          driver={existingDriver}
          onClose={jest.fn()}
          onSubmit={onSubmit}
          tString={tString}
        />
      );
      const saveBtn = screen.getByText("drivers.modal.save");
      await act(async () => {
        fireEvent.click(saveBtn);
      });
      await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1));
      // Give any stray rejection a macrotask to surface.
      await act(async () => {
        await new Promise((resolve) => setTimeout(resolve, 0));
      });
      expect(unhandled).toHaveLength(0);
      // Modal stays open with the save button interactive again (not stuck
      // loading) so the operator can retry.
      expect(screen.getByText("drivers.modal.editTitle")).toBeInTheDocument();
      await waitFor(() =>
        expect(
          screen.getByRole("button", { name: /drivers\.modal\.save/ })
        ).toBeEnabled()
      );
    } finally {
      window.removeEventListener("unhandledrejection", onUnhandled);
    }
  });

  it("shows validation error and does not submit when name is empty", async () => {
    const onSubmit = jest.fn().mockResolvedValue(undefined);
    render(
      <DriverEditorModal
        isOpen
        driver={null}
        onClose={jest.fn()}
        onSubmit={onSubmit}
        tString={tString}
      />
    );
    const saveBtn = screen.getByText("drivers.modal.save");
    await act(async () => {
      fireEvent.click(saveBtn);
    });
    expect(onSubmit).not.toHaveBeenCalled();
    expect(screen.getByText("drivers.validation.nameRequired")).toBeInTheDocument();
  });

  it("calls onSubmit with correct values when form is valid", async () => {
    const onSubmit = jest.fn().mockResolvedValue(undefined);
    render(
      <DriverEditorModal
        isOpen
        driver={null}
        onClose={jest.fn()}
        onSubmit={onSubmit}
        tString={tString}
      />
    );

    fireEvent.change(screen.getByLabelText(/drivers.fields.name/i), {
      target: { value: "New Driver" },
    });
    fireEvent.change(screen.getByLabelText(/drivers.fields.phone/i), {
      target: { value: "5551112222" },
    });

    const saveBtn = screen.getByText("drivers.modal.save");
    await act(async () => {
      fireEvent.click(saveBtn);
    });

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({ name: "New Driver", phone: "5551112222" })
      );
    });
  });

  describe("L3-42 driver phone format", () => {
    it('rejects phone "abc" with localized error and does not submit', async () => {
      const onSubmit = jest.fn().mockResolvedValue(undefined);
      render(
        <DriverEditorModal
          isOpen
          driver={null}
          onClose={jest.fn()}
          onSubmit={onSubmit}
          tString={tString}
        />,
      );
      fireEvent.change(screen.getByLabelText(/drivers.fields.name/i), {
        target: { value: "Sam" },
      });
      fireEvent.change(screen.getByTestId("driver-phone"), {
        target: { value: "abc" },
      });
      await act(async () => {
        fireEvent.click(screen.getByText("drivers.modal.save"));
      });
      expect(onSubmit).not.toHaveBeenCalled();
      expect(
        screen.getByText("drivers.validation.phoneInvalid"),
      ).toBeInTheDocument();
    });

    it("phone field is type=tel (controlled path, not native number)", () => {
      render(
        <DriverEditorModal
          isOpen
          driver={null}
          onClose={jest.fn()}
          onSubmit={jest.fn()}
          tString={tString}
        />,
      );
      const phone = screen.getByTestId("driver-phone");
      expect(phone).toHaveAttribute("type", "tel");
      expect(phone).toHaveAttribute("inputmode", "tel");
    });
  });
});
