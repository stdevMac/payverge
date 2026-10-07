/** @jest-environment jsdom */
import fs from "fs";
import path from "path";
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";

const confirmProps: Array<Record<string, unknown>> = [];

jest.mock("@/components/business/modals/ConfirmationModal", () => ({
  __esModule: true,
  default: (props: Record<string, unknown>) => {
    confirmProps.push(props);
    if (!props.isOpen) return null;
    return (
      <div data-testid="discard-confirm">
        <span>{String(props.title)}</span>
        <button
          type="button"
          data-testid="discard-confirm-yes"
          onClick={() => {
            void (props.onConfirm as () => void)();
          }}
        >
          Confirm
        </button>
      </div>
    );
  },
}));

jest.mock("@nextui-org/react", () => ({
  Drawer: (props: Record<string, unknown>) => {
    if (!props.isOpen) return null;
    return (
      <div data-testid="detail-drawer-root">
        <button
          type="button"
          data-testid="drawer-esc"
          onClick={() => {
            // Production NextUI routes Esc/backdrop through onOpenChange(false).
            const onOpenChange = props.onOpenChange;
            if (typeof onOpenChange === "function") {
              onOpenChange(false);
              return;
            }
            const onClose = props.onClose;
            if (typeof onClose === "function") onClose();
          }}
        >
          esc
        </button>
        {props.children as React.ReactNode}
      </div>
    );
  },
  DrawerContent: ({
    children,
    ...rest
  }: {
    children: unknown;
    [key: string]: unknown;
  }) => (
    <div data-testid="detail-drawer" {...rest}>
      {typeof children === "function"
        ? (children as () => React.ReactNode)()
        : (children as React.ReactNode)}
    </div>
  ),
  DrawerBody: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  DrawerFooter: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
}));

jest.mock("framer-motion", () => ({
  useReducedMotion: () => true,
}));

import DetailDrawer from "./DetailDrawer";

const DISCARD_COPY = {
  title: "¿Descartar esta nómina?",
  description: "Tenés cambios sin guardar.",
  confirmLabel: "Descartar",
  cancelLabel: "Seguir editando",
};

describe("DetailDrawer dirty confirm (L6-18)", () => {
  beforeEach(() => {
    confirmProps.length = 0;
  });

  it("calls onClose immediately when clean", () => {
    const onClose = jest.fn();
    render(
      <DetailDrawer open onClose={onClose} title="Payroll">
        <div>step</div>
      </DetailDrawer>,
    );
    fireEvent.click(screen.getByTestId("detail-drawer-close"));
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(screen.queryByTestId("discard-confirm")).toBeNull();
  });

  it("opens discard confirm instead of closing when dirty", () => {
    const onClose = jest.fn();
    render(
      <DetailDrawer
        open
        dirty
        onClose={onClose}
        title="Payroll"
        discardConfirm={DISCARD_COPY}
      >
        <div>step</div>
      </DetailDrawer>,
    );
    fireEvent.click(screen.getByTestId("detail-drawer-close"));
    expect(onClose).not.toHaveBeenCalled();
    expect(screen.getByTestId("discard-confirm")).toHaveTextContent(
      DISCARD_COPY.title,
    );
    fireEvent.click(screen.getByTestId("discard-confirm-yes"));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("routes Drawer onClose (Esc/backdrop) through the dirty gate", () => {
    const onClose = jest.fn();
    render(
      <DetailDrawer
        open
        dirty
        onClose={onClose}
        title="Payroll"
        discardConfirm={DISCARD_COPY}
      >
        <div>step</div>
      </DetailDrawer>,
    );
    fireEvent.click(screen.getByTestId("drawer-esc"));
    expect(onClose).not.toHaveBeenCalled();
    expect(screen.getByTestId("discard-confirm")).toBeInTheDocument();
  });

  // R2-3b: the shell used to hard-code English discard copy as `??` defaults
  // ("Discard changes?", "You have unsaved edits…", "Discard", "Keep editing"),
  // so a Spanish operator saw English in a money-adjacent confirm. Every string
  // must now come from the caller's already-translated `discardConfirm` prop.
  it("passes the caller's discard copy through verbatim, all four strings", () => {
    render(
      <DetailDrawer
        open
        dirty
        onClose={jest.fn()}
        title="Payroll"
        discardConfirm={DISCARD_COPY}
      >
        <div>step</div>
      </DetailDrawer>,
    );
    fireEvent.click(screen.getByTestId("detail-drawer-close"));
    const last = confirmProps[confirmProps.length - 1];
    expect(last.title).toBe(DISCARD_COPY.title);
    expect(last.description).toBe(DISCARD_COPY.description);
    expect(last.confirmLabel).toBe(DISCARD_COPY.confirmLabel);
    expect(last.cancelLabel).toBe(DISCARD_COPY.cancelLabel);
  });

  it("ships no hardcoded English discard literals", () => {
    const src = fs.readFileSync(
      path.join(__dirname, "DetailDrawer.tsx"),
      "utf8",
    );
    for (const literal of [
      "Discard changes?",
      "You have unsaved edits",
      '"Discard"',
      '"Keep editing"',
    ]) {
      expect(src).not.toContain(literal);
    }
  });
});
