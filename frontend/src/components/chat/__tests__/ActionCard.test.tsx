/**
 * @jest-environment jsdom
 */

import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { ActionCard } from "../ChatShell";
import type { ChatAction } from "@/components/chat/types";

jest.mock("framer-motion", () => {
  // Preserve the real module (NextUI's <Button> ripple relies on `m.span` and
  // LazyMotion), but stub `motion.*` to plain elements and flatten LazyMotion so
  // the ripple's async feature loader doesn't settle outside React's act() boundary.
  const actual = jest.requireActual("framer-motion") as Record<string, unknown>;
  const ReactMock = jest.requireActual("react") as typeof React;
  const Motion = new Proxy(
    {},
    {
      get: (_t, tag: string) =>
        ReactMock.forwardRef<HTMLElement, any>(
          ({ animate: _a, exit: _e, initial: _i, transition: _tr, ...props }, ref) =>
            ReactMock.createElement(tag, { ...props, ref }),
        ),
    },
  );
  return {
    ...actual,
    LazyMotion: ({ children }: any) => <>{children}</>,
    motion: Motion,
    AnimatePresence: ({ children }: any) => <>{children}</>,
  };
});

function nav(overrides: Partial<ChatAction> = {}): ChatAction {
  return { label: "Open Menu", href: "/business/2/dashboard?tab=menu", kind: "navigate", ...overrides };
}

describe("ActionCard", () => {
  it("fires onClick for a navigate action", () => {
    const onClick = jest.fn();
    render(<ActionCard action={nav()} onClick={onClick} />);
    fireEvent.click(screen.getByRole("button", { name: /Open Menu/ }));
    expect(onClick).toHaveBeenCalledWith(nav());
  });

  it("renders an external action as an anchor with safe rel + target", () => {
    render(<ActionCard action={nav({ kind: "external", href: "https://payverge.io/docs", label: "Docs" })} />);
    const link = screen.getByRole("link", { name: /Docs/ });
    expect(link).toHaveAttribute("href", "https://payverge.io/docs");
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", "noopener noreferrer");
  });

  it("renders a disabled action with its disabled_reason and does not fire onClick", () => {
    const onClick = jest.fn();
    render(
      <ActionCard
        action={nav({ disabled: true, disabled_reason: "Owner only", kind: "handoff" })}
        onClick={onClick}
      />,
    );
    expect(screen.getByText("Owner only")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /Open Menu/ }));
    expect(onClick).not.toHaveBeenCalled();
  });
});
