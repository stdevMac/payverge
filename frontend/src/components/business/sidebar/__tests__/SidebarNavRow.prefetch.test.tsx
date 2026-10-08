/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { SidebarNavRow } from "../SidebarNavRow";
import { Home } from "lucide-react";

/**
 * M1 — chunk prefetch intent: hovering or keyboard-focusing a rail row warms
 * that tab's webpack chunk so the click feels instant. The row only forwards
 * the intent; the memoized loader lives in the tab registry (prefetchTab).
 */

jest.mock("framer-motion", () => ({
  motion: {
    span: ({ children, ...rest }: any) => <span {...rest}>{children}</span>,
  },
}));
jest.mock("../../premium/useReducedDashboardMotion", () => ({
  useReducedDashboardMotion: () => true,
}));
jest.mock("../../premium/AnimatedNavIcon", () => ({
  AnimatedNavIcon: () => <span data-testid="nav-icon" />,
}));

function renderRow(extra: Partial<React.ComponentProps<typeof SidebarNavRow>> = {}) {
  const props: React.ComponentProps<typeof SidebarNavRow> = {
    label: "Overview",
    description: "Today at a glance",
    icon: Home,
    active: false,
    collapsed: false,
    title: "Overview",
    onClick: jest.fn(),
    ...extra,
  };
  render(<SidebarNavRow {...props} />);
  return screen.getByRole("button");
}

describe("SidebarNavRow — chunk prefetch intent", () => {
  it("fires onPrefetch on mouse enter", () => {
    const onPrefetch = jest.fn();
    const row = renderRow({ onPrefetch });
    fireEvent.mouseEnter(row);
    expect(onPrefetch).toHaveBeenCalledTimes(1);
  });

  it("fires onPrefetch on keyboard focus (focus-visible users get the same warm)", () => {
    const onPrefetch = jest.fn();
    const row = renderRow({ onPrefetch });
    fireEvent.focus(row);
    expect(onPrefetch).toHaveBeenCalledTimes(1);
  });

  it("still prefetches when the collapsed rail row is focused", () => {
    const onPrefetch = jest.fn();
    const row = renderRow({ collapsed: true, onPrefetch });
    fireEvent.focus(row);
    expect(onPrefetch).toHaveBeenCalledTimes(1);
  });

  it("does not fire onPrefetch on blur or click", () => {
    const onPrefetch = jest.fn();
    const row = renderRow({ onPrefetch });
    fireEvent.blur(row);
    fireEvent.click(row);
    expect(onPrefetch).not.toHaveBeenCalled();
  });

  it("works without an onPrefetch prop (footer/chrome rows)", () => {
    const row = renderRow();
    expect(() => {
      fireEvent.mouseEnter(row);
      fireEvent.focus(row);
    }).not.toThrow();
  });
});
