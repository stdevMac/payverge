/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";
import { CancelDeliveryModal } from "../CancelDeliveryModal";

jest.mock("@nextui-org/react", () => ({
  Button: ({ children, onPress, isLoading, isDisabled, "aria-label": ariaLabel, ...rest }: any) => (
    <button type="button" onClick={onPress} disabled={isDisabled || isLoading} aria-label={ariaLabel} {...rest}>
      {isLoading ? "loading" : children}
    </button>
  ),
  Textarea: ({ value, onValueChange, placeholder, isDisabled }: any) => (
    <textarea
      placeholder={placeholder}
      value={value ?? ""}
      disabled={isDisabled}
      onChange={(e) => onValueChange?.(e.target.value)}
    />
  ),
  Modal: ({ isOpen, children }: any) => (isOpen ? <div data-testid="modal">{children}</div> : null),
  ModalContent: ({ children }: any) => (
    <div>{typeof children === "function" ? children(() => {}) : children}</div>
  ),
  ModalHeader: ({ children }: any) => <div>{children}</div>,
  ModalBody: ({ children }: any) => <div>{children}</div>,
  ModalFooter: ({ children }: any) => <div>{children}</div>,
}));

const tString = (key: string) => key;

describe("CancelDeliveryModal", () => {
  // DEL-UX-3: the typed reason is rendered VERBATIM to the guest on the public
  // tracking page. The modal must say so, or operators will type internal notes
  // ("suspected fraud — do not redeliver") straight into a guest-facing field.
  it("discloses that the cancellation reason is visible to the customer (DEL-UX-3)", () => {
    render(
      <CancelDeliveryModal
        isOpen
        onClose={jest.fn()}
        onConfirm={jest.fn()}
        tString={tString}
      />,
    );
    expect(
      screen.getByText("dispatch.cancel.visibleToCustomer"),
    ).toBeInTheDocument();
  });

  it("submits the typed reason", async () => {
    const onConfirm = jest.fn().mockResolvedValue(undefined);
    const onClose = jest.fn();
    render(
      <CancelDeliveryModal isOpen onClose={onClose} onConfirm={onConfirm} tString={tString} />,
    );
    fireEvent.change(screen.getByPlaceholderText("dispatch.cancel.reasonPlaceholder"), {
      target: { value: "Customer not available" },
    });
    await act(async () => {
      fireEvent.click(screen.getByText("dispatch.cancel.confirm"));
    });
    await waitFor(() => expect(onConfirm).toHaveBeenCalledWith("Customer not available"));
    expect(onClose).toHaveBeenCalled();
  });
});
