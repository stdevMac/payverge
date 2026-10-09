/** @jest-environment jsdom */
import React, { useRef } from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { useDialogKeyboard } from "./useDialogKeyboard";

function DialogHarness({
  onClose,
  enabled = true,
}: {
  onClose: () => void;
  enabled?: boolean;
}) {
  const containerRef = useRef<HTMLDivElement>(null);
  useDialogKeyboard(containerRef, onClose, enabled);
  return (
    <div ref={containerRef} role="dialog" aria-modal="true" tabIndex={-1}>
      <button type="button">First action</button>
      <button type="button">Last action</button>
    </div>
  );
}

describe("useDialogKeyboard (Root C)", () => {
  it("closes on Escape when focus is on document.body (not the container)", () => {
    const onClose = jest.fn();
    render(
      <>
        <button type="button">Outside</button>
        <DialogHarness onClose={onClose} />
      </>,
    );

    (document.activeElement as HTMLElement | null)?.blur?.();
    expect(
      document.activeElement === document.body ||
        document.activeElement === document.documentElement,
    ).toBe(true);

    document.dispatchEvent(
      new KeyboardEvent("keydown", { key: "Escape", bubbles: true }),
    );
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("restores focus to the previously focused element on unmount", () => {
    const onClose = jest.fn();
    const outside = document.createElement("button");
    outside.textContent = "Prior focus";
    document.body.appendChild(outside);
    outside.focus();
    expect(outside).toHaveFocus();

    const { unmount } = render(<DialogHarness onClose={onClose} />);
    const dialog = screen.getByRole("dialog");
    expect(dialog.contains(document.activeElement)).toBe(true);

    unmount();
    expect(outside).toHaveFocus();
    outside.remove();
  });

  it("removes the document keydown listener on unmount", () => {
    const onClose = jest.fn();
    const { unmount } = render(<DialogHarness onClose={onClose} />);
    unmount();
    document.dispatchEvent(
      new KeyboardEvent("keydown", { key: "Escape", bubbles: true }),
    );
    expect(onClose).not.toHaveBeenCalled();
  });

  // R2-4: a portaled NextUI popover/select rendered inside the dialog handles
  // Escape itself and calls preventDefault, but the event still bubbles to
  // document — where this hook used to close the whole dialog too. One Escape
  // must dismiss one layer.
  it("ignores an Escape that a nested layer already handled (defaultPrevented)", () => {
    const onClose = jest.fn();
    render(<DialogHarness onClose={onClose} />);

    // Stand-in for the portaled popover: lives outside the dialog subtree, so
    // this is purely about the document-level listener seeing a handled event.
    const popover = document.createElement("div");
    document.body.appendChild(popover);
    popover.addEventListener("keydown", (e) => {
      e.preventDefault();
    });

    popover.dispatchEvent(
      new KeyboardEvent("keydown", {
        key: "Escape",
        bubbles: true,
        cancelable: true,
      }),
    );

    expect(onClose).not.toHaveBeenCalled();

    // An unhandled Escape still closes the dialog.
    document.dispatchEvent(
      new KeyboardEvent("keydown", {
        key: "Escape",
        bubbles: true,
        cancelable: true,
      }),
    );
    expect(onClose).toHaveBeenCalledTimes(1);

    popover.remove();
  });

  // L5-30: while a nested ConfirmationModal is open, the hand-rolled dialog
  // must not trap Tab (focus lives on the portaled confirm).
  it("does not trap Tab or handle Escape when enabled=false", () => {
    const onClose = jest.fn();
    const outside = document.createElement("button");
    outside.textContent = "Confirm discard";
    document.body.appendChild(outside);

    render(<DialogHarness onClose={onClose} enabled={false} />);

    outside.focus();
    expect(outside).toHaveFocus();

    // Tab while focus is outside the container — must NOT pull focus back in.
    fireEvent.keyDown(document, { key: "Tab", bubbles: true });
    expect(outside).toHaveFocus();

    document.dispatchEvent(
      new KeyboardEvent("keydown", { key: "Escape", bubbles: true }),
    );
    expect(onClose).not.toHaveBeenCalled();

    outside.remove();
  });
});
