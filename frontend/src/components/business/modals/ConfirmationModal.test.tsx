/** @jest-environment jsdom */
import React, { useState } from "react";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import ConfirmationModal from "./ConfirmationModal";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  // Passthrough: return keys so tests assert wiring without real message files.
  getTranslation: (key: string) => key,
}));

jest.mock("lucide-react", () => ({
  AlertCircle: () => <span data-testid="alert-icon" />,
}));

// Lightweight NextUI stubs: ModalContent render-prop onClose is wired to the
// parent Modal's onOpenChange so we can assert open/closed timing.
jest.mock("@nextui-org/react", () => {
  const React = require("react") as typeof import("react");
  let currentOnOpenChange: (() => void) | undefined;

  return {
    Modal: ({
      children,
      isOpen,
      onOpenChange,
    }: {
      children: React.ReactNode;
      isOpen: boolean;
      onOpenChange?: () => void;
    }) => {
      currentOnOpenChange = onOpenChange;
      if (!isOpen) return null;
      return <div data-testid="confirmation-modal">{children}</div>;
    },
    ModalContent: ({
      children,
    }: {
      children: (onClose: () => void) => React.ReactNode;
    }) => (
      <div>
        {typeof children === "function"
          ? children(() => {
              currentOnOpenChange?.();
            })
          : children}
      </div>
    ),
    ModalHeader: ({ children }: { children: React.ReactNode }) => (
      <div>{children}</div>
    ),
    ModalBody: ({ children }: { children: React.ReactNode }) => (
      <div>{children}</div>
    ),
    ModalFooter: ({ children }: { children: React.ReactNode }) => (
      <div>{children}</div>
    ),
    Button: ({
      children,
      onPress,
      isLoading,
      isDisabled,
      disabled,
    }: {
      children: React.ReactNode;
      onPress?: () => void | Promise<void>;
      isLoading?: boolean;
      isDisabled?: boolean;
      disabled?: boolean;
    }) => {
      const blocked = Boolean(isDisabled || isLoading || disabled);
      return (
        <button
          type="button"
          disabled={blocked}
          aria-busy={isLoading ? true : undefined}
          onClick={() => {
            if (blocked) return;
            void onPress?.();
          }}
        >
          {children}
        </button>
      );
    },
  };
});

type HarnessProps = {
  onConfirm: () => void | Promise<void>;
  isLoading?: boolean;
  isDanger?: boolean;
};

/** Controlled wrapper so onClose → isOpen=false is observable. */
function Harness({ onConfirm, isLoading, isDanger }: HarnessProps) {
  const [isOpen, setIsOpen] = useState(true);
  return (
    <>
      <span data-testid="modal-status">{isOpen ? "open" : "closed"}</span>
      <ConfirmationModal
        isOpen={isOpen}
        onOpenChange={() => setIsOpen(false)}
        title="Confirm action"
        description="This cannot be undone."
        confirmLabel="Confirm"
        cancelLabel="Cancel"
        onConfirm={onConfirm}
        isLoading={isLoading}
        isDanger={isDanger}
      />
    </>
  );
}

describe("ConfirmationModal", () => {
  it("closes immediately after a sync onConfirm", async () => {
    const onConfirm = jest.fn();
    render(<Harness onConfirm={onConfirm} />);

    expect(screen.getByTestId("modal-status")).toHaveTextContent("open");
    fireEvent.click(screen.getByRole("button", { name: "Confirm" }));

    expect(onConfirm).toHaveBeenCalledTimes(1);
    await waitFor(() => {
      expect(screen.getByTestId("modal-status")).toHaveTextContent("closed");
    });
  });

  it("keeps the modal open with a spinner until an async onConfirm resolves, then closes", async () => {
    let resolveConfirm!: () => void;
    const onConfirm = jest.fn(
      () =>
        new Promise<void>((resolve) => {
          resolveConfirm = resolve;
        }),
    );
    render(<Harness onConfirm={onConfirm} />);

    fireEvent.click(screen.getByRole("button", { name: "Confirm" }));

    expect(onConfirm).toHaveBeenCalledTimes(1);
    // Still open while the promise is in flight.
    expect(screen.getByTestId("modal-status")).toHaveTextContent("open");
    expect(screen.getByTestId("confirmation-modal")).toBeInTheDocument();

    const confirmBtn = screen.getByRole("button", { name: "Confirm" });
    expect(confirmBtn).toBeDisabled();
    expect(confirmBtn).toHaveAttribute("aria-busy", "true");

    await act(async () => {
      resolveConfirm();
    });

    await waitFor(() => {
      expect(screen.getByTestId("modal-status")).toHaveTextContent("closed");
    });
  });

  it("closes on rejected async onConfirm (finally) without an unhandled rejection", async () => {
    const onConfirm = jest.fn(() => Promise.reject(new Error("boom")));
    // Surface any unhandled rejection as a test failure.
    const unhandled: unknown[] = [];
    const onUnhandled = (reason: unknown) => {
      unhandled.push(reason);
    };
    process.on("unhandledRejection", onUnhandled);

    try {
      render(<Harness onConfirm={onConfirm} />);
      fireEvent.click(screen.getByRole("button", { name: "Confirm" }));

      await waitFor(() => {
        expect(screen.getByTestId("modal-status")).toHaveTextContent("closed");
      });
      expect(onConfirm).toHaveBeenCalledTimes(1);
      // Give the microtask queue a tick to surface any stray rejection.
      await act(async () => {
        await Promise.resolve();
      });
      expect(unhandled).toHaveLength(0);
    } finally {
      process.off("unhandledRejection", onUnhandled);
    }
  });

  it("fires onConfirm only once when confirm is clicked twice during pending", async () => {
    let resolveConfirm!: () => void;
    const onConfirm = jest.fn(
      () =>
        new Promise<void>((resolve) => {
          resolveConfirm = resolve;
        }),
    );
    render(<Harness onConfirm={onConfirm} />);

    const confirmBtn = screen.getByRole("button", { name: "Confirm" });
    fireEvent.click(confirmBtn);
    // Second click while pending — button is disabled / guard blocks re-entry.
    fireEvent.click(confirmBtn);
    fireEvent.click(confirmBtn);

    expect(onConfirm).toHaveBeenCalledTimes(1);

    await act(async () => {
      resolveConfirm();
    });
    await waitFor(() => {
      expect(screen.getByTestId("modal-status")).toHaveTextContent("closed");
    });
  });

  it("disables both buttons when isLoading is true", () => {
    render(<Harness onConfirm={jest.fn()} isLoading />);

    expect(screen.getByRole("button", { name: "Confirm" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Cancel" })).toBeDisabled();
  });

  it("disables cancel while an async onConfirm is pending", async () => {
    let resolveConfirm!: () => void;
    const onConfirm = jest.fn(
      () =>
        new Promise<void>((resolve) => {
          resolveConfirm = resolve;
        }),
    );
    render(<Harness onConfirm={onConfirm} />);

    fireEvent.click(screen.getByRole("button", { name: "Confirm" }));

    const cancelBtn = screen.getByRole("button", { name: "Cancel" });
    expect(cancelBtn).toBeDisabled();
    fireEvent.click(cancelBtn);
    // Dismiss blocked — still open.
    expect(screen.getByTestId("modal-status")).toHaveTextContent("open");

    await act(async () => {
      resolveConfirm();
    });
    await waitFor(() => {
      expect(screen.getByTestId("modal-status")).toHaveTextContent("closed");
    });
  });
});
