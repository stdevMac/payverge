/** @jest-environment jsdom */
import { useRef } from "react";
import { render, screen } from "@testing-library/react";
import { useDialogBehavior } from "./useDialogBehavior";

function DialogHarness({
  isOpen,
  onClose,
}: {
  isOpen: boolean;
  onClose: () => void;
}) {
  const triggerRef = useRef<HTMLButtonElement>(null);
  const dialogRef = useRef<HTMLDivElement>(null);
  const initialFocusRef = useRef<HTMLButtonElement>(null);
  useDialogBehavior({
    isOpen,
    onClose,
    containerRef: dialogRef,
    initialFocusRef,
    restoreFocusRef: triggerRef,
  });

  return (
    <>
      <button ref={triggerRef}>Open dialog</button>
      <div ref={dialogRef} role="dialog" aria-modal="true" tabIndex={-1}>
        <button ref={initialFocusRef}>First action</button>
      </div>
    </>
  );
}

describe("useDialogBehavior lifecycle", () => {
  it("uses the latest close callback, restores focus, and removes its listener", () => {
    const firstClose = jest.fn();
    const latestClose = jest.fn();
    const { rerender, unmount } = render(
      <DialogHarness isOpen onClose={firstClose} />,
    );
    expect(screen.getByRole("button", { name: "First action" })).toHaveFocus();

    rerender(<DialogHarness isOpen onClose={latestClose} />);
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
    expect(latestClose).toHaveBeenCalledTimes(1);
    expect(firstClose).not.toHaveBeenCalled();

    rerender(<DialogHarness isOpen={false} onClose={latestClose} />);
    expect(screen.getByRole("button", { name: "Open dialog" })).toHaveFocus();

    unmount();
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
    expect(latestClose).toHaveBeenCalledTimes(1);
  });
});
