/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import { DashboardTabTransition } from "../DashboardTabTransition";

jest.mock("framer-motion", () => {
  const ReactActual = jest.requireActual("react");
  return {
    AnimatePresence: ({ children }: { children: React.ReactNode }) =>
      ReactActual.createElement(ReactActual.Fragment, null, children),
    motion: {
      div: ({
        children,
        variants: _variants,
        initial: _initial,
        animate: _animate,
        exit: _exit,
        custom: _custom,
        transition: _transition,
        ...props
      }: React.HTMLAttributes<HTMLDivElement> & Record<string, unknown>) =>
        ReactActual.createElement("div", props, children),
    },
    useReducedMotion: () => false,
  };
});

describe("DashboardTabTransition", () => {
  it("renders the current tab content and drops stale content on key changes", () => {
    const { rerender } = render(
      <DashboardTabTransition tabKey="overview">
        <div>Overview content</div>
      </DashboardTabTransition>,
    );

    expect(screen.getByText("Overview content")).toBeTruthy();

    rerender(
      <DashboardTabTransition tabKey="bills">
        <div>Bills content</div>
      </DashboardTabTransition>,
    );

    expect(screen.getByText("Bills content")).toBeTruthy();
    expect(screen.queryByText("Overview content")).toBeNull();
  });

  it("forwards remaining pane height so Tablero can fill an overflow-hidden shell", () => {
    const { container } = render(
      <DashboardTabTransition tabKey="reservations">
        <div>Board</div>
      </DashboardTabTransition>,
    );
    const root = container.firstElementChild;
    expect(root?.className).toMatch(/\bh-full\b/);
    expect(root?.className).toMatch(/min-h-0/);
    expect(root?.className).toMatch(/min-w-0/);
    expect(root?.className).toMatch(/flex-col/);
  });
});
