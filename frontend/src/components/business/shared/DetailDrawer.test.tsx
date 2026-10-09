/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";

const drawerProps: Array<Record<string, unknown>> = [];

jest.mock("@/components/business/modals/ConfirmationModal", () => ({
  __esModule: true,
  default: ({
    isOpen,
    title,
  }: {
    isOpen: boolean;
    title: string;
  }) => (isOpen ? <div data-testid="confirm-modal">{title}</div> : null),
}));

jest.mock("@nextui-org/react", () => ({
  Drawer: (props: Record<string, unknown>) => {
    drawerProps.push(props);
    if (!props.isOpen) return null;
    return (
      <div data-testid="detail-drawer-root">
        <button
          type="button"
          data-testid="drawer-esc"
          onClick={() => {
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
    <div data-testid="detail-drawer-content" {...rest}>
      {typeof children === "function"
        ? (children as () => React.ReactNode)()
        : (children as React.ReactNode)}
    </div>
  ),
  DrawerBody: ({
    children,
    className,
  }: {
    children: React.ReactNode;
    className?: string;
  }) => (
    <div data-testid="detail-drawer-body" className={className}>
      {children}
    </div>
  ),
  DrawerFooter: ({
    children,
    className,
  }: {
    children: React.ReactNode;
    className?: string;
  }) => (
    <div data-testid="detail-drawer-footer" className={className}>
      {children}
    </div>
  ),
}));

let mockReducedMotion = false;
jest.mock("framer-motion", () => ({
  ...jest.requireActual("framer-motion"),
  useReducedMotion: () => mockReducedMotion,
}));

import DetailDrawer from "./DetailDrawer";

describe("DetailDrawer", () => {
  beforeEach(() => {
    drawerProps.length = 0;
    mockReducedMotion = false;
  });

  it("renders title and children when open", () => {
    render(
      <DetailDrawer open onClose={jest.fn()} title="Entry details">
        <p>Drawer body content</p>
      </DetailDrawer>,
    );

    expect(
      screen.getByRole("heading", { name: "Entry details" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Drawer body content")).toBeInTheDocument();
  });

  it("renders optional subtitle", () => {
    render(
      <DetailDrawer
        open
        onClose={jest.fn()}
        title="Entry details"
        subtitle="Posted 12 Mar 2026"
      >
        <p>Body</p>
      </DetailDrawer>,
    );

    expect(screen.getByText("Posted 12 Mar 2026")).toBeInTheDocument();
  });

  it("calls onClose when the close button is pressed", () => {
    const onClose = jest.fn();
    render(
      <DetailDrawer open onClose={onClose} title="Entry details">
        <p>Body</p>
      </DetailDrawer>,
    );

    fireEvent.click(screen.getByRole("button", { name: /close/i }));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("closes via a capture-phase document Escape even if NextUI swallows the key (issue 259)", () => {
    const onClose = jest.fn();
    render(
      <DetailDrawer open onClose={onClose} title="Entry details">
        <p>Body</p>
      </DetailDrawer>,
    );

    fireEvent.keyDown(document, { key: "Escape" });
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("gives the header X a 44px hit target so it is not covered (issue 259)", () => {
    render(
      <DetailDrawer open onClose={jest.fn()} title="Entry details">
        <p>Body</p>
      </DetailDrawer>,
    );

    const close = screen.getByTestId("detail-drawer-close");
    expect(close.className).toMatch(/min-h-11/);
    expect(close.className).toMatch(/min-w-11/);
    expect(close.className).toMatch(/z-50/);
  });

  it("closes via onOpenChange(false) for Escape/backdrop (#259)", () => {
    const onClose = jest.fn();
    render(
      <DetailDrawer open onClose={onClose} title="Entry details">
        <p>Body</p>
      </DetailDrawer>,
    );

    fireEvent.click(screen.getByTestId("drawer-esc"));
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(drawerProps[drawerProps.length - 1].onOpenChange).toEqual(
      expect.any(Function),
    );
  });

  it("renders the footer slot when provided", () => {
    render(
      <DetailDrawer
        open
        onClose={jest.fn()}
        title="Entry details"
        footer={<button type="button">Save entry</button>}
      >
        <p>Body</p>
      </DetailDrawer>,
    );

    expect(
      screen.getByRole("button", { name: "Save entry" }),
    ).toBeInTheDocument();
    expect(screen.getByTestId("detail-drawer-footer")).toBeInTheDocument();
  });

  it("omits the footer region when footer is not provided", () => {
    render(
      <DetailDrawer open onClose={jest.fn()} title="Entry details">
        <p>Body</p>
      </DetailDrawer>,
    );

    expect(screen.queryByTestId("detail-drawer-footer")).not.toBeInTheDocument();
  });

  it("does not render content when closed", () => {
    render(
      <DetailDrawer open={false} onClose={jest.fn()} title="Entry details">
        <p>Hidden body</p>
      </DetailDrawer>,
    );

    expect(screen.queryByText("Hidden body")).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Entry details" })).not.toBeInTheDocument();
  });

  it("places the drawer on the right with size md by default", () => {
    render(
      <DetailDrawer open onClose={jest.fn()} title="Entry details">
        <p>Body</p>
      </DetailDrawer>,
    );

    expect(drawerProps).toHaveLength(1);
    expect(drawerProps[0].placement).toBe("right");
    expect(drawerProps[0].size).toBe("md");
  });

  it("forwards size lg when requested", () => {
    render(
      <DetailDrawer open onClose={jest.fn()} title="Entry details" size="lg">
        <p>Body</p>
      </DetailDrawer>,
    );

    expect(drawerProps[0].size).toBe("lg");
  });

  it("disables animation when the user prefers reduced motion", () => {
    mockReducedMotion = true;
    render(
      <DetailDrawer open onClose={jest.fn()} title="Entry details">
        <p>Body</p>
      </DetailDrawer>,
    );

    expect(drawerProps[0].disableAnimation).toBe(true);
  });

  it("keeps animation enabled when reduced motion is not requested", () => {
    mockReducedMotion = false;
    render(
      <DetailDrawer open onClose={jest.fn()} title="Entry details">
        <p>Body</p>
      </DetailDrawer>,
    );

    expect(drawerProps[0].disableAnimation).toBe(false);
  });
});
